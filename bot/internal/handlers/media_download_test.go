package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureMediaToken = "synthetic-media-token"

func testMediaDownloader(server *httptest.Server) mediaDownloader {
	d := telegramMediaDownloader
	d.apiEndpoint = server.URL + "/bot%s/%s"
	d.fileEndpoint = server.URL + "/file/bot%s/%s"
	d.maxBytes = 8
	d.timeout = 2 * time.Second
	return d
}

func TestMediaDownloadSizeAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name         string
		metadataSize int
		writeFile    func(http.ResponseWriter)
		wantError    string
	}{
		{"exact boundary", 8, func(w http.ResponseWriter) { _, _ = fmt.Fprint(w, "12345678") }, ""},
		{"metadata limit", 9, nil, "exceeds download limit"},
		{"content length limit", 0, func(w http.ResponseWriter) { _, _ = fmt.Fprint(w, "123456789") }, "exceeds download limit"},
		{"stream limit", 0, func(w http.ResponseWriter) {
			w.(http.Flusher).Flush()
			_, _ = fmt.Fprint(w, "123456789")
		}, "exceeds download limit"},
		{"truncated", 0, func(w http.ResponseWriter) {
			w.Header().Set("Content-Length", "8")
			_, _ = fmt.Fprint(w, "123")
		}, "could not read or save response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/bot" + fixtureMediaToken + "/getFile":
					if r.Method != http.MethodPost || r.FormValue("file_id") != "fixture" {
						t.Error("unexpected metadata request")
					}
					_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"file_path":"fixture","file_size":%d}}`, tc.metadataSize)
				case "/file/bot" + fixtureMediaToken + "/fixture":
					if tc.writeFile == nil {
						t.Error("download started despite metadata size")
						return
					}
					tc.writeFile(w)
				default:
					t.Error("unexpected path")
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			dest := filepath.Join(dir, "media")
			err := testMediaDownloader(server).download(context.Background(), fixtureMediaToken, "fixture", dest)
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || len(entries) != 0 {
					t.Fatalf("expected %q and no files, got %v and %d files", tc.wantError, err, len(entries))
				}
				return
			}
			data, readErr := os.ReadFile(dest)
			if err != nil || readErr != nil || string(data) != "12345678" || len(entries) != 1 {
				t.Fatalf("unexpected download result: %v, read: %v, file count: %d", err, readErr, len(entries))
			}
		})
	}
}

func TestMediaDownloadDeadlineAndCancellation(t *testing.T) {
	for _, stage := range []string{"file metadata", "file download"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "file download" && strings.HasSuffix(r.URL.Path, "/getFile") {
					_, _ = fmt.Fprint(w, `{"ok":true,"result":{"file_path":"fixture"}}`)
					return
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			d := testMediaDownloader(server)
			d.timeout = 100 * time.Millisecond
			dir := t.TempDir()
			dest := filepath.Join(dir, "media")
			err := d.download(context.Background(), fixtureMediaToken, "fixture", dest)
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), stage) {
				t.Fatalf("expected timeout at %s, got %v", stage, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := d.download(ctx, fixtureMediaToken, "fixture", dest); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("expected no files, got %d, %v", len(entries), err)
			}
		})
	}
}

func TestMediaDownloadErrorsDoNotLogToken(t *testing.T) {
	for _, stage := range []string{"file metadata", "file download"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "file download" {
					_, _ = fmt.Fprint(w, `{"ok":true,"result":{"file_path":"fixture"}}`)
					return
				}
				_, _ = fmt.Fprintf(w, `{"ok":false,"description":"%s"}`, fixtureMediaToken)
			}))
			defer server.Close()
			d := testMediaDownloader(server)
			if stage == "file download" {
				closed := httptest.NewServer(http.NotFoundHandler())
				closed.Close()
				d.fileEndpoint = closed.URL + "/file/bot%s/%s"
			}
			err := d.download(context.Background(), fixtureMediaToken, "fixture", filepath.Join(t.TempDir(), "media"))
			var logs bytes.Buffer
			slog.New(slog.NewTextHandler(&logs, nil)).Error("download failed", "error", err)
			if err == nil || !strings.Contains(logs.String(), stage) || strings.Contains(logs.String(), fixtureMediaToken) {
				t.Fatal("expected a stage-specific error without token")
			}
		})
	}
}

func TestMediaDownloadDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()
	for _, stage := range []string{"file metadata", "file download"} {
		t.Run(stage, func(t *testing.T) {
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "file download" && strings.HasSuffix(r.URL.Path, "/getFile") {
					_, _ = fmt.Fprint(w, `{"ok":true,"result":{"file_path":"fixture"}}`)
					return
				}
				http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			}))
			defer source.Close()
			err := testMediaDownloader(source).download(context.Background(), fixtureMediaToken, "fixture", filepath.Join(t.TempDir(), "media"))
			if err == nil || !strings.Contains(err.Error(), "307") || redirected.Load() {
				t.Fatalf("expected redirect rejection, got %v", err)
			}
		})
	}
}
