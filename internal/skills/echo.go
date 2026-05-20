package skills

import (
	"context"
	"encoding/json"
	"fmt"
)

// NewEcho is the simplest possible skill — proof that the model → tool →
// reply path is wired. Keeps no state and reads no config.
func NewEcho(_ map[string]any) (Skill, error) {
	return Skill{
		Name:        "echo",
		Description: "Repeat the user's input verbatim. Use it only when explicitly asked to echo or repeat something; otherwise just answer normally.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The exact text to echo back.",
				},
			},
			"required": []string{"text"},
		},
		Run: func(ctx context.Context, sc *Ctx, raw json.RawMessage) (string, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return "", fmt.Errorf("echo: bad input: %w", err)
			}
			return in.Text, nil
		},
	}, nil
}
