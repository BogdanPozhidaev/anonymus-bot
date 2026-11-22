package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const shutdownTimeout = 100 * time.Second

func serve(ctx context.Context, srv *http.Server, listener net.Listener, worker func(context.Context), gracePeriod time.Duration) error {
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker(workerCtx)
	}()

	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(listener) }()

	var serveErr error
	serving := true
	select {
	case <-ctx.Done():
	case serveErr = <-serveDone:
		serving = false
	}
	stopWorker()

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gracePeriod)
	defer cancel()
	var shutdownErr error
	if err := srv.Shutdown(shutdownCtx); err != nil {
		shutdownErr = errors.Join(fmt.Errorf("drain HTTP requests: %w", err), srv.Close())
	}
	if serving {
		serveErr = <-serveDone
	}
	select {
	case <-workerDone:
	default:
		select {
		case <-workerDone:
		case <-shutdownCtx.Done():
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("drain delivery worker: %w", shutdownCtx.Err()))
		}
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("serve HTTP: %w", serveErr))
	}
	return shutdownErr
}
