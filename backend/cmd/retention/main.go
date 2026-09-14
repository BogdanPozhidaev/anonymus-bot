package main

import (
	"context"
	"encoding/base64"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/db"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/retention"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Run without making any actual changes, only report what would happen")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Error("DATABASE_URL is not set")
		os.Exit(1)
	}

	photosDir := os.Getenv("PHOTOS_DIR")
	if photosDir == "" {
		logger.Error("PHOTOS_DIR is not set")
		os.Exit(1)
	}

	voiceDir := os.Getenv("VOICE_DIR")
	if voiceDir == "" {
		logger.Error("VOICE_DIR is not set")
		os.Exit(1)
	}

	encryptionKeyBase64 := os.Getenv("PII_ENCRYPTION_KEY")
	if encryptionKeyBase64 == "" {
		logger.Error("PII_ENCRYPTION_KEY is not set")
		os.Exit(1)
	}

	encryptionKey, err := base64.StdEncoding.DecodeString(encryptionKeyBase64)
	if err != nil {
		logger.Error("failed to decode PII_ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}

	encryptor, err := crypto.NewEncryptor(encryptionKey)
	if err != nil {
		logger.Error("failed to create encryptor", "error", err)
		os.Exit(1)
	}

	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	userRepo := repository.NewUserRepository(pool, encryptor)
	sessionRepo := repository.NewSessionRepository(pool)
	messageRepo := repository.NewMessageRepository(pool)
	auditLogRepo := repository.NewAuditLogRepository(pool)

	auditHasher := retention.NewAuditLogHasher(auditLogRepo, logger)
	config := retention.DefaultConfig()

	logger.Info("starting retention run", "dry_run", *dryRun)

	piiAnonymizer := retention.NewPIIAnonymizer(sessionRepo, userRepo, auditLogRepo, auditHasher, config, logger)
	piiResult, err := piiAnonymizer.Run(ctx, *dryRun)
	if err != nil {
		logger.Error("PII anonymization run failed", "error", err)
		os.Exit(1)
	}

	logger.Info("PII anonymization complete",
		"sessions_processed", piiResult.SessionsProcessed,
		"users_anonymized", piiResult.UsersAnonymized,
		"errors", len(piiResult.Errors),
	)
	for _, e := range piiResult.Errors {
		logger.Error("pii anonymization error", "error", e)
	}

	messageCleaner := retention.NewMessageCleaner(messageRepo, photosDir, voiceDir, config, logger)
	cleanupResult, err := messageCleaner.Run(ctx, *dryRun)
	if err != nil {
		logger.Error("message cleanup run failed", "error", err)
		os.Exit(1)
	}

	logger.Info("message cleanup complete",
		"messages_deleted", cleanupResult.MessagesDeleted,
		"files_deleted", cleanupResult.FilesDeleted,
		"errors", len(cleanupResult.Errors),
	)
	for _, e := range cleanupResult.Errors {
		logger.Error("message cleanup error", "error", e)
	}

	totalErrors := len(piiResult.Errors) + len(cleanupResult.Errors)
	if totalErrors > 0 {
		logger.Warn("retention run completed with errors", "total_errors", totalErrors)
		os.Exit(1)
	}

	logger.Info("retention run completed successfully")
}
