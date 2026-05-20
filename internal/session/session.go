// Package session keeps short-term conversation history per chat, in memory.
//
// Design (see localdocs/dev-plan.md): a session accumulates turns until the
// user explicitly resets it with the configured command (e.g. /new). There is
// no hardcoded turn window driving the UX — the boundary is command-controlled.
// Optional safety valves (MaxTurns, IdleTTL) exist to bound token cost and
// stale context, but both default to "off". Nothing is persisted: a restart
// starts everyone fresh, which keeps us clear of Openclaw-style memory bloat.
package session

import (
	"sync"
	"time"

	"github.com/Cogria-AI/cogriaclaw/internal/llm"
)

type conversation struct {
	turns   []llm.Message
	updated time.Time
}

type Store struct {
	mu       sync.Mutex
	byChat   map[string]*conversation
	maxTurns int           // 0 = unlimited; otherwise keep only the most recent N turns
	idleTTL  time.Duration // 0 = never expire; otherwise drop a session idle longer than this
	now      func() time.Time
}

func NewStore(maxTurns int, idleTTL time.Duration) *Store {
	return &Store{
		byChat:   map[string]*conversation{},
		maxTurns: maxTurns,
		idleTTL:  idleTTL,
		now:      time.Now,
	}
}

// History returns a copy of the current turns for chat, after applying idle
// expiry. The copy means callers can append without mutating stored state.
func (s *Store) History(chat string) []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv := s.byChat[chat]
	if conv == nil {
		return nil
	}
	if s.idleTTL > 0 && s.now().Sub(conv.updated) > s.idleTTL {
		delete(s.byChat, chat)
		return nil
	}
	out := make([]llm.Message, len(conv.turns))
	copy(out, conv.turns)
	return out
}

// Append adds turns to chat's history and bumps its updated time.
func (s *Store) Append(chat string, turns ...llm.Message) {
	if len(turns) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	conv := s.byChat[chat]
	if conv == nil {
		conv = &conversation{}
		s.byChat[chat] = conv
	}
	conv.turns = append(conv.turns, turns...)
	if s.maxTurns > 0 && len(conv.turns) > s.maxTurns {
		conv.turns = conv.turns[len(conv.turns)-s.maxTurns:]
	}
	conv.updated = s.now()
}

// Reset clears chat's history. Reports whether a session existed.
func (s *Store) Reset(chat string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existed := s.byChat[chat]
	delete(s.byChat, chat)
	return existed
}
