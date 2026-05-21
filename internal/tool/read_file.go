package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const readFileMaxBytes = 64 * 1024

// NewReadFile returns a tool that reads a file from within the skills root.
// Paths are taken relative to root and jailed to it — any attempt to escape
// (via .. or absolute paths resolving outside root) is rejected. This is how
// the model loads a skill's SKILL.md (L2) and bundled docs (L3).
func NewReadFile(root string) Tool {
	absRoot, _ := filepath.Abs(root)
	return Tool{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from the skills directory (e.g. a skill's SKILL.md or a bundled reference doc). Path is relative to the skills root. Use this to load a skill's instructions before following them.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path relative to the skills root, e.g. weather/SKILL.md",
				},
			},
			"required": []string{"path"},
		},
		Run: func(ctx context.Context, tc *Ctx, raw json.RawMessage) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return "", fmt.Errorf("read_file: bad input: %w", err)
			}
			full, err := jailPath(absRoot, in.Path)
			if err != nil {
				return "", err
			}
			info, err := os.Stat(full)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}
			if info.IsDir() {
				return listDir(full)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}
			if len(data) > readFileMaxBytes {
				return string(data[:readFileMaxBytes]) + "\n[truncated]", nil
			}
			return string(data), nil
		},
	}
}

func listDir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	var b strings.Builder
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(&b, "%s%s\n", e.Name(), suffix)
	}
	return b.String(), nil
}

// jailPath resolves rel against absRoot and verifies the result stays within
// absRoot. Returns the cleaned absolute path or an error.
func jailPath(absRoot, rel string) (string, error) {
	if absRoot == "" {
		return "", fmt.Errorf("skills root not configured")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	full := filepath.Clean(filepath.Join(absRoot, rel))
	if full != absRoot && !strings.HasPrefix(full, absRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the skills directory", rel)
	}
	return full, nil
}
