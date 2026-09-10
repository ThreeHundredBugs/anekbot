package handlers

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
)

//go:embed swearwords.txt
var embeddedSwearWords string

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
