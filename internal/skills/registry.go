// Package skills defines the Skill type, factories, and a Registry the
// LLM dispatcher exposes as tools.
//
// Design constraints (see localdocs/dev-plan.md §3.6, §3.9):
//   - A skill is data + a closure. The closure captures any secrets from
//     the skill's config block; secrets never enter the LLM's context.
//   - Skills are SDK-agnostic: InputSchema is plain map[string]any (raw
//     JSON schema). The LLM package adapts this to anthropic.BetaTool at
//     dispatch time.
//   - main.go registers skills explicitly via a Factory map — no init()
//     side-effects. New skills are visible from one place.
package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

// Skill is a tool the LLM can call. The Run closure typically captures
// per-skill config (URLs, API tokens, timeouts) at construction time.
type Skill struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema (object); marshaled to bytes by the dispatcher
	Run         func(ctx context.Context, sc *Ctx, input json.RawMessage) (string, error)
}

// Factory creates a Skill from the raw `config:` block under a skill name.
// rawCfg is nil if no config block was provided.
type Factory func(rawCfg map[string]any) (Skill, error)

// Ctx is the runtime context handed to a skill's Run closure.
//
// Inbound is the WhatsApp message that triggered this dispatch; nil when the
// skill is invoked via HTTP /trigger (phase 5).
//
// WA lets a skill send messages independently of the dispatcher's reply path,
// useful for long-running tasks that want to post intermediate progress.
type Ctx struct {
	WA      *wa.Client
	Inbound *wa.InboundMessage
}

type Registry struct {
	order   []string
	bySkill map[string]Skill
}

func NewRegistry() *Registry {
	return &Registry{bySkill: map[string]Skill{}}
}

func (r *Registry) Register(s Skill) error {
	if s.Name == "" {
		return errors.New("skills: refusing to register skill with empty Name")
	}
	if _, dup := r.bySkill[s.Name]; dup {
		return fmt.Errorf("skills: duplicate registration: %s", s.Name)
	}
	r.bySkill[s.Name] = s
	r.order = append(r.order, s.Name)
	return nil
}

func (r *Registry) Get(name string) (Skill, bool) {
	s, ok := r.bySkill[name]
	return s, ok
}

// List returns skills in registration order (deterministic for logging and
// for the order in which tools are presented to the model).
func (r *Registry) List() []Skill {
	out := make([]Skill, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.bySkill[name])
	}
	return out
}

// decodeSkillConfig parses a YAML-derived raw map into a typed struct,
// using JSON tags on the target (we round-trip via JSON because that's what
// the SDKs already understand; round-tripping via YAML would require yaml
// tags everywhere).
func DecodeSkillConfig(raw map[string]any, target any) error {
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal skill config: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode skill config: %w", err)
	}
	return nil
}
