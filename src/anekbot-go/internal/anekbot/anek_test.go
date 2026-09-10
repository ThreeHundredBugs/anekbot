package anekbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot/models"
	"golang.org/x/text/encoding/charmap"
)

func newTestAnekHandler(t *testing.T, body string, randValue float64) (h *AnekHandler, lastQuery func() url.Values) {
	t.Helper()

	win1251Body, err := charmap.Windows1251.NewEncoder().String(body)
	if err != nil {
		t.Fatalf("encode fixture as windows-1251: %v", err)
	}

	var mu sync.Mutex
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = r.URL.Query()
		mu.Unlock()
		w.Write([]byte(win1251Body))
	}))
	t.Cleanup(server.Close)

	h = &AnekHandler{
		client:    server.Client(),
		baseURL:   server.URL,
		randFloat: func() float64 { return randValue },
	}
	lastQuery = func() url.Values {
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
	return h, lastQuery
}

func TestAnekHandler_Trigger(t *testing.T) {
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
	if got := query().Get("CType"); got != "1" {
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

	if got := query().Get("CType"); got != "11" {
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

func TestAnekHandler_HandleInline(t *testing.T) {
	wantJoke := "Штирлиц зашел в комнату\nи сказал что-то очень длинное и запутанное про партизан и явки"
	fixture := `{"content":"` + wantJoke + `"}`

	h, _ := newTestAnekHandler(t, fixture, 0.1)
	sender := &fakeSender{}

	update := &models.Update{InlineQuery: &models.InlineQuery{ID: "query-1", Query: ""}}

	h.HandleInline(context.Background(), sender, update)

	if len(sender.inlineAnswers) != 1 {
		t.Fatalf("expected 1 AnswerInlineQuery call, got %d", len(sender.inlineAnswers))
	}
	answer := sender.inlineAnswers[0]
	if answer.InlineQueryID != "query-1" {
		t.Errorf("inline query id = %q, want %q", answer.InlineQueryID, "query-1")
	}
	if len(answer.Results) != inlineSuggestionCount {
		t.Fatalf("expected %d results, got %d", inlineSuggestionCount, len(answer.Results))
	}

	seenIDs := make(map[string]bool)
	for _, result := range answer.Results {
		article, ok := result.(*models.InlineQueryResultArticle)
		if !ok {
			t.Fatalf("result type = %T, want *models.InlineQueryResultArticle", result)
		}
		if seenIDs[article.ID] {
			t.Errorf("duplicate result id %q", article.ID)
		}
		seenIDs[article.ID] = true

		content, ok := article.InputMessageContent.(models.InputTextMessageContent)
		if !ok {
			t.Fatalf("input message content type = %T, want models.InputTextMessageContent", article.InputMessageContent)
		}
		if content.MessageText != wantJoke {
			t.Errorf("message text = %q, want %q", content.MessageText, wantJoke)
		}
		if strings.Contains(article.Title, "\n") {
			t.Errorf("title = %q, should not contain newlines", article.Title)
		}
	}
}

func TestAnekHandler_HandleInline_NoInlineQuery(t *testing.T) {
	h, _ := newTestAnekHandler(t, `{"content":"joke"}`, 0.1)
	sender := &fakeSender{}

	h.HandleInline(context.Background(), sender, &models.Update{})

	if len(sender.inlineAnswers) != 0 {
		t.Errorf("expected no AnswerInlineQuery call when InlineQuery is nil, got %d", len(sender.inlineAnswers))
	}
}

func TestInlineTitle_TruncatesLongJokes(t *testing.T) {
	joke := strings.Repeat("а", inlineTitleMaxRunes+20)

	title := inlineTitle(joke)

	if got := []rune(title); len(got) != inlineTitleMaxRunes+1 { // +1 for the trailing "…"
		t.Errorf("title length = %d runes, want %d", len(got), inlineTitleMaxRunes+1)
	}
	if !strings.HasSuffix(title, "…") {
		t.Errorf("title = %q, want it truncated with a trailing ellipsis", title)
	}
}
