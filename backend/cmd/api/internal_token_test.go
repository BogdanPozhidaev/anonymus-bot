package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadInternalAPIToken(t *testing.T) {
	token := strings.Repeat("aB", 32)
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"valid", token + "\n", true},
		{"empty", "", false},
		{"short", token[:63], false},
		{"non-hex", "g" + token[1:], false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("BOT_INTERNAL_TOKEN_FILE", path)
			got, err := readInternalAPIToken("BOT_INTERNAL_TOKEN_FILE")
			if tc.valid {
				if err != nil || got != token {
					t.Fatal("valid token was not loaded")
				}
			} else if err == nil || got != "" {
				t.Fatal("invalid token was accepted")
			}
		})
	}
	t.Setenv("BOT_INTERNAL_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := readInternalAPIToken("BOT_INTERNAL_TOKEN_FILE"); err == nil {
		t.Fatal("missing token file was accepted")
	}
}
