package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	TelegramBotToken    string
	BackendURL          string
	InternalAPIToken    string
	BotInternalToken    string
	RedisURL            string
	PhotosDir           string
	VoiceDir            string
	OperatorGroupChatID int64
	PanelBaseURL        string
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
		RedisURL:         os.Getenv("REDIS_URL"),
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
	redisURL, err := url.Parse(cfg.RedisURL)
	if err != nil || redisURL.Hostname() == "" || (redisURL.Scheme != "redis" && redisURL.Scheme != "rediss") {
		return nil, fmt.Errorf("REDIS_URL must be a valid Redis connection URL")
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

	return cfg, nil
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "http" || u.Scheme == "https")
}
