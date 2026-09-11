package anekbot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot/models"
)

func newTestGeminiHandler(t *testing.T, statusCode int, responseBody string) (h *GeminiHandler, lastRequest func() geminiRequest) {
	t.Helper()

	var mu sync.Mutex
	var captured geminiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		json.Unmarshal(body, &captured)
		mu.Unlock()
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)

	h = NewGeminiHandler("test-key", "", "anekbot")
	h.client = server.Client()
	h.baseURL = server.URL

	lastRequest = func() geminiRequest {
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
	return h, lastRequest
}

func TestGeminiHandler_Trigger(t *testing.T) {
	h, lastRequest := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"42"}]}}]}`)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   5,
		Chat: models.Chat{ID: 7},
		Text: "@anekbot what is the answer to everything?",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sentMessages))
	}
	sent := sender.sentMessages[0]
	if sent.Text != "42" {
		t.Errorf("text = %q, want %q", sent.Text, "42")
	}
	if sent.ChatID != int64(7) {
		t.Errorf("chat id = %v, want 7", sent.ChatID)
	}
	if sent.ReplyParameters == nil || sent.ReplyParameters.MessageID != 5 {
		t.Errorf("reply parameters = %+v, want reply to message 5", sent.ReplyParameters)
	}

	req := lastRequest()
	if len(req.Contents) != 1 || len(req.Contents[0].Parts) != 1 {
		t.Fatalf("request contents = %+v, want a single user part", req.Contents)
	}
	if want := "what is the answer to everything?"; req.Contents[0].Parts[0].Text != want {
		t.Errorf("question = %q, want %q", req.Contents[0].Parts[0].Text, want)
	}
}

func TestGeminiHandler_CaseInsensitiveMention(t *testing.T) {
	h, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"fact"}]}}]}`)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@AnekBot tell me a fact",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Errorf("expected mention to match case-insensitively, got %d messages", len(sender.sentMessages))
	}
}

func TestGeminiHandler_NoMention(t *testing.T) {
	h, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"fact"}]}}]}`)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "просто привет",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no message sent without a mention, got %d", len(sender.sentMessages))
	}
}

func TestGeminiHandler_MentionWithoutQuestion(t *testing.T) {
	h, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"fact"}]}}]}`)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no message sent for a bare mention with no question, got %d", len(sender.sentMessages))
	}
}

func TestGeminiHandler_NoMessage(t *testing.T) {
	h, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"fact"}]}}]}`)
	sender := &fakeSender{}

	h.Handle(context.Background(), sender, &models.Update{})

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no message sent when Message is nil, got %d", len(sender.sentMessages))
	}
}

func TestGeminiHandler_APIError(t *testing.T) {
	h, _ := newTestGeminiHandler(t, http.StatusInternalServerError, `{"error":{"message":"boom"}}`)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot are you ok",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no message sent on API error, got %d", len(sender.sentMessages))
	}
}

func TestGeminiHandler_TruncatesLongAnswer(t *testing.T) {
	longAnswer := strings.Repeat("a", telegramMessageMaxRunes+100)
	h, _ := newTestGeminiHandler(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"`+longAnswer+`"}]}}]}`)
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "@anekbot tell me a long story",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sentMessages))
	}
	if got := len([]rune(sender.sentMessages[0].Text)); got != telegramMessageMaxRunes {
		t.Errorf("sent text length = %d runes, want %d", got, telegramMessageMaxRunes)
	}
}
