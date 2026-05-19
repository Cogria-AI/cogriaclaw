package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Cogria-AI/cogriaclaw/internal/config"
	"github.com/Cogria-AI/cogriaclaw/internal/filter"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel(cfg.LogLevel)})))

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler := func(ctx context.Context, msg wa.InboundMessage) {
		if ok, reason := f.ShouldHandle(msg); !ok {
			// Drop logs intentionally omit message text and chat JID — the bot may sit
			// silently in many groups, and we don't want to leak those IDs by default.
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
			"text", msg.Text,
		)
		if err := client.SendText(ctx, msg.Chat, "echo: "+msg.Text); err != nil {
			slog.Error("send", "err", err)
		}
	}

	slog.Info("starting cogriaclaw",
		"phase", "3 (echo + DM/group allowlist + mention gate)",
		"dms", len(cfg.Filter.AllowedDMs),
		"groups", len(cfg.Filter.AllowedGroups),
		"group_require_mention", requireMention,
	)
	slog.Debug("filter detail",
		"allowed_dms_raw", cfg.Filter.AllowedDMs,
		"allowed_dms_normalized", f.AllowedDMs(),
		"allowed_groups_normalized", f.AllowedGroups(),
	)

	if err := client.Start(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
	slog.Info("stopped")
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
