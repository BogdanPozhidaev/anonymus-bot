package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestScrapeAuthorizationAndMetricLabels(t *testing.T) {
	var queries atomic.Int32
	monitor := New(func(context.Context) (int64, int64, error) {
		queries.Add(1)
		return 4, 2, nil
	})
	const token = "fixture-metrics-token"
	router := gin.New()
	router.Use(monitor.Middleware, gin.Recovery())
	router.GET(Path, gin.WrapH(monitor.Handler(token)))
	router.GET("/api/sessions/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/api/failure", func(*gin.Context) { panic("fixture panic") })
	get := func(path string, auth ...string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, value := range auth {
			req.Header.Add("Authorization", value)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, headers := range [][]string{nil, {"Bearer fixture-internal-token"}, {"Bearer " + token, "Bearer " + token}} {
		if rec := get(Path, headers...); rec.Code != http.StatusUnauthorized || queries.Load() != 0 {
			t.Fatalf("denied scrape reached the exporter or returned %d", rec.Code)
		}
	}
	get("/api/sessions/314159?value=fixture-private-data")
	get("/unknown/fixture-private-data")
	get("/api/failure")
	monitor.RecordDeliveryAttempt(false, true)
	monitor.RecordDeliveryAttempt(true, false)
	response := get(Path, "Bearer "+token)
	if response.Code != http.StatusOK || queries.Load() != 1 {
		t.Fatalf("authorized scrape returned %d", response.Code)
	}
	body := response.Body.String()
	for _, want := range []string{
		`anonymus_http_requests_total{route="/api/sessions/:id",status_class="2xx"} 1`,
		`anonymus_http_requests_total{route="/api/failure",status_class="5xx"} 1`,
		`anonymus_http_requests_total{route="unmatched",status_class="4xx"} 1`,
		`anonymus_http_request_duration_seconds_count{route="/api/sessions/:id"} 1`,
		`anonymus_delivery_attempts_total{kind="message",result="success"} 1`,
		`anonymus_delivery_attempts_total{kind="notice",result="failure"} 1`,
		`anonymus_outbox_pending{status="queued"} 4`,
		`anonymus_outbox_pending{status="retrying"} 2`,
		"anonymus_outbox_collection_success 1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing metric %q", want)
		}
	}
	for _, private := range []string{`route="/api/sessions/314159"`, "fixture-private-data", token, `route="/metrics"`} {
		if strings.Contains(body, private) {
			t.Error("export included a request identifier, credential or scrape label")
		}
	}
}

func TestOutboxScrapeMarksUnavailableData(t *testing.T) {
	m := New(func(ctx context.Context) (int64, int64, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("outbox collection has no deadline")
		}
		return 0, 0, errors.New("fixture database unavailable")
	})
	req := httptest.NewRequest(http.MethodGet, Path, nil)
	req.Header.Set("Authorization", "Bearer fixture")
	response := httptest.NewRecorder()
	m.Handler("fixture").ServeHTTP(response, req)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "anonymus_outbox_collection_success 0") ||
		strings.Contains(body, "anonymus_outbox_pending{") || strings.Contains(body, "fixture database unavailable") {
		t.Fatal("outbox error should omit backlog samples and expose only collection status")
	}
}
