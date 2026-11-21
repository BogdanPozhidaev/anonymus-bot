package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestDeliveryEndpointSendsAndRetainsMedia(t *testing.T) {
	var received atomic.Int32
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			kind = "photo"
		case strings.HasSuffix(r.URL.Path, "/sendVoice"):
			kind = "voice"
		}
		if kind != "" {
			file, _, err := r.FormFile(kind)
			if err != nil {
				t.Error("fake Telegram did not receive media")
				http.Error(w, "invalid fixture", 400)
				return
			}
			defer func() { _ = file.Close() }()
			data, err := io.ReadAll(file)
			if err != nil || string(data) != kind {
				t.Error("unexpected uploaded body")
			}
			received.Add(1)
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	defer telegram.Close()
	bot, err := tgbotapi.NewBotAPIWithClient("fixture", telegram.URL+"/bot%s/%s", telegram.Client())
	if err != nil {
		t.Fatal(err)
	}
	photos, voices := t.TempDir(), t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	token := strings.Repeat("a", 64)
	handler := New(bot, 1, logger, token, photos, voices, "").handler()
	const id = "38c45a07-ff42-40ce-92f5-4f6e03dfaf22"
	for _, tc := range []struct{ kind, ext, dir string }{
		{"photo", ".jpg", photos}, {"voice", ".ogg", voices},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			path := filepath.Join(tc.dir, id+tc.ext)
			if err := os.WriteFile(path, []byte(tc.kind), 0600); err != nil {
				t.Fatal(err)
			}
			before := received.Load()
			req := httptest.NewRequest(http.MethodPost, "/internal/send-message", strings.NewReader(
				fmt.Sprintf(`{"recipient_telegram_id":1,"session_id":1,"content_type":%q,"file_id":%q}`, tc.kind, id)))
			req.Header.Set("X-Internal-Token", token)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			data, err := os.ReadFile(path)
			if res.Code != 200 || err != nil || string(data) != tc.kind || received.Load() != before+1 {
				t.Fatal("media was not delivered and retained")
			}
		})
	}
}
