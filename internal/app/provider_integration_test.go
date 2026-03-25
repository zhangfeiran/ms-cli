package app

import (
	"context"
	"testing"

	"github.com/vigo999/ms-cli/integrations/llm"
)

type fixedProvider struct {
	name string
}

func (p *fixedProvider) Name() string { return p.name }

func (p *fixedProvider) Complete(_ context.Context, _ *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{Content: "ok"}, nil
}

func (p *fixedProvider) CompleteStream(context.Context, *llm.CompletionRequest) (llm.StreamIterator, error) {
	return nil, nil
}

func (p *fixedProvider) SupportsTools() bool { return true }

func (p *fixedProvider) AvailableModels() []llm.ModelInfo { return nil }

func TestWire_DefaultAnthropicRouting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearWireTestEnv(t)

	t.Setenv("MSCLI_PROVIDER", "")
	t.Setenv("MSCLI_API_KEY", "mscli-token")

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

	if got, want := app.provider.Name(), "anthropic"; got != want {
		t.Fatalf("provider.Name() = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.URL, "https://api.kimi.com/coding/"; got != want {
		t.Fatalf("config.Model.URL = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Model, "kimi-k2.5"; got != want {
		t.Fatalf("config.Model.Model = %q, want %q", got, want)
	}
	if got, want := gotResolved.Kind, llm.ProviderAnthropic; got != want {
		t.Fatalf("resolved.Kind = %q, want %q", got, want)
	}
	if got, want := gotResolved.BaseURL, "https://api.kimi.com/coding/"; got != want {
		t.Fatalf("resolved.BaseURL = %q, want %q", got, want)
	}
}

func clearWireTestEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"MSCLI_PROVIDER",
		"MSCLI_API_KEY",
		"MSCLI_BASE_URL",
		"MSCLI_MODEL",
		"MSCLI_TEMPERATURE",
		"MSCLI_MAX_TOKENS",
		"MSCLI_TIMEOUT",
		"MSCLI_CONTEXT_WINDOW",
		"MSCLI_CONTEXT_RESERVE",
		"MSCLI_SERVER_URL",
	} {
		t.Setenv(key, "")
	}
}
