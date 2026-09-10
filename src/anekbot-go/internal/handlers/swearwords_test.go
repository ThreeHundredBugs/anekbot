package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSwearWords_EmbeddedOnly(t *testing.T) {
	words, err := loadSwearWords("")
	if err != nil {
		t.Fatalf("loadSwearWords: %v", err)
	}
	if _, ok := words["пиздец"]; !ok {
		t.Error("expected the embedded word list to be loaded")
	}
}

func TestLoadSwearWords_MergesAndDedupes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extra.txt")
	if err := os.WriteFile(path, []byte("новоеслово\n  ХУЙ  \n\nдругоеслово\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := loadSwearWords("")
	if err != nil {
		t.Fatalf("loadSwearWords: %v", err)
	}

	merged, err := loadSwearWords(path)
	if err != nil {
		t.Fatalf("loadSwearWords: %v", err)
	}

	for _, word := range []string{"новоеслово", "хуй", "другоеслово"} {
		if _, ok := merged[word]; !ok {
			t.Errorf("expected %q in merged set", word)
		}
	}

	// Exactly 2 new words were added ("хуй" was already present), no duplicates.
	wantSize := len(before) + 2
	if len(merged) != wantSize {
		t.Errorf("merged set size = %d, want %d (embedded=%d + 2 new words, no duplicates)", len(merged), wantSize, len(before))
	}
}

func TestLoadSwearWords_MissingFile(t *testing.T) {
	_, err := loadSwearWords(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Error("expected an error for a missing extra word list file")
	}
}
