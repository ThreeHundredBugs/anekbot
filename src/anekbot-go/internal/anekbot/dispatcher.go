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
}

type Dispatcher struct {
	anek     *AnekHandler
	swearing *SwearingHandler
}

func NewDispatcher(anek *AnekHandler, swearing *SwearingHandler) *Dispatcher {
	return &Dispatcher{anek: anek, swearing: swearing}
}

func (d *Dispatcher) Dispatch(ctx context.Context, sender Sender, update *models.Update) {
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		d.anek.Handle(ctx, sender, update)
	}()
	go func() {
		defer wg.Done()
		d.swearing.Handle(ctx, sender, update)
	}()

	wg.Wait()
}
