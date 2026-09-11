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

func TestDispatch_Message_RunsLLMHandlerWhenConfigured(t *testing.T) {
	anek, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	swearing, err := NewSwearingHandler("")
	if err != nil {
		t.Fatalf("NewSwearingHandler: %v", err)
	}
	llm := NewLLMHandler("anekbot", &fakeLLMProvider{answer: "42"}, nil)
	d := NewDispatcher(anek, swearing, llm)

	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot what's up",
	}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Errorf("expected the llm handler to send 1 message, got %d", len(sender.sentMessages))
	}
}

func TestDispatch_Message_RunsLLMAndSwearingHandlers(t *testing.T) {
	anek, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	swearing, err := NewSwearingHandler("")
	if err != nil {
		t.Fatalf("NewSwearingHandler: %v", err)
	}
	llm := NewLLMHandler("anekbot", &fakeLLMProvider{answer: "42"}, nil)
	d := NewDispatcher(anek, swearing, llm)

	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot хуйло",
	}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Errorf("expected the llm handler to send 1 message, got %d", len(sender.sentMessages))
	}
	if len(sender.reactions) != 1 {
		t.Errorf("expected the swearing handler to react once, got %d reactions", len(sender.reactions))
	}
}

func TestDispatch_Message_SkipsDisabledAnekAndSwearing(t *testing.T) {
	d := NewDispatcher(nil, nil, nil)
	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "анек! нам всем пиздец",
	}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected a disabled anek handler to send no messages, got %d", len(sender.sentMessages))
	}
	if len(sender.reactions) != 0 {
		t.Errorf("expected a disabled swearing handler to set no reactions, got %d", len(sender.reactions))
	}
}

func TestDispatch_InlineQuery_SkipsDisabledAnek(t *testing.T) {
	d := NewDispatcher(nil, nil, nil)
	sender := &fakeSender{}
	update := &models.Update{InlineQuery: &models.InlineQuery{ID: "q1"}}

	d.Dispatch(context.Background(), sender, update)

	if len(sender.inlineAnswers) != 0 {
		t.Errorf("expected a disabled anek handler to answer no inline queries, got %d", len(sender.inlineAnswers))
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
