package config

import (
	"fmt"
	"os"
)

type Config struct {
	TelegramBotToken string
	BackendURL       string
	RedisURL         string
}

func Load() (*Config, error) {
	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		BackendURL:       os.Getenv("BACKEND_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
	}

	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}
	if cfg.BackendURL == "" {
		return nil, fmt.Errorf("BACKEND_URL is not set")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is not set")
	}

	return cfg, nil
}
