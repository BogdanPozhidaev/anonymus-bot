package repository_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/delivery"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestDeliveryWorkerPersistsInFlightResultOnShutdown(t *testing.T) {
	pool := setupTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	users := repository.NewUserRepository(pool, nil)
	sessions := repository.NewSessionRepository(pool)
	messages := repository.NewMessageRepository(pool)
	client := models.User{TelegramID: 701, LanguageCode: "ru"}
	executor := models.User{TelegramID: 702, LanguageCode: "ru"}
	for _, user := range []*models.User{&client, &executor} {
		if err := users.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	session := models.Session{Title: "Fixture", Status: "active", Language: "ru", ClientUserID: &client.ID, ExecutorUserID: &executor.ID}
	if err := sessions.Create(ctx, &session); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"first fixture message", "second fixture message"} {
		if err := messages.Enqueue(ctx, &models.Message{
			SessionID: session.ID, SenderUserID: &client.ID, SenderRole: "client", ContentType: "text", Content: &text,
		}); err != nil {
			t.Fatal(err)
		}
	}
	started := make(chan string, 1)
	finish := make(chan struct{}, 1)
	var sends atomic.Int32
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sends.Add(1) != 1 {
			t.Error("worker sent another message after shutdown")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var request botclient.SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error("could not decode fixture request")
		}
		started <- request.Text
		select {
		case <-finish:
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			t.Error("shutdown canceled the in-flight delivery")
		case <-ctx.Done():
		}
	}))
	defer bot.Close()
	worker := delivery.New(messages, botclient.New(bot.URL, "synthetic-service-token"), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(runCtx)
	}()
	select {
	case text := <-started:
		if text != "first fixture message" {
			t.Fatal("worker did not claim the first message")
		}
	case <-ctx.Done():
		t.Fatal("worker did not start the first delivery")
	}
	stop()
	finish <- struct{}{}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("worker did not finish shutdown")
	}

	history, err := messages.ListBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || !history[0].Delivered || history[0].DeliveryStatus != "delivered" ||
		history[1].Delivered || history[1].DeliveryStatus != "queued" {
		t.Fatal("in-flight result was not saved or the next message was processed")
	}
	var unclaimed bool
	if err := pool.QueryRow(ctx, "SELECT lease_token IS NULL FROM message_outbox WHERE message_id=$1", history[1].ID).Scan(&unclaimed); err != nil || !unclaimed {
		t.Fatalf("the second message must remain unclaimed: %v", err)
	}
}
