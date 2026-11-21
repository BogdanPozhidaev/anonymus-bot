package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type brokenRequestBody struct{}

func (brokenRequestBody) Read([]byte) (int, error) { return 0, errors.New("fixture transport error") }
func (brokenRequestBody) Close() error             { return nil }

func TestRequestBodyLimit(t *testing.T) {
	r := gin.New()
	r.POST("/fixture", limitRequestBody, func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil || len(body) != maxJSONBodySize {
			t.Errorf("handler received %d bytes: %v", len(body), err)
		}
		c.Status(http.StatusNoContent)
	})
	for _, tc := range []struct {
		name   string
		body   io.Reader
		status int
	}{
		{"exact limit", strings.NewReader(strings.Repeat("a", maxJSONBodySize)), http.StatusNoContent},
		{"declared too large", strings.NewReader(strings.Repeat("a", maxJSONBodySize+1)), http.StatusRequestEntityTooLarge},
		{"stream too large", io.LimitReader(strings.NewReader(strings.Repeat("a", maxJSONBodySize+1)), maxJSONBodySize+1), http.StatusRequestEntityTooLarge},
		{"read error", brokenRequestBody{}, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/fixture", tc.body)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "fixture transport error") {
				t.Fatalf("status=%d want=%d, or response leaked an error", w.Code, tc.status)
			}
		})
	}
}
