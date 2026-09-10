package anekbot

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
)

func newTestDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	anek, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	swearing, err := NewSwearingHandler("")
	if err != nil {
		t.Fatalf("NewSwearingHandler: %v", err)
	}
	return NewDispatcher(anek, swearing)
}

func TestDispatch_Message_DoesNotAnswerInlineQuery(t *testing.T) {
	d := newTestDispatcher(t)
	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "нам всем пиздец",
	}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.inlineAnswers) != 0 {
		t.Errorf("expected a message update to never call AnswerInlineQuery, got %d calls", len(sender.inlineAnswers))
	}
	if len(sender.reactions) != 1 {
		t.Errorf("expected the swearing handler to still run for a message update, got %d reactions", len(sender.reactions))
	}
}

func TestDispatch_InlineQuery_DoesNotRunMessageHandlers(t *testing.T) {
	d := newTestDispatcher(t)
	sender := &fakeSender{}
	update := &models.Update{InlineQuery: &models.InlineQuery{ID: "q1"}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected an inline query update to never call SendMessage, got %d calls", len(sender.sentMessages))
	}
	if len(sender.reactions) != 0 {
		t.Errorf("expected an inline query update to never call SetMessageReaction, got %d calls", len(sender.reactions))
	}
}
