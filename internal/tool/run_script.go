package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// interpreters maps a script extension to the command used to run it. A file
// with no listed extension is executed directly (must have the executable bit).
var interpreters = map[string]string{
	".sh": "bash",
	".py": "python3",
	".js": "node",
	".rb": "ruby",
	".pl": "perl",
}

// NewRunScript returns a tool that executes a script bundled inside the skills
// root (Option B: folder-scoped execution, not arbitrary bash). The script path
// is jailed to root, execution has a timeout, and output is size-capped. The
// script itself can do anything its interpreter allows — trust comes from the
// skill being authored/installed by you, exactly as Anthropic Skills assume.
func NewRunScript(root string, timeoutSec, maxOutputBytes int) Tool {
	absRoot, _ := filepath.Abs(root)
	if timeoutSec < 1 {
		timeoutSec = 30
	}
	if maxOutputBytes < 256 {
		maxOutputBytes = 8 * 1024
	}
	return Tool{
		Name:        "run_script",
		Description: "Execute a script bundled inside a skill folder and return its combined stdout/stderr. Path is relative to the skills root (e.g. weather/scripts/fetch.py). Use only scripts that a skill's SKILL.md tells you to run. Cannot run commands outside the skills directory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Script path relative to the skills root, e.g. weather/scripts/fetch.py",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional command-line arguments passed to the script.",
				},
			},
			"required": []string{"path"},
		},
		Run: func(ctx context.Context, tc *Ctx, raw json.RawMessage) (string, error) {
			var in struct {
				Path string   `json:"path"`
				Args []string `json:"args"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return "", fmt.Errorf("run_script: bad input: %w", err)
			}
			full, err := jailPath(absRoot, in.Path)
			if err != nil {
				return "", err
			}
			info, err := os.Stat(full)
			if err != nil {
				return "", fmt.Errorf("run_script: %w", err)
			}
			if info.IsDir() {
				return "", fmt.Errorf("run_script: %q is a directory", in.Path)
			}

			name, args := buildCommand(full, in.Args)
			if name == "" {
				return "", fmt.Errorf("run_script: %q is not executable and has no known interpreter extension", in.Path)
			}

			runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(runCtx, name, args...)
			cmd.Dir = filepath.Dir(full)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out

			runErr := cmd.Run()
			result := out.Bytes()
			if len(result) > maxOutputBytes {
				result = append(result[:maxOutputBytes], []byte("\n[truncated]")...)
			}
			if runCtx.Err() == context.DeadlineExceeded {
				return string(result), fmt.Errorf("run_script: timed out after %ds", timeoutSec)
			}
			if runErr != nil {
				return fmt.Sprintf("%s\n[exit error: %v]", result, runErr), nil
			}
			return string(result), nil
		},
	}
}

// buildCommand picks how to invoke the script: by extension interpreter, or
// directly if the file is executable. Returns ("", nil) if neither applies.
func buildCommand(full string, args []string) (string, []string) {
	if interp, ok := interpreters[filepath.Ext(full)]; ok {
		return interp, append([]string{full}, args...)
	}
	if info, err := os.Stat(full); err == nil && info.Mode()&0o111 != 0 {
		return full, args
	}
	return "", nil
}
