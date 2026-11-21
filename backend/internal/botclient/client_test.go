package botclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotCallsSendServiceToken(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, tc := range []struct {
		path string
		send func(*Client) error
	}{
		{"/internal/send-message", func(c *Client) error { return c.SendMessage(context.Background(), SendMessageRequest{}) }},
		{"/internal/send-alert", func(c *Client) error { return c.SendAlert(context.Background(), SendAlertRequest{}) }},
	} {
		t.Run(tc.path, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path || r.Method != http.MethodPost {
					t.Error("unexpected request route")
				}
				if values := r.Header.Values("X-Internal-Token"); len(values) != 1 || values[0] != token {
					t.Error("expected one service token header")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			if err := tc.send(New(server.URL, token)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBotServiceTokenIsNotForwardedOnRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target received a request")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	if err := New(source.URL, strings.Repeat("a", 64)).SendAlert(context.Background(), SendAlertRequest{}); err == nil {
		t.Fatal("expected an error for redirect")
	}
}
