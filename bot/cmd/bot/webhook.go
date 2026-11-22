package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/fastcheck/anonymus_bot/bot/internal/config"
	"github.com/fastcheck/anonymus_bot/bot/internal/dispatcher"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	webhookHeader    = "X-Telegram-Bot-Api-Secret-Token"
	webhookBodyLimit = 256 << 10
	recentUpdates    = 4096
)

type telegramRequester interface {
	MakeRequest(method string, params tgbotapi.Params) (*tgbotapi.APIResponse, error)
}

func usePolling(bot telegramRequester) error {
	res, err := bot.MakeRequest("getWebhookInfo", nil)
	var current tgbotapi.WebhookInfo
	if err != nil || res == nil || !res.Ok || json.Unmarshal(res.Result, &current) != nil {
		return errors.New("cannot check Telegram webhook before starting polling")
	}
	if current.URL == "" {
		return nil
	}
	return telegramWebhookSetting(bot, "deleteWebhook", tgbotapi.Params{"drop_pending_updates": "false"})
}

func registerWebhook(bot telegramRequester, endpoint, secret string) error {
	return telegramWebhookSetting(bot, "setWebhook", tgbotapi.Params{
		"url": endpoint, "secret_token": secret, "max_connections": "1",
		"drop_pending_updates": "false", "allowed_updates": `["message","callback_query"]`,
	})
}

func telegramWebhookSetting(bot telegramRequester, method string, params tgbotapi.Params) error {
	response, err := bot.MakeRequest(method, params)
	var accepted bool
	if err != nil || response == nil || !response.Ok || json.Unmarshal(response.Result, &accepted) != nil || !accepted {
		return errors.New("Telegram did not confirm " + method)
	}
	return nil
}

func newWebhookHandler(secret string, process dispatcher.HandlerFunc, logger *slog.Logger) http.Handler {
	slot := make(chan struct{}, 1)
	seen := make(map[int]struct{}, recentUpdates)
	var ring [recentUpdates]int
	position := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != config.WebhookPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		values := r.Header.Values(webhookHeader)
		if secret == "" || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte(secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		select {
		case slot <- struct{}{}:
			defer func() { <-slot }()
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyLimit))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		var update tgbotapi.Update
		if err != nil || json.Unmarshal(body, &update) != nil || update.UpdateID <= 0 {
			http.Error(w, "invalid update", http.StatusBadRequest)
			return
		}
		if _, duplicate := seen[update.UpdateID]; duplicate {
			w.WriteHeader(http.StatusOK)
			return
		}
		defer func() {
			if recover() != nil {
				logger.Error("recovered from panic in webhook update handler")
				http.Error(w, "update processing failed", http.StatusInternalServerError)
			}
		}()
		// Finish an accepted update if Telegram disconnects or shutdown starts.
		process(context.WithoutCancel(r.Context()), update)
		delete(seen, ring[position])
		ring[position] = update.UpdateID
		seen[update.UpdateID] = struct{}{}
		position = (position + 1) % len(ring)
		w.WriteHeader(http.StatusOK)
	})
}

func runWebhook(ctx context.Context, listener net.Listener, handler http.Handler, register func() error) error {
	srv := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 100 * time.Second, IdleTimeout: 30 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	registrationErr := register()
	var serveErr error
	serving := true
	if registrationErr == nil {
		select {
		case <-ctx.Done():
		case serveErr = <-done:
			serving = false
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = srv.Close()
	}
	if serving {
		serveErr = <-done
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(registrationErr, serveErr, shutdownErr)
}
