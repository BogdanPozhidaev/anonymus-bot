package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestStoredLanguageOverridesTelegramHint(t *testing.T) {
	f := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/users/by-telegram/1":
			_, _ = fmt.Fprint(w, `{"language_code":"en"}`)
		case "/internal/operators/by-telegram/2":
			_, _ = fmt.Fprint(w, `{"id":1,"language":"ru","status":"active"}`)
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	for _, tc := range []struct {
		id       int64
		hint     string
		language string
	}{
		{1, "ru", "en"},
		{2, "en-US", "ru"},
		{3, "en-US", "en"},
		{4, "fr", "ru"},
	} {
		m, c := testUpdate()
		m.From.ID = tc.id
		m.From.LanguageCode = tc.hint
		update := LocalizeUpdate(ctx, f.backend, tgbotapi.Update{Message: m})
		callback := LocalizeUpdate(ctx, f.backend, tgbotapi.Update{CallbackQuery: c}).CallbackQuery
		if update.Message.From.LanguageCode != tc.language || callback.From.LanguageCode != tc.language || m.From.LanguageCode != tc.hint {
			t.Fatal("stored language precedence or update copy is incorrect")
		}
		NewHelpHandler(f.bot, f.logger).Handle(update.Message)
		if tc.language == "en" {
			f.expect(t, "Available commands")
		} else {
			f.expect(t, "Доступные команды")
		}
	}
}

func TestLanguageWriteFailureDoesNotConfirmChange(t *testing.T) {
	f := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	_, callback := testUpdate()
	callback.From.LanguageCode = "en"
	callback.Data = "set_lang:ru"
	NewLanguageHandler(f.bot, f.backend, f.logger).HandleCallback(context.Background(), callback)
	f.expect(t, "Join a session or link your operator account first.")
	if len(f.sent) != 0 {
		t.Fatal("failed update edited the language confirmation")
	}
}

func TestOperatorLinkRequiresPrivateSenderAndReturnsConfirmation(t *testing.T) {
	f := newHandlerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code       string `json:"code"`
			TelegramID int64  `json:"telegram_id"`
		}
		if r.URL.Path != "/internal/operators/telegram-link/claim" || r.Method != http.MethodPost ||
			json.NewDecoder(r.Body).Decode(&body) != nil || body.TelegramID != 1 {
			t.Error("link claim must use the verified private sender")
		}
		if body.Code != "fixture-code" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"confirmation_code":"fixture-confirmation"}`)
	})
	handler := NewOperatorHandler(f.bot, f.backend, f.logger)
	ctx := context.Background()
	m, _ := testUpdate()
	m.From.LanguageCode = "en"
	m.Text = "/link fixture-code"
	m.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Length: 5}}
	m.Chat.Type = "group"
	handler.HandleLink(ctx, m)
	m.Chat.Type, m.Chat.ID = "private", 2
	handler.HandleLink(ctx, m)
	if len(f.sent) != 0 {
		t.Fatal("linking replied outside the sender's private chat")
	}
	m.Chat.ID = 1
	handler.HandleLink(ctx, m)
	f.expect(t, "Enter this confirmation code in the panel:", "fixture-confirmation")
	m.Text = "/link expired-code"
	handler.HandleLink(ctx, m)
	f.expect(t, "The code is unavailable.")
}
