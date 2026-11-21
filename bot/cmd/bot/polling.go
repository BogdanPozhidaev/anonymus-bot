package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/fastcheck/anonymus_bot/bot/internal/dispatcher"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type pollingHTTPClient struct {
	ctx    context.Context
	client *http.Client
}

func (p pollingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return p.client.Do(req.WithContext(p.ctx))
}

type updatePoller interface {
	GetUpdates(tgbotapi.UpdateConfig) ([]tgbotapi.Update, error)
}

func pollUpdates(ctx context.Context, bot updatePoller, d *dispatcher.Dispatcher, logger *slog.Logger) {
	config := tgbotapi.NewUpdate(0)
	config.Timeout = 30
	config.Limit = 10
	for ctx.Err() == nil {
		batch, err := bot.GetUpdates(config)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("Telegram update poll failed")
			retry := time.NewTimer(3 * time.Second)
			select {
			case <-ctx.Done():
				retry.Stop()
				return
			case <-retry.C:
				continue
			}
		}
		if len(batch) == 0 {
			continue
		}
		updates := make(chan tgbotapi.Update, len(batch))
		for _, update := range batch {
			if update.UpdateID >= config.Offset {
				updates <- update
				config.Offset = update.UpdateID + 1
			}
		}
		close(updates)
		// A later poll acknowledges this offset only after the current batch drains.
		d.Run(context.WithoutCancel(ctx), updates)
	}
}
