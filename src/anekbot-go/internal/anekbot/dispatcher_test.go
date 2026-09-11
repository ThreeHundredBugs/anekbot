package anekbot

import (
	"context"
	"net/http"
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
	return NewDispatcher(anek, swearing, nil)
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

func TestDispatch_Message_RunsGeminiHandlerWhenConfigured(t *testing.T) {
	anek, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	swearing, err := NewSwearingHandler("")
	if err != nil {
		t.Fatalf("NewSwearingHandler: %v", err)
	}
	gemini, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"42"}]}}]}`)
	d := NewDispatcher(anek, swearing, gemini)

	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot what's up",
	}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Errorf("expected the gemini handler to send 1 message, got %d", len(sender.sentMessages))
	}
}

func TestDispatch_Message_RunsGeminiAndSwearingHandlers(t *testing.T) {
	anek, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	swearing, err := NewSwearingHandler("")
	if err != nil {
		t.Fatalf("NewSwearingHandler: %v", err)
	}
	gemini, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"42"}]}}]}`)
	d := NewDispatcher(anek, swearing, gemini)

	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot хуйло",
	}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.sentMessages) != 2 {
		t.Errorf("expected gemini and swearing handler to send each 1 message, got %d", len(sender.sentMessages))
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
