package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		logger.Warn("TELEGRAM_BOT_TOKEN is not set, running in stub mode")
	}

	backendURL := os.Getenv("BACKEND_URL")
	if backendURL == "" {
		logger.Error("BACKEND_URL is not set")
		os.Exit(1)
	}

	logger.Info("bot service starting", "backend_url", backendURL)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		checkBackendHealth(backendURL, logger)
	}
}

func checkBackendHealth(backendURL string, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL+"/health", nil)
	if err != nil {
		logger.Error("failed to create health check request", "error", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("backend health check failed", "error", err)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("failed to close response body", "error", err)
		}
	}()
	if resp.StatusCode == http.StatusOK {
		logger.Info("backend is healthy")
	} else {
		logger.Warn("backend reported unhealthy", "status_code", resp.StatusCode)
	}
}
