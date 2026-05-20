package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Cogria-AI/cogriaclaw/internal/api"
	"github.com/Cogria-AI/cogriaclaw/internal/config"
	"github.com/Cogria-AI/cogriaclaw/internal/dispatcher"
	"github.com/Cogria-AI/cogriaclaw/internal/filter"
	"github.com/Cogria-AI/cogriaclaw/internal/llm"
	"github.com/Cogria-AI/cogriaclaw/internal/session"
	"github.com/Cogria-AI/cogriaclaw/internal/skills"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

// skillFactories is the static catalogue of skills cogriaclaw knows how to
// instantiate. Anything in the YAML config under `skills:` must appear here.
// Adding a new skill = add a file in internal/skills and a line below.
var skillFactories = map[string]skills.Factory{
	"echo":     skills.NewEcho,
	"http_get": skills.NewHTTPGet,
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel(cfg.LogLevel)})))

	registry, err := buildRegistry(cfg.Skills)
	if err != nil {
		slog.Error("build skill registry", "err", err)
		os.Exit(1)
	}

	llmClient, err := llm.New(llm.Config{
		BaseURL:     cfg.LLM.BaseURL,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		MaxToolHops: cfg.LLM.MaxToolHops,
		Headers:     cfg.LLM.Headers,
	})
	if err != nil {
		slog.Error("init llm", "err", err)
		os.Exit(1)
	}

	client, err := wa.New(wa.Config{
		DataDir:    cfg.Data.Dir,
		DeviceName: cfg.WhatsApp.DeviceName,
		LogLevel:   strings.ToUpper(cfg.LogLevel),
	})
	if err != nil {
		slog.Error("init wa client", "err", err)
		os.Exit(1)
	}

	requireMention := cfg.Filter.GroupRequireMentionResolved()
	f := filter.New(cfg.Filter.AllowedDMs, cfg.Filter.AllowedGroups, requireMention)

	var sessions *session.Store
	if cfg.Conversation.Enabled {
		sessions = session.NewStore(
			cfg.Conversation.MaxTurns,
			time.Duration(cfg.Conversation.IdleTTLMinutes)*time.Minute,
		)
	}
	disp := dispatcher.New(client, llmClient, registry, dispatcher.Options{
		SystemPrompt: resolveSystemPrompt(cfg.LLM.SystemPrompt),
		Sessions:     sessions,
		ResetCommand: cfg.Conversation.ResetCommand,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.API.Listen != "" {
		apiServer := api.New(cfg.API.Listen, api.Deps{
			WA:      client,
			Skills:  registry,
			Token:   cfg.API.Token,
			Started: time.Now(),
		})
		go func() {
			slog.Info("http api listening", "addr", cfg.API.Listen)
			if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("http api", "err", err)
			}
		}()
		go func() {
			<-ctx.Done()
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = apiServer.Shutdown(shutCtx)
		}()
	}

	handler := func(ctx context.Context, msg wa.InboundMessage) {
		if ok, reason := f.ShouldHandle(msg); !ok {
			level := slog.LevelInfo
			if msg.IsGroup {
				level = slog.LevelDebug
			}
			slog.Log(ctx, level, "drop",
				"reason", reason,
				"is_group", msg.IsGroup,
				"sender_phone", msg.SenderPhone.User,
			)
			return
		}
		slog.Info("rx",
			"is_group", msg.IsGroup,
			"sender_phone", msg.SenderPhone.User,
			"mentioned_me", msg.MentionedMe,
		)
		// Each dispatch runs in its own goroutine so a slow LLM call doesn't
		// block whatsmeow's event loop from processing the next message.
		go disp.Handle(ctx, msg)
	}

	skillNames := make([]string, 0, len(registry.List()))
	for _, s := range registry.List() {
		skillNames = append(skillNames, s.Name)
	}
	slog.Info("starting cogriaclaw",
		"phase", "4 (llm + skills)",
		"model", cfg.LLM.Model,
		"max_tool_hops", cfg.LLM.MaxToolHops,
		"skills", skillNames,
		"dms", len(cfg.Filter.AllowedDMs),
		"groups", len(cfg.Filter.AllowedGroups),
		"group_require_mention", requireMention,
		"conversation", cfg.Conversation.Enabled,
		"reset_command", cfg.Conversation.ResetCommand,
	)

	if err := client.Start(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
	slog.Info("stopped")
}

// buildRegistry instantiates skills listed in cfgSkills via the static factory
// map. Disabled skills are skipped silently; unknown names error out so
// typos in config don't disappear into the void.
func buildRegistry(cfgSkills map[string]config.SkillEntry) (*skills.Registry, error) {
	reg := skills.NewRegistry()
	for name, entry := range cfgSkills {
		if !entry.Enabled {
			continue
		}
		factory, ok := skillFactories[name]
		if !ok {
			return nil, fmt.Errorf("unknown skill %q in config (known: %v)", name, knownSkillNames())
		}
		s, err := factory(entry.Config)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", name, err)
		}
		if err := reg.Register(s); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func knownSkillNames() []string {
	out := make([]string, 0, len(skillFactories))
	for name := range skillFactories {
		out = append(out, name)
	}
	return out
}

func resolveSystemPrompt(fromConfig string) string {
	if strings.TrimSpace(fromConfig) != "" {
		return fromConfig
	}
	return "You are an assistant operating inside a WhatsApp chat. " +
		"Be concise — replies are read on a phone. " +
		"When a request requires action, prefer calling a tool. " +
		"Reply in the same language as the user."
}

func slogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
