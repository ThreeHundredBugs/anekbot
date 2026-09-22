package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ThreeHundredBugs/anekbot/internal/llm"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"BOT_TOKEN", "ANEKBOT_MODE", "LOG_LEVEL", "PORT", "WEBHOOK_SECRET_TOKEN", "ANEKBOT_CONFIG", "GEMINI_API_KEY", "HF_API_KEY"} {
		t.Setenv(k, "")
	}
}

func TestLoadConfig_ExampleFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("GEMINI_API_KEY", "g")
	t.Setenv("HF_API_KEY", "h")

	cfg, err := loadConfig([]string{"-config", "../../config.example.json"})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.mode != "poll" || cfg.promotions == nil || len(cfg.llmProviders) != 2 {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	clearEnv(t)
	cfg, err := loadConfig([]string{"-config", writeConfig(t, `{"bot": {"token": "t"}}`)})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !cfg.anekEnabled || !cfg.inlineEnabled || !cfg.aiJokesEnabled || !cfg.questionsEnabled || !cfg.swearingEnabled {
		t.Errorf("features should default to enabled: %+v", cfg)
	}
	if cfg.mode != "webhook" || cfg.port != "8080" || cfg.webhookPath != "/webhook" || cfg.logLevel != "warn" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if len(cfg.llmProviders) != 0 {
		t.Errorf("no llm section must mean no providers, got %d", len(cfg.llmProviders))
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `{"bot": {"token": "file-token", "mode": "poll"}, "server": {"port": "1111"}}`)
	t.Setenv("PORT", "2222")
	t.Setenv("BOT_TOKEN", "env-token")

	cfg, err := loadConfig([]string{"-config", path})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.botToken != "env-token" || cfg.port != "2222" || cfg.mode != "poll" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfig_ConfigPathFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ANEKBOT_CONFIG", writeConfig(t, `{"bot": {"token": "t", "mode": "poll"}}`))

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.mode != "poll" {
		t.Errorf("mode = %q, want poll from the file named by ANEKBOT_CONFIG", cfg.mode)
	}
}

func TestLoadConfig_DisabledFeatures(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `{"bot": {"token": "t"},
		"anek": {"enabled": false, "inline": {"ai_jokes": false}},
		"questions": {"enabled": false}, "swearing": {"enabled": false}}`)

	cfg, err := loadConfig([]string{"-config", path})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.anekEnabled || cfg.aiJokesEnabled || cfg.questionsEnabled || cfg.swearingEnabled || !cfg.inlineEnabled {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfig_LLMProviderKeysFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("GEMINI_API_KEY", "default-gemini-key")
	t.Setenv("MY_HF_KEY", "custom-hf-key")
	path := writeConfig(t, `{"bot": {"token": "t"}, "llm": {"providers": [
		{"type": "gemini"},
		{"type": "huggingface", "api_key_env": "MY_HF_KEY"},
		{"type": "huggingface", "api_key_env": "UNSET_KEY"}
	]}}`)

	cfg, err := loadConfig([]string{"-config", path})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.llmProviders) != 2 {
		t.Fatalf("providers = %d, want 2 (provider with empty key is skipped)", len(cfg.llmProviders))
	}
	if cfg.llmProviders[0].Name() != "Gemini" {
		t.Errorf("first provider = %q, want Gemini (order must be kept)", cfg.llmProviders[0].Name())
	}
}

func TestLoadConfig_LLMRateLimit(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `{"bot": {"token": "t"}, "llm": {"rate_limit": {
		"max_concurrent": 4,
		"per_user_limit": 2,
		"per_user_window_seconds": 30,
		"max_users": 100,
		"prune_interval_seconds": 60
	}}}`)

	cfg, err := loadConfig([]string{"-config", path})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := llm.Limits{
		MaxConcurrent: 4,
		PerUserLimit:  2,
		PerUserWindow: 30 * time.Second,
		MaxUsers:      100,
		PruneInterval: 60 * time.Second,
	}
	if cfg.llmLimits != want {
		t.Errorf("llmLimits = %+v, want %+v", cfg.llmLimits, want)
	}
}

func TestLoadConfig_LLMRateLimit_DefaultsToZeroValue(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `{"bot": {"token": "t"}}`)

	cfg, err := loadConfig([]string{"-config", path})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.llmLimits != (llm.Limits{}) {
		t.Errorf("llmLimits = %+v, want zero value (llm.New applies its own defaults)", cfg.llmLimits)
	}
}

func TestLoadConfig_Invalid(t *testing.T) {
	clearEnv(t)
	tests := map[string]string{
		"unknown key":       `{"bot": {"token": "t", "tokn": "x"}}`,
		"api key in file":   `{"bot": {"token": "t"}, "llm": {"providers": [{"type": "gemini", "api_key": "x"}]}}`,
		"unknown provider":  `{"bot": {"token": "t"}, "llm": {"providers": [{"type": "gpt"}]}}`,
		"bad promotions":    `{"bot": {"token": "t"}, "anek": {"inline": {"promotions": {"frequency": 2}}}}`,
		"invalid json":      `{`,
		"missing bot token": `{}`,
		"invalid mode":      `{"bot": {"token": "t", "mode": "carrier-pigeon"}}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := loadConfig([]string{"-config", writeConfig(t, content)}); err == nil {
				t.Error("expected error")
			}
		})
	}
}
