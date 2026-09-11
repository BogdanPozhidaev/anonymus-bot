package repository_test

import (
	"context"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestSessionRepository_CreateAndGetByID(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewSessionRepository(pool)
	ctx := context.Background()

	session := &models.Session{
		Title:    "Test Session",
		Status:   models.SessionStatusActive,
		Language: "ru",
	}

	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if session.ID == 0 {
		t.Fatal("expected session ID to be set after creation")
	}

	fetched, err := repo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get session by id: %v", err)
	}

	if fetched.Title != session.Title {
		t.Errorf("expected title %q, got %q", session.Title, fetched.Title)
	}
	if fetched.Status != models.SessionStatusActive {
		t.Errorf("expected status %q, got %q", models.SessionStatusActive, fetched.Status)
	}
}

func TestSessionRepository_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewSessionRepository(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 999999)
	if err != repository.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_ListByOperator(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repository.NewSessionRepository(pool)
	operatorRepo := repository.NewOperatorRepository(pool)
	ctx := context.Background()

	operator := &models.Operator{
		Name:     "Test Operator",
		Login:    "test_op_1",
		Role:     models.OperatorRoleOperator,
		Status:   models.OperatorStatusActive,
		Language: "ru",
	}
	if err := operatorRepo.Create(ctx, operator); err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}

	session1 := &models.Session{
		Title:           "Session 1",
		Status:          models.SessionStatusActive,
		Language:        "ru",
		OwnerOperatorID: &operator.ID,
	}
	session2 := &models.Session{
		Title:           "Session 2",
		Status:          models.SessionStatusActive,
		Language:        "ru",
		OwnerOperatorID: &operator.ID,
	}

	if err := sessionRepo.Create(ctx, session1); err != nil {
		t.Fatalf("failed to create session1: %v", err)
	}
	if err := sessionRepo.Create(ctx, session2); err != nil {
		t.Fatalf("failed to create session2: %v", err)
	}

	sessions, err := sessionRepo.ListByOperator(ctx, operator.ID)
	if err != nil {
		t.Fatalf("failed to list sessions by operator: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionRepository_UpdateStatus(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewSessionRepository(pool)
	ctx := context.Background()

	session := &models.Session{
		Title:    "Test Session",
		Status:   models.SessionStatusActive,
		Language: "ru",
	}
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.UpdateStatus(ctx, session.ID, models.SessionStatusPaused); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	fetched, err := repo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if fetched.Status != models.SessionStatusPaused {
		t.Errorf("expected status %q, got %q", models.SessionStatusPaused, fetched.Status)
	}
}

func TestSessionRepository_AssignOwner(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repository.NewSessionRepository(pool)
	operatorRepo := repository.NewOperatorRepository(pool)
	ctx := context.Background()

	operator := &models.Operator{
		Name:     "New Owner",
		Login:    "new_owner_1",
		Role:     models.OperatorRoleOperator,
		Status:   models.OperatorStatusActive,
		Language: "ru",
	}
	if err := operatorRepo.Create(ctx, operator); err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}

	session := &models.Session{
		Title:    "Test Session",
		Status:   models.SessionStatusActive,
		Language: "ru",
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := sessionRepo.AssignOwner(ctx, session.ID, operator.ID); err != nil {
		t.Fatalf("failed to assign owner: %v", err)
	}

	fetched, err := sessionRepo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if fetched.OwnerOperatorID == nil || *fetched.OwnerOperatorID != operator.ID {
		t.Errorf("expected owner_operator_id %d, got %v", operator.ID, fetched.OwnerOperatorID)
	}
}
