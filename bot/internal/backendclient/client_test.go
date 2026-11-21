package backendclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceTokenOnEveryMethod(t *testing.T) {
	token := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if values := r.Header.Values("X-Internal-Token"); len(values) != 1 || values[0] != token {
			t.Error("expected one service token header")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, token)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch} {
		if err := client.doRequest(context.Background(), method, "/internal/test", nil, nil); err != nil {
			t.Errorf("%s: %v", method, err)
		}
	}
}

func TestServiceTokenIsNotForwardedOnRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target received a request")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client := New(source.URL, strings.Repeat("a", 64))
	if err := client.doRequest(context.Background(), http.MethodGet, "/internal/test", nil, nil); err == nil {
		t.Fatal("expected an error for redirect")
	}
}
