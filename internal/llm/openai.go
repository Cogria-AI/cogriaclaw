// Package llm talks to any OpenAI-Chat-Completions-compatible endpoint
// (OpenAI, Kimi/Moonshot, DeepSeek, Groq, OpenRouter, local Ollama, …).
// Pick a backend purely by config: base_url + api_key + model. The
// multi-hop tool-use loop (model calls a tool → skill runs → result fed
// back → model continues) is implemented here since the OpenAI SDK has no
// built-in runner.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Cogria-AI/cogriaclaw/internal/tool"
)

type Config struct {
	BaseURL     string // empty = OpenAI default; set to e.g. https://api.kimi.com/coding/v1
	APIKey      string
	Model       string
	MaxTokens   int
	MaxToolHops int
	Headers     map[string]string // extra request headers, e.g. {"User-Agent": "KimiCLI/0.77"} for Kimi's coding endpoint
	Extra       map[string]any    // extra request-body fields merged into every chat completion (provider-specific knobs, e.g. {"thinking": {"type": "disabled"}})
}

type Client struct {
	inner openai.Client
	cfg   Config
}

type Result struct {
	Text       string
	StopReason string
	Iterations int
}

// Message is one turn of visible conversation history (no tool-call internals).
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("llm: empty API key")
	}
	if cfg.Model == "" {
		return nil, errors.New("llm: empty model")
	}
	if cfg.MaxToolHops < 1 {
		cfg.MaxToolHops = 5
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	for k, v := range cfg.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}
	return &Client{inner: openai.NewClient(opts...), cfg: cfg}, nil
}

// Run sends the conversation (system prompt + history, where the last history
// entry is the new user message) and lets the model use the registered skills
// as tools until it returns a tool-call-free message, or the hop cap is reached.
func (c *Client) Run(ctx context.Context, system string, history []Message, registry *tool.Registry, sc *tool.Ctx) (Result, error) {
	tools := buildTools(registry)
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+1)
	messages = append(messages, openai.SystemMessage(system))
	for _, m := range history {
		if m.Role == "assistant" {
			messages = append(messages, openai.AssistantMessage(m.Content))
		} else {
			messages = append(messages, openai.UserMessage(m.Content))
		}
	}

	for hop := 1; hop <= c.cfg.MaxToolHops; hop++ {
		msg, finish, err := c.complete(ctx, messages, tools)
		if err != nil {
			return Result{}, err
		}

		if len(msg.ToolCalls) == 0 {
			return Result{Text: msg.Content, StopReason: finish, Iterations: hop}, nil
		}

		// Record the assistant turn (carries the tool calls), then run each
		// tool and append its result keyed by tool-call ID.
		messages = append(messages, msg.ToParam())
		for _, tc := range msg.ToolCalls {
			out := c.runTool(ctx, registry, sc, tc.Function.Name, tc.Function.Arguments)
			messages = append(messages, openai.ToolMessage(out, tc.ID))
		}
	}

	// Hop cap reached while still calling tools. Force a final, tool-free
	// answer so the user gets a reply instead of silence.
	msg, _, err := c.complete(ctx, messages, nil)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: msg.Content, StopReason: "max_tool_hops", Iterations: c.cfg.MaxToolHops}, nil
}

func (c *Client) complete(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (openai.ChatCompletionMessage, string, error) {
	params := openai.ChatCompletionNewParams{
		Model:    c.cfg.Model,
		Messages: messages,
	}
	if c.cfg.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(c.cfg.MaxTokens))
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	if len(c.cfg.Extra) > 0 {
		params.SetExtraFields(c.cfg.Extra)
	}

	resp, err := c.inner.Chat.Completions.New(ctx, params)
	if err != nil {
		return openai.ChatCompletionMessage{}, "", fmt.Errorf("llm: chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return openai.ChatCompletionMessage{}, "", errors.New("llm: response had no choices")
	}
	return resp.Choices[0].Message, resp.Choices[0].FinishReason, nil
}

func (c *Client) runTool(ctx context.Context, registry *tool.Registry, sc *tool.Ctx, name, argsJSON string) string {
	s, ok := registry.Get(name)
	if !ok {
		return fmt.Sprintf("tool error: unknown tool %q", name)
	}
	if argsJSON == "" {
		argsJSON = "{}"
	}
	out, err := s.Run(ctx, sc, json.RawMessage(argsJSON))
	if err != nil {
		return "tool error: " + err.Error()
	}
	return out
}

func buildTools(registry *tool.Registry) []openai.ChatCompletionToolUnionParam {
	list := registry.List()
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(list))
	for _, s := range list {
		tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        s.Name,
			Description: openai.String(s.Description),
			Parameters:  shared.FunctionParameters(s.InputSchema),
		}))
	}
	return tools
}
