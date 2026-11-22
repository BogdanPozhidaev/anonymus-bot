package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalRequestLoggingUsesRouteTemplate(t *testing.T) {
	var logs bytes.Buffer
	router := newAPIRouter(slog.New(slog.NewTextHandler(&logs, nil)))
	const route = "/internal/users/by-telegram/:telegramID"
	router.GET(route, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.POST(route, func(*gin.Context) {
		panic("synthetic-sensitive-body")
	})
	for method, status := range map[string]int{http.MethodGet: http.StatusNoContent, http.MethodPost: http.StatusInternalServerError} {
		req := httptest.NewRequest(method, "/internal/users/by-telegram/123456789?key=synthetic-query", nil)
		req.Header.Set("X-Internal-Token", "synthetic-service-token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s: got status %d, want %d", method, response.Code, status)
		}
	}
	output := logs.String()
	if strings.Count(output, route) != 3 || !strings.Contains(output, "API handler panicked") {
		t.Fatal("internal requests should log their route template, including panics")
	}
	for _, value := range []string{"123456789", "synthetic-query", "synthetic-service-token", "synthetic-sensitive-body"} {
		if strings.Contains(output, value) {
			t.Fatal("logs contained an unredacted request or panic value")
		}
	}
}
