package configs

import "strings"

// BuiltinCredentialStrategy defines how a builtin model preset obtains runtime credentials.
type BuiltinCredentialStrategy string

const (
	BuiltinCredentialStrategyMSCLIServer BuiltinCredentialStrategy = "mscli-server"
)

// BuiltinModelPreset defines a built-in model option exposed by the client UI.
type BuiltinModelPreset struct {
	ID                 string                    `yaml:"id"`
	Label              string                    `yaml:"label"`
	Provider           string                    `yaml:"provider"`
	BaseURL            string                    `yaml:"base_url"`
	Model              string                    `yaml:"model"`
	CredentialStrategy BuiltinCredentialStrategy `yaml:"credential_strategy"`
	CredentialRef      string                    `yaml:"credential_ref,omitempty"`
}

var builtinModelPresets = []BuiltinModelPreset{
	{
		ID:                 "kimi-k2.5",
		Label:              "kimi-k2.5 [free]",
		Provider:           "anthropic",
		BaseURL:            "https://api.kimi.com/coding/",
		Model:              "kimi-k2.5",
		CredentialStrategy: BuiltinCredentialStrategyMSCLIServer,
		CredentialRef:      "kimi-k2.5",
	},
}

// BuiltinModelPresets returns a copy of the built-in preset list.
func BuiltinModelPresets() []BuiltinModelPreset {
	if len(builtinModelPresets) == 0 {
		return nil
	}

	result := make([]BuiltinModelPreset, len(builtinModelPresets))
	copy(result, builtinModelPresets)
	return result
}

// LookupBuiltinModelPreset resolves a built-in preset by id or label.
func LookupBuiltinModelPreset(raw string) (BuiltinModelPreset, bool) {
	normalized := normalizeBuiltinModelPresetLookup(raw)
	if normalized == "" {
		return BuiltinModelPreset{}, false
	}

	for _, preset := range builtinModelPresets {
		if normalizeBuiltinModelPresetLookup(preset.ID) == normalized {
			return preset, true
		}
		if normalizeBuiltinModelPresetLookup(preset.Label) == normalized {
			return preset, true
		}
	}

	return BuiltinModelPreset{}, false
}

// ApplyBuiltinModelPreset overlays preset provider routing onto an existing model config.
func ApplyBuiltinModelPreset(base ModelConfig, preset BuiltinModelPreset, apiKey string) ModelConfig {
	applied := cloneModelConfig(base)
	applied.Provider = strings.TrimSpace(preset.Provider)
	applied.URL = strings.TrimSpace(preset.BaseURL)
	applied.Model = strings.TrimSpace(preset.Model)
	applied.Key = strings.TrimSpace(apiKey)
	return applied
}

func normalizeBuiltinModelPresetLookup(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(raw)), " "))
}

func cloneModelConfig(cfg ModelConfig) ModelConfig {
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
