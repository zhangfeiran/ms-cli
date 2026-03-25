package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/integrations/llm"
	"github.com/vigo999/ms-cli/tools"
	"github.com/vigo999/ms-cli/ui/model"
)

type blockingStreamProvider struct {
	started chan struct{}
}

func (p *blockingStreamProvider) Name() string {
	return "blocking"
}

func (p *blockingStreamProvider) Complete(context.Context, *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return nil, io.EOF
}

func (p *blockingStreamProvider) CompleteStream(ctx context.Context, req *llm.CompletionRequest) (llm.StreamIterator, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	return &blockingStreamIterator{ctx: ctx}, nil
}

func (p *blockingStreamProvider) SupportsTools() bool {
	return true
}

func (p *blockingStreamProvider) AvailableModels() []llm.ModelInfo {
	return nil
}

type blockingStreamIterator struct {
	ctx context.Context
}

func (it *blockingStreamIterator) Next() (*llm.StreamChunk, error) {
	<-it.ctx.Done()
	return nil, it.ctx.Err()
}

func (it *blockingStreamIterator) Close() error {
	return nil
}

func TestInterruptTokenCancelsActiveTask(t *testing.T) {
	provider := &blockingStreamProvider{started: make(chan struct{})}
	engine := loop.NewEngine(loop.EngineConfig{
		MaxIterations: 1,
		MaxTokens:     4096,
	}, provider, tools.NewRegistry())

	app := &Application{
		Engine:   engine,
		EventCh:  make(chan model.Event, 32),
		llmReady: true,
	}

	done := make(chan struct{})
	go func() {
		app.runTask("hello")
		close(done)
	}()

	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task to start")
	}

	app.processInput(interruptActiveTaskToken)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task cancellation")
	}

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case ev := <-app.EventCh:
			if ev.Type == model.ToolError && strings.Contains(strings.ToLower(ev.Message), "canceled") {
				t.Fatalf("expected interrupt cancellation to stay silent, got tool error %q", ev.Message)
			}
		case <-deadline:
			return
		}
	}
}

type singleReplyProvider struct{}

func (p *singleReplyProvider) Name() string {
	return "anthropic"
}

func (p *singleReplyProvider) Complete(context.Context, *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Content:      "ok",
		FinishReason: llm.FinishStop,
	}, nil
}

func (p *singleReplyProvider) CompleteStream(context.Context, *llm.CompletionRequest) (llm.StreamIterator, error) {
	return &singleReplyStreamIterator{done: false}, nil
}

func (p *singleReplyProvider) SupportsTools() bool {
	return true
}

func (p *singleReplyProvider) AvailableModels() []llm.ModelInfo {
	return nil
}

type singleReplyStreamIterator struct {
	done bool
}

func (it *singleReplyStreamIterator) Next() (*llm.StreamChunk, error) {
	if it.done {
		return nil, io.EOF
	}
	it.done = true
	return &llm.StreamChunk{
		Content:      "ok",
		FinishReason: llm.FinishStop,
	}, nil
}

func (it *singleReplyStreamIterator) Close() error {
	return nil
}

func TestRunTask_RetriesServerAPIKeyFetchBeforePrompting(t *testing.T) {
	clearWireTestEnv(t)

	origHTTPClientFactory := newServerProfileHTTPClient
	newServerProfileHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if got, want := r.URL.String(), "https://mscli.test/me"; got != want {
					t.Fatalf("request URL = %q, want %q", got, want)
				}
				if got, want := r.Header.Get("Authorization"), "Bearer saved-login-token"; got != want {
					t.Fatalf("Authorization header = %q, want %q", got, want)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"user":"alice","role":"member","api_key":"server-runtime-key"}`)),
				}, nil
			}),
		}
	}
	defer func() { newServerProfileHTTPClient = origHTTPClientFactory }()

	origBuildProvider := buildProvider
	buildProvider = func(resolved llm.ResolvedConfig) (llm.Provider, error) {
		if got, want := resolved.APIKey, "server-runtime-key"; got != want {
			t.Fatalf("resolved.APIKey = %q, want %q", got, want)
		}
		return &singleReplyProvider{}, nil
	}
	defer func() { buildProvider = origBuildProvider }()

	cfg := configs.DefaultConfig()
	cfg.Model.Key = ""

	app := &Application{
		EventCh:      make(chan model.Event, 32),
		Config:       cfg,
		llmReady:     false,
		toolRegistry: tools.NewRegistry(),
		loginCred: &credentials{
			ServerURL: "https://mscli.test",
			Token:     "saved-login-token",
			User:      "alice",
			Role:      "member",
		},
	}

	app.runTask("hello")

	deadline := time.After(2 * time.Second)
	var sawReply bool
	for {
		select {
		case ev := <-app.EventCh:
			if ev.Type == model.AgentReply && ev.Message == provideAPIKeyFirstMsg {
				t.Fatalf("unexpected provide-api-key prompt after runtime key retry")
			}
			if ev.Type == model.AgentReply && strings.Contains(ev.Message, "ok") {
				sawReply = true
			}
		case <-deadline:
			if !sawReply {
				t.Fatal("timed out waiting for successful agent reply")
			}
			if !app.llmReady {
				t.Fatal("llmReady = false, want true after runtime key retry")
			}
			if got, want := app.Config.Model.Key, "server-runtime-key"; got != want {
				t.Fatalf("config.Model.Key = %q, want %q", got, want)
			}
			return
		}
	}
}

func TestRunTask_ReportsServerRuntimeAPIKeyMissingBeforePrompting(t *testing.T) {
	clearWireTestEnv(t)

	origHTTPClientFactory := newServerProfileHTTPClient
	newServerProfileHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"user":"alice","role":"member"}`)),
				}, nil
			}),
		}
	}
	defer func() { newServerProfileHTTPClient = origHTTPClientFactory }()

	cfg := configs.DefaultConfig()
	cfg.Model.Key = ""

	app := &Application{
		EventCh:  make(chan model.Event, 32),
		Config:   cfg,
		llmReady: false,
		loginCred: &credentials{
			ServerURL: "https://mscli.test",
			Token:     "saved-login-token",
			User:      "alice",
			Role:      "member",
		},
	}

	app.runTask("hello")

	var sawDiag bool
	var sawProvide bool
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-app.EventCh:
			if ev.Type == model.ToolError && strings.Contains(ev.Message, "returned no api_key") {
				sawDiag = true
			}
			if ev.Type == model.AgentReply && ev.Message == provideAPIKeyFirstMsg {
				sawProvide = true
			}
		case <-deadline:
			if !sawDiag {
				t.Fatal("expected runtime api key diagnostic before provide-api-key prompt")
			}
			if !sawProvide {
				t.Fatal("expected provide-api-key prompt after runtime api key diagnostic")
			}
			return
		}
	}
}

func TestRunTask_WithoutLoginOrProviderKeyPromptsForEither(t *testing.T) {
	app := &Application{
		EventCh:  make(chan model.Event, 8),
		Config:   configs.DefaultConfig(),
		llmReady: false,
	}
	app.Config.Model.Key = ""

	app.runTask("hello")

	ev := drainUntilEventType(t, app, model.AgentReply)
	if got, want := ev.Message, provideProviderAPIKeyOrLoginFirstMsg; got != want {
		t.Fatalf("agent reply = %q, want %q", got, want)
	}
}
