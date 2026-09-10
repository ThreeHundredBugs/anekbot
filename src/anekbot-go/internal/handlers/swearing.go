package handlers

import (
	"context"
	"log"
	"regexp"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// wordPattern must be Unicode-aware: Go's regexp \w/\b are ASCII-only, unlike
// Python's re module, so a naive \b\w+\b port would silently fail to
// tokenize Cyrillic text.
var wordPattern = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// HandleSwearing reacts with 🤬 to messages containing profanity, mirroring
// the exact-token matching behavior of the Python swearing_handler.
func HandleSwearing(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	words := wordPattern.FindAllString(strings.ToLower(msg.Text), -1)
	for _, word := range words {
		if _, isSwearing := swearWords[word]; !isSwearing {
			continue
		}

		if _, err := sender.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Reaction: []models.ReactionType{
				{
					Type:              models.ReactionTypeTypeEmoji,
					ReactionTypeEmoji: &models.ReactionTypeEmoji{Emoji: "🤬"},
				},
			},
		}); err != nil {
			log.Printf("swearing handler: set message reaction: %v", err)
		}
		return
	}
}
