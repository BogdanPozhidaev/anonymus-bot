package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalRoutesRequireServiceToken(t *testing.T) {
	token := strings.Repeat("a", 64)
	router := gin.New()
	(&Server{}).RegisterRoutes(router, token)

	routes := []struct{ method, path string }{
		{"POST", "/internal/sessions/bind"},
		{"POST", "/internal/incoming-requests"},
		{"GET", "/internal/users/by-telegram/1"},
		{"PATCH", "/internal/users/by-telegram/1/language"},
		{"GET", "/internal/operators/by-telegram/1"},
		{"POST", "/internal/operators/telegram-link/claim"},
		{"POST", "/internal/messages/relay"},
		{"GET", "/internal/users/by-telegram/1/sessions"},
		{"PATCH", "/internal/users/by-telegram/1/active-session"},
		{"POST", "/internal/sessions/stop"},
		{"POST", "/internal/incoming-requests/1/mark-processed"},
		{"GET", "/internal/operators/by-telegram/1/sessions"},
		{"GET", "/internal/incoming-requests"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			// Malformed JSON and absent repositories make entering a handler observable.
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(route.method, route.path, strings.NewReader("{")))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", response.Code)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		headers []string
		want    int
	}{
		{"wrong token", []string{strings.Repeat("b", 64)}, http.StatusUnauthorized},
		{"duplicate token", []string{token, token}, http.StatusUnauthorized},
		{"correct token reaches validation", []string{token}, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/internal/sessions/bind", strings.NewReader("{"))
			req.Header.Set("Content-Type", "application/json")
			for _, value := range tc.headers {
				req.Header.Add("X-Internal-Token", value)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-Internal-Token", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing session token") {
		t.Fatal("operator API did not require its own session token")
	}
}

func TestInternalTokenAllowsValidRequest(t *testing.T) {
	token := strings.Repeat("a", 64)
	router := gin.New()
	router.GET("/internal/test", requireInternalToken(token), func(c *gin.Context) {
		if c.GetHeader("X-Internal-Token") != "" {
			t.Error("service token remained visible to the handler")
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/test", nil)
	req.Header.Set("X-Internal-Token", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}
