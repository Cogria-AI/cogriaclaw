// Package dispatcher handles a filtered inbound WhatsApp message: maintain
// short-term conversation history, run it through the LLM with skills as
// tools, and send the final text back to the chat. Skills can also send their
// own messages via the *wa.Client they receive in tool.Ctx.
package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Cogria-AI/cogriaclaw/internal/llm"
	"github.com/Cogria-AI/cogriaclaw/internal/session"
	"github.com/Cogria-AI/cogriaclaw/internal/tool"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

type Dispatcher struct {
	wa           *wa.Client
	llm          *llm.Client
	skills       *tool.Registry
	sysPrompt    string
	sessions     *session.Store // nil when conversation history is disabled
	resetCommand string
}

type Options struct {
	SystemPrompt string
	Sessions     *session.Store // nil = stateless (no history)
	ResetCommand string         // e.g. "/new"; ignored when Sessions is nil
}

func New(client *wa.Client, llmc *llm.Client, registry *tool.Registry, opts Options) *Dispatcher {
	return &Dispatcher{
		wa:           client,
		llm:          llmc,
		skills:       registry,
		sysPrompt:    opts.SystemPrompt,
		sessions:     opts.Sessions,
		resetCommand: opts.ResetCommand,
	}
}

// Handle is the MessageHandler passed to wa.Client.Start.
func (d *Dispatcher) Handle(ctx context.Context, msg wa.InboundMessage) {
	chat := msg.Chat.String()

	// Session reset command short-circuits the LLM.
	if d.sessions != nil && strings.TrimSpace(msg.Text) == d.resetCommand {
		existed := d.sessions.Reset(chat)
		reply := "New session started."
		if !existed {
			reply = "Already a fresh session — go ahead."
		}
		if err := d.wa.SendText(ctx, msg.Chat, reply); err != nil {
			slog.Error("dispatch: reset reply send failed", "err", err)
		}
		slog.Info("session reset", "chat", chat, "had_history", existed)
		return
	}

	userTurn := llm.Message{Role: "user", Content: buildUserMessage(msg)}

	var history []llm.Message
	if d.sessions != nil {
		history = d.sessions.History(chat)
	}
	history = append(history, userTurn)

	sc := &tool.Ctx{WA: d.wa, Inbound: &msg}
	res, err := d.llm.Run(ctx, d.sysPrompt, history, d.skills, sc)
	if err != nil {
		slog.Error("dispatch: llm run failed", "err", err, "chat", chat)
		if sendErr := d.wa.SendText(ctx, msg.Chat, "Sorry, something went wrong on my end."); sendErr != nil {
			slog.Error("dispatch: error-reply send failed", "err", sendErr)
		}
		return
	}

	slog.Info("dispatch ok",
		"iterations", res.Iterations,
		"stop_reason", res.StopReason,
		"reply_chars", len(res.Text),
		"history_turns", len(history),
	)

	if res.Text == "" {
		slog.Warn("dispatch: empty model output", "stop_reason", res.StopReason)
		return
	}

	if err := d.wa.SendText(ctx, msg.Chat, res.Text); err != nil {
		slog.Error("dispatch: reply send failed", "err", err)
		return
	}

	// Persist the visible turns (not the tool-call internals) so the next
	// message in this chat has context.
	if d.sessions != nil {
		d.sessions.Append(chat, userTurn, llm.Message{Role: "assistant", Content: res.Text})
	}
}

// buildUserMessage prefixes the raw text with light metadata so the model
// knows who sent it and where.
func buildUserMessage(msg wa.InboundMessage) string {
	where := "DM"
	if msg.IsGroup {
		where = "group"
	}
	from := msg.SenderPhone.User
	if from == "" {
		from = "unknown"
	}
	return fmt.Sprintf("[from +%s in %s]\n%s", from, where, msg.Text)
}
