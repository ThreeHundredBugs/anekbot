package llm

import (
	"context"
	"errors"
	"time"

	"github.com/ThreeHundredBugs/anekbot/internal/flowcontrol"
	"github.com/ThreeHundredBugs/anekbot/internal/logging"
)

const (
	maxOutputTokens  = 1024
	maxResponseBytes = 1 << 20

	defaultMaxConcurrent = 16
	defaultPerUserLimit  = 5
	defaultPerUserWindow = time.Minute
	defaultPruneInterval = 30 * time.Minute
	defaultMaxUsers      = 100_000
)

var ErrBusy = errors.New("llm: rate limit or concurrency limit reached")

type UserID int64

type Provider interface {
	Name() string
	Ask(ctx context.Context, systemPrompt, question string) (string, error)
}

// Limits configures AskFor's rate limiting. A zero value in any field falls back to its default.
type Limits struct {
	MaxConcurrent int
	PerUserLimit  int
	PerUserWindow time.Duration
	MaxUsers      int
	PruneInterval time.Duration
}

func (l Limits) withDefaults() Limits {
	if l.MaxConcurrent <= 0 {
		l.MaxConcurrent = defaultMaxConcurrent
	}
	if l.PerUserLimit <= 0 {
		l.PerUserLimit = defaultPerUserLimit
	}
	if l.PerUserWindow <= 0 {
		l.PerUserWindow = defaultPerUserWindow
	}
	if l.MaxUsers <= 0 {
		l.MaxUsers = defaultMaxUsers
	}
	if l.PruneInterval <= 0 {
		l.PruneInterval = defaultPruneInterval
	}
	return l
}

type LLM struct {
	systemPrompt string
	providers    []Provider

	concurrency *flowcontrol.Semaphore
	perUser     *flowcontrol.PerKeyLimiter[UserID]
}

func New(systemPrompt string, limits Limits, providers ...Provider) *LLM {
	limits = limits.withDefaults()
	return &LLM{
		systemPrompt: systemPrompt,
		providers:    providers,
		concurrency:  flowcontrol.NewSemaphore(limits.MaxConcurrent),
		perUser: &flowcontrol.PerKeyLimiter[UserID]{
			Limit:         limits.PerUserLimit,
			Window:        limits.PerUserWindow,
			PruneInterval: limits.PruneInterval,
			MaxKeys:       limits.MaxUsers,
		},
	}
}

func (l *LLM) AskFor(ctx context.Context, userID UserID, question string) (answer, providerName string, err error) {
	if !l.perUser.Allow(userID) {
		logging.Debugf("llm: user %d over quota, rejecting", userID)
		return "", "", ErrBusy
	}
	release, ok := l.concurrency.TryAcquire()
	if !ok {
		logging.Warnf("llm: at max concurrency, rejecting request for user %d", userID)
		return "", "", ErrBusy
	}
	defer release()

	return l.Ask(ctx, question)
}

func (l *LLM) Ask(ctx context.Context, question string) (answer, providerName string, err error) {
	for i, provider := range l.providers {
		answer, err = provider.Ask(ctx, l.systemPrompt, question)
		if err == nil {
			logging.Debugf("llm: %s answered", provider.Name())
			return answer, provider.Name(), nil
		}
		if i == 0 {
			logging.Warnf("llm: primary provider: %v", err)
		} else {
			logging.Warnf("llm: fallback provider %s: %v", provider.Name(), err)
		}
	}
	return "", "", err
}
