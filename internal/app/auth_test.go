package app

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/integrations/llm"
	"github.com/vigo999/ms-cli/tools"
	"github.com/vigo999/ms-cli/ui/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestCmdLogin_UsesServerAPIKeyInMemoryOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origHTTPClientFactory := newServerProfileHTTPClient
	newServerProfileHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if got, want := r.URL.String(), "https://mscli.test/me"; got != want {
					t.Fatalf("request URL = %q, want %q", got, want)
				}
				if got, want := r.Header.Get("Authorization"), "Bearer login-token"; got != want {
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

	var gotResolved llm.ResolvedConfig
	origBuildProvider := buildProvider
	buildProvider = func(resolved llm.ResolvedConfig) (llm.Provider, error) {
		gotResolved = resolved
		return &fixedProvider{name: string(resolved.Kind)}, nil
	}
	defer func() { buildProvider = origBuildProvider }()

	cfg := configs.DefaultConfig()
	cfg.Model.Key = ""
	cfg.Issues.ServerURL = "https://mscli.test"

	app := &Application{
		EventCh:       make(chan model.Event, 16),
		Config:        cfg,
		toolRegistry:  tools.NewRegistry(),
		replayBacklog: nil,
	}

	app.cmdLogin([]string{"login-token"})

	drainUntilEventType(t, app, model.IssueUserUpdate)
	drainUntilEventType(t, app, model.AgentReply)

	if got, want := app.Config.Model.Key, "server-runtime-key"; got != want {
		t.Fatalf("config.Model.Key = %q, want %q", got, want)
	}
	if got, want := app.llmReady, true; got != want {
		t.Fatalf("llmReady = %v, want %v", got, want)
	}
	if got, want := gotResolved.APIKey, "server-runtime-key"; got != want {
		t.Fatalf("resolved.APIKey = %q, want %q", got, want)
	}
	if got, want := gotResolved.Kind, llm.ProviderAnthropic; got != want {
		t.Fatalf("resolved.Kind = %q, want %q", got, want)
	}

	rawCreds, err := os.ReadFile(filepath.Join(home, ".ms-cli", "credentials.json"))
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if strings.Contains(string(rawCreds), "server-runtime-key") {
		t.Fatalf("credentials file should not persist runtime api key, got %s", string(rawCreds))
	}
}

func TestCmdLogin_WarnsWhenServerReturnsNoAPIKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	cfg.Issues.ServerURL = "https://mscli.test"

	app := &Application{
		EventCh: make(chan model.Event, 16),
		Config:  cfg,
	}

	app.cmdLogin([]string{"login-token"})

	drainUntilEventType(t, app, model.IssueUserUpdate)
	drainUntilEventType(t, app, model.AgentReply)
	ev := drainUntilEventType(t, app, model.ToolError)

	if !strings.Contains(ev.Message, "returned no api_key") {
		t.Fatalf("warning message = %q, want missing api_key details", ev.Message)
	}
	if !strings.Contains(ev.Message, "MSCLI_API_KEY") {
		t.Fatalf("warning message = %q, want MSCLI_API_KEY hint", ev.Message)
	}
}
