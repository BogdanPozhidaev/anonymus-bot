package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type handlerFixture struct {
	bot      *tgbotapi.BotAPI
	backend  *backendclient.Client
	telegram *httptest.Server
	logger   *slog.Logger
	sent     chan string
}

func newHandlerFixture(t *testing.T, backend http.HandlerFunc) handlerFixture {
	t.Helper()
	sent := make(chan string, 64)
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "getMe":
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"id":1}}`)
			return
		case "getFile":
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"file_path":"media"}}`)
			return
		case "media":
			_, _ = fmt.Fprint(w, "fixture")
			return
		}
		sent <- r.FormValue("text") + r.FormValue("reply_markup")
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	t.Cleanup(telegram.Close)
	svc := httptest.NewServer(backend)
	t.Cleanup(svc.Close)
	bot, err := tgbotapi.NewBotAPIWithClient("fixture", telegram.URL+"/bot%s/%s", telegram.Client())
	if err != nil {
		t.Fatal(err)
	}
	return handlerFixture{bot: bot, backend: backendclient.New(svc.URL, strings.Repeat("a", 64)), telegram: telegram, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), sent: sent}
}

func (f handlerFixture) expect(t *testing.T, substrings ...string) {
	t.Helper()
	select {
	case got := <-f.sent:
		for _, part := range substrings {
			if !strings.Contains(got, part) {
				t.Fatalf("response %q does not contain %q", got, part)
			}
		}
	default:
		t.Fatal("no Telegram reply")
	}
}

func testUpdate() (*tgbotapi.Message, *tgbotapi.CallbackQuery) {
	m := &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: 1, Type: "private"}, From: &tgbotapi.User{ID: 1}, Text: "fixture"}
	return m, &tgbotapi.CallbackQuery{ID: "fixture", From: m.From, Message: m}
}

func TestHandlerCommandsAndCallbacks(t *testing.T) {
	f := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/active-session"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/language"):
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["language_code"] != "en" {
				t.Error("unexpected language request")
			}
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/operators/by-telegram/"):
			if strings.HasSuffix(r.URL.Path, "/sessions") {
				_, _ = fmt.Fprint(w, `{"sessions":[{"title":"Fixture assignment","status":"active"}]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"id":1,"status":"active"}`)
			}
		case r.URL.Path == "/internal/incoming-requests" && r.Method == http.MethodGet:
			_, _ = fmt.Fprint(w, `{"requests":[{"id":1,"first_name":"Fixture sender","first_message_text":"Fixture body"}]}`)
		case r.URL.Path == "/internal/incoming-requests" && r.Method == http.MethodPost:
			_, _ = fmt.Fprint(w, `{"id":1}`)
		case strings.HasSuffix(r.URL.Path, "/mark-processed"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			if r.URL.Query().Get("limit") != "10" {
				t.Error("session request did not set a page size")
			}
			if r.URL.Query().Get("offset") == "10" {
				_, _ = fmt.Fprint(w, `{"sessions":[{"session_id":4,"title":"Second page fixture","status":"active","role":"client"}]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"sessions":[{"session_id":3,"title":"Fixture session","status":"active","role":"client","is_current":true}],"has_more":true}`)
			}
		case strings.HasSuffix(r.URL.Path, "/stop"):
			_, _ = fmt.Fprint(w, `{"session_id":3,"counterpart_telegram_id":2}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	m, c := testUpdate()
	sessions := NewSessionsListHandler(f.bot, f.backend, f.logger)
	sessions.HandleSessionsCommand(ctx, m)
	f.expect(t, "Fixture session", "switch_session:3", "sessions_page:10")
	c.Data = "sessions_page:10"
	sessions.HandlePageCallback(ctx, c)
	f.expect(t)
	f.expect(t, "Second page fixture", "sessions_page:0")
	sessions.HandleCurrentCommand(ctx, m)
	f.expect(t, "Fixture session")
	c.Data = "switch_session:3"
	sessions.HandleSwitchCallback(ctx, c)
	f.expect(t, "Сессия переключена")
	f.expect(t, "Активная сессия изменена")
	c.Data = "switch_session:invalid"
	sessions.HandleSwitchCallback(ctx, c)
	f.expect(t, "Ошибка")
	language := NewLanguageHandler(f.bot, f.backend, f.logger)
	language.HandleCommand(m)
	f.expect(t, "set_lang:en")
	c.Data = "set_lang:en"
	language.HandleCallback(ctx, c)
	f.expect(t)
	f.expect(t, "Language changed to English")
	operator := NewOperatorHandler(f.bot, f.backend, f.logger)
	operator.Handle(ctx, m)
	f.expect(t, "op_menu:sessions")
	c.Data = "op_menu:sessions"
	operator.HandleMenuCallback(ctx, c)
	f.expect(t)
	f.expect(t, "Fixture assignment")
	c.Data = "op_menu:incoming"
	operator.HandleMenuCallback(ctx, c)
	f.expect(t)
	f.expect(t, "Fixture sender", "Fixture body")
	alerts := NewAlertCallbackHandler(f.bot, f.backend, "https://panel.example.invalid/", f.logger)
	c.Data = "open_session:3"
	alerts.Handle(ctx, c)
	f.expect(t, "https://panel.example.invalid/sessions/3")
	c.Data = "take_request:1"
	alerts.Handle(ctx, c)
	f.expect(t, "Взято в работу")
	f.expect(t, "Взято в работу")
	c.Data = "close_session:3"
	alerts.Handle(ctx, c)
	f.expect(t, "https://panel.example.invalid/sessions/3")
	NewStopHandler(f.bot, f.backend, f.logger).Handle(ctx, m)
	f.expect(t)
	f.expect(t)
	NewStartHandler(f.bot, f.backend, f.logger).Handle(ctx, m)
	f.expect(t, "Спасибо")
	NewHelpHandler(f.bot, f.logger).Handle(m)
	f.expect(t, "/start")
	NewBlockedContentHandler(f.bot, f.logger).Handle(m, "document")
	f.expect(t)
}

func TestRelayHandlersReportAndRetainMediaOnUncertainFailure(t *testing.T) {
	for _, tc := range []struct {
		name, reason      string
		status, wantFiles int
	}{
		{"stored", "", 200, 1},
		{"awaiting counterpart", "counterparty_not_bound_yet", 200, 1},
		{"no active session", "no_active_session", 200, 0},
		{"paused", "session_not_active", 200, 0},
		{"rejected", "sender_not_in_session", 200, 0},
		{"blocked by moderation", "moderation_violation", 200, 0},
		{"moderation unavailable", "moderation_unavailable", 200, 0},
		{"ambiguous failure", "", 503, 1},
	} {
		for _, kind := range []string{"text", "photo", "voice"} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				var mu sync.Mutex
				var received backendclient.RelayMessageRequest
				f := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
					if kind == "text" && tc.reason == "no_active_session" && r.URL.Path == "/internal/incoming-requests" {
						var incoming backendclient.CreateIncomingRequestRequest
						if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil || incoming.FirstMessageText != "fixture" || incoming.TelegramID != 1 {
							t.Error("first private message did not reach incoming requests")
						}
						_, _ = fmt.Fprint(w, `{"id":1}`)
						return
					}
					if r.URL.Path != "/internal/messages/relay" {
						t.Error("wrong relay route")
					}
					mu.Lock()
					if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
						t.Error(err)
					}
					mu.Unlock()
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprintf(w, `{"blocked":%t,"block_reason":%q}`, tc.reason != "", tc.reason)
				})
				ctx := context.Background()
				m, _ := testUpdate()
				m.Caption = "fixture caption"
				switch kind {
				case "text":
					NewMessageRelayHandler(f.bot, f.backend, f.logger).HandleText(ctx, m)
				case "photo":
					m.Photo = []tgbotapi.PhotoSize{{FileID: "fixture"}}
					handler := NewPhotoHandler(f.bot, f.backend, dir, f.logger)
					handler.downloader = testMediaDownloader(f.telegram)
					handler.downloader.fileEndpoint = f.telegram.URL + "/file/bot%s/%s"
					handler.strip = func(src, dst string) error {
						data, err := os.ReadFile(src)
						if err != nil {
							return err
						}
						return os.WriteFile(dst, data, 0600)
					}
					handler.Handle(ctx, m)
				case "voice":
					m.Voice = &tgbotapi.Voice{FileID: "fixture"}
					handler := NewVoiceHandler(f.bot, f.backend, dir, f.logger)
					handler.downloader = testMediaDownloader(f.telegram)
					handler.Handle(ctx, m)
				}
				mu.Lock()
				if received.ContentType != kind || received.SenderTelegramID != 1 || (kind != "text" && (received.FileID == "" || received.Content != m.Caption)) {
					t.Error("wrong relay payload")
				}
				mu.Unlock()
				if tc.reason != "" || tc.status != 200 {
					f.expect(t)
				} else if len(f.sent) != 0 {
					t.Fatal("successful relay prematurely sent a Telegram message")
				}
				if kind != "text" {
					files, err := os.ReadDir(dir)
					if err != nil || len(files) != tc.wantFiles {
						t.Fatalf("unexpected number of media files: %d (%v)", len(files), err)
					}
					for _, file := range files {
						if strings.Contains(file.Name(), "_orig") {
							t.Fatal("unstripped original was left on disk")
						}
					}
				}
			})
		}
	}
}
