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
	// nil disables the handler
	anek     *AnekHandler
	swearing *SwearingHandler
	gemini   *GeminiHandler
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
		var handlers []func(context.Context, Sender, *models.Update)
		if d.anek != nil {
			handlers = append(handlers, d.anek.Handle)
		}
		if d.swearing != nil {
			handlers = append(handlers, d.swearing.Handle)
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
		if d.anek != nil {
			d.anek.HandleInline(ctx, sender, update)
		}
	}
}
