package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/db"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Подключение к Postgres
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Error("DATABASE_URL is not set")
		os.Exit(1)
	}

	dbPool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	logger.Info("connected to postgres")

	// Подключение к Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		logger.Error("REDIS_URL is not set")
		os.Exit(1)
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error("failed to parse redis url", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(opt)
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Error("failed to close redis client", "error", err)
		}
	}()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("failed to ping redis", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to redis")

	// HTTP сервер
	encryptorKey, err := base64.StdEncoding.DecodeString(os.Getenv("PII_ENCRYPTION_KEY"))
	if err != nil {
		logger.Error("failed to decode PII_ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}
	encryptor, err := crypto.NewEncryptor(encryptorKey)
	if err != nil {
		logger.Error("failed to create encryptor", "error", err)
		os.Exit(1)
	}

	userRepo := repository.NewUserRepository(dbPool, encryptor)
	sessionRepo := repository.NewSessionRepository(dbPool)
	incomingRequestRepo := repository.NewIncomingRequestRepository(dbPool)

	operatorRepo := repository.NewOperatorRepository(dbPool)
	messageRepo := repository.NewMessageRepository(dbPool)
	moderationService := moderation.NewService(redisClient)
	sessionService := auth.NewSessionService(redisClient)
	pendingAuthService := auth.NewPendingAuthService(redisClient)
	auditLogRepo := repository.NewAuditLogRepository(dbPool)

	server := api.NewServer(
		dbPool, redisClient, logger,
		userRepo, sessionRepo, incomingRequestRepo, operatorRepo, messageRepo, auditLogRepo,
		moderationService, sessionService, pendingAuthService,
	)
	r := gin.Default()
	server.RegisterRoutes(r)

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("starting server", "port", port)
	if err := r.Run(":" + port); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
