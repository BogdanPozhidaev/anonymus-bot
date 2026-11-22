package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestWebhookAuthorizationBodyAndReplay(t *testing.T) {
	const secret = "fixture-secret"
	var processed int
	handler := newWebhookHandler(secret, func(_ context.Context, update tgbotapi.Update) {
		processed++
		if update.UpdateID == 9 && processed == 2 {
			panic("fixture failure")
		}
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	check := func(method, path, body string, status int, tokens ...string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, token := range tokens {
			req.Header.Add(webhookHeader, token)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("received %d, expected %d", res.Code, status)
		}
	}
	check("POST", "/internal/send-alert", "", 404, secret)
	check("POST", config.WebhookPath+"/extra", "", 404, secret)
	check("GET", config.WebhookPath, "", 405, secret)
	check("POST", config.WebhookPath, "", 401)
	check("POST", config.WebhookPath, "", 401, "wrong")
	check("POST", config.WebhookPath, "", 401, secret, secret)
	check("POST", config.WebhookPath, `{"update_id":0}`, 400, secret)
	check("POST", config.WebhookPath, `{"update_id":1} {}`, 400, secret)
	check("POST", config.WebhookPath, strings.Repeat(" ", webhookBodyLimit+1), 413, secret)
	if processed != 0 {
		t.Fatal("invalid requests reached the update handler")
	}
	for i := 0; i < 2; i++ {
		check("POST", config.WebhookPath, `{"update_id":1,"message":{"message_id":2}}`, 200, secret)
	}
	if processed != 1 {
		t.Fatal("recent update was processed twice")
	}
	check("POST", config.WebhookPath, `{"update_id":9}`, 500, secret)
	check("POST", config.WebhookPath, `{"update_id":9}`, 200, secret)
	if processed != 3 {
		t.Fatal("failed processing was acknowledged as a duplicate")
	}
}

func TestTelegramWebhookModeSwitchKeepsPendingUpdates(t *testing.T) {
	const endpoint = "https://fixture.example.invalid/telegram/webhook"
	const secret = "fixture-secret"
	var registered atomic.Bool
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/botfixture/getMe":
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"id":1}}`)
		case "/botfixture/setWebhook":
			calls.Add(1)
			if r.FormValue("url") != endpoint || r.FormValue("secret_token") != secret || r.FormValue("max_connections") != "1" ||
				r.FormValue("drop_pending_updates") != "false" || r.FormValue("allowed_updates") != `["message","callback_query"]` {
				t.Error("incorrect registration settings")
			}
			registered.Store(true)
			_, _ = fmt.Fprint(w, `{"ok":true,"result":true}`)
		case "/botfixture/getWebhookInfo":
			calls.Add(1)
			url := ""
			if registered.Load() {
				url = endpoint
			}
			_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"url":%q}}`, url)
		case "/botfixture/deleteWebhook":
			calls.Add(1)
			if r.FormValue("drop_pending_updates") != "false" {
				t.Error("polling transition would drop queued updates")
			}
			registered.Store(false)
			_, _ = fmt.Fprint(w, `{"ok":true,"result":true}`)
		default:
			t.Error("unexpected Telegram method")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()
	bot, err := tgbotapi.NewBotAPIWithClient("fixture", fake.URL+"/bot%s/%s", fake.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := usePolling(bot); err != nil || calls.Load() != 1 {
		t.Fatalf("polling updated an unregistered webhook: %v", err)
	}
	if err := registerWebhook(bot, endpoint, secret); err != nil || !registered.Load() {
		t.Fatalf("registration failed: %v", err)
	}
	if err := usePolling(bot); err != nil || registered.Load() || calls.Load() != 4 {
		t.Fatalf("polling transition failed: %v", err)
	}
}

func TestWebhookRegistrationFailureClosesListenerWithoutLeakingError(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botfixture/getMe" {
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"id":1}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"fixture-private-detail"}`)
	}))
	defer fake.Close()
	bot, err := tgbotapi.NewBotAPIWithClient("fixture", fake.URL+"/bot%s/%s", fake.Client())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	err = runWebhook(t.Context(), listener, http.NotFoundHandler(), func() error {
		return registerWebhook(bot, "https://fixture.example.invalid/telegram/webhook", "fixture-secret")
	})
	if err == nil || err.Error() != "Telegram did not confirm setWebhook" {
		t.Fatal("registration error was ignored or exposed the upstream response")
	}
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		_ = conn.Close()
		t.Fatal("failed registration left the listener open")
	}
}

func TestWebhookDrainsAcceptedUpdateBeforeShutdown(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case release <- struct{}{}:
		default:
		}
	})
	handler := newWebhookHandler("fixture", func(ctx context.Context, _ tgbotapi.Update) {
		close(started)
		<-release
		if ctx.Err() != nil {
			t.Error("accepted update was cancelled during shutdown")
		}
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan error, 1)
	go func() { done <- runWebhook(ctx, listener, handler, func() error { return nil }) }()
	client := &http.Client{Timeout: 3 * time.Second}
	send := func() (int, error) {
		req, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+config.WebhookPath, strings.NewReader(`{"update_id":1}`))
		if err != nil {
			return 0, err
		}
		req.Header.Set(webhookHeader, "fixture")
		res, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		_ = res.Body.Close()
		return res.StatusCode, nil
	}
	ack := make(chan error, 1)
	go func() {
		status, err := send()
		if err == nil && status != http.StatusOK {
			err = fmt.Errorf("unexpected acknowledgement %d", status)
		}
		ack <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook request did not reach the handler")
	}
	if status, err := send(); err != nil || status != http.StatusServiceUnavailable {
		t.Fatalf("parallel update should be retried: %d, %v", status, err)
	}
	stop()
	select {
	case err := <-ack:
		t.Fatalf("premature acknowledgement: %v", err)
	case err := <-done:
		t.Fatalf("premature shutdown: %v", err)
	default:
	}
	release <- struct{}{}
	if err := <-ack; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not shut down")
	}
}
