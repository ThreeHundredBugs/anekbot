package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-telegram/bot/models"
)

func newTestAnekHandler(t *testing.T, body string, randValue float64) (*AnekHandler, *url.Values) {
	t.Helper()

	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query()
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	h := &AnekHandler{
		client:    server.Client(),
		baseURL:   server.URL,
		randFloat: func() float64 { return randValue },
	}
	return h, &captured
}

func TestAnekHandler_Trigger(t *testing.T) {
	// Real rzhunemogu.ru responses are NOT valid JSON: the content can contain
	// unescaped quotes and raw newlines, so the handler must use prefix/suffix
	// trimming rather than encoding/json.
	wantJoke := `Штирлиц вошел в комнату и сказал: "привет"` + "\n" + `всем`
	fixture := `{"content":"` + wantJoke + `"}`

	h, query := newTestAnekHandler(t, fixture, 0.5) // below 0.85 -> normal joke
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   10,
		Chat: models.Chat{ID: 123},
		Text: "хочу анек! срочно",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sentMessages))
	}
	sent := sender.sentMessages[0]
	if sent.Text != wantJoke {
		t.Errorf("joke text = %q, want %q", sent.Text, wantJoke)
	}
	if sent.ChatID != int64(123) {
		t.Errorf("chat id = %v, want 123", sent.ChatID)
	}
	if sent.ReplyParameters == nil || sent.ReplyParameters.MessageID != 10 {
		t.Errorf("reply parameters = %+v, want reply to message 10", sent.ReplyParameters)
	}
	if got := query.Get("CType"); got != "1" {
		t.Errorf("CType = %q, want 1 for randFloat below 0.85", got)
	}
}

func TestAnekHandler_18PlusBranch(t *testing.T) {
	h, query := newTestAnekHandler(t, `{"content":"joke"}`, 0.9) // above 0.85 -> 18+ joke
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "анек!",
	}}

	h.Handle(context.Background(), sender, update)

	if got := query.Get("CType"); got != "11" {
		t.Errorf("CType = %q, want 11 for randFloat above 0.85", got)
	}
}

func TestAnekHandler_CaseInsensitiveSubstring(t *testing.T) {
	h, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "ну давай АНЕК! мне",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected trigger to match case-insensitively, got %d messages", len(sender.sentMessages))
	}
}

func TestAnekHandler_NoTrigger(t *testing.T) {
	h, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "просто привет",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no message sent without the trigger, got %d", len(sender.sentMessages))
	}
}

func TestAnekHandler_NoMessage(t *testing.T) {
	h, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	sender := &fakeSender{}

	h.Handle(context.Background(), sender, &models.Update{})

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no message sent when Message is nil, got %d", len(sender.sentMessages))
	}
}
