// Package config parses anekbot-go's startup configuration from flags, with
// environment variables as fallback defaults (so it works identically when
// run manually or via systemd/docker with only env vars set).
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

const (
	ModeWebhook = "webhook"
	ModePoll    = "poll"
)

type Config struct {
	BotToken      string
	Mode          string
	Port          string
	WebhookPath   string
	WebhookSecret string
}

// Load parses configuration from command-line flags (given in args, without
// the program name) with environment variables as defaults.
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("anekbot-go", flag.ContinueOnError)

	botToken := fs.String("bot-token", os.Getenv("BOT_TOKEN"), "Telegram bot token (env BOT_TOKEN)")
	mode := fs.String("mode", envOrDefault("ANEKBOT_MODE", ModeWebhook), "run mode: webhook or poll (env ANEKBOT_MODE)")
	port := fs.String("port", envOrDefault("PORT", "8080"), "HTTP port to listen on in webhook mode (env PORT)")
	webhookPath := fs.String("webhook-path", envOrDefault("WEBHOOK_PATH", "/webhook"), "HTTP path Telegram will POST updates to (env WEBHOOK_PATH)")
	webhookSecret := fs.String("webhook-secret", os.Getenv("WEBHOOK_SECRET_TOKEN"), "optional secret validated against X-Telegram-Bot-Api-Secret-Token (env WEBHOOK_SECRET_TOKEN)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{
		BotToken:      *botToken,
		Mode:          *mode,
		Port:          *port,
		WebhookPath:   *webhookPath,
		WebhookSecret: *webhookSecret,
	}

	if cfg.BotToken == "" {
		return nil, errors.New("bot token is required: set -bot-token or BOT_TOKEN")
	}
	if cfg.Mode != ModeWebhook && cfg.Mode != ModePoll {
		return nil, fmt.Errorf("invalid mode %q: must be %q or %q", cfg.Mode, ModeWebhook, ModePoll)
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
