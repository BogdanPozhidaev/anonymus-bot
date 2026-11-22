package retention_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/retention"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func TestPIIAnonymizer_Run_AnonymizesClosedSessionParticipants(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)

	userRepo := repository.NewUserRepository(pool, encryptor)
	sessionRepo := repository.NewSessionRepository(pool)
	auditLogRepo := repository.NewAuditLogRepository(pool)
	ctx := context.Background()

	client := &models.User{TelegramID: 5001, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, client); err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := &models.User{TelegramID: 5002, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, executor); err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	session := &models.Session{
		Title:          "Old Closed Session",
		Status:         models.SessionStatusActive,
		Language:       "ru",
		ClientUserID:   &client.ID,
		ExecutorUserID: &executor.ID,
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Set the closure time four months in the past, beyond the three-month default.
	if err := sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusClosed); err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	ageSessionClosedAt(t, pool, session.ID, 4*30*24*time.Hour)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := retention.DefaultConfig()
	entry := &models.AuditLog{ActorType: "user", ActorID: &client.ID, Action: "fixture",
		Payload: []byte(`{"username":"fixture-participant","status":"kept"}`)}
	if err := auditLogRepo.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	broken := retention.NewPIIAnonymizer(sessionRepo, userRepo, auditLogRepo, retention.NewAuditLogHasher(nil), config, logger)
	result, err := broken.Run(ctx, false)
	if err != nil || len(result.Errors) != 1 || result.UsersAnonymized != 1 {
		t.Fatalf("expected audit hash failure only for the participant with audit PII: %+v, %v", result, err)
	}
	if got, err := userRepo.GetByID(ctx, client.ID); err != nil || got.TelegramID != 5001 {
		t.Fatalf("audit failure must leave the participant eligible for retry: %v", err)
	}
	auditHasher := retention.NewAuditLogHasher(make([]byte, 32))
	anonymizer := retention.NewPIIAnonymizer(sessionRepo, userRepo, auditLogRepo, auditHasher, config, logger)

	result, err = anonymizer.Run(ctx, false)
	if err != nil {
		t.Fatalf("anonymizer run failed: %v", err)
	}

	if result.SessionsProcessed != 1 {
		t.Errorf("expected 1 session processed, got %d", result.SessionsProcessed)
	}
	if result.UsersAnonymized != 1 || len(result.Errors) != 0 {
		t.Errorf("expected remaining user anonymized, got %+v", result)
	}
	entries, err := auditLogRepo.List(ctx, repository.AuditLogFilter{RelatedUserID: &client.ID})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	for _, candidate := range entries {
		if candidate.ID == entry.ID {
			if err := json.Unmarshal(candidate.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if !strings.HasPrefix(payload["username"], "hmac-sha256:v1:") || payload["status"] != "kept" {
		t.Fatal("retention did not persist the HMAC payload")
	}

	fetchedClient, err := userRepo.GetByID(ctx, client.ID)
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}
	if fetchedClient.TelegramID >= 0 {
		t.Errorf("expected client telegram_id to be negative after anonymization, got %d", fetchedClient.TelegramID)
	}
	if fetchedClient.Username != nil {
		t.Errorf("expected client username to be nil, got %v", *fetchedClient.Username)
	}
}

func TestPIIAnonymizer_Run_DryRunDoesNotModifyData(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)

	userRepo := repository.NewUserRepository(pool, encryptor)
	sessionRepo := repository.NewSessionRepository(pool)
	auditLogRepo := repository.NewAuditLogRepository(pool)
	ctx := context.Background()

	client := &models.User{TelegramID: 6001, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, client); err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	session := &models.Session{
		Title:        "Old Closed Session Dry Run",
		Status:       models.SessionStatusActive,
		Language:     "ru",
		ClientUserID: &client.ID,
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusClosed); err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	ageSessionClosedAt(t, pool, session.ID, 4*30*24*time.Hour)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := retention.DefaultConfig()
	auditHasher := retention.NewAuditLogHasher(make([]byte, 32))
	anonymizer := retention.NewPIIAnonymizer(sessionRepo, userRepo, auditLogRepo, auditHasher, config, logger)
	result, err := anonymizer.Run(ctx, true) // dry run
	if err != nil {
		t.Fatalf("anonymizer dry run failed: %v", err)
	}

	if result.UsersAnonymized != 1 {
		t.Errorf("expected 1 user reported in dry run, got %d", result.UsersAnonymized)
	}

	// Confirm that dry-run leaves the user data intact.
	fetchedClient, err := userRepo.GetByID(ctx, client.ID)
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}
	if fetchedClient.TelegramID != 6001 {
		t.Errorf("expected telegram_id to remain 6001 after dry run, got %d", fetchedClient.TelegramID)
	}
}

func TestPIIAnonymizer_Run_SkipsRecentlyClosedSessions(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)

	userRepo := repository.NewUserRepository(pool, encryptor)
	sessionRepo := repository.NewSessionRepository(pool)
	auditLogRepo := repository.NewAuditLogRepository(pool)
	ctx := context.Background()

	client := &models.User{TelegramID: 7001, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, client); err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	session := &models.Session{
		Title:        "Recently Closed Session",
		Status:       models.SessionStatusActive,
		Language:     "ru",
		ClientUserID: &client.ID,
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Use the current closure time to keep the session within its retention period.
	if err := sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusClosed); err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := retention.DefaultConfig()
	auditHasher := retention.NewAuditLogHasher(make([]byte, 32))
	anonymizer := retention.NewPIIAnonymizer(sessionRepo, userRepo, auditLogRepo, auditHasher, config, logger)
	result, err := anonymizer.Run(ctx, false)
	if err != nil {
		t.Fatalf("anonymizer run failed: %v", err)
	}

	if result.SessionsProcessed != 0 {
		t.Errorf("expected 0 sessions processed for recently closed session, got %d", result.SessionsProcessed)
	}

	fetchedClient, err := userRepo.GetByID(ctx, client.ID)
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}
	if fetchedClient.TelegramID != 7001 {
		t.Errorf("expected telegram_id unchanged for recently closed session, got %d", fetchedClient.TelegramID)
	}
}

func testEncryptor(t *testing.T) *crypto.Encryptor {
	t.Helper()
	key := make([]byte, 32)
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("failed to create test encryptor: %v", err)
	}
	return enc
}
