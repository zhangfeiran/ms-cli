package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/integrations/llm"
)

func TestWire_OpenAICompletionDefaultRouting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("MSCLI_PROVIDER", "")
	t.Setenv("MSCLI_API_KEY", "mscli-token")
	t.Setenv("MSCLI_BASE_URL", "https://example.test/v1")

	var gotPath string
	origBuildProvider := buildProvider
	buildProvider = func(resolved llm.ResolvedConfig) (llm.Provider, error) {
		return newOpenAICompletionTestProvider(t, resolved, func(req *http.Request) {
			gotPath = req.URL.Path
		}), nil
	}
	defer func() { buildProvider = origBuildProvider }()

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	defaultCfg := configs.DefaultConfig()
	defaultCfg.Model.Provider = "openai-completion"
	defaultCfg.Model.Model = "gpt-4o-mini"
	defaultCfg.Model.Key = ""
	configPath := filepath.Join(tempDir, ".ms-cli", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := configs.SaveToFile(defaultCfg, configPath); err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	app, err := Wire(BootstrapConfig{})
	if err != nil {
		t.Fatalf("Wire() error = %v", err)
	}

	if got, want := app.provider.Name(), "openai-completion"; got != want {
		t.Fatalf("provider.Name() = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.URL, "https://example.test/v1"; got != want {
		t.Fatalf("config.Model.URL = %q, want %q", got, want)
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
}
