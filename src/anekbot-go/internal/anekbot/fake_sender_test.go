package anekbot

import (
	"context"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// fakeSender records calls instead of talking to Telegram
type fakeSender struct {
	mu            sync.Mutex
	sentMessages  []*bot.SendMessageParams
	reactions     []*bot.SetMessageReactionParams
	inlineAnswers []*bot.AnswerInlineQueryParams
}

func (f *fakeSender) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentMessages = append(f.sentMessages, params)
	return &models.Message{}, nil
}

func (f *fakeSender) SetMessageReaction(_ context.Context, params *bot.SetMessageReactionParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reactions = append(f.reactions, params)
	return true, nil
}

func (f *fakeSender) AnswerInlineQuery(_ context.Context, params *bot.AnswerInlineQueryParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inlineAnswers = append(f.inlineAnswers, params)
	return true, nil
}
