package repository_test

import (
	"context"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestOperatorRepository_CreateAndGetByID(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewOperatorRepository(pool)
	ctx := context.Background()

	operator := &models.Operator{
		Name:     "Alice Admin",
		Login:    "alice",
		Role:     models.OperatorRoleAdmin,
		Status:   models.OperatorStatusActive,
		Language: "ru",
	}

	if err := repo.Create(ctx, operator); err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}

	fetched, err := repo.GetByID(ctx, operator.ID)
	if err != nil {
		t.Fatalf("failed to get operator by id: %v", err)
	}

	if fetched.Login != operator.Login {
		t.Errorf("expected login %q, got %q", operator.Login, fetched.Login)
	}
	if fetched.Role != models.OperatorRoleAdmin {
		t.Errorf("expected role %q, got %q", models.OperatorRoleAdmin, fetched.Role)
	}
}

func TestOperatorRepository_GetByLogin(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewOperatorRepository(pool)
	ctx := context.Background()

	operator := &models.Operator{
		Name:     "Bob Operator",
		Login:    "bob",
		Role:     models.OperatorRoleOperator,
		Status:   models.OperatorStatusInvited,
		Language: "en",
	}
	if err := repo.Create(ctx, operator); err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}

	fetched, err := repo.GetByLogin(ctx, "bob")
	if err != nil {
		t.Fatalf("failed to get operator by login: %v", err)
	}

	if fetched.ID != operator.ID {
		t.Errorf("expected id %d, got %d", operator.ID, fetched.ID)
	}
}

func TestOperatorRepository_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewOperatorRepository(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 999999)
	if err != repository.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestOperatorRepository_Update(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewOperatorRepository(pool)
	ctx := context.Background()

	operator := &models.Operator{
		Name:     "Carol",
		Login:    "carol",
		Role:     models.OperatorRoleOperator,
		Status:   models.OperatorStatusInvited,
		Language: "ru",
	}
	if err := repo.Create(ctx, operator); err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}

	operator.Status = models.OperatorStatusActive
	operator.Name = "Carol Updated"

	if err := repo.Update(ctx, operator); err != nil {
		t.Fatalf("failed to update operator: %v", err)
	}

	fetched, err := repo.GetByID(ctx, operator.ID)
	if err != nil {
		t.Fatalf("failed to get operator: %v", err)
	}

	if fetched.Status != models.OperatorStatusActive {
		t.Errorf("expected status %q, got %q", models.OperatorStatusActive, fetched.Status)
	}
	if fetched.Name != "Carol Updated" {
		t.Errorf("expected name %q, got %q", "Carol Updated", fetched.Name)
	}
}
