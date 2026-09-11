package repository_test

import (
	"context"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

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
