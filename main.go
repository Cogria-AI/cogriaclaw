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

	f := filter.New(cfg.Filter.AllowedDMs)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler := func(ctx context.Context, msg wa.InboundMessage) {
		slog.Info("rx",
			"chat", msg.Chat.String(),
			"sender", msg.Sender.String(),
			"sender_alt", msg.SenderAlt.String(),
			"sender_phone", msg.SenderPhone.String(),
			"addr_mode", string(msg.AddressingMode),
			"is_group", msg.IsGroup,
			"is_from_me", msg.IsFromMe,
			"text", msg.Text,
		)
		if ok, reason := f.ShouldHandle(msg); !ok {
			slog.Info("drop", "reason", reason, "sender_phone_user", msg.SenderPhone.User)
			return
		}
		slog.Info("pass → echo", "sender_phone_user", msg.SenderPhone.User)
		if err := client.SendText(ctx, msg.Chat, "echo: "+msg.Text); err != nil {
			slog.Error("send", "err", err)
		}
	}

	slog.Info("starting cogriaclaw (phase 2: echo + DM allowlist)",
		"raw_allowlist", cfg.Filter.AllowedDMs,
		"normalized_allowlist", f.AllowedDMs(),
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
