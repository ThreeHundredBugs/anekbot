package handlers

import (
	"context"
	"sync"

	"github.com/go-telegram/bot/models"
)

// Dispatcher fans every update out to all registered handlers concurrently,
// mirroring the Python dispatcher's asyncio.gather(handle_anek, handle_swearing).
type Dispatcher struct {
	anek *AnekHandler
}

func NewDispatcher(anek *AnekHandler) *Dispatcher {
	return &Dispatcher{anek: anek}
}

// Dispatch runs all handlers concurrently. Each handler logs and swallows
// its own errors, so one handler's failure never affects the other.
func (d *Dispatcher) Dispatch(ctx context.Context, sender Sender, update *models.Update) {
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		d.anek.Handle(ctx, sender, update)
	}()
	go func() {
		defer wg.Done()
		HandleSwearing(ctx, sender, update)
	}()

	wg.Wait()
}
