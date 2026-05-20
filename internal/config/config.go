// Package config loads cogriaclaw's YAML config. Phase 4 adds LLM and
// skill configuration on top of phase 2/3's WhatsApp + filter setup.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogLevel     string                `yaml:"log_level"`
	Data         DataConfig            `yaml:"data"`
	WhatsApp     WAConfig              `yaml:"whatsapp"`
	Filter       FilterConfig          `yaml:"filter"`
	LLM          LLMConfig             `yaml:"llm"`
	Conversation ConversationConfig    `yaml:"conversation"`
	Skills       map[string]SkillEntry `yaml:"skills"`
}

// ConversationConfig controls short-term, in-memory conversation history.
// The session boundary is command-controlled (ResetCommand) rather than a
// fixed turn count. MaxTurns/IdleTTLMinutes are optional safety valves.
type ConversationConfig struct {
	Enabled        bool   `yaml:"enabled"`
	ResetCommand   string `yaml:"reset_command"`    // message that clears the session (default "/new")
	MaxTurns       int    `yaml:"max_turns"`        // 0 = unlimited (keep until reset)
	IdleTTLMinutes int    `yaml:"idle_ttl_minutes"` // 0 = never auto-expire
}

type DataConfig struct {
	Dir string `yaml:"dir"`
}

type WAConfig struct {
	DeviceName string `yaml:"device_name"`
}

type FilterConfig struct {
	AllowedDMs          []string `yaml:"allowed_dms"`
	AllowedGroups       []string `yaml:"allowed_groups"`
	GroupRequireMention *bool    `yaml:"group_require_mention"`
}

func (f FilterConfig) GroupRequireMentionResolved() bool {
	if f.GroupRequireMention == nil {
		return false
	}
	return *f.GroupRequireMention
}

// LLMConfig targets any OpenAI-Chat-Completions-compatible endpoint.
// Switch backend (Kimi, Moonshot, DeepSeek, OpenAI, Groq, Ollama, …) by
// changing base_url + model + api_key — no code change.
type LLMConfig struct {
	BaseURL      string            `yaml:"base_url"` // e.g. https://api.kimi.com/coding/v1; empty = OpenAI default
	APIKey       string            `yaml:"api_key"`  // supports ${ENV_NAME} interpolation
	Model        string            `yaml:"model"`    // e.g. kimi-for-coding
	Headers      map[string]string `yaml:"headers"`  // extra request headers (e.g. User-Agent for Kimi's coding endpoint)
	SystemPrompt string            `yaml:"system_prompt"`
	MaxTokens    int               `yaml:"max_tokens"`
	MaxToolHops  int               `yaml:"max_tool_hops"`
}

// SkillEntry is one block under `skills:` in the YAML. Config is the per-skill
// options block (untyped here; each skill factory parses its own fields).
type SkillEntry struct {
	Enabled bool           `yaml:"enabled"`
	Config  map[string]any `yaml:"config"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s\n  → copy config.example.yaml to %s and edit it", path, path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Defaults
	if cfg.Data.Dir == "" {
		cfg.Data.Dir = "data"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.WhatsApp.DeviceName == "" {
		cfg.WhatsApp.DeviceName = "cogriaclaw"
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 2048
	}
	if cfg.LLM.MaxToolHops == 0 {
		cfg.LLM.MaxToolHops = 5
	}
	if cfg.Conversation.ResetCommand == "" {
		cfg.Conversation.ResetCommand = "/new"
	}

	// ${ENV_NAME} interpolation. We apply it only to fields where it's expected
	// (LLM api_key + everything under skills.*.config) — applying it broadly
	// could accidentally rewrite user-authored content like system prompts.
	cfg.LLM.APIKey = interpolateEnv(cfg.LLM.APIKey)
	cfg.LLM.BaseURL = interpolateEnv(cfg.LLM.BaseURL)
	for name, entry := range cfg.Skills {
		if m, ok := interpolateInTree(entry.Config).(map[string]any); ok {
			entry.Config = m
			cfg.Skills[name] = entry
		}
	}

	// Validation
	if cfg.LLM.Model == "" {
		return nil, errors.New("llm.model is required (e.g. kimi-for-coding)")
	}
	if cfg.LLM.APIKey == "" {
		return nil, errors.New("llm.api_key is empty — set it directly or via ${ENV_NAME} interpolation")
	}
	if cfg.LLM.MaxToolHops < 1 || cfg.LLM.MaxToolHops > 20 {
		return nil, fmt.Errorf("llm.max_tool_hops out of range (1-20): %d", cfg.LLM.MaxToolHops)
	}

	if len(cfg.Filter.AllowedDMs) == 0 && len(cfg.Filter.AllowedGroups) == 0 {
		return nil, errors.New("filter has no allowed_dms and no allowed_groups — refusing to start with a fully closed inbound (configure at least one)")
	}
	for i, dm := range cfg.Filter.AllowedDMs {
		s := strings.TrimSpace(dm)
		if s == "" || strings.Contains(s, "CHANGE_ME") {
			return nil, fmt.Errorf("filter.allowed_dms[%d] is a placeholder (%q) — edit %s", i, dm, path)
		}
	}

	return &cfg, nil
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// interpolateEnv replaces ${ENV_NAME} occurrences with os.Getenv("ENV_NAME").
// Strict $VAR form is deliberately not supported — too easy to collide with
// literal text in a user-authored system prompt.
func interpolateEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(m[2 : len(m)-1])
	})
}

// interpolateInTree walks v and applies interpolateEnv to every string value.
func interpolateInTree(v any) any {
	switch t := v.(type) {
	case string:
		return interpolateEnv(t)
	case map[string]any:
		for k, val := range t {
			t[k] = interpolateInTree(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = interpolateInTree(val)
		}
		return t
	default:
		return v
	}
}
