package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/bot/internal/dispatcher"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakePoller func(tgbotapi.UpdateConfig) ([]tgbotapi.Update, error)

func (f fakePoller) GetUpdates(config tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
	return f(config)
}

func TestPollDrainsBeforeAcknowledgingAndStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var processed []int
	log := slog.Default()
	d := dispatcher.New(func(ctx context.Context, u tgbotapi.Update) {
		if ctx.Err() != nil {
			t.Error("shutdown cancelled a previously accepted update")
		}
		processed = append(processed, u.UpdateID)
		if u.UpdateID == 7 {
			cancel()
		}
	}, 1, log)
	var offsets []int
	poll := fakePoller(func(cfg tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		offsets = append(offsets, cfg.Offset)
		switch cfg.Offset {
		case 0:
			return []tgbotapi.Update{{UpdateID: 4}, {UpdateID: 5}}, nil
		case 6:
			if !slices.Equal(processed, []int{4, 5}) {
				t.Fatal("offset advanced before previous batch was processed")
			}
			return []tgbotapi.Update{{UpdateID: 7}, {UpdateID: 8}}, nil
		default:
			t.Fatal("new poll started after shutdown")
			return nil, nil
		}
	})
	pollUpdates(ctx, poll, d, log)
	if !slices.Equal(processed, []int{4, 5, 7, 8}) || !slices.Equal(offsets, []int{0, 6}) {
		t.Fatal("shutdown skipped an accepted update or acknowledged it early")
	}
}

func TestPollFailureDuringCancellationHidesTelegramToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	const marker = "synthetic-token"
	d := dispatcher.New(func(context.Context, tgbotapi.Update) { t.Error("failed poll dispatched an update") }, 1, logger)
	pollUpdates(ctx, fakePoller(func(tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		cancel()
		return nil, errors.New("transport failed /bot" + marker)
	}), d, logger)
	if strings.Contains(logs.String(), marker) {
		t.Fatal("poll logged an error containing the bot token")
	}
}
