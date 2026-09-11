package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ThreeHundredBugs/anekbot/src/anekbot-go/internal/anekbot"
)

const shutdownTimeout = 5 * time.Second

type config struct {
	botToken        string
	mode            string
	port            string
	webhookPath     string
	webhookSecret   string
	swearWordsFile  string
	geminiAPIKey    string
	geminiModel     string
	disableAnek     bool
	disableSwearing bool
	disableGemini   bool
}

func loadConfig(args []string) (*config, error) {
	fs := flag.NewFlagSet("anekbot", flag.ContinueOnError)

	botToken := fs.String("bot-token", os.Getenv("BOT_TOKEN"), "Telegram bot token (env BOT_TOKEN)")
	mode := fs.String("mode", envOrDefault("ANEKBOT_MODE", "webhook"), "run mode: webhook or poll (env ANEKBOT_MODE)")
	port := fs.String("port", envOrDefault("PORT", "8080"), "HTTP port to listen on in webhook mode (env PORT)")
	webhookPath := fs.String("webhook-path", envOrDefault("WEBHOOK_PATH", "/webhook"), "HTTP path Telegram will POST updates to (env WEBHOOK_PATH)")
	webhookSecret := fs.String("webhook-secret", os.Getenv("WEBHOOK_SECRET_TOKEN"), "optional secret validated against X-Telegram-Bot-Api-Secret-Token (env WEBHOOK_SECRET_TOKEN)")
	swearWordsFile := fs.String("swearwords-file", os.Getenv("SWEARWORDS_FILE"), "optional path to an extra swear word list (one word per line) merged with the built-in list (env SWEARWORDS_FILE)")
	geminiAPIKey := fs.String("gemini-api-key", os.Getenv("GEMINI_API_KEY"), "optional Gemini API key; when set, @mentioning the bot asks Gemini and replies with the answer (env GEMINI_API_KEY)")
	geminiModel := fs.String("gemini-model", os.Getenv("GEMINI_MODEL"), "Gemini model used for the @mention LLM feature, defaults to gemini-3.6-flash (env GEMINI_MODEL)")
	disableAnek := fs.Bool("disable-anek", envBool("DISABLE_ANEK"), "disable the \"анек!\" joke trigger and inline joke queries (env DISABLE_ANEK)")
	disableSwearing := fs.Bool("disable-swearing", envBool("DISABLE_SWEARING"), "disable reacting to swear words (env DISABLE_SWEARING)")
	disableGemini := fs.Bool("disable-gemini", envBool("DISABLE_GEMINI"), "disable the @mention LLM feature even if -gemini-api-key is set (env DISABLE_GEMINI)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &config{
		botToken:        *botToken,
		mode:            *mode,
		port:            *port,
		webhookPath:     *webhookPath,
		webhookSecret:   *webhookSecret,
		swearWordsFile:  *swearWordsFile,
		geminiAPIKey:    *geminiAPIKey,
		geminiModel:     *geminiModel,
		disableAnek:     *disableAnek,
		disableSwearing: *disableSwearing,
		disableGemini:   *disableGemini,
	}

	if cfg.botToken == "" {
		return nil, errors.New("bot token is required: set -bot-token or BOT_TOKEN")
	}
	if cfg.mode != "webhook" && cfg.mode != "poll" {
		return nil, fmt.Errorf("invalid mode %q: must be %q or %q", cfg.mode, "webhook", "poll")
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	v, _ := strconv.ParseBool(os.Getenv(key))
	return v
}

func main() {
	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var anek *anekbot.AnekHandler
	if !cfg.disableAnek {
		anek = anekbot.NewAnekHandler()
	}

	var swearing *anekbot.SwearingHandler
	if !cfg.disableSwearing {
		swearing, err = anekbot.NewSwearingHandler(cfg.swearWordsFile)
		if err != nil {
			log.Fatalf("swearing handler: %v", err)
		}
	}

	dispatcher := anekbot.NewDispatcher(anek, swearing, nil)

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

	if cfg.geminiAPIKey != "" && !cfg.disableGemini {
		me, err := b.GetMe(ctx)
		if err != nil {
			log.Fatalf("get bot info: %v", err)
		}
		dispatcher.SetGemini(anekbot.NewGeminiHandler(cfg.geminiAPIKey, cfg.geminiModel, me.Username))
	}

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
	srv := &http.Server{Addr: ":" + cfg.port, Handler: mux}

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
