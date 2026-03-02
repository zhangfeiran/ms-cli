package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL      = "http://localhost:4000/v1"
	defaultSystemPrompt = "You are a pragmatic coding assistant. Answer clearly and briefly."
)

// Config configures an OpenAI-compatible client.
type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
	HTTPClient   *http.Client
}

// Message is one chat message for OpenAI-compatible APIs.
type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall is one assistant function/tool call.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolSpec declares one tool the model can call.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ChatResponse is one assistant response.
type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// Client calls OpenAI-compatible chat completion APIs.
type Client struct {
	baseURL      string
	apiKey       string
	systemPrompt string
	httpClient   *http.Client

	mu    sync.Mutex
	model string
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = defaultBaseURL
	}
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	return &Client{
		baseURL:      strings.TrimRight(base, "/"),
		apiKey:       strings.TrimSpace(cfg.APIKey),
		model:        strings.TrimSpace(cfg.Model),
		systemPrompt: systemPrompt,
		httpClient:   httpClient,
	}, nil
}

// Generate sends one prompt and returns one completion.
func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	userPrompt := strings.TrimSpace(prompt)
	if userPrompt == "" {
		return "", fmt.Errorf("prompt is empty")
	}

	response, err := c.Chat(ctx, []Message{
		{Role: "system", Content: c.systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Content) == "" {
		return "", fmt.Errorf("completion response content is empty")
	}
	return response.Content, nil
}

// Chat sends a multi-message chat completion request with optional tools.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (ChatResponse, error) {
	if len(messages) == 0 {
		return ChatResponse{}, fmt.Errorf("messages are empty")
	}

	model, err := c.resolveModel(ctx)
	if err != nil {
		return ChatResponse{}, err
	}

	reqBody := chatCompletionRequest{
		Model:    model,
		Messages: make([]chatMessageRequest, 0, len(messages)),
	}
	for _, msg := range messages {
		reqMsg := chatMessageRequest{
			Role:       strings.TrimSpace(msg.Role),
			Content:    msg.Content,
			ToolCallID: strings.TrimSpace(msg.ToolCallID),
		}
		if reqMsg.Role == "" {
			return ChatResponse{}, fmt.Errorf("message role is empty")
		}
		if len(msg.ToolCalls) > 0 {
			reqMsg.ToolCalls = make([]chatToolCallRequest, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				reqMsg.ToolCalls = append(reqMsg.ToolCalls, chatToolCallRequest{
					ID:   strings.TrimSpace(tc.ID),
					Type: "function",
					Function: chatFunctionCallRequest{
						Name:      strings.TrimSpace(tc.Name),
						Arguments: tc.Arguments,
					},
				})
			}
		}
		reqBody.Messages = append(reqBody.Messages, reqMsg)
	}

	if len(tools) > 0 {
		reqBody.Tools = make([]chatToolSpec, 0, len(tools))
		for _, t := range tools {
			name := strings.TrimSpace(t.Name)
			if name == "" {
				return ChatResponse{}, fmt.Errorf("tool name is empty")
			}
			reqBody.Tools = append(reqBody.Tools, chatToolSpec{
				Type: "function",
				Function: chatToolFunction{
					Name:        name,
					Description: strings.TrimSpace(t.Description),
					Parameters:  t.Parameters,
				},
			})
		}
		reqBody.ToolChoice = "auto"
	}

	rawResp, err := c.doChatCompletion(ctx, reqBody)
	if err != nil {
		return ChatResponse{}, err
	}

	if len(rawResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("completion response has no choices")
	}

	choice := rawResp.Choices[0]
	resp := ChatResponse{}
	if choice.Message.Content != nil {
		resp.Content = strings.TrimSpace(*choice.Message.Content)
	} else {
		resp.Content = strings.TrimSpace(choice.Text)
	}

	if len(choice.Message.ToolCalls) > 0 {
		resp.ToolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))
		for _, tc := range choice.Message.ToolCalls {
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:        strings.TrimSpace(tc.ID),
				Name:      strings.TrimSpace(tc.Function.Name),
				Arguments: normalizeArguments(tc.Function.Arguments),
			})
		}
	}

	return resp, nil
}

func (c *Client) resolveModel(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.model != "" {
		model := c.model
		c.mu.Unlock()
		return model, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/models"), nil)
	if err != nil {
		return "", fmt.Errorf("create models request: %w", err)
	}
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request models list: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("models API %s: %s", resp.Status, trimBody(raw))
	}

	var out modelsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("parse models response: %w", err)
	}
	if len(out.Data) == 0 || strings.TrimSpace(out.Data[0].ID) == "" {
		return "", fmt.Errorf("models list is empty; set OPENAI_MODEL explicitly")
	}

	model := strings.TrimSpace(out.Data[0].ID)
	c.mu.Lock()
	c.model = model
	c.mu.Unlock()
	return model, nil
}

func (c *Client) endpoint(path string) string {
	if strings.HasPrefix(path, "/") {
		return c.baseURL + path
	}
	return c.baseURL + "/" + path
}

func (c *Client) applyAuth(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func trimBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) <= 600 {
		return s
	}
	return s[:600] + "..."
}

func normalizeArguments(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

func (c *Client) doChatCompletion(ctx context.Context, request chatCompletionRequest) (*chatCompletionResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request completion: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read completion response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("completion API %s: %s", resp.Status, trimBody(raw))
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse completion response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("completion API error: %s", out.Error.Message)
	}

	return &out, nil
}

type chatCompletionRequest struct {
	Model      string               `json:"model"`
	Messages   []chatMessageRequest `json:"messages"`
	Tools      []chatToolSpec       `json:"tools,omitempty"`
	ToolChoice string               `json:"tool_choice,omitempty"`
}

type chatMessageRequest struct {
	Role       string                `json:"role"`
	Content    string                `json:"content"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCallRequest `json:"tool_calls,omitempty"`
}

type chatToolCallRequest struct {
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type"`
	Function chatFunctionCallRequest `json:"function"`
}

type chatFunctionCallRequest struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatToolSpec struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessageResponse `json:"message"`
		Text    string              `json:"text"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type chatMessageResponse struct {
	Role      string                 `json:"role"`
	Content   *string                `json:"content"`
	ToolCalls []chatToolCallResponse `json:"tool_calls,omitempty"`
}

type chatToolCallResponse struct {
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function chatFunctionCallResponse `json:"function"`
}

type chatFunctionCallResponse struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}
