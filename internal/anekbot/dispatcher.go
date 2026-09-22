package anekbot

import (
	"context"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ThreeHundredBugs/anekbot/internal/llm"
)

type Sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SetMessageReaction(ctx context.Context, params *bot.SetMessageReactionParams) (bool, error)
	AnswerInlineQuery(ctx context.Context, params *bot.AnswerInlineQueryParams) (bool, error)
	EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
}

type Handler interface {
	Name() string
	Handle(ctx context.Context, sender Sender, update *models.Update)
}

type Dispatcher struct {
	// nil disables the handler
	anek      *AnekHandler
	swearing  *SwearingHandler
	questions *QuestionsHandler
	help      *HelpHandler
}

func NewDispatcher(anek *AnekHandler, swearing *SwearingHandler, questions *QuestionsHandler, help *HelpHandler) *Dispatcher {
	return &Dispatcher{anek: anek, swearing: swearing, questions: questions, help: help}
}

func (d *Dispatcher) SetQuestions(questions *QuestionsHandler) {
	d.questions = questions
}

func (d *Dispatcher) SetHelp(help *HelpHandler) {
	d.help = help
}

func (d *Dispatcher) Dispatch(ctx context.Context, sender Sender, update *models.Update) {
	logTracef("dispatcher: received update: %s", asJSON(update))

	switch {
	case update.Message != nil:
		msg := update.Message
		logDebugf("dispatcher: message from chat_id=%d user=%s", msg.Chat.ID, userLabel(msg.From))

		var handlers []Handler
		if d.anek != nil {
			handlers = append(handlers, d.anek)
		}
		if d.swearing != nil {
			handlers = append(handlers, d.swearing)
		}
		if d.questions != nil {
			handlers = append(handlers, d.questions)
		}
		if d.help != nil {
			handlers = append(handlers, d.help)
		}

		var wg sync.WaitGroup
		wg.Add(len(handlers))
		for _, h := range handlers {
			go func(h Handler) {
				defer wg.Done()
				logDebugf("dispatcher: firing %s handler", h.Name())
				h.Handle(ctx, sender, update)
			}(h)
		}
		wg.Wait()
	case update.InlineQuery != nil:
		logDebugf("dispatcher: inline query from user=%s", userLabel(update.InlineQuery.From))
		if d.anek != nil {
			logDebugf("dispatcher: firing %s inline handler", d.anek.Name())
			d.anek.HandleInline(ctx, sender, update)
		}
	case update.ChosenInlineResult != nil:
		logDebugf("dispatcher: chosen inline result from user=%s", userLabel(&update.ChosenInlineResult.From))
		if d.anek != nil {
			logDebugf("dispatcher: firing %s chosen-inline-result handler", d.anek.Name())
			d.anek.HandleChosenInlineResult(ctx, sender, update)
		}
	case update.CallbackQuery != nil:
		logDebugf("dispatcher: callback query from user=%s", userLabel(&update.CallbackQuery.From))
		if d.anek != nil {
			logDebugf("dispatcher: firing %s callback handler", d.anek.Name())
			d.anek.HandleCallback(ctx, sender, update)
		}
	}
}

func userLabel(u *models.User) string {
	if u == nil {
		return "unknown"
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return "unknown"
}

func userID(u *models.User) llm.UserID {
	if u == nil {
		return 0
	}
	return llm.UserID(u.ID)
}
