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
	"github.com/fastcheck/anonymus_bot/bot/internal/config"
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
	handler := NewStartHandler(bot, backendclient.New(backend.URL, serviceToken), slog.New(slog.NewTextHandler(io.Discard, nil)), config.StartBranding{})
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

func TestStartBrandingOnJoinAndIncomingRequest(t *testing.T) {
	fixture := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/sessions/bind":
			_, _ = fmt.Fprint(w, `{"session_id":1,"role":"client"}`)
		case "/internal/incoming-requests":
			_, _ = fmt.Fprint(w, `{"id":1}`)
		default:
			t.Error("unexpected backend request")
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ctx := t.Context()
	start := NewStartHandler(fixture.bot, fixture.backend, fixture.logger, config.StartBranding{
		OrganizationName:  "North & Co",
		JoinedRU:          "Добро пожаловать в тестовый диалог",
		RequestReceivedEN: "We received your fixture request.",
	})
	message, _ := testUpdate()
	message.From.LanguageCode = "ru"
	start.handleWithPayload(ctx, message, strings.Repeat("A", 43))
	fixture.expect(t, "Добро пожаловать в тестовый диалог")
	start.handleWithoutPayload(ctx, message)
	fixture.expect(t, "«North & Co»")
	message.From.LanguageCode = "en"
	start.handleWithPayload(ctx, message, strings.Repeat("A", 43))
	fixture.expect(t, "You have joined a session with North & Co.")
	start.handleWithoutPayload(ctx, message)
	fixture.expect(t, "We received your fixture request.")

	defaultStart := NewStartHandler(fixture.bot, fixture.backend, fixture.logger, config.StartBranding{})
	defaultStart.handleWithPayload(ctx, message, strings.Repeat("A", 43))
	fixture.expect(t, "You have joined the session.")
	message.From.LanguageCode = "ru"
	defaultStart.handleWithoutPayload(ctx, message)
	fixture.expect(t, "«Посредник»")
}

func TestStartExecutorInvitation(t *testing.T) {
	for _, tc := range []struct{ details, want string }{
		{"Fixture first message", "Request details:\nFixture first message"},
		{"", "Request details are currently unavailable."},
	} {
		t.Run(tc.want, func(t *testing.T) {
			fixture := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"session_id":1,"role":"executor","request_text":%q}`, tc.details)
			})
			start := NewStartHandler(fixture.bot, fixture.backend, fixture.logger, config.StartBranding{JoinedEN: "Client greeting"})
			message, _ := testUpdate()
			message.From.LanguageCode = "en"
			start.handleWithPayload(t.Context(), message, strings.Repeat("A", 43))
			fixture.expect(t, "You joined as the executor.", tc.want)
		})
	}
}

func TestStartReportsBackendOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, language, path, body, want string
		status                           int
	}{
		{"accepted", "ru", "/internal/incoming-requests", `{"id":19,"status":"new"}`, "Обращение №19 принято", http.StatusCreated},
		{"assigned", "en", "/internal/incoming-requests", `{"id":19,"status":"in_progress"}`, "Request #19 is in progress", http.StatusOK},
		{"handled", "ru", "/internal/incoming-requests", `{"id":19,"status":"processed"}`, "Обращение №19 уже обработано", http.StatusOK},
		{"request unavailable", "en", "/internal/incoming-requests", `{}`, "We couldn't confirm your request", http.StatusServiceUnavailable},
		{"malformed acknowledgement", "ru", "/internal/incoming-requests", `{}`, "Не удалось подтвердить приём обращения", http.StatusOK},
		{"expired invite", "ru", "/internal/sessions/bind", `{}`, "Запросите новую ссылку у оператора", http.StatusNotFound},
		{"invite check unavailable", "en", "/internal/sessions/bind", `{}`, "Try opening the same link again later", http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Error("unexpected backend operation")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			message, _ := testUpdate()
			message.From.LanguageCode = tc.language
			start := NewStartHandler(f.bot, f.backend, f.logger, config.StartBranding{})
			if tc.path == "/internal/sessions/bind" {
				start.handleWithPayload(t.Context(), message, strings.Repeat("A", 43))
			} else {
				start.handleWithoutPayload(t.Context(), message)
			}
			f.expect(t, tc.want)
		})
	}
}
