package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Config struct {
	TelegramBotToken    string
	BackendURL          string
	InternalAPIToken    string
	BotInternalToken    string
	PhotosDir           string
	VoiceDir            string
	OperatorGroupChatID int64
	PanelBaseURL        string
	StartBranding       StartBranding
	UpdateMode          string
	WebhookURL          string
	WebhookSecret       string
}

const (
	UpdatePolling = "polling"
	UpdateWebhook = "webhook"
	WebhookPath   = "/telegram/webhook"
)

type StartBranding struct {
	OrganizationName                     string
	JoinedRU, JoinedEN                   string
	RequestReceivedRU, RequestReceivedEN string
}

var telegramTokenFormat = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)

// readInternalAPIToken loads a hex-encoded 32-byte service token.
func readInternalAPIToken(envName string) (string, error) {
	data, err := os.ReadFile(os.Getenv(envName))
	if err != nil {
		return "", fmt.Errorf("cannot read %s", envName)
	}
	token := strings.TrimSpace(string(data))
	if len(token) != 64 {
		return "", fmt.Errorf("%s must contain a 64-character hex token", envName)
	}
	if _, err := hex.DecodeString(token); err != nil {
		return "", fmt.Errorf("%s must contain a 64-character hex token", envName)
	}
	return token, nil
}

func Load() (*Config, error) {
	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		BackendURL:       os.Getenv("BACKEND_URL"),
		PhotosDir:        os.Getenv("PHOTOS_DIR"),
		VoiceDir:         os.Getenv("VOICE_DIR"),
	}

	if !telegramTokenFormat.MatchString(cfg.TelegramBotToken) {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN must have the expected bot token format")
	}
	if !validHTTPURL(cfg.BackendURL) {
		return nil, fmt.Errorf("BACKEND_URL must be an HTTP(S) URL without credentials")
	}
	token, err := readInternalAPIToken("INTERNAL_API_TOKEN_FILE")
	if err != nil {
		return nil, err
	}
	cfg.InternalAPIToken = token
	cfg.BotInternalToken, err = readInternalAPIToken("BOT_INTERNAL_TOKEN_FILE")
	if err != nil {
		return nil, err
	}
	if cfg.PhotosDir == "" {
		return nil, fmt.Errorf("PHOTOS_DIR is not set")
	}
	if cfg.VoiceDir == "" {
		return nil, fmt.Errorf("VOICE_DIR is not set")
	}

	panelBaseStr := os.Getenv("PANEL_BASE_URL")
	groupChatIDStr := os.Getenv("OPERATOR_GROUP_CHAT_ID")
	if groupChatIDStr == "" {
		return nil, fmt.Errorf("OPERATOR_GROUP_CHAT_ID is not set")
	}
	groupChatID, err := strconv.ParseInt(groupChatIDStr, 10, 64)
	if err != nil || groupChatID == 0 {
		return nil, fmt.Errorf("OPERATOR_GROUP_CHAT_ID must be a non-zero integer")
	}
	panelURL, panelErr := url.Parse(panelBaseStr)
	if panelErr != nil || !validHTTPURL(panelBaseStr) || panelURL.RawQuery != "" || panelURL.Fragment != "" ||
		strings.Trim(panelURL.Path, "/") != "" {
		return nil, fmt.Errorf("PANEL_BASE_URL must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	cfg.OperatorGroupChatID = groupChatID
	cfg.PanelBaseURL = strings.TrimRight(panelBaseStr, "/")
	cfg.StartBranding, err = loadStartBranding()
	if err != nil {
		return nil, err
	}
	if err := cfg.loadUpdateMode(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) loadUpdateMode() error {
	c.UpdateMode = os.Getenv("BOT_UPDATE_MODE")
	switch c.UpdateMode {
	case "", UpdatePolling:
		c.UpdateMode = UpdatePolling
		return nil
	case UpdateWebhook:
	default:
		return fmt.Errorf("BOT_UPDATE_MODE must be polling or webhook")
	}
	c.WebhookURL = os.Getenv("TELEGRAM_WEBHOOK_URL")
	u, err := url.Parse(c.WebhookURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.EscapedPath() != WebhookPath || strings.ContainsAny(c.WebhookURL, "?#") || strings.HasSuffix(u.Host, ":") {
		return fmt.Errorf("TELEGRAM_WEBHOOK_URL must be an HTTPS URL ending in %s without credentials, query or fragment", WebhookPath)
	}
	switch u.Port() {
	case "", "443", "80", "88", "8443":
	default:
		return fmt.Errorf("TELEGRAM_WEBHOOK_URL must use port 443, 80, 88 or 8443")
	}
	c.WebhookSecret, err = readInternalAPIToken("TELEGRAM_WEBHOOK_SECRET_FILE")
	if err != nil {
		return err
	}
	if strings.EqualFold(c.WebhookSecret, c.InternalAPIToken) || strings.EqualFold(c.WebhookSecret, c.BotInternalToken) {
		return fmt.Errorf("TELEGRAM_WEBHOOK_SECRET_FILE must contain a dedicated webhook secret")
	}
	return nil
}

func loadStartBranding() (StartBranding, error) {
	var branding StartBranding
	for _, field := range []struct {
		name        string
		destination *string
		maxBytes    int
	}{
		{"BOT_ORGANIZATION_NAME", &branding.OrganizationName, 120},
		{"BOT_JOINED_TEXT_RU", &branding.JoinedRU, 1000},
		{"BOT_JOINED_TEXT_EN", &branding.JoinedEN, 1000},
		{"BOT_REQUEST_RECEIVED_TEXT_RU", &branding.RequestReceivedRU, 1000},
		{"BOT_REQUEST_RECEIVED_TEXT_EN", &branding.RequestReceivedEN, 1000},
	} {
		value := strings.TrimSpace(os.Getenv(field.name))
		if !utf8.ValidString(value) || len(value) > field.maxBytes {
			return StartBranding{}, fmt.Errorf("%s must be valid UTF-8 with at most %d bytes", field.name, field.maxBytes)
		}
		*field.destination = value
	}
	return branding, nil
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "http" || u.Scheme == "https")
}
