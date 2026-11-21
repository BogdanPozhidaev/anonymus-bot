package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestStartRedeemsInvitationInPrivateChat(t *testing.T) {
	token := strings.Repeat("A", 43)
	serviceToken := strings.Repeat("a", 64)
	var binds, sends atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		binds.Add(1)
		var req backendclient.BindSessionRequest
		if r.Method != http.MethodPost || r.URL.Path != "/internal/sessions/bind" ||
			r.Header.Get("X-Internal-Token") != serviceToken || json.NewDecoder(r.Body).Decode(&req) != nil ||
			req.Token != token || req.TelegramID != 1 {
			t.Error("unexpected invitation request")
		}
		_, _ = fmt.Fprint(w, `{"session_id":1,"role":"client","undelivered_messages":[{"content_type":"text","content":"fixture"}]}`)
	}))
	defer backend.Close()
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			sends.Add(1)
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	defer telegram.Close()
	bot, err := tgbotapi.NewBotAPIWithClient("fixture", telegram.URL+"/bot%s/%s", telegram.Client())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewStartHandler(bot, backendclient.New(backend.URL, serviceToken), slog.New(slog.NewTextHandler(io.Discard, nil)))
	message := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1, Type: "private"}, From: &tgbotapi.User{ID: 1}}
	handler.handleWithPayload(context.Background(), message, "42-c")
	if binds.Load() != 0 || sends.Load() != 1 {
		t.Fatal("legacy link did not receive a local rejection")
	}
	handler.handleWithPayload(context.Background(), message, token)
	if binds.Load() != 1 || sends.Load() != 2 {
		t.Fatal("expected one redemption and one additional message")
	}
}
