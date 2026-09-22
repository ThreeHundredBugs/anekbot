package llm

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/ThreeHundredBugs/anekbot/internal/flowcontrol"
)

const (
	maxOutputTokens  = 1024
	maxResponseBytes = 1 << 20

	maxConcurrent = 16
	userLimit     = 5
	userWindow    = time.Minute
	pruneInterval = 30 * time.Minute
	maxUsers      = 100_000
)

var ErrBusy = errors.New("llm: rate limit or concurrency limit reached")

type Provider interface {
	Name() string
	Ask(ctx context.Context, systemPrompt, question string) (string, error)
}

type LLM struct {
	systemPrompt string
	providers    []Provider

	concurrency *flowcontrol.Semaphore
	perUser     *flowcontrol.PerKeyLimiter[int64]
}

func New(systemPrompt string, providers ...Provider) *LLM {
	return &LLM{
		systemPrompt: systemPrompt,
		providers:    providers,
		concurrency:  flowcontrol.NewSemaphore(maxConcurrent),
		perUser: &flowcontrol.PerKeyLimiter[int64]{
			Limit:         userLimit,
			Window:        userWindow,
			PruneInterval: pruneInterval,
			MaxKeys:       maxUsers,
		},
	}
}

func (l *LLM) AskFor(ctx context.Context, userID int64, question string) (answer, providerName string, err error) {
	if !l.perUser.Allow(userID) {
		return "", "", ErrBusy
	}
	release, ok := l.concurrency.TryAcquire()
	if !ok {
		return "", "", ErrBusy
	}
	defer release()

	return l.Ask(ctx, question)
}

// Ask tries each provider in order, returning the first successful answer.
func (l *LLM) Ask(ctx context.Context, question string) (answer, providerName string, err error) {
	for i, provider := range l.providers {
		answer, err = provider.Ask(ctx, l.systemPrompt, question)
		if err == nil {
			return answer, provider.Name(), nil
		}
		if i == 0 {
			log.Printf("WARN llm: primary provider: %v", err)
		} else {
			log.Printf("WARN llm: fallback provider %s: %v", provider.Name(), err)
		}
	}
	return "", "", err
}
