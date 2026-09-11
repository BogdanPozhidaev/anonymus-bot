package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestAuditLogRepository_Create(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewAuditLogRepository(pool)
	ctx := context.Background()

	action := "session_created"
	targetType := "session"
	targetID := int64(42)
	payload, _ := json.Marshal(map[string]string{"title": "Test Session"})

	entry := &models.AuditLog{
		Action:     action,
		TargetType: &targetType,
		TargetID:   &targetID,
		Payload:    payload,
	}

	if err := repo.Create(ctx, entry); err != nil {
		t.Fatalf("failed to create audit log entry: %v", err)
	}

	if entry.ID == 0 {
		t.Fatal("expected audit log ID to be set after creation")
	}
}

func TestAuditLogRepository_ListWithFilter(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewAuditLogRepository(pool)
	ctx := context.Background()

	action1 := "login"
	action2 := "logout"

	entry1 := &models.AuditLog{Action: action1}
	entry2 := &models.AuditLog{Action: action2}

	if err := repo.Create(ctx, entry1); err != nil {
		t.Fatalf("failed to create entry1: %v", err)
	}
	if err := repo.Create(ctx, entry2); err != nil {
		t.Fatalf("failed to create entry2: %v", err)
	}

	filtered, err := repo.List(ctx, repository.AuditLogFilter{
		Action: &action1,
	})
	if err != nil {
		t.Fatalf("failed to list audit log with filter: %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(filtered))
	}
	if filtered[0].Action != action1 {
		t.Errorf("expected action %q, got %q", action1, filtered[0].Action)
	}
}

func TestAuditLogRepository_ListNoFilter(t *testing.T) {
	pool := setupTestDB(t)
	repo := repository.NewAuditLogRepository(pool)
	ctx := context.Background()

	entry1 := &models.AuditLog{Action: "action_one"}
	entry2 := &models.AuditLog{Action: "action_two"}

	if err := repo.Create(ctx, entry1); err != nil {
		t.Fatalf("failed to create entry1: %v", err)
	}
	if err := repo.Create(ctx, entry2); err != nil {
		t.Fatalf("failed to create entry2: %v", err)
	}

	all, err := repo.List(ctx, repository.AuditLogFilter{})
	if err != nil {
		t.Fatalf("failed to list all audit log: %v", err)
	}

	if len(all) != 2 {
		t.Errorf("expected 2 entries, got %d", len(all))
	}
}
