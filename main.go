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
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Cogria-AI/cogriaclaw/internal/api"
	"github.com/Cogria-AI/cogriaclaw/internal/config"
	"github.com/Cogria-AI/cogriaclaw/internal/daemon"
	"github.com/Cogria-AI/cogriaclaw/internal/dispatcher"
	"github.com/Cogria-AI/cogriaclaw/internal/filter"
	"github.com/Cogria-AI/cogriaclaw/internal/llm"
	"github.com/Cogria-AI/cogriaclaw/internal/session"
	"github.com/Cogria-AI/cogriaclaw/internal/skill"
	"github.com/Cogria-AI/cogriaclaw/internal/tool"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

var version = "dev"

// toolFactories is the static catalogue of config-driven built-in tools.
// Anything listed under `tools:` in the config must appear here. The skill
// access tools read_file/run_script are registered separately (they need the
// skills directory).
var toolFactories = map[string]tool.Factory{
	"http_get": tool.NewHTTPGet,
}

func main() {
	args := os.Args[1:]

	// Help / version short-circuits, in any position-0 form.
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printHelp()
			return
		case "version", "-v", "--version":
			fmt.Println("cogriaclaw", version)
			return
		}
	}

	// First non-flag arg is the command; default is "run".
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "run":
		cmdRun(args)
	case "reload":
		cmdSignal(args, syscall.SIGHUP, "reload")
	case "start":
		cmdStart(args)
	case "stop":
		cmdStop(args)
	case "restart":
		cmdRestart(args)
	case "status":
		cmdStatus(args)
	case "install":
		fatalIf(installService(configFlag(args, "install", "config.yaml")))
	case "uninstall":
		fatalIf(daemon.Uninstall())
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`cogriaclaw — a minimalist WhatsApp <-> LLM bridge

Usage:
  cogriaclaw [run] [-config FILE]   Run in the foreground (default; logs to terminal)
  cogriaclaw install [-config FILE] Install to ~/.local/bin + ~/.cogriaclaw and register a service
  cogriaclaw start                  Start the installed background service
  cogriaclaw stop                   Stop it (service if installed, else the running process)
  cogriaclaw restart                Restart it
  cogriaclaw reload                 Re-read config of the running instance (SIGHUP, no disconnect)
  cogriaclaw status                 Show whether an instance is running
  cogriaclaw uninstall              Stop + remove the service
  cogriaclaw help | version

Flags:
  -config FILE   Path to the YAML config (default: ~/.cogriaclaw/config.yaml if installed, else ./config.yaml)

First-time setup (from your project directory):
  cogriaclaw install     # copies binary + config + skills to your home dir
  cogriaclaw run         # if not logged in: scan the QR, then Ctrl+C
  cogriaclaw start       # run as a background service

reload hot-applies: filter allowlists, skills, system prompt, LLM settings.
A full restart is needed for: api.listen, data.dir, and the WhatsApp account.
`)
}

func configFlag(args []string, name, def string) string {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	configPath := fs.String("config", def, "path to YAML config file")
	_ = fs.Parse(args)
	return *configPath
}

func pidFileFor(configPath string) *daemon.PIDFile {
	return daemon.NewPIDFile(filepath.Join(config.PeekDataDir(configPath), "cogriaclaw.pid"))
}

// cmdSignal sends a signal to the running process via its pid file. Used by
// reload (SIGHUP) — works whether the instance is a service or standalone.
func cmdSignal(args []string, sig syscall.Signal, name string) {
	pf := pidFileFor(configFlag(args, name, config.DefaultConfigPath()))
	pid, err := pf.Signal(sig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("%s: signalled pid %d\n", name, pid)
}

func cmdStart(args []string) {
	_ = configFlag(args, "start", config.DefaultConfigPath())
	if !daemon.ServiceInstalled() {
		fmt.Fprintln(os.Stderr, "start: no service installed — run 'cogriaclaw install' first, or 'cogriaclaw run' for foreground")
		os.Exit(1)
	}
	fatalIf(daemon.StartService())
	fmt.Println("start: service started")
}

func cmdStop(args []string) {
	configPath := configFlag(args, "stop", config.DefaultConfigPath())
	if daemon.ServiceInstalled() {
		fatalIf(daemon.StopService())
		fmt.Println("stop: service stopped")
		return
	}
	pf := pidFileFor(configPath)
	pid, err := pf.Signal(syscall.SIGTERM)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stop:", err)
		os.Exit(1)
	}
	fmt.Printf("stop: signalled pid %d\n", pid)
}

func cmdStatus(args []string) {
	pf := pidFileFor(configFlag(args, "status", config.DefaultConfigPath()))
	if pid, ok := pf.RunningPID(); ok {
		fmt.Printf("running (pid %d)\n", pid)
		return
	}
	fmt.Println("not running")
	os.Exit(1)
}

func cmdRestart(args []string) {
	configPath := configFlag(args, "restart", config.DefaultConfigPath())
	if daemon.ServiceInstalled() {
		fatalIf(daemon.RestartService())
		fmt.Println("restart: service restarted")
		return
	}
	// Standalone: stop the running instance (if any), then run in the foreground.
	pf := pidFileFor(configPath)
	if _, err := pf.Signal(syscall.SIGTERM); err == nil {
		for i := 0; i < 50; i++ {
			if _, ok := pf.RunningPID(); !ok {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	run(configPath)
}

func cmdRun(args []string) {
	run(configFlag(args, "run", config.DefaultConfigPath()))
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// handlerState bundles the config-derived pieces that a SIGHUP reload swaps out
// atomically. The WhatsApp client, session store, API binding, and PID file are
// stable across reloads and live in run().
type handlerState struct {
	filter   *filter.Filter
	disp     *dispatcher.Dispatcher
	registry *tool.Registry
}

func run(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel(cfg.LogLevel)})))

	pf := daemon.NewPIDFile(filepath.Join(cfg.Data.Dir, "cogriaclaw.pid"))
	if err := pf.Acquire(); err != nil {
		slog.Error("start", "err", err)
		os.Exit(1)
	}
	defer pf.Release()

	client, err := wa.New(wa.Config{
		DataDir:    cfg.Data.Dir,
		DeviceName: cfg.WhatsApp.DeviceName,
		LogLevel:   strings.ToUpper(cfg.LogLevel),
	})
	if err != nil {
		slog.Error("init wa client", "err", err)
		os.Exit(1)
	}

	var sessions *session.Store
	if cfg.Conversation.Enabled {
		sessions = session.NewStore(
			cfg.Conversation.MaxTurns,
			time.Duration(cfg.Conversation.IdleTTLMinutes)*time.Minute,
		)
	}

	var state atomic.Pointer[handlerState]
	st, err := buildState(cfg, client, sessions)
	if err != nil {
		slog.Error("build runtime", "err", err)
		os.Exit(1)
	}
	state.Store(st)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// SIGHUP → reload config and swap reloadable state. A bad new config is
	// logged and ignored, keeping the running config intact.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			slog.Info("reload: SIGHUP received")
			newCfg, err := config.Load(configPath)
			if err != nil {
				slog.Error("reload: config invalid, keeping current", "err", err)
				continue
			}
			ns, err := buildState(newCfg, client, sessions)
			if err != nil {
				slog.Error("reload: build failed, keeping current", "err", err)
				continue
			}
			state.Store(ns)
			slog.Info("reload: applied",
				"tools", toolNames(ns.registry),
			)
		}
	}()

	if cfg.API.Listen != "" {
		apiServer := api.New(cfg.API.Listen, api.Deps{
			WA:      client,
			Tools:   func() *tool.Registry { return state.Load().registry },
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
		s := state.Load()
		if ok, reason := s.filter.ShouldHandle(msg); !ok {
			level := slog.LevelInfo
			if msg.IsGroup {
				level = slog.LevelDebug
			}
			slog.Log(ctx, level, "drop", "reason", reason, "is_group", msg.IsGroup, "sender_phone", msg.SenderPhone.User)
			return
		}
		slog.Info("rx", "is_group", msg.IsGroup, "sender_phone", msg.SenderPhone.User, "mentioned_me", msg.MentionedMe)
		// Each dispatch runs in its own goroutine so a slow LLM call doesn't
		// block whatsmeow's event loop.
		go s.disp.Handle(ctx, msg)
	}

	cur := state.Load()
	slog.Info("starting cogriaclaw",
		"version", version,
		"pid", os.Getpid(),
		"model", cfg.LLM.Model,
		"tools", toolNames(cur.registry),
		"dms", len(cfg.Filter.AllowedDMs),
		"groups", len(cfg.Filter.AllowedGroups),
		"conversation", cfg.Conversation.Enabled,
		"api", cfg.API.Listen,
	)

	if err := client.Start(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
	slog.Info("stopped")
}

// buildState assembles the reloadable runtime (tools, skills, llm, dispatcher,
// filter) from cfg. The WhatsApp client and session store are passed in because
// they persist across reloads.
func buildState(cfg *config.Config, client *wa.Client, sessions *session.Store) (*handlerState, error) {
	registry, err := buildRegistry(cfg.Tools)
	if err != nil {
		return nil, err
	}

	catalog, warnings, err := skill.Load(cfg.Skills.Dir)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		slog.Warn("skill skipped", "detail", w)
	}
	if !catalog.Empty() {
		if err := registry.Register(tool.NewReadFile(catalog.Root())); err != nil {
			return nil, err
		}
		if cfg.Skills.Exec.Enabled {
			if err := registry.Register(tool.NewRunScript(catalog.Root(), cfg.Skills.Exec.TimeoutSec, cfg.Skills.Exec.MaxOutputBytes)); err != nil {
				return nil, err
			}
		}
	}

	llmClient, err := llm.New(llm.Config{
		BaseURL:     cfg.LLM.BaseURL,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		MaxToolHops: cfg.LLM.MaxToolHops,
		Headers:     cfg.LLM.Headers,
		Extra:       cfg.LLM.ExtraBody,
	})
	if err != nil {
		return nil, err
	}

	systemPrompt := resolveSystemPrompt(cfg.LLM.SystemPrompt)
	if block := catalog.PromptBlock(); block != "" {
		systemPrompt = systemPrompt + "\n\n" + block
	}
	disp := dispatcher.New(client, llmClient, registry, dispatcher.Options{
		SystemPrompt: systemPrompt,
		Sessions:     sessions,
		ResetCommand: cfg.Conversation.ResetCommand,
	})

	f := filter.New(cfg.Filter.AllowedDMs, cfg.Filter.AllowedGroups, cfg.Filter.GroupRequireMentionResolved())
	return &handlerState{filter: f, disp: disp, registry: registry}, nil
}

func buildRegistry(cfgTools map[string]config.ToolEntry) (*tool.Registry, error) {
	reg := tool.NewRegistry()
	for name, entry := range cfgTools {
		if !entry.Enabled {
			continue
		}
		factory, ok := toolFactories[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q in config (known: %v)", name, knownToolNames())
		}
		t, err := factory(entry.Config)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", name, err)
		}
		if err := reg.Register(t); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func toolNames(reg *tool.Registry) []string {
	list := reg.List()
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Name)
	}
	return out
}

func knownToolNames() []string {
	out := make([]string, 0, len(toolFactories))
	for name := range toolFactories {
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
