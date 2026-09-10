package handlers

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestHandleSwearing_Negative(t *testing.T) {
	cases := []string{
		"застрахуй меня", // "хуй" appears only as a substring of "застрахуй", not a whole word
		"привет",
		"Клавиши низко посажены",
		"Анек!дот", // the anek trigger text itself must not be flagged as profanity
		"totally unrelated text with numbers 123",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			sender := &fakeSender{}
			update := &models.Update{Message: &models.Message{
				ID:   1,
				Chat: models.Chat{ID: 42},
				Text: text,
			}}

			HandleSwearing(context.Background(), sender, update)

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

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			sender := &fakeSender{}
			update := &models.Update{Message: &models.Message{
				ID:   7,
				Chat: models.Chat{ID: 99},
				Text: text,
			}}

			HandleSwearing(context.Background(), sender, update)

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

// TestHandleSwearing_UnicodeTokenizer regression-tests that Cyrillic words are
// tokenized at all: Go's regexp \w/\b are ASCII-only unlike Python's re, so a
// naive port would silently never match any Cyrillic swear word.
func TestHandleSwearing_UnicodeTokenizer(t *testing.T) {
	sender := &fakeSender{}
	update := &models.Update{Message: &models.Message{
		ID:   3,
		Chat: models.Chat{ID: 5},
		Text: "Слушай,ты-пиздец,чувак!",
	}}

	HandleSwearing(context.Background(), sender, update)

	if len(sender.reactions) != 1 {
		t.Fatalf("expected the Cyrillic swear word to be tokenized and matched, got %d reactions", len(sender.reactions))
	}
}

func TestHandleSwearing_NoMessage(t *testing.T) {
	sender := &fakeSender{}
	HandleSwearing(context.Background(), sender, &models.Update{})

	if len(sender.reactions) != 0 {
		t.Errorf("expected no reaction when Message is nil, got %d", len(sender.reactions))
	}
}
