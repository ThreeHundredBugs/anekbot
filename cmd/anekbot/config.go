package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ThreeHundredBugs/anekbot/internal/anekbot"
	"github.com/ThreeHundredBugs/anekbot/internal/llm"
	"github.com/ThreeHundredBugs/anekbot/internal/logging"
)

type config struct {
	botToken      string
	mode          string
	logLevel      string
	port          string
	webhookPath   string
	webhookSecret string

	// llmProviders is tried in order; empty means no LLM.
	llmProviders []llm.Provider
	llmLimits    llm.Limits

	anekEnabled       bool
	inlineEnabled     bool
	aiJokesEnabled    bool
	promotions        *anekbot.Promotions
	questionsEnabled  bool
	swearingEnabled   bool
	swearingWordsFile string
}

type fileConfig struct {
	Bot struct {
		Token    string `json:"token"`
		Mode     string `json:"mode"`
		LogLevel string `json:"log_level"`
	} `json:"bot"`
	Server struct {
		Port          string `json:"port"`
		WebhookPath   string `json:"webhook_path"`
		WebhookSecret string `json:"webhook_secret"`
	} `json:"server"`
	LLM struct {
		Providers []providerConfig `json:"providers"`
		RateLimit rateLimitConfig  `json:"rate_limit"`
	} `json:"llm"`
	Anek struct {
		Enabled *bool `json:"enabled"`
		Inline  struct {
			Enabled    *bool                     `json:"enabled"`
			AIJokes    *bool                     `json:"ai_jokes"`
			Promotions *anekbot.PromotionsConfig `json:"promotions"`
		} `json:"inline"`
	} `json:"anek"`
	Questions struct {
		Enabled *bool `json:"enabled"`
	} `json:"questions"`
	Swearing struct {
		Enabled   *bool  `json:"enabled"`
		WordsFile string `json:"words_file"`
	} `json:"swearing"`
}

type providerConfig struct {
	Type  string `json:"type"`
	Model string `json:"model"`
	// APIKeyEnv overrides the default env var for Type.
	APIKeyEnv string `json:"api_key_env"`
}

// rateLimitConfig is the "llm.rate_limit" section of the config file; a zero value in any
// field falls back to llm.Limits' default for it.
type rateLimitConfig struct {
	MaxConcurrent        int `json:"max_concurrent"`
	PerUserLimit         int `json:"per_user_limit"`
	PerUserWindowSeconds int `json:"per_user_window_seconds"`
	MaxUsers             int `json:"max_users"`
	PruneIntervalSeconds int `json:"prune_interval_seconds"`
}

func (c rateLimitConfig) toLimits() llm.Limits {
	return llm.Limits{
		MaxConcurrent: c.MaxConcurrent,
		PerUserLimit:  c.PerUserLimit,
		PerUserWindow: time.Duration(c.PerUserWindowSeconds) * time.Second,
		MaxUsers:      c.MaxUsers,
		PruneInterval: time.Duration(c.PruneIntervalSeconds) * time.Second,
	}
}

var defaultAPIKeyEnv = map[string]string{
	"gemini":      "GEMINI_API_KEY",
	"huggingface": "HF_API_KEY",
}

func buildProvider(pc providerConfig) (llm.Provider, error) {
	defEnv, ok := defaultAPIKeyEnv[pc.Type]
	if !ok {
		return nil, fmt.Errorf("llm.providers: unknown type %q: must be %q or %q", pc.Type, "gemini", "huggingface")
	}
	keyEnv := or(pc.APIKeyEnv, defEnv)
	key := os.Getenv(keyEnv)
	if key == "" {
		logging.Warnf("llm provider %s skipped: env var %s is empty", pc.Type, keyEnv)
		return nil, nil
	}
	if pc.Type == "gemini" {
		return llm.NewGeminiProvider(key, pc.Model), nil
	}
	return llm.NewHuggingFaceProvider(key, pc.Model), nil
}

func loadFileConfig(path string) (*fileConfig, error) {
	if path == "" {
		return &fileConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var fc fileConfig
	if err := dec.Decode(&fc); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	return &fc, nil
}

func enabled(v *bool) bool {
	return v == nil || *v
}

func loadConfig(args []string) (*config, error) {
	fs := flag.NewFlagSet("anekbot", flag.ContinueOnError)
	configPath := fs.String("config", os.Getenv("ANEKBOT_CONFIG"), "path to the JSON config file (env ANEKBOT_CONFIG)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	fc, err := loadFileConfig(*configPath)
	if err != nil {
		return nil, err
	}

	cfg := &config{
		botToken:      envOrDefault("BOT_TOKEN", fc.Bot.Token),
		mode:          envOrDefault("ANEKBOT_MODE", or(fc.Bot.Mode, "webhook")),
		logLevel:      envOrDefault("LOG_LEVEL", or(fc.Bot.LogLevel, "warn")),
		port:          envOrDefault("PORT", or(fc.Server.Port, "8080")),
		webhookPath:   or(fc.Server.WebhookPath, "/webhook"),
		webhookSecret: envOrDefault("WEBHOOK_SECRET_TOKEN", fc.Server.WebhookSecret),

		anekEnabled:       enabled(fc.Anek.Enabled),
		inlineEnabled:     enabled(fc.Anek.Inline.Enabled),
		aiJokesEnabled:    enabled(fc.Anek.Inline.AIJokes),
		questionsEnabled:  enabled(fc.Questions.Enabled),
		swearingEnabled:   enabled(fc.Swearing.Enabled),
		swearingWordsFile: fc.Swearing.WordsFile,
		llmLimits:         fc.LLM.RateLimit.toLimits(),
	}

	for _, pc := range fc.LLM.Providers {
		provider, err := buildProvider(pc)
		if err != nil {
			return nil, err
		}
		if provider != nil {
			cfg.llmProviders = append(cfg.llmProviders, provider)
		}
	}

	if fc.Anek.Inline.Promotions != nil {
		if cfg.promotions, err = anekbot.NewPromotions(*fc.Anek.Inline.Promotions); err != nil {
			return nil, err
		}
	}

	if cfg.botToken == "" {
		return nil, errors.New("bot token is required: set BOT_TOKEN or bot.token in the config file")
	}
	if cfg.mode != "webhook" && cfg.mode != "poll" {
		return nil, fmt.Errorf("invalid mode %q: must be %q or %q", cfg.mode, "webhook", "poll")
	}
	if _, err := logging.ParseLevel(cfg.logLevel); err != nil {
		return nil, err
	}

	return cfg, nil
}

func or(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
