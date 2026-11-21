package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPServerBoundsConnectionsAndUsesConfiguredHandler(t *testing.T) {
	srv := newHTTPServer("8180", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if srv.Addr != ":8180" || srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout < srv.ReadHeaderTimeout ||
		srv.WriteTimeout < srv.ReadTimeout || srv.IdleTimeout <= 0 {
		t.Fatal("unexpected server address or connection timeouts")
	}
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNoContent {
		t.Fatal("configured handler did not serve the request")
	}
}
