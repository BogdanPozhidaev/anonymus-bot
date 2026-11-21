package retention_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/retention"
)

func TestRetentionPreservesActiveParticipantAndFailedFiles(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	users := repository.NewUserRepository(pool, testEncryptor(t))
	sessions := repository.NewSessionRepository(pool)
	messages := repository.NewMessageRepository(pool)
	audit := repository.NewAuditLogRepository(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	user := models.User{TelegramID: 501, LanguageCode: "ru"}
	if err := users.Create(ctx, &user); err != nil {
		t.Fatal(err)
	}
	old := models.Session{Title: "old fixture", Status: models.SessionStatusActive, Language: "ru", ClientUserID: &user.ID}
	if err := sessions.Create(ctx, &old); err != nil {
		t.Fatal(err)
	}
	if err := sessions.UpdateStatus(ctx, old.ID, models.SessionStatusClosed); err != nil {
		t.Fatal(err)
	}
	ageSessionClosedAt(t, pool, old.ID, 400*24*time.Hour)
	open := models.Session{Title: "open fixture", Status: models.SessionStatusPaused, Language: "ru", ClientUserID: &user.ID}
	if err := sessions.Create(ctx, &open); err != nil {
		t.Fatal(err)
	}
	if err := users.SetActiveSession(ctx, user.ID, open.ID); err != nil {
		t.Fatal(err)
	}
	config := retention.DefaultConfig()
	anonymizer := retention.NewPIIAnonymizer(sessions, users, audit, retention.NewAuditLogHasher(audit, logger), config, logger)
	for _, dryRun := range []bool{true, false} {
		result, err := anonymizer.Run(ctx, dryRun)
		if err != nil || result.UsersAnonymized != 0 || len(result.Errors) != 0 {
			t.Fatalf("active participant selected, result=%+v err=%v", result, err)
		}
	}
	found, err := users.GetByTelegramID(ctx, user.TelegramID)
	if err != nil || found.ActiveSessionID == nil || *found.ActiveSessionID != open.ID {
		t.Fatal("routing for active participant changed")
	}

	root := t.TempDir()
	dir := filepath.Join(root, "media")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.jpg")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	blocked := "38c45a07-ff42-40ce-92f5-4f6e03dfaf22"
	if err := os.Mkdir(filepath.Join(dir, blocked+".jpg"), 0700); err != nil {
		t.Fatal(err)
	}
	invalid := "../outside"
	for _, fileID := range []string{blocked, invalid} {
		msg := models.Message{SessionID: old.ID, SenderRole: models.SenderRoleClient, ContentType: models.ContentTypePhoto, FileID: &fileID}
		if err := messages.Create(ctx, &msg); err != nil {
			t.Fatal(err)
		}
	}
	cleaner := retention.NewMessageCleaner(messages, dir, dir, config, logger)
	result, err := cleaner.Run(ctx, false)
	if err != nil || result.MessagesDeleted != 0 || len(result.Errors) != 2 {
		t.Fatalf("unexpected cleanup result: %+v, %v", result, err)
	}
	remaining, err := messages.ListBySessionID(ctx, old.ID)
	if err != nil || len(remaining) != 2 {
		t.Fatal("failed media cleanup lost message records")
	}
	blockedPath := filepath.Join(dir, blocked+".jpg")
	if err := os.Remove(blockedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blockedPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = cleaner.Run(ctx, false)
	if err != nil || result.MessagesDeleted != 1 || result.FilesDeleted != 1 || len(result.Errors) != 1 {
		t.Fatalf("retry did not remove repaired media: %+v, %v", result, err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("file outside the media root changed: %v", err)
	}
}

func TestIncomingRequestRetention(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	requests := repository.NewIncomingRequestRepository(pool)
	threshold := time.Now().Add(-90 * 24 * time.Hour)
	for _, status := range []string{"new", "processed"} {
		old := models.IncomingRequest{TelegramID: 510, Status: status}
		if err := requests.Create(ctx, &old); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE incoming_requests SET created_at=$1 WHERE id=$2", threshold.Add(-time.Hour), old.ID); err != nil {
			t.Fatal(err)
		}
	}
	recent := models.IncomingRequest{TelegramID: 511}
	if err := requests.Create(ctx, &recent); err != nil {
		t.Fatal(err)
	}
	for _, dryRun := range []bool{true, false} {
		count, err := requests.PurgeBefore(ctx, threshold, dryRun)
		if err != nil || count != 2 {
			t.Fatalf("purge count=%d dry_run=%v err=%v", count, dryRun, err)
		}
		var remaining int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM incoming_requests").Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if (dryRun && remaining != 3) || (!dryRun && remaining != 1) {
			t.Fatalf("unexpected count %d after dry_run=%v", remaining, dryRun)
		}
	}
	if _, err := requests.GetByID(ctx, recent.ID); err != nil {
		t.Fatal("recent request was removed")
	}
}
