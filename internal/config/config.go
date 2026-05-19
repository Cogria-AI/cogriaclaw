// Package config loads cogriaclaw's YAML config. Phase 2 only carries the
// fields needed for echo + DM allowlist; later phases will extend the schema.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogLevel string       `yaml:"log_level"`
	Data     DataConfig   `yaml:"data"`
	WhatsApp WAConfig     `yaml:"whatsapp"`
	Filter   FilterConfig `yaml:"filter"`
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
	GroupRequireMention *bool    `yaml:"group_require_mention"` // pointer so absence-vs-false is distinguishable
}

// GroupRequireMentionResolved returns the configured value, defaulting to false.
// The bot already requires a group to be explicitly allowlisted, so by default
// it participates in conversation freely there (more natural interaction).
// Set group_require_mention: true if you want strict @-only behaviour.
func (f FilterConfig) GroupRequireMentionResolved() bool {
	if f.GroupRequireMention == nil {
		return false
	}
	return *f.GroupRequireMention
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

	if cfg.Data.Dir == "" {
		cfg.Data.Dir = "data"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.WhatsApp.DeviceName == "" {
		cfg.WhatsApp.DeviceName = "cogriaclaw"
	}

	if len(cfg.Filter.AllowedDMs) == 0 {
		return nil, errors.New("filter.allowed_dms is empty — refusing to start with an open allowlist (set at least one number)")
	}
	for i, dm := range cfg.Filter.AllowedDMs {
		s := strings.TrimSpace(dm)
		if s == "" || strings.Contains(s, "CHANGE_ME") {
			return nil, fmt.Errorf("filter.allowed_dms[%d] is a placeholder (%q) — edit %s", i, dm, path)
		}
	}

	return &cfg, nil
}
