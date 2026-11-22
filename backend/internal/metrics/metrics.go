package metrics

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const Path = "/metrics"

type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	attempts *prometheus.CounterVec
}

func New(backlog func(context.Context) (queued, retrying int64, err error)) *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anonymus_http_requests_total",
			Help: "HTTP requests by route template and response status class.",
		}, []string{"route", "status_class"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "anonymus_http_request_duration_seconds",
			Help: "HTTP request duration in seconds by route template.",
		}, []string{"route"}),
		attempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anonymus_delivery_attempts_total",
			Help: "Delivery calls to the bot service by kind and observed result.",
		}, []string{"kind", "result"}),
	}
	for _, kind := range []string{"message", "notice"} {
		for _, result := range []string{"success", "failure"} {
			m.attempts.WithLabelValues(kind, result)
		}
	}
	m.registry.MustRegister(m.requests, m.duration, m.attempts, collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), newOutboxCollector(backlog))
	return m
}

func (m *Metrics) Middleware(c *gin.Context) {
	start := time.Now()
	c.Next()
	route := c.FullPath()
	if route == Path {
		return
	}
	if route == "" {
		route = "unmatched"
	}
	m.requests.WithLabelValues(route, strconv.Itoa(c.Writer.Status()/100)+"xx").Inc()
	m.duration.WithLabelValues(route).Observe(time.Since(start).Seconds())
}

func (m *Metrics) RecordDeliveryAttempt(notice, success bool) {
	kind, result := "message", "failure"
	if notice {
		kind = "notice"
	}
	if success {
		result = "success"
	}
	m.attempts.WithLabelValues(kind, result).Inc()
}

func (m *Metrics) Handler(token string) http.Handler {
	exporter := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{MaxRequestsInFlight: 2})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) == 1 && token != "" {
			provided, bearer := strings.CutPrefix(values[0], "Bearer ")
			if bearer && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
				exporter.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

type outboxCollector struct {
	read    func(context.Context) (queued, retrying int64, err error)
	pending *prometheus.Desc
	success *prometheus.Desc
}

func newOutboxCollector(read func(context.Context) (queued, retrying int64, err error)) *outboxCollector {
	return &outboxCollector{
		read: read,
		pending: prometheus.NewDesc("anonymus_outbox_pending",
			"Messages queued or retrying, including those waiting for an eligible recipient.", []string{"status"}, nil),
		success: prometheus.NewDesc("anonymus_outbox_collection_success",
			"Whether the outbox query succeeded during this scrape.", nil, nil),
	}
}

func (o *outboxCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- o.pending
	ch <- o.success
}

func (o *outboxCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	queued, retrying, err := o.read(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(o.success, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(o.success, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(o.pending, prometheus.GaugeValue, float64(queued), "queued")
	ch <- prometheus.MustNewConstMetric(o.pending, prometheus.GaugeValue, float64(retrying), "retrying")
}
