// Package filter decides whether an inbound WhatsApp message should be
// processed. It enforces DM and group allowlists, plus an optional
// "@me required" gate inside groups.
package filter

import (
	"strings"

	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

type Filter struct {
	allowedDMs          map[string]struct{} // digits only, no leading +
	allowedGroups       map[string]struct{} // full group JID string ("<id>@g.us")
	groupRequireMention bool
}

func New(allowedDMs, allowedGroups []string, groupRequireMention bool) *Filter {
	dms := make(map[string]struct{}, len(allowedDMs))
	for _, raw := range allowedDMs {
		if n := normalizeE164(raw); n != "" {
			dms[n] = struct{}{}
		}
	}
	groups := make(map[string]struct{}, len(allowedGroups))
	for _, raw := range allowedGroups {
		if g := normalizeGroupJID(raw); g != "" {
			groups[g] = struct{}{}
		}
	}
	return &Filter{
		allowedDMs:          dms,
		allowedGroups:       groups,
		groupRequireMention: groupRequireMention,
	}
}

// AllowedDMs / AllowedGroups expose the normalized sets for diagnostic logging.
func (f *Filter) AllowedDMs() []string {
	out := make([]string, 0, len(f.allowedDMs))
	for k := range f.allowedDMs {
		out = append(out, k)
	}
	return out
}

func (f *Filter) AllowedGroups() []string {
	out := make([]string, 0, len(f.allowedGroups))
	for k := range f.allowedGroups {
		out = append(out, k)
	}
	return out
}

// ShouldHandle returns (true, "") to process the message or (false, reason)
// to drop it. The reason is intended for debug logs, not user-facing replies.
//
// DM matching uses msg.SenderPhone (the phone-mapped JID), not the wire JID —
// WhatsApp now uses LID addressing for many contacts, so the wire JID is an
// opaque ID that can't be allowlisted directly.
func (f *Filter) ShouldHandle(msg wa.InboundMessage) (bool, string) {
	if msg.IsFromMe {
		return false, "from-me"
	}
	if msg.IsGroup {
		if _, ok := f.allowedGroups[msg.Chat.String()]; !ok {
			return false, "group-not-in-allowlist"
		}
		if f.groupRequireMention && !msg.MentionedMe {
			return false, "group-mention-required"
		}
		return true, ""
	}
	// DM
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
func normalizeE164(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeGroupJID trims whitespace and verifies the value looks like a
// WhatsApp group JID (suffix "@g.us"). Returns "" for malformed input.
func normalizeGroupJID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || !strings.HasSuffix(s, "@g.us") {
		return ""
	}
	return s
}
