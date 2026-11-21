package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/config"
	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/db"
	"github.com/fastcheck/anonymus_bot/backend/internal/delivery"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// @title           Anonymus Bot Backend API
// @version         1.0
// @description     API для веб-панели и внутреннего общения bot↔backend
// @contact.name    API Support
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Введите "Bearer <ваш_session_token>"

// readInternalAPIToken loads a hex-encoded 32-byte service token.
func readInternalAPIToken(envName string) (string, error) {
	data, err := os.ReadFile(os.Getenv(envName))
	if err != nil {
		return "", fmt.Errorf("cannot read %s", envName)
	}
	token := strings.TrimSpace(string(data))
	if len(token) != 64 {
		return "", fmt.Errorf("%s must contain a 64-character hex token", envName)
	}
	if _, err := hex.DecodeString(token); err != nil {
		return "", fmt.Errorf("%s must contain a 64-character hex token", envName)
	}
	return token, nil
}

func configureClientIP(r *gin.Engine, proxyIP string) error {
	r.RemoteIPHeaders = []string{"X-Real-IP"}
	if proxyIP == "" {
		return r.SetTrustedProxies(nil)
	}
	ip := net.ParseIP(proxyIP)
	if ip == nil {
		return fmt.Errorf("TRUSTED_PROXY_IP must be a single IP address")
	}
	return r.SetTrustedProxies([]string{ip.String()})
}

func newHTTPServer(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadAPI()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	internalToken, err := readInternalAPIToken("INTERNAL_API_TOKEN_FILE")
	if err != nil {
		logger.Error("failed to load internal API token", "error", err)
		os.Exit(1)
	}
	botToken, err := readInternalAPIToken("BOT_INTERNAL_TOKEN_FILE")
	if err != nil {
		logger.Error("failed to load bot internal API token", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Подключение к Postgres
	dbPool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot connect to PostgreSQL; check database settings and availability")
		os.Exit(1)
	}
	defer dbPool.Close()
	redisClient := redis.NewClient(cfg.Redis)
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("cannot connect to Redis; check Redis settings and availability")
		os.Exit(1)
	}
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		logger.Error("could not initialize PII encryption")
		os.Exit(1)
	}
	userRepo := repository.NewUserRepository(dbPool, encryptor)
	sessionRepo := repository.NewSessionRepository(dbPool)
	incomingRequestRepo := repository.NewIncomingRequestRepository(dbPool)

	operatorRepo := repository.NewOperatorRepository(dbPool)
	messageRepo := repository.NewMessageRepository(dbPool)
	moderationService := moderation.NewService(redisClient, cfg.Moderation)
	sessionService := auth.NewSessionService(redisClient)
	totpKey := hmac.New(sha256.New, cfg.EncryptionKey)
	_, _ = totpKey.Write([]byte("operator TOTP encryption v1"))
	totpEncryptor, err := crypto.NewEncryptor(totpKey.Sum(nil))
	if err != nil {
		logger.Error("could not initialize TOTP encryption")
		os.Exit(1)
	}
	pendingAuthService := auth.NewPendingAuthService(redisClient, totpEncryptor)
	auditLogRepo := repository.NewAuditLogRepository(dbPool)
	botClient := botclient.New(cfg.BotURL, botToken)

	server := api.NewServer(
		dbPool, redisClient, logger,
		userRepo, sessionRepo, incomingRequestRepo, operatorRepo, messageRepo, auditLogRepo,
		moderationService, sessionService, pendingAuthService, botClient,
		cfg.PhotosDir, cfg.VoiceDir,
	)
	go delivery.New(messageRepo, botClient, logger).Run(context.Background())
	r := gin.Default()
	if err := configureClientIP(r, cfg.TrustedProxyIP); err != nil {
		logger.Error("invalid trusted proxy configuration", "error", err)
		os.Exit(1)
	}
	server.RegisterRoutes(r, internalToken)
	if cfg.Swagger {
		api.RegisterSwagger(r)
	}

	port := cfg.Port

	logger.Info("starting server", "port", port)
	if err := newHTTPServer(port, r).ListenAndServe(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
