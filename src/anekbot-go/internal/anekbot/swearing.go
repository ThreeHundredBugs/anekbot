package anekbot

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

//go:embed swearwords.txt
var embeddedSwearWords string

// Regex must work with cyrillic letters
var wordPattern = regexp.MustCompile(`[\p{L}\p{N}_]+`)

type SwearingHandler struct {
	words map[string]struct{}
}

func NewSwearingHandler(extraWordListPath string) (*SwearingHandler, error) {
	words, err := loadSwearWords(extraWordListPath)
	if err != nil {
		return nil, err
	}
	return &SwearingHandler{words: words}, nil
}

func (h *SwearingHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	words := wordPattern.FindAllString(strings.ToLower(msg.Text), -1)
	for _, word := range words {
		if _, isSwearing := h.words[word]; !isSwearing {
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

func loadSwearWords(extraPath string) (map[string]struct{}, error) {
	words := make(map[string]struct{})
	addWords(words, embeddedSwearWords)

	if extraPath == "" {
		return words, nil
	}

	data, err := os.ReadFile(extraPath)
	if err != nil {
		return nil, fmt.Errorf("read extra swear word list %q: %w", extraPath, err)
	}
	addWords(words, string(data))

	return words, nil
}

func addWords(set map[string]struct{}, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		word := strings.ToLower(strings.TrimSpace(line))
		if word == "" {
			continue
		}
		set[word] = struct{}{}
	}
}
