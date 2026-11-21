package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestClientIPTrustsOnlyConfiguredProxy(t *testing.T) {
	for _, tc := range []struct{ name, proxy, peer, want string }{
		{"without proxy", "", "192.0.2.10:1234", "192.0.2.10"},
		{"direct with proxy configured", "192.0.2.20", "192.0.2.10:1234", "192.0.2.10"},
		{"from proxy", "192.0.2.20", "192.0.2.20:1234", "198.51.100.15"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			if err := configureClientIP(r, tc.proxy); err != nil {
				t.Fatal(err)
			}
			r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.peer
			req.Header.Set("X-Real-IP", "198.51.100.15")
			req.Header.Set("X-Forwarded-For", "203.0.113.15")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if got := w.Body.String(); got != tc.want {
				t.Fatalf("client IP = %q, want %q", got, tc.want)
			}
		})
	}
	if err := configureClientIP(gin.New(), "0.0.0.0/0"); err == nil {
		t.Fatal("accepted network instead of one proxy IP")
	}
}
