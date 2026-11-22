package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func localListener(t *testing.T) net.Listener {
	t.Helper()
	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func receive[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for shutdown event")
		var zero T
		return zero
	}
}

func TestServeDrainsHTTPAndWorker(t *testing.T) {
	listener := localListener(t)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	requestStarted := make(chan struct{})
	workerStopping := make(chan struct{})
	httpStopping := make(chan struct{})
	finishRequest := make(chan struct{}, 1)
	finishWorker := make(chan struct{}, 1)
	srv := newHTTPServer("0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		select {
		case <-finishRequest:
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
		}
	}))
	srv.RegisterOnShutdown(func() { close(httpStopping) })

	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, srv, listener, func(ctx context.Context) {
			<-ctx.Done()
			close(workerStopping)
			select {
			case <-finishWorker:
			case <-t.Context().Done():
			}
		}, 5*time.Second)
	}()
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	requestStatus := make(chan int, 1)
	go func() {
		resp, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			requestStatus <- 0
			return
		}
		_ = resp.Body.Close()
		requestStatus <- resp.StatusCode
	}()

	receive(t, requestStarted)
	stop()
	receive(t, workerStopping)
	receive(t, httpStopping)

	dialer := net.Dialer{Timeout: time.Second}
	if conn, err := dialer.DialContext(t.Context(), "tcp", listener.Addr().String()); err == nil {
		_ = conn.Close()
		t.Fatal("server still accepts connections during shutdown")
	}
	finishRequest <- struct{}{}
	if status := receive(t, requestStatus); status != http.StatusNoContent {
		t.Fatalf("in-flight request ended with status %d", status)
	}
	select {
	case err := <-result:
		t.Fatalf("shutdown returned before the worker finished: %v", err)
	default:
	}
	finishWorker <- struct{}{}
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestServeClosesHTTPWhenDrainTimesOut(t *testing.T) {
	listener := localListener(t)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	started := make(chan struct{})
	interrupted := make(chan struct{})
	srv := newHTTPServer("0", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(interrupted)
	}))
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, srv, listener, func(ctx context.Context) { <-ctx.Done() }, 50*time.Millisecond)
	}()
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	go func() {
		resp, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	receive(t, started)
	stop()
	if err := receive(t, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected graceful shutdown timeout, got %v", err)
	}
	receive(t, interrupted)
}

func TestServeStopsWorkerOnListenerFailure(t *testing.T) {
	listener := localListener(t)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	srv := newHTTPServer("0", http.NotFoundHandler())
	err := serve(t.Context(), srv, listener, func(ctx context.Context) {
		<-ctx.Done()
		close(stopped)
	}, time.Second)
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected listener failure, got %v", err)
	}
	receive(t, stopped)
}

func TestServeReportsWorkerDrainTimeout(t *testing.T) {
	listener := localListener(t)
	ctx, stop := context.WithCancel(t.Context())
	stop()
	allowExit := make(chan struct{})
	defer close(allowExit)

	srv := newHTTPServer("0", http.NotFoundHandler())
	err := serve(ctx, srv, listener, func(context.Context) { <-allowExit }, 50*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "drain delivery worker") {
		t.Fatalf("expected delivery worker shutdown timeout, got %v", err)
	}
}
