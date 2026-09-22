package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ThreeHundredBugs/anekbot/internal/anekbot"
	"github.com/ThreeHundredBugs/anekbot/internal/llm"
)

const shutdownTimeout = 5 * time.Second

type config struct {
	botToken      string
	mode          string
	logLevel      string
	port          string
	webhookPath   string
	webhookSecret string

	// llmProviders are tried in order; empty means no LLM is available.
	llmProviders []llm.Provider

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
	// APIKeyEnv overrides which env var holds the key; defaults depend on Type.
	APIKeyEnv string `json:"api_key_env"`
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
		log.Printf("llm provider %s skipped: env var %s is empty", pc.Type, keyEnv)
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

// enabled reports whether an optional "enabled"-style setting is on; unset means on.
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
	if _, err := anekbot.ParseLogLevel(cfg.logLevel); err != nil {
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

func main() {
	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logLevel, err := anekbot.ParseLogLevel(cfg.logLevel)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	anekbot.SetLogLevel(logLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var anek *anekbot.AnekHandler
	if cfg.anekEnabled {
		anek = anekbot.NewAnekHandler()
		anek.SetInline(cfg.inlineEnabled, cfg.aiJokesEnabled)
		anek.SetPromotions(cfg.promotions)
	}

	var swearing *anekbot.SwearingHandler
	if cfg.swearingEnabled {
		swearing, err = anekbot.NewSwearingHandler(cfg.swearingWordsFile)
		if err != nil {
			log.Fatalf("swearing handler: %v", err)
		}
	}

	dispatcher := anekbot.NewDispatcher(anek, swearing, nil, nil)

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
			dispatcher.Dispatch(ctx, b, update)
		}),
	}
	if cfg.webhookSecret != "" {
		opts = append(opts, bot.WithWebhookSecretToken(cfg.webhookSecret))
	}

	b, err := bot.New(cfg.botToken, opts...)
	if err != nil {
		log.Fatalf("create bot: %v", err)
	}

	me, err := b.GetMe(ctx)
	if err != nil {
		log.Fatalf("get bot info: %v", err)
	}

	var llmClient *llm.LLM
	if len(cfg.llmProviders) > 0 {
		llmClient = anekbot.NewLLM(cfg.llmProviders...)
	}
	if anek != nil {
		anek.SetLLM(llmClient)
	}
	hasQuestions := llmClient != nil && cfg.questionsEnabled
	if hasQuestions {
		dispatcher.SetQuestions(anekbot.NewQuestionsHandler(me.Username, llmClient))
	}

	dispatcher.SetHelp(anekbot.NewHelpHandler(me.Username, cfg.anekEnabled, cfg.swearingEnabled, hasQuestions))

	switch cfg.mode {
	case "poll":
		log.Println("anekbot starting in poll mode")
		b.Start(ctx)
	case "webhook":
		runWebhook(ctx, cfg, b)
	}
}

func runWebhook(ctx context.Context, cfg *config, b *bot.Bot) {
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.webhookPath, b.WebhookHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("anekbot listening on %s (webhook path %s)", srv.Addr, cfg.webhookPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	go b.StartWebhook(ctx)

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown: %v", err)
	}
}
