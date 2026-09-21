package anekbot

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type QuestionsHandler struct {
	llm            *LLM
	mentionPattern *regexp.Regexp
}

func NewQuestionsHandler(botUsername string, llm *LLM) *QuestionsHandler {
	return &QuestionsHandler{
		llm:            llm,
		mentionPattern: regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(botUsername) + `\b`),
	}
}

func (h *QuestionsHandler) Name() string {
	return "questions"
}

func (h *QuestionsHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	question, ok := h.extractQuestion(msg.Text)
	if !ok {
		return
	}
	logDebugf("questions handler: answering question in chat_id=%d", msg.Chat.ID)

	answer, providerName, err := h.llm.Ask(ctx, question)
	if err != nil {
		if _, sendErr := sender.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			Text:            llmUnavailableMessage,
			ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		}); sendErr != nil {
			logWarnf("questions handler: send unavailable message: %v", sendErr)
		}
		return
	}

	signature := fmt.Sprintf("\n\nby %s", providerName)
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
	logWarnf("questions handler: send message: %v", err)

	// The model's HTML may be malformed; fall back to plain text rather than dropping the answer.
	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            answer,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	}); err != nil {
		logWarnf("questions handler: send plain-text fallback: %v", err)
	}
}

func (h *QuestionsHandler) extractQuestion(text string) (question string, ok bool) {
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
