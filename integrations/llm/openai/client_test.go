package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGenerateWithExplicitModel(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("unexpected auth header: %q", got)
			}

			var req chatCompletionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Model != "test-model" {
				t.Fatalf("unexpected model: %s", req.Model)
			}

			return jsonResponse(http.StatusOK, map[string]any{
				"choices": []any{
					map[string]any{
						"message": map[string]any{
							"content": "ok-from-explicit-model",
						},
					},
				},
			}), nil
		}),
	}

	client, err := NewClient(Config{
		BaseURL:    "http://localhost:4000/v1",
		APIKey:     "test-key",
		Model:      "test-model",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := client.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got != "ok-from-explicit-model" {
		t.Fatalf("unexpected response: %q", got)
	}
}

func TestGenerateAutoDiscoversAndCachesModel(t *testing.T) {
	t.Parallel()

	var modelRequests int32

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/v1/models":
				atomic.AddInt32(&modelRequests, 1)
				return jsonResponse(http.StatusOK, map[string]any{
					"data": []any{
						map[string]any{"id": "auto-model"},
					},
				}), nil
			case "/v1/chat/completions":
				var req chatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if req.Model != "auto-model" {
					t.Fatalf("unexpected model: %s", req.Model)
				}
				return jsonResponse(http.StatusOK, map[string]any{
					"choices": []any{
						map[string]any{
							"message": map[string]any{
								"content": "ok-from-auto-model",
							},
						},
					},
				}), nil
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
				return nil, nil
			}
		}),
	}

	client, err := NewClient(Config{
		BaseURL:    "http://localhost:4000/v1",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	for i := 0; i < 2; i++ {
		got, err := client.Generate(context.Background(), "hello")
		if err != nil {
			t.Fatalf("generate #%d: %v", i+1, err)
		}
		if got != "ok-from-auto-model" {
			t.Fatalf("unexpected response #%d: %q", i+1, got)
		}
	}

	if got := atomic.LoadInt32(&modelRequests); got != 1 {
		t.Fatalf("expected /models called once, got %d", got)
	}
}

func TestChatParsesToolCalls(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var req chatCompletionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if len(req.Tools) != 1 || req.Tools[0].Function.Name != "bash" {
				t.Fatalf("unexpected tools payload: %+v", req.Tools)
			}

			return jsonResponse(http.StatusOK, map[string]any{
				"choices": []any{
					map[string]any{
						"message": map[string]any{
							"content": "running command",
							"tool_calls": []any{
								map[string]any{
									"id":   "call_1",
									"type": "function",
									"function": map[string]any{
										"name":      "bash",
										"arguments": `{"command":"pwd"}`,
									},
								},
							},
						},
					},
				},
			}), nil
		}),
	}

	client, err := NewClient(Config{
		BaseURL:    "http://localhost:4000/v1",
		Model:      "test-model",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	reply, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	}, []ToolSpec{
		{
			Name:       "bash",
			Parameters: map[string]any{"type": "object"},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if reply.Content != "running command" {
		t.Fatalf("unexpected content: %q", reply.Content)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("unexpected toolcalls count: %d", len(reply.ToolCalls))
	}
	if reply.ToolCalls[0].Name != "bash" || reply.ToolCalls[0].Arguments != `{"command":"pwd"}` {
		t.Fatalf("unexpected toolcall: %+v", reply.ToolCalls[0])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(statusCode int, payload any) *http.Response {
	raw, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
