package repository_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestIncomingRequestReuseAndCooldown(t *testing.T) {
	pool := setupTestDB(t)
	requests := repository.NewIncomingRequestRepository(pool)
	ctx := context.Background()
	const telegramID = 6601
	ids := make(chan int64, 12)
	var wg sync.WaitGroup
	var inserted atomic.Int32
	for range cap(ids) {
		wg.Go(func() {
			ir := models.IncomingRequest{TelegramID: telegramID}
			created, err := requests.CreateOrReuse(ctx, &ir)
			if err != nil {
				t.Errorf("upsert: %v", err)
				return
			}
			if created {
				inserted.Add(1)
			}
			ids <- ir.ID
		})
	}
	wg.Wait()
	close(ids)
	first := <-ids
	if first == 0 || inserted.Load() != 1 {
		t.Fatal("concurrent duplicates did not collapse into one request")
	}
	for id := range ids {
		if first != id {
			t.Fatal("inconsistent request ID for repeated submission")
		}
	}
	for _, text := range []string{"first fixture", "later fixture"} {
		ir := models.IncomingRequest{TelegramID: telegramID, FirstMessageText: &text}
		if created, err := requests.CreateOrReuse(ctx, &ir); err != nil || created || ir.ID != first {
			t.Fatalf("could not reuse open request: %v", err)
		}
	}
	stored, err := requests.GetByID(ctx, first)
	if err != nil || stored.FirstMessageText == nil || *stored.FirstMessageText != "first fixture" {
		t.Fatal("later updates overwrote the first message")
	}
	if err := requests.UpdateStatus(ctx, first, "processed"); err != nil {
		t.Fatal(err)
	}
	ir := models.IncomingRequest{TelegramID: telegramID}
	if created, err := requests.CreateOrReuse(ctx, &ir); err != nil || created || ir.ID != first {
		t.Fatalf("recent processed request bypassed cooldown: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE incoming_requests SET created_at=now()-interval '11 minutes' WHERE id=$1", first); err != nil {
		t.Fatal(err)
	}
	if created, err := requests.CreateOrReuse(ctx, &ir); err != nil || !created || ir.ID == first {
		t.Fatalf("request was not accepted after cooldown: %v", err)
	}
}

func TestIncomingRequestRepository_CreateAndGetByID(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewIncomingRequestRepository(pool)
	ctx := context.Background()

	text := "Здравствуйте, интересует услуга"
	ir := &models.IncomingRequest{
		TelegramID:       555666777,
		FirstMessageText: &text,
		Status:           models.IncomingRequestStatusNew,
	}

	if err := repo.Create(ctx, ir); err != nil {
		t.Fatalf("failed to create incoming request: %v", err)
	}

	fetched, err := repo.GetByID(ctx, ir.ID)
	if err != nil {
		t.Fatalf("failed to get incoming request: %v", err)
	}

	if fetched.TelegramID != ir.TelegramID {
		t.Errorf("expected telegram_id %d, got %d", ir.TelegramID, fetched.TelegramID)
	}
	if fetched.Status != models.IncomingRequestStatusNew {
		t.Errorf("expected status %q, got %q", models.IncomingRequestStatusNew, fetched.Status)
	}
}

func TestIncomingRequestRepository_ListByStatus(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewIncomingRequestRepository(pool)
	ctx := context.Background()

	ir1 := &models.IncomingRequest{TelegramID: 111, Status: models.IncomingRequestStatusNew}
	ir2 := &models.IncomingRequest{TelegramID: 222, Status: models.IncomingRequestStatusNew}
	ir3 := &models.IncomingRequest{TelegramID: 333, Status: models.IncomingRequestStatusProcessed}

	for _, ir := range []*models.IncomingRequest{ir1, ir2, ir3} {
		if err := repo.Create(ctx, ir); err != nil {
			t.Fatalf("failed to create incoming request: %v", err)
		}
	}

	newRequests, err := repo.ListByStatus(ctx, models.IncomingRequestStatusNew)
	if err != nil {
		t.Fatalf("failed to list by status: %v", err)
	}

	if len(newRequests) != 2 {
		t.Errorf("expected 2 new requests, got %d", len(newRequests))
	}
}

func TestIncomingRequestRepository_UpdateStatus(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewIncomingRequestRepository(pool)
	ctx := context.Background()

	ir := &models.IncomingRequest{TelegramID: 444, Status: models.IncomingRequestStatusNew}
	if err := repo.Create(ctx, ir); err != nil {
		t.Fatalf("failed to create incoming request: %v", err)
	}

	if err := repo.UpdateStatus(ctx, ir.ID, models.IncomingRequestStatusProcessed); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	fetched, err := repo.GetByID(ctx, ir.ID)
	if err != nil {
		t.Fatalf("failed to get incoming request: %v", err)
	}

	if fetched.Status != models.IncomingRequestStatusProcessed {
		t.Errorf("expected status %q, got %q", models.IncomingRequestStatusProcessed, fetched.Status)
	}
}
