// Package filter decides whether an inbound WhatsApp message should be
// processed. Phase 2 only handles DM allowlisting; group rules arrive later.
package filter

import (
	"strings"

	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

type Filter struct {
	allowedDMs map[string]struct{} // normalized digits only, no leading +
}

func New(allowedDMs []string) *Filter {
	set := make(map[string]struct{}, len(allowedDMs))
	for _, raw := range allowedDMs {
		if n := normalizeE164(raw); n != "" {
			set[n] = struct{}{}
		}
	}
	return &Filter{allowedDMs: set}
}

// AllowedDMs returns the normalized DM allowlist (digits only, no +).
// For diagnostic logging.
func (f *Filter) AllowedDMs() []string {
	out := make([]string, 0, len(f.allowedDMs))
	for k := range f.allowedDMs {
		out = append(out, k)
	}
	return out
}

// ShouldHandle returns (true, "") to process the message or (false, reason)
// to drop it. The reason is intended for debug logs, not user-facing replies.
//
// Matching is done against the sender's phone-mapped JID (msg.SenderPhone),
// not the on-the-wire JID — WhatsApp now uses LID addressing for many
// contacts, so the wire JID is an opaque ID that can't be allowlisted.
func (f *Filter) ShouldHandle(msg wa.InboundMessage) (bool, string) {
	if msg.IsFromMe {
		return false, "from-me"
	}
	if msg.IsGroup {
		return false, "group-not-yet-allowed"
	}
	if msg.SenderPhone.IsEmpty() {
		return false, "no-phone-jid-for-sender"
	}
	sender := normalizeE164(msg.SenderPhone.User)
	if sender == "" {
		return false, "non-user-sender"
	}
	if _, ok := f.allowedDMs[sender]; !ok {
		return false, "dm-sender-not-in-allowlist"
	}
	return true, ""
}

// normalizeE164 strips +, whitespace, hyphens, and any non-digit characters.
// WhatsApp's JID User part is already digit-only, so this is mainly for
// sanitising config input like "+86 138-0013-8000".
func normalizeE164(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
