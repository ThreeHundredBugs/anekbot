package anekbot

import (
	"context"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/ThreeHundredBugs/anekbot/internal/stats"
)

func TestStatsHandler_RepliesToAdminInPrivateChat(t *testing.T) {
	s := stats.New()
	s.RecordAnek(1, "alice", "message", "classic")
	h := NewStatsHandler(s, []string{"@Admin"})
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate},
		From: &models.User{ID: 42, Username: "admin"},
		Text: "/stats",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sentMessages))
	}
	if !strings.Contains(sender.sentMessages[0].Text, "Всего анеков: 1") {
		t.Errorf("text = %q, want it to include the total", sender.sentMessages[0].Text)
	}
}

func TestStatsHandler_IgnoresNonAdmin(t *testing.T) {
	h := NewStatsHandler(stats.New(), []string{"admin"})
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate},
		From: &models.User{ID: 7, Username: "rando"},
		Text: "/stats",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected no reply for a non-admin, got %d messages", len(sender.sentMessages))
	}
}

func TestStatsHandler_IgnoresGroupChat(t *testing.T) {
	h := NewStatsHandler(stats.New(), []string{"admin"})
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: -100, Type: models.ChatTypeGroup},
		From: &models.User{ID: 7, Username: "admin"},
		Text: "/stats",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected /stats to be ignored outside a private chat, got %d messages", len(sender.sentMessages))
	}
}

func TestStatsHandler_IgnoresSpoofedPrivateChat(t *testing.T) {
	h := NewStatsHandler(stats.New(), []string{"admin"})
	sender := &fakeSender{}

	// chat.type claims private but chat.id != from.id: not an actual DM with the sender.
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 999, Type: models.ChatTypePrivate},
		From: &models.User{ID: 7, Username: "admin"},
		Text: "/stats",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected a mismatched chat/from id to be ignored, got %d messages", len(sender.sentMessages))
	}
}

func TestStatsHandler_UsernameMatchIsCaseInsensitiveAndStripsAt(t *testing.T) {
	h := NewStatsHandler(stats.New(), []string{"@Admin"})
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate},
		From: &models.User{ID: 7, Username: "ADMIN"},
		Text: "/stats",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 1 {
		t.Errorf("expected case-insensitive username match, got %d messages", len(sender.sentMessages))
	}
}

func TestStatsHandler_IgnoresOtherText(t *testing.T) {
	h := NewStatsHandler(stats.New(), []string{"admin"})
	sender := &fakeSender{}

	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate},
		From: &models.User{ID: 7, Username: "admin"},
		Text: "/stats please",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("expected exact /stats match only, got %d messages", len(sender.sentMessages))
	}
}

func TestFormatStats_NoDataYet(t *testing.T) {
	text := formatStats(stats.Snapshot{})
	if !strings.Contains(text, "пока нет данных") {
		t.Errorf("text = %q, want a no-data message for an empty snapshot", text)
	}
}
