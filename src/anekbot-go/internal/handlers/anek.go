package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/text/encoding/charmap"
)

const (
	anekTrigger    = "анек!"
	defaultBaseURL = "http://rzhunemogu.ru"

	// http://rzhunemogu.ru/FAQ.aspx
	anekTypeNormal = 1
	anekType18Plus = 11
)

type AnekHandler struct {
	client    *http.Client
	baseURL   string
	randFloat func() float64
}

func NewAnekHandler() *AnekHandler {
	return &AnekHandler{
		client:    &http.Client{Timeout: 10 * time.Second},
		baseURL:   defaultBaseURL,
		randFloat: rand.Float64,
	}
}

func (h *AnekHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	if !strings.Contains(strings.ToLower(msg.Text), anekTrigger) {
		return
	}

	joke, err := h.fetchJoke(ctx)
	if err != nil {
		log.Printf("anek handler: fetch joke: %v", err)
		return
	}

	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            joke,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	}); err != nil {
		log.Printf("anek handler: send message: %v", err)
	}
}

func (h *AnekHandler) fetchJoke(ctx context.Context) (string, error) {
	anekType := anekTypeNormal
	if h.randFloat() > 0.85 {
		anekType = anekType18Plus
	}

	url := fmt.Sprintf("%s/RandJSON.aspx?CType=%d", h.baseURL, anekType)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// rzhunemogu.ru uses windows-1251 encoding, telegram wants utf8
	utf8Body, err := charmap.Windows1251.NewDecoder().Bytes(body)
	if err != nil {
		return "", fmt.Errorf("decode windows-1251 response: %w", err)
	}

	// The response body is NOT really a JSON
	joke := strings.TrimPrefix(string(utf8Body), `{"content":"`)
	joke = strings.TrimSuffix(joke, `"}`)
	return joke, nil
}
