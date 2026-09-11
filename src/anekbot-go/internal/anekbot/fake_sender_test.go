package anekbot

import (
	"context"
	"errors"
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

	// failSendMessageIf, when set, makes SendMessage fail (without recording the
	// call) for any params it returns true for. Used to exercise fallback paths.
	failSendMessageIf func(*bot.SendMessageParams) bool
}

func (f *fakeSender) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSendMessageIf != nil && f.failSendMessageIf(params) {
		return nil, errors.New("fakeSender: simulated SendMessage failure")
	}
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
