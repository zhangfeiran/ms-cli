package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/integrations/llm"
)

type modelSelectionSource string

const (
	modelSelectionSourceDefault       modelSelectionSource = "default"
	modelSelectionSourceManual        modelSelectionSource = "manual"
	modelSelectionSourceBuiltinPreset modelSelectionSource = "builtin-preset"
)

type modelSelectionState struct {
	Source     modelSelectionSource
	BuiltinID  string
	BuiltinRef string
}

type builtinModelCredentialResponse struct {
	ID     string `json:"id"`
	APIKey string `json:"api_key"`
}

var fetchBuiltinModelCredential = defaultBuiltinModelCredentialFetcher

func (a *Application) applyModelConfig(next configs.ModelConfig, selection modelSelectionState, resolveOpts llm.ResolveOptions) error {
	previousModel := a.Config.Model.Model

	a.Config.Model = copyModelConfig(next)
	if selection.Source == modelSelectionSourceDefault {
		a.Config.Context = a.baseContextConfig
	}
	configs.RefreshModelTokenDefaults(a.Config, previousModel)

	provider, err := initProvider(a.Config.Model, resolveOpts)
	if err != nil {
		if err == errAPIKeyNotFound {
			a.llmReady = false
			provider = nil
		} else {
			return fmt.Errorf("init provider: %w", err)
		}
	} else {
		a.llmReady = true
	}

	engineCfg := a.currentEngineConfig()
	newEngine := loop.NewEngine(engineCfg, provider, a.toolRegistry)
	if a.ctxManager != nil {
		if err := a.ctxManager.SetTokenLimits(a.Config.Context.Window, a.Config.Context.ReserveTokens); err != nil {
			return fmt.Errorf("update context limits: %w", err)
		}
	}
	newEngine.SetContextManager(a.ctxManager)
	newEngine.SetPermissionService(a.permService)
	newEngine.SetTrajectoryRecorder(newTrajectoryRecorder(a.session))

	a.Engine = newEngine
	a.provider = provider
	a.activeModelSelection = selection

	return nil
}

func (a *Application) switchBuiltinModelPreset(preset configs.BuiltinModelPreset) error {
	apiKey, err := a.resolveBuiltinModelCredential(preset)
	if err != nil {
		return err
	}

	next := configs.ApplyBuiltinModelPreset(a.Config.Model, preset, apiKey)
	return a.applyModelConfig(next, modelSelectionState{
		Source:     modelSelectionSourceBuiltinPreset,
		BuiltinID:  preset.ID,
		BuiltinRef: preset.CredentialRef,
	}, llm.ResolveOptions{
		PreferConfigAPIKey:  true,
		PreferConfigBaseURL: true,
	})
}

func (a *Application) restoreDefaultModelConfig() error {
	return a.applyModelConfig(copyModelConfig(a.baseModelConfig), modelSelectionState{
		Source: modelSelectionSourceDefault,
	}, llm.ResolveOptions{
		PreferConfigAPIKey:  strings.TrimSpace(a.baseModelConfig.Key) != "",
		PreferConfigBaseURL: strings.TrimSpace(a.baseModelConfig.URL) != "",
	})
}

func (a *Application) resolveBuiltinModelCredential(preset configs.BuiltinModelPreset) (string, error) {
	switch preset.CredentialStrategy {
	case configs.BuiltinCredentialStrategyMSCLIServer:
		cred, err := requireSavedCredentials()
		if err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return fetchBuiltinModelCredential(ctx, cred, preset)
	default:
		return "", fmt.Errorf("unsupported builtin model credential strategy: %s", preset.CredentialStrategy)
	}
}

func (a *Application) currentModelSourceLabel() string {
	switch a.activeModelSelection.Source {
	case modelSelectionSourceBuiltinPreset:
		if preset, ok := configs.LookupBuiltinModelPreset(a.activeModelSelection.BuiltinID); ok {
			return "builtin preset: " + preset.Label
		}
		if ref := strings.TrimSpace(a.activeModelSelection.BuiltinID); ref != "" {
			return "builtin preset: " + ref
		}
		return "builtin preset"
	case modelSelectionSourceManual:
		return "runtime override"
	default:
		return "startup config"
	}
}

func (a *Application) currentEngineConfig() loop.EngineConfig {
	return loop.EngineConfig{
		MaxIterations:  10,
		MaxTokens:      a.Config.Budget.MaxTokens,
		Temperature:    float32(a.Config.Model.Temperature),
		TimeoutPerTurn: time.Duration(a.Config.Model.TimeoutSec) * time.Second,
	}
}

func requireSavedCredentials() (*credentials, error) {
	cred, err := loadCredentials()
	if err != nil {
		return nil, fmt.Errorf("not logged in. Run /login <token> first")
	}

	if strings.TrimSpace(cred.ServerURL) == "" {
		return nil, fmt.Errorf("saved login is missing server URL")
	}
	if strings.TrimSpace(cred.Token) == "" {
		return nil, fmt.Errorf("saved login is missing token")
	}

	return cred, nil
}

func defaultBuiltinModelCredentialFetcher(ctx context.Context, cred *credentials, preset configs.BuiltinModelPreset) (string, error) {
	serverURL := strings.TrimRight(strings.TrimSpace(cred.ServerURL), "/")
	if serverURL == "" {
		return "", fmt.Errorf("saved login is missing server URL")
	}

	credentialID := strings.TrimSpace(preset.CredentialRef)
	if credentialID == "" {
		credentialID = strings.TrimSpace(preset.ID)
	}
	if credentialID == "" {
		return "", fmt.Errorf("builtin model preset %q has no credential reference", preset.Label)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/builtin-models/"+url.PathEscape(credentialID)+"/credential", nil)
	if err != nil {
		return "", fmt.Errorf("create builtin model credential request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cred.Token)

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("request builtin model credential: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", fmt.Errorf("read builtin model credential response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return "", fmt.Errorf("request builtin model credential: %s", msg)
	}

	var payload builtinModelCredentialResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode builtin model credential response: %w", err)
	}

	apiKey := strings.TrimSpace(payload.APIKey)
	if apiKey == "" {
		return "", fmt.Errorf("server returned empty api key for builtin model %q", preset.ID)
	}

	return apiKey, nil
}

func copyModelConfig(cfg configs.ModelConfig) configs.ModelConfig {
	cloned := cfg
	if len(cfg.Headers) == 0 {
		cloned.Headers = nil
		return cloned
	}

	cloned.Headers = make(map[string]string, len(cfg.Headers))
	for key, value := range cfg.Headers {
		cloned.Headers[key] = value
	}
	return cloned
}
