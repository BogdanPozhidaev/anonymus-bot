package retention_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/retention"
)

func TestMessageCleaner_Run_DeletesOldMessagesAndFiles(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repository.NewSessionRepository(pool)
	messageRepo := repository.NewMessageRepository(pool)
	ctx := context.Background()

	tmpDir := t.TempDir()
	photosDir := filepath.Join(tmpDir, "photos")
	voiceDir := filepath.Join(tmpDir, "voice")
	if err := os.MkdirAll(photosDir, 0755); err != nil {
		t.Fatalf("failed to create photos dir: %v", err)
	}
	if err := os.MkdirAll(voiceDir, 0755); err != nil {
		t.Fatalf("failed to create voice dir: %v", err)
	}

	session := &models.Session{Title: "Old Session", Status: models.SessionStatusActive, Language: "ru"}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if err := sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusClosed); err != nil {
		t.Fatalf("failed to close session: %v", err)
	}
	ageSessionClosedAt(t, pool, session.ID, 13*30*24*time.Hour) // старше 12 месяцев

	fileID := "test-file-123"
	photoPath := filepath.Join(photosDir, fileID+".jpg")
	if err := os.WriteFile(photoPath, []byte("fake image data"), 0644); err != nil {
		t.Fatalf("failed to create test photo file: %v", err)
	}

	textContent := "hello"
	msg1 := &models.Message{
		SessionID:   session.ID,
		SenderRole:  models.SenderRoleClient,
		ContentType: models.ContentTypeText,
		Content:     &textContent,
	}
	if err := messageRepo.Create(ctx, msg1); err != nil {
		t.Fatalf("failed to create text message: %v", err)
	}

	msg2 := &models.Message{
		SessionID:   session.ID,
		SenderRole:  models.SenderRoleClient,
		ContentType: models.ContentTypePhoto,
		FileID:      &fileID,
	}
	if err := messageRepo.Create(ctx, msg2); err != nil {
		t.Fatalf("failed to create photo message: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := retention.DefaultConfig()
	cleaner := retention.NewMessageCleaner(messageRepo, photosDir, voiceDir, config, logger)

	result, err := cleaner.Run(ctx, false)
	if err != nil {
		t.Fatalf("cleaner run failed: %v", err)
	}

	if result.MessagesDeleted != 2 {
		t.Errorf("expected 2 messages deleted, got %d", result.MessagesDeleted)
	}
	if result.FilesDeleted != 1 {
		t.Errorf("expected 1 file deleted, got %d", result.FilesDeleted)
	}

	if _, err := os.Stat(photoPath); !os.IsNotExist(err) {
		t.Error("expected photo file to be deleted from disk")
	}

	remaining, err := messageRepo.ListBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to list remaining messages: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining messages, got %d", len(remaining))
	}
}

func TestMessageCleaner_Run_DryRunLeavesDataIntact(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repository.NewSessionRepository(pool)
	messageRepo := repository.NewMessageRepository(pool)
	ctx := context.Background()

	tmpDir := t.TempDir()

	session := &models.Session{Title: "Old Session Dry Run", Status: models.SessionStatusActive, Language: "ru"}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if err := sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusClosed); err != nil {
		t.Fatalf("failed to close session: %v", err)
	}
	ageSessionClosedAt(t, pool, session.ID, 13*30*24*time.Hour)

	textContent := "hello dry run"
	msg := &models.Message{
		SessionID:   session.ID,
		SenderRole:  models.SenderRoleClient,
		ContentType: models.ContentTypeText,
		Content:     &textContent,
	}
	if err := messageRepo.Create(ctx, msg); err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := retention.DefaultConfig()
	cleaner := retention.NewMessageCleaner(messageRepo, tmpDir, tmpDir, config, logger)

	result, err := cleaner.Run(ctx, true)
	if err != nil {
		t.Fatalf("cleaner dry run failed: %v", err)
	}

	if result.MessagesDeleted != 1 {
		t.Errorf("expected 1 message reported in dry run, got %d", result.MessagesDeleted)
	}

	remaining, err := messageRepo.ListBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected message to still exist after dry run, got %d remaining", len(remaining))
	}
}
