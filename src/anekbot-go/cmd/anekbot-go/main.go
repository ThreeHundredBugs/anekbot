// Command anekbot-go runs the anekbot Telegram bot, either as a long-running
// HTTP server accepting Telegram webhooks (for production, behind a
// reverse-proxied VPS) or in long-poll mode (for local testing against a
// real bot token, with no public endpoint needed).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ThreeHundredBugs/anekbot/src/anekbot-go/internal/config"
	"github.com/ThreeHundredBugs/anekbot/src/anekbot-go/internal/handlers"
	"github.com/ThreeHundredBugs/anekbot/src/anekbot-go/internal/server"
)

const shutdownTimeout = 5 * time.Second

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	swearing, err := handlers.NewSwearingHandler(cfg.ExtraSwearWordListPath)
	if err != nil {
		log.Fatalf("swearing handler: %v", err)
	}
	dispatcher := handlers.NewDispatcher(handlers.NewAnekHandler(), swearing)

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
			dispatcher.Dispatch(ctx, b, update)
		}),
	}
	if cfg.WebhookSecret != "" {
		opts = append(opts, bot.WithWebhookSecretToken(cfg.WebhookSecret))
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		log.Fatalf("create bot: %v", err)
	}

	switch cfg.Mode {
	case config.ModePoll:
		log.Println("anekbot-go starting in poll mode")
		b.Start(ctx)
	case config.ModeWebhook:
		runWebhook(ctx, cfg, b)
	}
}

func runWebhook(ctx context.Context, cfg *config.Config, b *bot.Bot) {
	srv := server.New(":"+cfg.Port, cfg.WebhookPath, b.WebhookHandler())

	go func() {
		log.Printf("anekbot-go listening on %s (webhook path %s)", srv.Addr, cfg.WebhookPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
