package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type httpGetConfig struct {
	UserAgent  string `json:"user_agent"`
	TimeoutSec int    `json:"timeout_sec"`
	MaxBytes   int    `json:"max_bytes"` // body truncation cap; the model gets at most this many bytes
}

// NewHTTPGet shows the secret-handling pattern even though this particular
// skill takes no secrets: the closure captures `cfg` and `client` so per-call
// invocations never re-read config or re-construct an http.Client.
func NewHTTPGet(raw map[string]any) (Skill, error) {
	cfg := httpGetConfig{
		UserAgent:  "cogriaclaw/0.1",
		TimeoutSec: 10,
		MaxBytes:   4 * 1024,
	}
	if err := DecodeSkillConfig(raw, &cfg); err != nil {
		return Skill{}, fmt.Errorf("http_get: %w", err)
	}
	if cfg.TimeoutSec < 1 || cfg.TimeoutSec > 60 {
		return Skill{}, fmt.Errorf("http_get: timeout_sec out of range (1-60): %d", cfg.TimeoutSec)
	}
	if cfg.MaxBytes < 256 || cfg.MaxBytes > 64*1024 {
		return Skill{}, fmt.Errorf("http_get: max_bytes out of range (256-65536): %d", cfg.MaxBytes)
	}

	client := &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second}

	return Skill{
		Name:        "http_get",
		Description: "Fetch a URL with HTTP GET and return its response body (truncated). Use for reading the visible text/JSON of a public web resource. Do not use for sending data, authentication, or any non-GET request.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Absolute URL to fetch, with http:// or https:// scheme.",
				},
			},
			"required": []string{"url"},
		},
		Run: func(ctx context.Context, sc *Ctx, rawInput json.RawMessage) (string, error) {
			var in struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return "", fmt.Errorf("http_get: bad input: %w", err)
			}
			parsed, err := url.Parse(in.URL)
			if err != nil {
				return "", fmt.Errorf("http_get: invalid url: %w", err)
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return "", fmt.Errorf("http_get: only http/https schemes allowed (got %q)", parsed.Scheme)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
			if err != nil {
				return "", fmt.Errorf("http_get: build request: %w", err)
			}
			req.Header.Set("User-Agent", cfg.UserAgent)

			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("http_get: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(io.LimitReader(resp.Body, int64(cfg.MaxBytes)+1))
			if err != nil {
				return "", fmt.Errorf("http_get: read body: %w", err)
			}
			truncated := ""
			if len(body) > cfg.MaxBytes {
				body = body[:cfg.MaxBytes]
				truncated = "\n[truncated]"
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "HTTP %d %s\n", resp.StatusCode, resp.Status)
			if ct := resp.Header.Get("Content-Type"); ct != "" {
				fmt.Fprintf(&sb, "Content-Type: %s\n", ct)
			}
			sb.WriteString("\n")
			sb.Write(body)
			sb.WriteString(truncated)
			return sb.String(), nil
		},
	}, nil
}
