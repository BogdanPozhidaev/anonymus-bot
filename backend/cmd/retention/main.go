package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/config"
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

	dbURL, err := config.DatabaseURL()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	photosDir, voiceDir, err := config.MediaSettings()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	encryption, err := config.LoadEncryption()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		logger.Error("cannot connect to PostgreSQL; check database settings and availability")
		os.Exit(1)
	}
	defer pool.Close()

	userRepo := repository.NewUserRepository(pool, encryption.PII)
	sessionRepo := repository.NewSessionRepository(pool)
	messageRepo := repository.NewMessageRepository(pool)
	auditLogRepo := repository.NewAuditLogRepository(pool)

	auditHasher := retention.NewAuditLogHasher(encryption.AuditKey)
	config, err := retention.LoadConfig()
	if err != nil {
		logger.Error("failed to load retention config", "error", err)
		os.Exit(1)
	}
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

	requestCount, err := repository.NewIncomingRequestRepository(pool).PurgeBefore(
		ctx, time.Now().Add(-config.PIIRetentionPeriod), *dryRun,
	)
	totalErrors := len(piiResult.Errors) + len(cleanupResult.Errors)
	if err != nil {
		totalErrors++
		logger.Error("incoming request retention failed", "error", err)
	} else {
		logger.Info("incoming request retention complete", "records", requestCount, "dry_run", *dryRun)
	}
	linkCount, err := repository.NewOperatorRepository(pool).PurgeExpiredTelegramLinks(ctx, *dryRun)
	if err != nil {
		totalErrors++
		logger.Error("expired Telegram link cleanup failed")
	} else {
		logger.Info("expired Telegram link cleanup complete", "records", linkCount, "dry_run", *dryRun)
	}
	if totalErrors > 0 {
		logger.Warn("retention run completed with errors", "total_errors", totalErrors)
		os.Exit(1)
	}

	logger.Info("retention run completed successfully")
}
