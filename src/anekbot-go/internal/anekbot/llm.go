package anekbot

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	telegramMessageMaxRunes = 4096

	llmUnavailableMessage = "ИИ сейчас недоступен, попробуйте ещё раз позже."
)

const llmSystemPrompt = "You are a helpful assistant replying in a Telegram chat. Keep answers concise. " +
	"This is a one-shot reply: the user cannot follow up or continue the conversation, so make your answer " +
	"self-contained and don't ask clarifying questions or offer to elaborate further. " +
	"Reply in Russian by default, unless the user's message is clearly written in another language, in which " +
	"case reply in that language instead. Don't format the message unless needed. " +
	"If you need to format, reply as Telegram HTML: only <b>, <i>, <u>, <s>, <code>, <pre> and <a href=\"...\"> tags are " +
	"supported, no other tags or Markdown syntax. Escape any literal <, > and & that aren't part of a tag."

type LLMProvider interface {
	Name() string
	Ask(ctx context.Context, question string) (string, error)
}

type LLMHandler struct {
	primary        LLMProvider
	fallback       LLMProvider // nil if there's no fallback provider configured
	mentionPattern *regexp.Regexp
}

func NewLLMHandler(botUsername string, primary, fallback LLMProvider) *LLMHandler {
	return &LLMHandler{
		primary:        primary,
		fallback:       fallback,
		mentionPattern: regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(botUsername) + `\b`),
	}
}

func (h *LLMHandler) Name() string {
	return "llm"
}

func (h *LLMHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	question, ok := h.extractQuestion(msg.Text)
	if !ok {
		return
	}
	logDebugf("llm handler: answering question in chat_id=%d", msg.Chat.ID)

	provider := h.primary
	answer, err := provider.Ask(ctx, question)
	if err != nil {
		logWarnf("llm handler: primary provider: %v", err)
		if h.fallback != nil {
			provider = h.fallback
			answer, err = provider.Ask(ctx, question)
			if err != nil {
				logWarnf("llm handler: fallback provider: %v", err)
			}
		}
	}
	if err != nil {
		if _, sendErr := sender.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			Text:            llmUnavailableMessage,
			ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		}); sendErr != nil {
			logWarnf("llm handler: send unavailable message: %v", sendErr)
		}
		return
	}

	signature := fmt.Sprintf("\n\nby %s", provider.Name())
	answer = truncateToRunes(answer, telegramMessageMaxRunes-len([]rune(signature))) + signature

	_, err = sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            answer,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	})
	if err == nil {
		return
	}
	logWarnf("llm handler: send message: %v", err)

	// The model's HTML may be malformed; fall back to plain text rather than dropping the answer.
	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            answer,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	}); err != nil {
		logWarnf("llm handler: send plain-text fallback: %v", err)
	}
}

func (h *LLMHandler) extractQuestion(text string) (question string, ok bool) {
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

func truncateToRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
