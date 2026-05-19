package wa

import (
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// InboundMessage is the subset of an incoming whatsmeow message event that
// cogriaclaw cares about.
//
// Sender vs SenderPhone: as of mid-2025 WhatsApp routes messages from many
// contacts using LID (Linked Identity, server="lid"), a privacy-preserving
// opaque ID instead of a phone number. whatsmeow surfaces both addresses
// when known. SenderPhone is the phone-mapped JID — empty when WhatsApp
// doesn't expose a phone for this sender (e.g. strict-privacy contacts).
type InboundMessage struct {
	ID             string
	Timestamp      time.Time
	Chat           types.JID            // group or DM peer (in either addressing mode)
	Sender         types.JID            // address actually used on the wire (lid or phone)
	SenderAlt      types.JID            // the other address for the same sender, or zero
	SenderPhone    types.JID            // resolved phone JID (Server == DefaultUserServer), or zero
	AddressingMode types.AddressingMode // "pn" or "lid"
	IsGroup        bool
	IsFromMe       bool
	MentionedMe    bool // self JID appears in ExtendedTextMessage.ContextInfo.MentionedJID
	Text           string
}

// extractInbound builds an InboundMessage from a whatsmeow event. selfJIDs is the
// bot's own JIDs (PN + LID, either of which may be zero) — used to detect "@me"
// mentions across addressing modes.
func extractInbound(evt *events.Message, selfJIDs []types.JID) InboundMessage {
	out := InboundMessage{
		ID:             evt.Info.ID,
		Timestamp:      evt.Info.Timestamp,
		Chat:           evt.Info.Chat,
		Sender:         evt.Info.Sender,
		SenderAlt:      evt.Info.SenderAlt,
		AddressingMode: evt.Info.AddressingMode,
		IsGroup:        evt.Info.IsGroup,
		IsFromMe:       evt.Info.IsFromMe,
	}
	out.SenderPhone = resolveSenderPhone(evt.Info.MessageSource)

	if evt.Message != nil {
		switch {
		case evt.Message.Conversation != nil:
			out.Text = evt.Message.GetConversation()
		case evt.Message.ExtendedTextMessage != nil:
			ext := evt.Message.GetExtendedTextMessage()
			out.Text = ext.GetText()
			out.MentionedMe = mentionsAny(ext.GetContextInfo().GetMentionedJID(), selfJIDs)
		}
	}
	return out
}

// resolveSenderPhone picks whichever of Sender/SenderAlt is the phone-mapped JID.
// Returns zero JID if neither is on s.whatsapp.net.
func resolveSenderPhone(src types.MessageSource) types.JID {
	if src.Sender.Server == types.DefaultUserServer {
		return src.Sender
	}
	if !src.SenderAlt.IsEmpty() && src.SenderAlt.Server == types.DefaultUserServer {
		return src.SenderAlt
	}
	return types.JID{}
}

// mentionsAny reports whether any of mentioned (raw JID strings from the proto)
// matches one of selfJIDs after stripping device/agent suffixes.
func mentionsAny(mentioned []string, selfJIDs []types.JID) bool {
	if len(mentioned) == 0 || len(selfJIDs) == 0 {
		return false
	}
	selfStrs := make([]string, 0, len(selfJIDs))
	for _, j := range selfJIDs {
		if !j.IsEmpty() {
			selfStrs = append(selfStrs, j.ToNonAD().String())
		}
	}
	for _, raw := range mentioned {
		jid, err := types.ParseJID(raw)
		if err != nil {
			continue
		}
		norm := jid.ToNonAD().String()
		for _, s := range selfStrs {
			if norm == s {
				return true
			}
		}
	}
	return false
}
