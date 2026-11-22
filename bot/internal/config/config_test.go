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
		"BOT_UPDATE_MODE":         "",
		"PANEL_BASE_URL":          "http://panel",
		"INTERNAL_API_TOKEN_FILE": path, "BOT_INTERNAL_TOKEN_FILE": path,
		"PHOTOS_DIR": "/photos", "VOICE_DIR": "/voice", "OPERATOR_GROUP_CHAT_ID": "-1",
	} {
		t.Setenv(name, value)
	}
	t.Setenv("REDIS_URL", "")
	cfg, err := Load()
	if err != nil || cfg.PanelBaseURL != "http://panel" {
		t.Fatal("valid configuration rejected")
	}
	if cfg.UpdateMode != UpdatePolling {
		t.Fatal("polling must remain the default update mode")
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
	for _, name := range []string{"TELEGRAM_BOT_TOKEN", "BACKEND_URL", "PANEL_BASE_URL", "OPERATOR_GROUP_CHAT_ID"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "synthetic-secret")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("expected a diagnostic without the original value")
			}
		})
	}
}

func TestWebhookConfiguration(t *testing.T) {
	const endpoint = "https://fixture.example.invalid:8443/telegram/webhook"
	secret := strings.Repeat("b", 64)
	path := filepath.Join(t.TempDir(), "webhook-token")
	if err := os.WriteFile(path, []byte(secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOT_UPDATE_MODE", UpdateWebhook)
	t.Setenv("TELEGRAM_WEBHOOK_URL", endpoint)
	t.Setenv("TELEGRAM_WEBHOOK_SECRET_FILE", path)
	cfg := Config{InternalAPIToken: strings.Repeat("a", 64)}
	if err := cfg.loadUpdateMode(); err != nil || cfg.WebhookURL != endpoint || cfg.WebhookSecret != secret {
		t.Fatalf("valid webhook configuration rejected: %v", err)
	}
	for _, url := range []string{
		"", "http://fixture.example.invalid/telegram/webhook", "https://fixture.example.invalid/other",
		endpoint + "?secret=fixture", endpoint + "#fragment",
		"https://user:secret@fixture.example.invalid/telegram/webhook",
		"https://fixture.example.invalid:8080/telegram/webhook",
	} {
		t.Setenv("TELEGRAM_WEBHOOK_URL", url)
		if err := cfg.loadUpdateMode(); err == nil || !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_URL") || strings.Contains(err.Error(), "fixture") {
			t.Fatal("invalid webhook URL accepted or exposed")
		}
	}
	t.Setenv("TELEGRAM_WEBHOOK_URL", endpoint)
	if err := (&Config{BotInternalToken: secret}).loadUpdateMode(); err == nil || !strings.Contains(err.Error(), "dedicated webhook secret") {
		t.Fatal("reused internal credential accepted")
	}
	t.Setenv("TELEGRAM_WEBHOOK_SECRET_FILE", "")
	if err := cfg.loadUpdateMode(); err == nil || !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_SECRET_FILE") {
		t.Fatal("missing webhook secret accepted")
	}
	t.Setenv("BOT_UPDATE_MODE", "invalid")
	if err := cfg.loadUpdateMode(); err == nil || !strings.Contains(err.Error(), "BOT_UPDATE_MODE") {
		t.Fatal("unsupported update mode accepted")
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

func TestLoadStartBranding(t *testing.T) {
	t.Setenv("BOT_ORGANIZATION_NAME", "  North & Co  ")
	t.Setenv("BOT_JOINED_TEXT_RU", "Привет\nВторая строка")
	t.Setenv("BOT_JOINED_TEXT_EN", "")
	t.Setenv("BOT_REQUEST_RECEIVED_TEXT_RU", " ")
	t.Setenv("BOT_REQUEST_RECEIVED_TEXT_EN", "Fixture request")
	got, err := loadStartBranding()
	if err != nil || got.OrganizationName != "North & Co" || got.JoinedRU != "Привет\nВторая строка" ||
		got.JoinedEN != "" || got.RequestReceivedRU != "" || got.RequestReceivedEN != "Fixture request" {
		t.Fatalf("unexpected optional branding: %v", err)
	}
	t.Setenv("BOT_JOINED_TEXT_RU", strings.Repeat("fixture", 150))
	if _, err := loadStartBranding(); err == nil || !strings.Contains(err.Error(), "BOT_JOINED_TEXT_RU") || strings.Contains(err.Error(), "fixture") {
		t.Fatal("expected a sanitized diagnostic for oversized text")
	}
}
