package handlers

import (
	_ "embed"
	"strings"
)

//go:embed swearwords.txt
var swearWordsRaw string

var swearWords = loadSwearWords(swearWordsRaw)

func loadSwearWords(raw string) map[string]struct{} {
	lines := strings.Split(raw, "\n")
	set := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		word := strings.ToLower(strings.TrimSpace(line))
		if word == "" {
			continue
		}
		set[word] = struct{}{}
	}
	return set
}
