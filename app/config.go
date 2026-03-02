package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath = "configs/mscli.yaml"
	defaultAPIBase    = "http://localhost:4000/v1"
	defaultTimeoutSec = 120
)

// RuntimeConfig is the minimal runtime config needed for model calls.
type RuntimeConfig struct {
	Model ModelConfig `yaml:"model"`
}

type ModelConfig struct {
	Provider      string `yaml:"provider"`
	Endpoint      string `yaml:"endpoint"`
	Name          string `yaml:"name"`
	SystemPrompt  string `yaml:"system_prompt"`
	TimeoutSecond int    `yaml:"timeout_seconds"`
	APIKey        string `yaml:"-"`
}

func loadRuntimeConfig(path string) (RuntimeConfig, error) {
	cfg := RuntimeConfig{
		Model: ModelConfig{
			Provider:      "openai",
			Endpoint:      defaultAPIBase,
			TimeoutSecond: defaultTimeoutSec,
		},
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return cfg, fmt.Errorf("read config %q: %w", path, err)
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %q: %w", path, err)
		}
	}

	applyEnvString(&cfg.Model.Endpoint, "OPENAI_BASE_URL")
	applyEnvString(&cfg.Model.Endpoint, "OPENAI_API_BASE")
	applyEnvString(&cfg.Model.Name, "OPENAI_MODEL")
	applyEnvString(&cfg.Model.APIKey, "OPENAI_API_KEY")
	applyEnvString(&cfg.Model.Provider, "MSCLI_MODEL_PROVIDER")
	applyEnvString(&cfg.Model.SystemPrompt, "MSCLI_SYSTEM_PROMPT")
	if err := applyEnvInt(&cfg.Model.TimeoutSecond, "MSCLI_LLM_TIMEOUT_SECONDS"); err != nil {
		return cfg, err
	}

	cfg.Model.Provider = strings.ToLower(strings.TrimSpace(cfg.Model.Provider))
	if cfg.Model.Provider == "" {
		cfg.Model.Provider = "openai"
	}
	cfg.Model.Endpoint = strings.TrimSpace(cfg.Model.Endpoint)
	if cfg.Model.Endpoint == "" {
		cfg.Model.Endpoint = defaultAPIBase
	}
	cfg.Model.Name = strings.TrimSpace(cfg.Model.Name)
	cfg.Model.SystemPrompt = strings.TrimSpace(cfg.Model.SystemPrompt)
	if cfg.Model.TimeoutSecond <= 0 {
		cfg.Model.TimeoutSecond = defaultTimeoutSec
	}

	return cfg, nil
}

func (m ModelConfig) Timeout() time.Duration {
	return time.Duration(m.TimeoutSecond) * time.Second
}

func applyEnvString(dst *string, key string) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		*dst = v
	}
}

func applyEnvInt(dst *int, key string) error {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid %s=%q: %w", key, v, err)
	}
	*dst = parsed
	return nil
}
