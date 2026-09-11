package anekbot

import (
	"context"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SetMessageReaction(ctx context.Context, params *bot.SetMessageReactionParams) (bool, error)
	AnswerInlineQuery(ctx context.Context, params *bot.AnswerInlineQueryParams) (bool, error)
}

type Dispatcher struct {
	anek     *AnekHandler
	swearing *SwearingHandler
	gemini   *GeminiHandler // nil when the @mention LLM feature is disabled
}

func NewDispatcher(anek *AnekHandler, swearing *SwearingHandler, gemini *GeminiHandler) *Dispatcher {
	return &Dispatcher{anek: anek, swearing: swearing, gemini: gemini}
}

func (d *Dispatcher) SetGemini(gemini *GeminiHandler) {
	d.gemini = gemini
}

func (d *Dispatcher) Dispatch(ctx context.Context, sender Sender, update *models.Update) {
	switch {
	case update.Message != nil:
		handlers := []func(context.Context, Sender, *models.Update){
			d.anek.Handle,
			d.swearing.Handle,
		}
		if d.gemini != nil {
			handlers = append(handlers, d.gemini.Handle)
		}

		var wg sync.WaitGroup
		wg.Add(len(handlers))
		for _, handle := range handlers {
			go func(handle func(context.Context, Sender, *models.Update)) {
				defer wg.Done()
				handle(ctx, sender, update)
			}(handle)
		}
		wg.Wait()
	case update.InlineQuery != nil:
		d.anek.HandleInline(ctx, sender, update)
	}
}
