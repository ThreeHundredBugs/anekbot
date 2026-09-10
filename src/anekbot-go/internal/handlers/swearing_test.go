package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-telegram/bot/models"
)

func newTestSwearingHandler(t *testing.T, extraWordListPath string) *SwearingHandler {
	t.Helper()
	h, err := NewSwearingHandler(extraWordListPath)
	if err != nil {
		t.Fatalf("NewSwearingHandler: %v", err)
	}
	return h
}

func TestHandleSwearing_Negative(t *testing.T) {
	cases := []string{
		"застрахуй меня", // "хуй" appears only as a substring of "застрахуй", not a whole word
		"привет",
		"Клавиши низко посажены",
		"Анек!дот", // the anek trigger text itself must not be flagged as profanity
		"totally unrelated text with numbers 123",
	}

	h := newTestSwearingHandler(t, "")

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			sender := &fakeSender{}
			update := &models.Update{Message: &models.Message{
				ID:   1,
				Chat: models.Chat{ID: 42},
				Text: text,
			}}

			h.Handle(context.Background(), sender, update)

			if len(sender.reactions) != 0 {
				t.Errorf("expected no reaction for %q, got %d", text, len(sender.reactions))
			}
		})
	}
}

func TestHandleSwearing_Positive(t *testing.T) {
	cases := []string{
		"что эта пизда себе позволяет?",
		"ёбнул бы ему",
		"нам всем пиздец!",
		"тебя давно пиздили?",
		"здарова, заебал",
	}

	h := newTestSwearingHandler(t, "")

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			sender := &fakeSender{}
			update := &models.Update{Message: &models.Message{
				ID:   7,
				Chat: models.Chat{ID: 99},
				Text: text,
			}}

			h.Handle(context.Background(), sender, update)

			if len(sender.reactions) != 1 {
				t.Fatalf("expected exactly 1 reaction for %q, got %d", text, len(sender.reactions))
			}
			reaction := sender.reactions[0]
			if reaction.ChatID != int64(99) {
				t.Errorf("chat id = %v, want 99", reaction.ChatID)
			}
			if reaction.MessageID != 7 {
				t.Errorf("message id = %v, want 7", reaction.MessageID)
			}
			if len(reaction.Reaction) != 1 || reaction.Reaction[0].ReactionTypeEmoji == nil ||
				reaction.Reaction[0].ReactionTypeEmoji.Emoji != "🤬" {
				t.Errorf("reaction = %+v, want single 🤬 emoji reaction", reaction.Reaction)
			}
		})
	}
}

func TestHandleSwearing_UnicodeTokenizer(t *testing.T) {
	h := newTestSwearingHandler(t, "")
	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   3,
		Chat: models.Chat{ID: 5},
		Text: "Слушай,ты-пиздец,чувак!",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.reactions) != 1 {
		t.Fatalf("expected the Cyrillic swear word to be tokenized and matched, got %d reactions", len(sender.reactions))
	}
}

func TestHandleSwearing_NoMessage(t *testing.T) {
	h := newTestSwearingHandler(t, "")
	sender := &fakeSender{}
	h.Handle(context.Background(), sender, &models.Update{})

	if len(sender.reactions) != 0 {
		t.Errorf("expected no reaction when Message is nil, got %d", len(sender.reactions))
	}
}

func TestHandleSwearing_ExtraWordList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extra.txt")
	if err := os.WriteFile(path, []byte("флюродендрон\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestSwearingHandler(t, path)
	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   1,
		Chat: models.Chat{ID: 1},
		Text: "ты флюродендрон!",
	}}

	h.Handle(context.Background(), sender, update)

	if len(sender.reactions) != 1 {
		t.Fatalf("expected a reaction for a word only present in the extra list, got %d", len(sender.reactions))
	}
}
