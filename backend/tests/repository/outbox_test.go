package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/delivery"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestOutboxWaitingRetryFailureAndDuplicateProcessing(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	users := repository.NewUserRepository(pool, nil)
	sessions := repository.NewSessionRepository(pool)
	messages := repository.NewMessageRepository(pool)
	client := models.User{TelegramID: 701, LanguageCode: "ru"}
	if err := users.Create(ctx, &client); err != nil {
		t.Fatal(err)
	}
	session := models.Session{Title: "fixture", Status: "active", Language: "ru", ClientUserID: &client.ID}
	if err := sessions.Create(ctx, &session); err != nil {
		t.Fatal(err)
	}
	enqueue := func(kind string) *models.Message {
		t.Helper()
		m := &models.Message{SessionID: session.ID, SenderUserID: &client.ID, SenderRole: "client", ContentType: kind}
		if kind == "text" {
			text := "fixture"
			m.Content = &text
		} else {
			id := "38c45a07-ff42-40ce-92f5-4f6e03dfaf22"
			m.FileID = &id
		}
		if err := messages.Enqueue(ctx, m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	assertState := func(m *models.Message, status string, delivered bool) {
		t.Helper()
		rows, err := messages.ListBySessionID(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ID == m.ID {
				if row.DeliveryStatus != status || row.Delivered != delivered {
					t.Fatalf("got %s/%v, want %s/%v", row.DeliveryStatus, row.Delivered, status, delivered)
				}
				return
			}
		}
		t.Fatal("message missing from history")
	}
	requests := make(chan botclient.SendMessageRequest, 20)
	statuses := make(chan int, 20)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req botclient.SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error("bad fixture request")
		}
		requests <- req
		select {
		case status := <-statuses:
			w.WriteHeader(status)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer fake.Close()
	worker := delivery.New(messages, botclient.New(fake.URL, "fixture"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	next := func(want bool) {
		t.Helper()
		ok, err := worker.ProcessNext(ctx)
		if err != nil || ok != want {
			t.Fatalf("processed=%v want=%v err=%v", ok, want, err)
		}
	}
	ready := func(m *models.Message) {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE message_outbox SET available_at=clock_timestamp() WHERE message_id=$1", m.ID); err != nil {
			t.Fatal(err)
		}
	}
	// A failed outbox insert must roll back the message insert as well.
	if _, err := pool.Exec(ctx, "ALTER TABLE message_outbox ADD CONSTRAINT fixture_reject_insert CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	if err := messages.Enqueue(ctx, &models.Message{SessionID: session.ID, SenderUserID: &client.ID, SenderRole: "client", ContentType: "text"}); err == nil {
		t.Fatal("fixture did not reject the outbox insert")
	}
	if rows, err := messages.ListBySessionID(ctx, session.ID); err != nil || len(rows) != 0 {
		t.Fatal("outbox failure left a partial message")
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE message_outbox DROP CONSTRAINT fixture_reject_insert"); err != nil {
		t.Fatal(err)
	}
	text := enqueue("text")
	assertState(text, "queued", false)
	next(false)

	hash := sha256.Sum256(bytes.Repeat([]byte{1}, 32))
	if _, err := sessions.ManageInvite(ctx, session.ID, "executor", 1, true, hash[:]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sessions.BindInvite(ctx, users, hash[:], &models.User{TelegramID: 702, LanguageCode: "ru"}); err != nil {
		t.Fatal(err)
	}
	assertState(text, "queued", false)
	statuses <- 503
	next(true)
	first := <-requests
	if first.RecipientTelegramID != 702 || first.Text != "fixture" {
		t.Fatal("incorrect queued text delivery")
	}
	assertState(text, "retrying", false)
	next(false)
	ready(text)
	next(true)
	<-requests
	assertState(text, "delivered", true)
	next(false)

	photo := enqueue("photo")
	// Claim without sending, then confirm no other worker takes an active lease.
	old, err := messages.ClaimDelivery(ctx, time.Minute)
	if err != nil || old == nil {
		t.Fatalf("lease: %v", err)
	}
	next(false)
	ready(photo)
	fresh, err := messages.ClaimDelivery(ctx, time.Minute)
	if err != nil || fresh == nil || old.Lease == fresh.Lease {
		t.Fatalf("recovered lease: %v", err)
	}
	if err := messages.CompleteDelivery(ctx, old, true, 0, 5); err != nil {
		t.Fatal(err)
	}
	assertState(photo, "queued", false)
	ready(photo)
	next(true)
	if req := <-requests; req.ContentType != "photo" || req.FileID != *photo.FileID {
		t.Fatal("photo was not routed as media")
	}
	assertState(photo, "delivered", true)

	voice := enqueue("voice")
	for i := range 5 {
		statuses <- 503
		ready(voice)
		next(true)
		if req := <-requests; req.ContentType != "voice" || req.FileID != *voice.FileID {
			t.Fatal("voice was not routed as media")
		}
		if i < 4 {
			assertState(voice, "retrying", false)
		}
	}
	assertState(voice, "failed", false)
	statuses <- 503
	next(true)
	notice := <-requests
	if notice.RecipientTelegramID != 701 || notice.ContentType != "text" || notice.SessionID != 0 || notice.FileID != "" || notice.Text == "" {
		t.Fatal("failure notification did not go privately to the sender")
	}
	ready(voice)
	next(true)
	<-requests
	assertState(voice, "failed", false)
	next(false)
}
