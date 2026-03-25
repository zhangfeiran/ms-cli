package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/ui/model"
)

func TestCmdModel_UnprefixedKeepsProvider(t *testing.T) {
	app := newModelCommandTestApp()
	app.Config.Model.Provider = "anthropic"
	app.Config.Model.Model = "claude-3-5-sonnet"

	app.cmdModel([]string{"claude-3-5-haiku"})

	drainUntilEventType(t, app, model.AgentThinking)
	drainUntilEventType(t, app, model.ModelUpdate)
	drainUntilEventType(t, app, model.AgentReply)

	if got, want := app.Config.Model.Provider, "anthropic"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Model, "claude-3-5-haiku"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
}

func TestCmdModel_PrefixedUpdatesProviderAndModel(t *testing.T) {
	app := newModelCommandTestApp()
	app.Config.Model.Provider = "openai-completion"
	app.Config.Model.Model = "gpt-4o-mini"

	app.cmdModel([]string{"anthropic:claude-3-5-sonnet"})

	drainUntilEventType(t, app, model.AgentThinking)
	drainUntilEventType(t, app, model.ModelUpdate)
	drainUntilEventType(t, app, model.AgentReply)

	if got, want := app.Config.Model.Provider, "anthropic"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Model, "claude-3-5-sonnet"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
}

func TestCmdModel_ModelUpdateCarriesContextWindow(t *testing.T) {
	app := newModelCommandTestApp()
	app.Config.Context.Window = 200000

	app.cmdModel([]string{"gpt-4o"})

	drainUntilEventType(t, app, model.AgentThinking)
	ev := drainUntilEventType(t, app, model.ModelUpdate)

	if got, want := ev.CtxMax, 200000; got != want {
		t.Fatalf("model update ctx max = %d, want %d", got, want)
	}
}

func TestCmdModel_InvalidPrefixNoMutation(t *testing.T) {
	app := newModelCommandTestApp()
	app.Config.Model.Provider = "openai-completion"
	app.Config.Model.Model = "gpt-4o-mini"

	app.cmdModel([]string{"invalid:gpt-4o"})

	ev := drainUntilEventType(t, app, model.AgentReply)
	if !strings.Contains(ev.Message, "Unsupported provider prefix") {
		t.Fatalf("unexpected message: %q", ev.Message)
	}

	if got, want := app.Config.Model.Provider, "openai-completion"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Model, "gpt-4o-mini"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
}

func TestCmdModel_ShowCurrentModelIncludesBuiltinCandidates(t *testing.T) {
	app := newModelCommandTestApp()

	app.cmdModel(nil)

	ev := drainUntilEventType(t, app, model.AgentReply)
	if !strings.Contains(ev.Message, "Builtin Model Candidates") {
		t.Fatalf("expected builtin model candidates in message: %q", ev.Message)
	}
	if !strings.Contains(ev.Message, "kimi-k2.5 [free]") {
		t.Fatalf("expected kimi preset label in message: %q", ev.Message)
	}
}

func TestCmdModel_SelectBuiltinPresetAppliesRuntimeOverride(t *testing.T) {
	app := newModelCommandTestApp()
	app.Config.Model.Provider = "openai-completion"
	app.Config.Model.Model = "gpt-4o-mini"
	app.Config.Model.URL = "https://api.openai.com/v1"
	app.Config.Model.Key = "env-key"
	app.baseModelConfig = copyModelConfig(app.Config.Model)
	app.baseContextConfig = app.Config.Context

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := saveCredentials(&credentials{
		ServerURL: "https://server.test",
		Token:     "saved-token",
		User:      "alice",
		Role:      "member",
	}); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}

	origFetch := fetchBuiltinModelCredential
	fetchBuiltinModelCredential = func(ctx context.Context, cred *credentials, preset configs.BuiltinModelPreset) (string, error) {
		if got, want := cred.Token, "saved-token"; got != want {
			t.Fatalf("credentials token = %q, want %q", got, want)
		}
		if got, want := preset.ID, "kimi-k2.5"; got != want {
			t.Fatalf("preset id = %q, want %q", got, want)
		}
		return "free-key", nil
	}
	defer func() { fetchBuiltinModelCredential = origFetch }()

	app.cmdModel([]string{"kimi-k2.5"})

	drainUntilEventType(t, app, model.AgentThinking)
	drainUntilEventType(t, app, model.ModelUpdate)
	drainUntilEventType(t, app, model.AgentReply)

	if got, want := app.Config.Model.Provider, "anthropic"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Model, "kimi-k2.5"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.URL, "https://api.kimi.com/coding/"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Key, "free-key"; got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
	if got, want := app.baseModelConfig.Key, "env-key"; got != want {
		t.Fatalf("base key = %q, want %q", got, want)
	}
	if got, want := app.activeModelSelection.Source, modelSelectionSourceBuiltinPreset; got != want {
		t.Fatalf("selection source = %q, want %q", got, want)
	}
}

func TestCmdModel_BuiltinPresetRequiresLogin(t *testing.T) {
	app := newModelCommandTestApp()
	t.Setenv("HOME", t.TempDir())

	app.cmdModel([]string{"kimi-k2.5"})

	drainUntilEventType(t, app, model.AgentThinking)
	ev := drainUntilEventType(t, app, model.ToolError)
	if !strings.Contains(ev.Message, "not logged in") {
		t.Fatalf("expected login error, got %q", ev.Message)
	}
}

func TestCmdModel_DefaultRestoresBaseConfigAfterBuiltinPreset(t *testing.T) {
	app := newModelCommandTestApp()
	app.Config.Model.Provider = "openai-completion"
	app.Config.Model.Model = "gpt-4o-mini"
	app.Config.Model.URL = "https://example.test/v1"
	app.Config.Model.Key = "env-key"
	app.baseModelConfig = copyModelConfig(app.Config.Model)
	app.baseContextConfig = app.Config.Context

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := saveCredentials(&credentials{
		ServerURL: "https://server.test",
		Token:     "saved-token",
		User:      "alice",
		Role:      "member",
	}); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}

	origFetch := fetchBuiltinModelCredential
	fetchBuiltinModelCredential = func(ctx context.Context, cred *credentials, preset configs.BuiltinModelPreset) (string, error) {
		return "free-key", nil
	}
	defer func() { fetchBuiltinModelCredential = origFetch }()

	app.cmdModel([]string{"kimi-k2.5"})
	drainUntilEventType(t, app, model.AgentThinking)
	drainUntilEventType(t, app, model.ModelUpdate)
	drainUntilEventType(t, app, model.AgentReply)

	app.cmdModel([]string{"default"})
	drainUntilEventType(t, app, model.AgentThinking)
	drainUntilEventType(t, app, model.ModelUpdate)
	drainUntilEventType(t, app, model.AgentReply)

	if got, want := app.Config.Model.Provider, "openai-completion"; got != want {
		t.Fatalf("provider after restore = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Model, "gpt-4o-mini"; got != want {
		t.Fatalf("model after restore = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.URL, "https://example.test/v1"; got != want {
		t.Fatalf("url after restore = %q, want %q", got, want)
	}
	if got, want := app.Config.Model.Key, "env-key"; got != want {
		t.Fatalf("key after restore = %q, want %q", got, want)
	}
	if got, want := app.activeModelSelection.Source, modelSelectionSourceDefault; got != want {
		t.Fatalf("selection source after restore = %q, want %q", got, want)
	}
}

func newModelCommandTestApp() *Application {
	cfg := configs.DefaultConfig()
	cfg.Model.Key = "test-key"
	return &Application{
		EventCh:              make(chan model.Event, 16),
		Config:               cfg,
		baseModelConfig:      copyModelConfig(cfg.Model),
		baseContextConfig:    cfg.Context,
		activeModelSelection: modelSelectionState{Source: modelSelectionSourceDefault},
	}
}

func drainUntilEventType(t *testing.T, app *Application, target model.EventType) model.Event {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for {
		select {
		case ev := <-app.EventCh:
			if ev.Type == target {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for event type %s", target)
		}
	}
}
