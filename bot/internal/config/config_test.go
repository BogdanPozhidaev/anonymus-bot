package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidatesSettingsWithoutEchoingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service-token")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"TELEGRAM_BOT_TOKEN": "1:synthetic", "BACKEND_URL": "http://backend",
		"PANEL_BASE_URL": "http://panel", "REDIS_URL": "redis://redis",
		"INTERNAL_API_TOKEN_FILE": path, "BOT_INTERNAL_TOKEN_FILE": path,
		"PHOTOS_DIR": "/photos", "VOICE_DIR": "/voice", "OPERATOR_GROUP_CHAT_ID": "-1",
	} {
		t.Setenv(name, value)
	}
	cfg, err := Load()
	if err != nil || cfg.PanelBaseURL != "http://panel" {
		t.Fatal("valid configuration rejected")
	}
	t.Setenv("PANEL_BASE_URL", "https://another.example.invalid/")
	cfg, err = Load()
	if err != nil || cfg.PanelBaseURL != "https://another.example.invalid" {
		t.Fatal("runtime panel origin was not applied")
	}
	t.Run("panel mount path", func(t *testing.T) {
		t.Setenv("PANEL_BASE_URL", "https://panel.example.invalid/subpath")
		if _, err := Load(); err == nil {
			t.Fatal("accepted a panel origin with a path")
		}
	})
	for _, name := range []string{"TELEGRAM_BOT_TOKEN", "BACKEND_URL", "REDIS_URL", "PANEL_BASE_URL", "OPERATOR_GROUP_CHAT_ID"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "synthetic-secret")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("expected a diagnostic without the original value")
			}
		})
	}
}

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
