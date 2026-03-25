package configs

import "testing"

func TestLookupBuiltinModelPresetByIDAndLabel(t *testing.T) {
	byID, ok := LookupBuiltinModelPreset("kimi-k2.5")
	if !ok {
		t.Fatal("LookupBuiltinModelPreset(kimi-k2.5) = false, want true")
	}
	if got, want := byID.CredentialStrategy, BuiltinCredentialStrategyMSCLIServer; got != want {
		t.Fatalf("credential strategy = %q, want %q", got, want)
	}

	byLabel, ok := LookupBuiltinModelPreset("kimi-k2.5 [free]")
	if !ok {
		t.Fatal("LookupBuiltinModelPreset(label) = false, want true")
	}
	if got, want := byLabel.BaseURL, "https://api.kimi.com/coding/"; got != want {
		t.Fatalf("base_url = %q, want %q", got, want)
	}
}

func TestApplyBuiltinModelPresetOverlaysProviderRouting(t *testing.T) {
	base := ModelConfig{
		URL:         "https://api.openai.com/v1",
		Key:         "env-key",
		Provider:    "openai-completion",
		Model:       "gpt-4o-mini",
		Temperature: 0.2,
		Headers: map[string]string{
			"X-Test": "1",
		},
	}
	preset, ok := LookupBuiltinModelPreset("kimi-k2.5")
	if !ok {
		t.Fatal("LookupBuiltinModelPreset() = false, want true")
	}

	applied := ApplyBuiltinModelPreset(base, preset, "free-key")

	if got, want := applied.Provider, "anthropic"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := applied.URL, "https://api.kimi.com/coding/"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if got, want := applied.Model, "kimi-k2.5"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := applied.Key, "free-key"; got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
	if got, want := applied.Temperature, base.Temperature; got != want {
		t.Fatalf("temperature = %v, want %v", got, want)
	}
	if got, want := applied.Headers["X-Test"], "1"; got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
}
