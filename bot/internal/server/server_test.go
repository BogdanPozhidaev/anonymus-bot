package server

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type recordingSender struct {
	messages []tgbotapi.Chattable
	err      error
}

func (s *recordingSender) Send(msg tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.messages = append(s.messages, msg)
	return tgbotapi.Message{}, s.err
}

func TestDeliveryFailureIsRetryableAndRedactsTelegramError(t *testing.T) {
	const marker = "synthetic-telegram-token"
	for _, route := range []struct{ path, body string }{
		{"/internal/send-message", `{"recipient_telegram_id":1,"sender_label":"Operator","text":"test"}`},
		{"/internal/send-alert", `{"text":"test alert"}`},
	} {
		t.Run(route.path, func(t *testing.T) {
			var logs bytes.Buffer
			sender := &recordingSender{err: errors.New("transport error for /bot" + marker)}
			token := strings.Repeat("a", 64)
			handler := New(sender, 2, slog.New(slog.NewTextHandler(&logs, nil)), token, "", "", "").handler()
			req := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.body))
			req.Header.Set("X-Internal-Token", token)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusInternalServerError || len(sender.messages) != 1 || logs.Len() == 0 {
				t.Fatalf("failed Telegram submission was not reported: status=%d", res.Code)
			}
			if strings.Contains(res.Body.String(), marker) || strings.Contains(logs.String(), marker) {
				t.Fatal("Telegram transport error leaked a synthetic token marker")
			}
		})
	}
}

func TestInternalRoutesAuthenticateBeforeSending(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, route := range []struct {
		path, body, text string
		chatID           int64
	}{
		{"/internal/send-message", `{"recipient_telegram_id":1,"sender_label":"Operator","text":"test"}`, "Operator test", 1},
		{"/internal/send-alert", `{"text":"test alert"}`, "test alert", 2},
	} {
		t.Run(route.path, func(t *testing.T) {
			sender := &recordingSender{}
			handler := New(sender, 2, nil, token, "", "", "").handler()
			for _, tc := range []struct {
				name          string
				headers       []string
				status, sends int
			}{
				{"missing", nil, http.StatusUnauthorized, 0},
				{"wrong", []string{strings.Repeat("b", 64)}, http.StatusUnauthorized, 0},
				{"duplicate", []string{token, token}, http.StatusUnauthorized, 0},
				{"valid", []string{token}, http.StatusOK, 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.body))
					for _, value := range tc.headers {
						req.Header.Add("X-Internal-Token", value)
					}
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, req)
					if response.Code != tc.status || len(sender.messages) != tc.sends {
						t.Fatalf("status/sends = %d/%d, want %d/%d", response.Code, len(sender.messages), tc.status, tc.sends)
					}
					if tc.sends == 1 {
						msg, ok := sender.messages[0].(tgbotapi.MessageConfig)
						if !ok || msg.ChatID != route.chatID || msg.Text != route.text {
							t.Fatal("unexpected Telegram message")
						}
					}
				})
			}
		})
	}
}

func TestBotHealthDoesNotSendTelegramMessages(t *testing.T) {
	sender := &recordingSender{}
	handler := New(sender, 0, nil, strings.Repeat("a", 64), "", "", "").handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
	if res.Code != http.StatusOK || len(sender.messages) != 0 {
		t.Fatalf("unexpected readiness result: %d", res.Code)
	}
}

func TestDeliveryReplyButtonUsesRecipientLanguage(t *testing.T) {
	token := strings.Repeat("a", 64)
	for language, label := range map[string]string{"en": "Reply here", "ru": "Ответить сюда", "": "Ответить сюда"} {
		sender := &recordingSender{}
		req := httptest.NewRequest(http.MethodPost, "/internal/send-message", strings.NewReader(
			`{"recipient_telegram_id":1,"session_id":2,"text":"fixture","language":"`+language+`"}`))
		req.Header.Set("X-Internal-Token", token)
		res := httptest.NewRecorder()
		New(sender, 0, nil, token, "", "", "").handler().ServeHTTP(res, req)
		if res.Code != http.StatusOK || len(sender.messages) != 1 {
			t.Fatalf("unexpected delivery response: %d", res.Code)
		}
		markup := sender.messages[0].(tgbotapi.MessageConfig).ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
		if button := markup.InlineKeyboard[0][0]; button.Text != label || *button.CallbackData != "switch_session:2" {
			t.Fatal("incorrect localized reply button")
		}
	}
}

func TestSessionAlertsLinkDirectlyToHTTPSPanel(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, fixture := range []struct{ base, url string }{
		{"https://panel.example.invalid", "https://panel.example.invalid/sessions/42"},
		{"http://localhost:8080", ""},
	} {
		sender := &recordingSender{}
		handler := New(sender, 2, nil, token, "", "", fixture.base).handler()
		req := httptest.NewRequest(http.MethodPost, "/internal/send-alert", strings.NewReader(`{
			"text":"fixture",
			"buttons":[{"label":"Open","callback_data":"open_session:42"},{"label":"Close","callback_data":"close_session:42"},{"label":"Take","callback_data":"take_request:42"}]
		}`))
		req.Header.Set("X-Internal-Token", token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK || len(sender.messages) != 1 {
			t.Fatal("operator alert failed")
		}
		message := sender.messages[0].(tgbotapi.MessageConfig)
		row := message.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup).InlineKeyboard[0]
		for i, callback := range []string{"open_session:42", "close_session:42", "take_request:42"} {
			if i < 2 && fixture.url != "" {
				if row[i].URL == nil || *row[i].URL != fixture.url || row[i].CallbackData != nil {
					t.Fatalf("button %d did not link to the configured panel", i)
				}
			} else if row[i].URL != nil || row[i].CallbackData == nil || *row[i].CallbackData != callback {
				t.Fatalf("button %d did not retain its callback", i)
			}
		}
	}
}
