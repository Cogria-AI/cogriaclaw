// Package tool defines function-calling primitives the LLM can invoke
// directly (the "building blocks"). These are distinct from skills:
//
//   - tool  = a primitive the model calls via function-calling (http_get,
//     read_file, run_script). Implemented in Go.
//   - skill = a SKILL.md folder of markdown instructions (+ optional scripts)
//     that the model enacts *using* these tools. See internal/skill.
//
// A tool is data + a Run closure that may capture per-tool config (so secrets
// stay out of the model's context). main registers tools explicitly via a
// factory map — no init() magic.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema (object); marshaled to bytes by the LLM layer
	Run         func(ctx context.Context, tc *Ctx, input json.RawMessage) (string, error)
}

// Factory creates a Tool from the raw `config:` block under a tool name.
// rawCfg is nil if no config block was provided.
type Factory func(rawCfg map[string]any) (Tool, error)

// Ctx is the runtime context handed to a tool's Run closure.
// Inbound is the WhatsApp message that triggered this dispatch; nil when the
// tool runs via HTTP /trigger. WA lets a tool send messages independently.
type Ctx struct {
	WA      *wa.Client
	Inbound *wa.InboundMessage
}

type Registry struct {
	order  []string
	byName map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{byName: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) error {
	if t.Name == "" {
		return errors.New("tool: refusing to register tool with empty Name")
	}
	if _, dup := r.byName[t.Name]; dup {
		return fmt.Errorf("tool: duplicate registration: %s", t.Name)
	}
	r.byName[t.Name] = t
	r.order = append(r.order, t.Name)
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// List returns tools in registration order.
func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name])
	}
	return out
}

// DecodeConfig parses a YAML-derived raw map into a typed struct via JSON tags
// (round-tripping through JSON, which the SDKs already understand).
func DecodeConfig(raw map[string]any, target any) error {
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal tool config: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode tool config: %w", err)
	}
	return nil
}
