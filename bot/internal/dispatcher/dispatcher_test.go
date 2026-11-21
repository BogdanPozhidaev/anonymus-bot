package dispatcher

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestRunOrdersUpdatesFromSameUser(t *testing.T) {
	updates := make(chan tgbotapi.Update, 3)
	started := make(chan int, 3)
	release := make(chan struct{})
	defer close(release)
	resume := make(chan struct{})
	done := make(chan struct{})
	var finished, outOfOrder atomic.Bool
	d := New(func(_ context.Context, update tgbotapi.Update) {
		if update.UpdateID == 2 && !finished.Load() {
			outOfOrder.Store(true)
		}
		started <- update.UpdateID
		if update.UpdateID == 1 {
			select {
			case <-release:
			case <-resume:
			}
			finished.Store(true)
		}
	}, 3, slog.Default())
	updates <- tgbotapi.Update{UpdateID: 1, Message: &tgbotapi.Message{From: &tgbotapi.User{ID: 1}}}
	updates <- tgbotapi.Update{UpdateID: 2, CallbackQuery: &tgbotapi.CallbackQuery{From: &tgbotapi.User{ID: 1}}}
	updates <- tgbotapi.Update{UpdateID: 3, Message: &tgbotapi.Message{From: &tgbotapi.User{ID: 2}}}
	close(updates)
	go func() {
		d.Run(context.Background(), updates)
		close(done)
	}()
	seen := map[int]bool{}
	for range 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(3 * time.Second):
			t.Fatal("independent updates did not start")
		}
	}
	if !seen[1] || !seen[3] {
		t.Fatalf("same-user callback ran before the first update finished: %v", seen)
	}
	close(resume)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dispatcher did not drain updates")
	}
	select {
	case id := <-started:
		if id != 2 {
			t.Fatalf("expected callback, got update %d", id)
		}
	default:
		t.Fatal("callback was dropped")
	}
	if outOfOrder.Load() {
		t.Fatal("callback started before the preceding same-user handler completed")
	}
}
