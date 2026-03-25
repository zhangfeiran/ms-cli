package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/integrations/llm"
)

func TestInitProviderAnthropic(t *testing.T) {
	provider, err := initProvider(configs.ModelConfig{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
		Key:      "anthropic-token",
	}, providerResolveNoOverrides())
	if err != nil {
		t.Fatalf("initProvider() error = %v", err)
	}
	if provider == nil {
		t.Fatal("initProvider() provider = nil, want provider")
	}
	if got, want := provider.Name(), "anthropic"; got != want {
		t.Fatalf("provider.Name() = %q, want %q", got, want)
	}
}

func TestInitProviderAnthropicDefault(t *testing.T) {
	provider, err := initProvider(configs.ModelConfig{Model: "kimi-k2.5", Key: "mscli-token"}, providerResolveNoOverrides())
	if err != nil {
		t.Fatalf("initProvider() error = %v", err)
	}
	if provider == nil {
		t.Fatal("initProvider() provider = nil, want provider")
	}
	if got, want := provider.Name(), "anthropic"; got != want {
		t.Fatalf("provider.Name() = %q, want %q", got, want)
	}
}

func TestInitProviderMapsMissingKeyToAppSentinel(t *testing.T) {
	_, err := initProvider(configs.ModelConfig{Model: "kimi-k2.5"}, providerResolveNoOverrides())
	if err == nil {
		t.Fatal("initProvider() error = nil, want missing api key error")
	}
	if !errors.Is(err, errAPIKeyNotFound) {
		t.Fatalf("initProvider() error = %v, want errAPIKeyNotFound", err)
	}
}

func TestWireBootstrapKeyAndURLOverrideEnvDuringProviderInit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearWireTestEnv(t)

	t.Setenv("MSCLI_PROVIDER", "openai-completion")
	t.Setenv("MSCLI_API_KEY", "env-key")
	t.Setenv("MSCLI_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	var gotAuth string
	var gotPath string
	origBuildProvider := buildProvider
	buildProvider = func(resolved llm.ResolvedConfig) (llm.Provider, error) {
		return newOpenAICompletionTestProvider(t, resolved, func(req *http.Request) {
			gotPath = req.URL.Path
			gotAuth = req.Header.Get("Authorization")
		}), nil
	}
	defer func() { buildProvider = origBuildProvider }()

	app, err := Wire(BootstrapConfig{
		URL:   "https://example.test/v1",
		Key:   "cli-key",
		Model: "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("Wire() error = %v", err)
	}
	if got, want := app.Config.Model.URL, "https://example.test/v1"; got != want {
		t.Fatalf("config.Model.URL = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Key, "cli-key"; got != want {
		t.Fatalf("config.Model.Key = %q, want %q", got, want)
	}

	_, err = app.provider.Complete(context.Background(), &llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("provider.Complete() error = %v", err)
	}

	if got, want := gotPath, "/v1/chat/completions"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := gotAuth, "Bearer cli-key"; got != want {
		t.Fatalf("Authorization header = %q, want %q", got, want)
	}
}

func TestWire_LoadsRuntimeAPIKeyFromSavedLoginWhenConfigKeyMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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

	if err := saveCredentials(&credentials{
		ServerURL: "https://mscli.test",
		Token:     "saved-login-token",
		User:      "alice",
		Role:      "member",
	}); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}

	var gotResolved llm.ResolvedConfig
	origBuildProvider := buildProvider
	buildProvider = func(resolved llm.ResolvedConfig) (llm.Provider, error) {
		gotResolved = resolved
		return &fixedProvider{name: string(resolved.Kind)}, nil
	}
	defer func() { buildProvider = origBuildProvider }()

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	app, err := Wire(BootstrapConfig{})
	if err != nil {
		t.Fatalf("Wire() error = %v", err)
	}

	if got, want := app.Config.Model.Key, "server-runtime-key"; got != want {
		t.Fatalf("config.Model.Key = %q, want %q", got, want)
	}
	if got, want := app.llmReady, true; got != want {
		t.Fatalf("llmReady = %v, want %v", got, want)
	}
	if app.provider == nil {
		t.Fatal("provider = nil, want provider")
	}
	if got, want := gotResolved.APIKey, "server-runtime-key"; got != want {
		t.Fatalf("resolved.APIKey = %q, want %q", got, want)
	}
	if got, want := app.issueUser, "alice"; got != want {
		t.Fatalf("issueUser = %q, want %q", got, want)
	}
}

func providerResolveNoOverrides() llm.ResolveOptions {
	return llm.ResolveOptions{}
}
