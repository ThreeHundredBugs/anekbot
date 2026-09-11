package anekbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultGeminiModel   = "gemini-3.6-flash"
	geminiRequestTimeout = 30 * time.Second
	geminiSystemPrompt   = "You are a helpful assistant replying in a Telegram chat. Keep answers concise. " +
		"This is a one-shot reply: the user cannot follow up or continue the conversation, so make your answer " +
		"self-contained and don't ask clarifying questions or offer to elaborate further. " +
		"Reply in Russian by default, unless the user's message is clearly written in another language, in which " +
		"case reply in that language instead. Don't format the message unless needed. " +
		"If you need to format, reply as Telegram HTML: only <b>, <i>, <u>, <s>, <code>, <pre> and <a href=\"...\"> tags are " +
		"supported, no other tags or Markdown syntax. Escape any literal <, > and & that aren't part of a tag."

	telegramMessageMaxRunes = 4096

	geminiUnavailableMessage = "Gemini сейчас недоступен, попробуйте ещё раз позже."
)

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type GeminiHandler struct {
	client         *http.Client
	baseURL        string
	apiKey         string
	model          string
	mentionPattern *regexp.Regexp
}

func NewGeminiHandler(apiKey, model, botUsername string) *GeminiHandler {
	if model == "" {
		model = defaultGeminiModel
	}
	return &GeminiHandler{
		client:         &http.Client{Timeout: geminiRequestTimeout},
		baseURL:        defaultGeminiBaseURL,
		apiKey:         apiKey,
		model:          model,
		mentionPattern: regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(botUsername) + `\b`),
	}
}

func (h *GeminiHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	question, ok := h.extractQuestion(msg.Text)
	if !ok {
		return
	}

	answer, err := h.ask(ctx, question)
	if err != nil {
		log.Printf("gemini handler: ask: %v", err)
		if _, sendErr := sender.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			Text:            geminiUnavailableMessage,
			ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		}); sendErr != nil {
			log.Printf("gemini handler: send unavailable message: %v", sendErr)
		}
		return
	}
	answer = truncateToRunes(answer, telegramMessageMaxRunes)

	_, err = sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            answer,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	})
	if err == nil {
		return
	}
	log.Printf("gemini handler: send message: %v", err)

	// The model's HTML may be malformed; fall back to plain text rather than dropping the answer.
	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            answer,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	}); err != nil {
		log.Printf("gemini handler: send plain-text fallback: %v", err)
	}
}

func (h *GeminiHandler) extractQuestion(text string) (question string, ok bool) {
	loc := h.mentionPattern.FindStringIndex(text)
	if loc == nil {
		return "", false
	}

	question = strings.TrimSpace(text[:loc[0]] + text[loc[1]:])
	if question == "" {
		return "", false
	}
	return question, true
}

func (h *GeminiHandler) ask(ctx context.Context, question string) (string, error) {
	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: geminiSystemPrompt}}},
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: question}}},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", h.baseURL, h.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", h.apiKey)

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed geminiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil {
			return "", fmt.Errorf("gemini api: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("gemini api: unexpected status %d", resp.StatusCode)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini api: no candidates returned")
	}

	return strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text), nil
}

func truncateToRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
