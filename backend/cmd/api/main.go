package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/config"
	"github.com/fastcheck/anonymus_bot/backend/internal/db"
	"github.com/fastcheck/anonymus_bot/backend/internal/delivery"
	"github.com/fastcheck/anonymus_bot/backend/internal/metrics"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// @title           Anonymus Bot Backend API
// @version         1.0
// @description     API for the operator panel and internal bot-to-backend communication
// @contact.name    API Support
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter "Bearer <session_token>"

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

func isInternalRequest(c *gin.Context) bool {
	path := c.Request.URL.Path
	return path == "/internal" || strings.HasPrefix(path, "/internal/")
}

func newAPIRouter(logger *slog.Logger, monitor *metrics.Metrics) *gin.Engine {
	r := gin.New()
	if monitor != nil {
		r.Use(monitor.Middleware)
	}
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{Skip: func(c *gin.Context) bool {
		return isInternalRequest(c) || c.Request.URL.Path == config.OIDCCallbackPath
	}}))
	r.Use(func(c *gin.Context) {
		c.Next()
		if isInternalRequest(c) {
			logger.Info("internal API request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status())
		}
	})
	r.Use(gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, _ any) {
		logger.Error("API handler panicked", "method", c.Request.Method, "route", c.FullPath())
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	return r
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil && (ctx.Err() == nil || !errors.Is(err, context.Canceled)) {
		logger.Error("backend stopped with an error", "error", err)
		os.Exit(1)
	}
	logger.Info("backend stopped")
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.LoadAPI()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	internalToken, err := readInternalAPIToken("INTERNAL_API_TOKEN_FILE")
	if err != nil {
		return fmt.Errorf("failed to load internal API token: %w", err)
	}
	botToken, err := readInternalAPIToken("BOT_INTERNAL_TOKEN_FILE")
	if err != nil {
		return fmt.Errorf("failed to load bot internal API token: %w", err)
	}
	var metricsToken string
	if os.Getenv("METRICS_TOKEN_FILE") != "" {
		metricsToken, err = readInternalAPIToken("METRICS_TOKEN_FILE")
		if err != nil {
			return fmt.Errorf("failed to load metrics token: %w", err)
		}
		if metricsToken == internalToken || metricsToken == botToken {
			return errors.New("METRICS_TOKEN_FILE must use a separate credentials value")
		}
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	dbPool, err := db.NewPool(startupCtx, cfg.DatabaseURL)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("cannot connect to PostgreSQL, check database settings and availability")
	}
	defer dbPool.Close()
	redisClient := redis.NewClient(cfg.Redis)
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.Ping(startupCtx).Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("cannot connect to Redis, check Redis settings and availability")
	}
	cancel()
	userRepo := repository.NewUserRepository(dbPool, cfg.Encryption.PII)
	sessionRepo := repository.NewSessionRepository(dbPool)
	incomingRequestRepo := repository.NewIncomingRequestRepository(dbPool)

	operatorRepo := repository.NewOperatorRepository(dbPool)
	messageRepo := repository.NewMessageRepository(dbPool)
	moderationService := moderation.NewService(redisClient, cfg.Moderation)
	sessionService := auth.NewSessionService(redisClient)
	pendingAuthService := auth.NewPendingAuthService(redisClient, cfg.Encryption.TOTP)
	auditLogRepo := repository.NewAuditLogRepository(dbPool)
	botClient := botclient.New(cfg.BotURL, botToken)

	server := api.NewServer(
		dbPool, redisClient, logger,
		userRepo, sessionRepo, incomingRequestRepo, operatorRepo, messageRepo, auditLogRepo,
		moderationService, sessionService, pendingAuthService, botClient,
		cfg.PhotosDir, cfg.VoiceDir,
	)
	var monitor *metrics.Metrics
	var observeDelivery func(bool, bool)
	if metricsToken != "" {
		monitor = metrics.New(messageRepo.OutboxBacklog)
		observeDelivery = monitor.RecordDeliveryAttempt
	}
	r := newAPIRouter(logger, monitor)
	if err := configureClientIP(r, cfg.TrustedProxyIP); err != nil {
		return fmt.Errorf("invalid trusted proxy configuration: %w", err)
	}
	if cfg.OIDC != nil {
		server.EnableOIDC(auth.NewOIDC(*cfg.OIDC, &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}, redisClient))
	}
	server.RegisterRoutes(r, internalToken)
	if monitor != nil {
		r.GET(metrics.Path, gin.WrapH(monitor.Handler(metricsToken)))
	}
	if cfg.Swagger {
		api.RegisterSwagger(r)
	}

	httpServer := newHTTPServer(cfg.Port, r)
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	worker := delivery.New(messageRepo, botClient, logger, observeDelivery)
	logger.Info("starting server", "port", cfg.Port)
	return serve(ctx, httpServer, listener, worker.Run, shutdownTimeout)
}
