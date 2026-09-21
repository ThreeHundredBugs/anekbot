package anekbot

import "context"

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

type LLM struct {
	providers []LLMProvider
}

func NewLLM(providers ...LLMProvider) *LLM {
	return &LLM{providers: providers}
}

func (l *LLM) Ask(ctx context.Context, question string) (answer, providerName string, err error) {
	for i, provider := range l.providers {
		answer, err = provider.Ask(ctx, question)
		if err == nil {
			return answer, provider.Name(), nil
		}
		if i == 0 {
			logWarnf("llm: primary provider: %v", err)
		} else {
			logWarnf("llm: fallback provider %s: %v", provider.Name(), err)
		}
	}
	return "", "", err
}

func truncateToRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
