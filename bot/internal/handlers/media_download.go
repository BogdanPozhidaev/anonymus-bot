package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	telegramDownloadTimeout = 20 * time.Second
	telegramDownloadLimit   = 20 << 20
	telegramMetadataLimit   = 64 << 10
)

var telegramMediaDownloader = mediaDownloader{
	client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	},
	apiEndpoint:  tgbotapi.APIEndpoint,
	fileEndpoint: tgbotapi.FileEndpoint,
	timeout:      telegramDownloadTimeout,
	maxBytes:     telegramDownloadLimit,
}

type mediaDownloader struct {
	client                    *http.Client
	apiEndpoint, fileEndpoint string
	timeout                   time.Duration
	maxBytes                  int64
}

func (d mediaDownloader) download(ctx context.Context, token, fileID, destPath string) error {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	file, err := d.getFile(ctx, token, fileID)
	if err != nil {
		return err
	}
	if int64(file.FileSize) > d.maxBytes {
		return errors.New("media file exceeds download limit")
	}
	resp, err := d.request(ctx, "file download", http.MethodGet, fmt.Sprintf(d.fileEndpoint, token, file.FilePath), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.ContentLength > d.maxBytes {
		return errors.New("media file exceeds download limit")
	}

	out, err := os.CreateTemp(filepath.Dir(destPath), ".telegram-download-*")
	if err != nil {
		return errors.New("could not create temporary media file")
	}
	defer func() {
		_ = out.Close()
		_ = os.Remove(out.Name())
	}()

	written, err := io.Copy(out, io.LimitReader(resp.Body, d.maxBytes+1))
	if err != nil {
		return mediaOperationError(ctx, "file download", "could not read or save response")
	}
	if written > d.maxBytes {
		return errors.New("media file exceeds download limit")
	}
	if err := out.Close(); err != nil {
		return errors.New("could not close temporary media file")
	}
	if err := ctx.Err(); err != nil {
		return mediaOperationError(ctx, "file download", "request failed")
	}
	if err := os.Rename(out.Name(), destPath); err != nil {
		return errors.New("could not publish downloaded media file")
	}
	return nil
}

func (d mediaDownloader) getFile(ctx context.Context, token, fileID string) (tgbotapi.File, error) {
	form := url.Values{"file_id": {fileID}}
	resp, err := d.request(ctx, "file metadata", http.MethodPost, fmt.Sprintf(d.apiEndpoint, token, "getFile"), strings.NewReader(form.Encode()))
	if err != nil {
		return tgbotapi.File{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, telegramMetadataLimit+1))
	if err != nil {
		return tgbotapi.File{}, mediaOperationError(ctx, "file metadata", "could not read response")
	}
	var result struct {
		OK   bool          `json:"ok"`
		File tgbotapi.File `json:"result"`
	}
	if len(body) > telegramMetadataLimit || json.Unmarshal(body, &result) != nil || !result.OK || result.File.FilePath == "" {
		return tgbotapi.File{}, errors.New("file metadata: invalid response")
	}
	return result.File, nil
}

func (d mediaDownloader) request(ctx context.Context, stage, method, endpoint string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("%s: could not create request", stage)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, mediaOperationError(ctx, stage, "request failed")
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s: unexpected HTTP status %d", stage, resp.StatusCode)
	}
	return resp, nil
}

// Never return a transport or response error that could include the token.
func mediaOperationError(ctx context.Context, stage, fallback string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", stage, err)
	}
	return fmt.Errorf("%s: %s", stage, fallback)
}
