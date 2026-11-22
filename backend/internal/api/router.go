package api

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/fastcheck/anonymus_bot/backend/docs/swagger"
)

type Server struct {
	dbPool              *pgxpool.Pool
	redisClient         *redis.Client
	logger              *slog.Logger
	userRepo            *repository.UserRepository
	sessionRepo         *repository.SessionRepository
	incomingRequestRepo *repository.IncomingRequestRepository
	operatorRepo        *repository.OperatorRepository
	messageRepo         *repository.MessageRepository
	auditLogRepo        *repository.AuditLogRepository
	moderationService   *moderation.Service
	sessionService      *auth.SessionService
	pendingAuthService  *auth.PendingAuthService
	botClient           *botclient.Client
	photosDir           string
	voiceDir            string
}

func NewServer(
	dbPool *pgxpool.Pool,
	redisClient *redis.Client,
	logger *slog.Logger,
	userRepo *repository.UserRepository,
	sessionRepo *repository.SessionRepository,
	incomingRequestRepo *repository.IncomingRequestRepository,
	operatorRepo *repository.OperatorRepository,
	messageRepo *repository.MessageRepository,
	auditLogRepo *repository.AuditLogRepository,
	moderationService *moderation.Service,
	sessionService *auth.SessionService,
	pendingAuthService *auth.PendingAuthService,
	botClient *botclient.Client,
	photosDir string,
	voiceDir string,
) *Server {
	return &Server{
		dbPool:              dbPool,
		redisClient:         redisClient,
		logger:              logger,
		userRepo:            userRepo,
		sessionRepo:         sessionRepo,
		incomingRequestRepo: incomingRequestRepo,
		operatorRepo:        operatorRepo,
		messageRepo:         messageRepo,
		auditLogRepo:        auditLogRepo,
		moderationService:   moderationService,
		sessionService:      sessionService,
		pendingAuthService:  pendingAuthService,
		botClient:           botClient,
		photosDir:           photosDir,
		voiceDir:            voiceDir,
	}
}

func (s *Server) pingDB(ctx context.Context) error {
	return s.dbPool.Ping(ctx)
}

func (s *Server) pingRedis(ctx context.Context) error {
	return s.redisClient.Ping(ctx).Err()
}

func (s *Server) RegisterRoutes(r *gin.Engine, internalToken string) {
	r.GET("/health", s.handleHealth)
	r.GET("/api/health", s.handleHealth)

	internal := r.Group("/internal", requireInternalToken(internalToken), limitRequestBody)
	{
		internal.POST("/sessions/bind", s.handleBindSession)
		internal.POST("/incoming-requests", s.handleCreateIncomingRequest)
		internal.GET("/users/by-telegram/:telegramID", s.handleGetUserByTelegramID)
		internal.PATCH("/users/by-telegram/:telegramID/language", s.handleUpdateUserLanguage)
		internal.GET("/operators/by-telegram/:telegramID", s.handleGetOperatorByTelegramID)
		internal.POST("/operators/telegram-link/claim", s.handleClaimTelegramLink)
		internal.POST("/messages/relay", s.handleRelayMessage)
		internal.GET("/users/by-telegram/:telegramID/sessions", s.handleListUserSessions)
		internal.PATCH("/users/by-telegram/:telegramID/active-session", s.handleSwitchActiveSession)
		internal.POST("/sessions/stop", s.handleStopSession)
		internal.POST("/incoming-requests/:id/mark-processed", s.handleMarkIncomingRequestProcessedInternal)
		internal.GET("/operators/by-telegram/:telegramID/sessions", s.handleListOperatorSessionsInternal)
		internal.GET("/incoming-requests", s.handleListIncomingRequestsInternal)
	}

	apiGroup := r.Group("/api", limitRequestBody)
	{
		// Публичные эндпоинты аутентификации — без requireAuth
		apiGroup.POST("/auth/login", s.handleLogin)
		apiGroup.POST("/auth/totp/verify", s.handleVerifyTOTP)
		apiGroup.POST("/auth/totp/recover", s.handleRecoverTOTP)

		// Всё остальное в /api требует валидной сессии
		credentials := apiGroup.Group("", s.requireAuth(true))
		credentials.POST("/auth/logout", s.handleLogout)
		credentials.POST("/auth/password", s.handleChangePassword)

		authenticated := apiGroup.Group("", s.requireAuth(false))
		{
			authenticated.POST("/auth/totp/recovery-codes", s.handleIssueRecoveryCodes)
			authenticated.GET("/account/telegram-link", s.handleTelegramLinkStatus)
			authenticated.POST("/account/telegram-link", s.handleBeginTelegramLink)
			authenticated.POST("/account/telegram-link/confirm", s.handleConfirmTelegramLink)
			authenticated.GET("/sessions", s.handleListSessions)
			authenticated.GET("/sessions/:id", s.handleGetSession)
			authenticated.POST("/sessions", s.handleCreateSession)
			authenticated.POST("/sessions/:id/invites", s.handleIssueInvite)
			authenticated.DELETE("/sessions/:id/invites/:role", s.handleRevokeInvite)
			authenticated.PATCH("/sessions/:id/status", s.handleUpdateSessionStatus)
			authenticated.PATCH("/sessions/:id/owner", s.handleAssignSessionOwner)
			authenticated.GET("/sessions/:id/messages", s.handleListSessionMessages)
			authenticated.GET("/incoming-requests", s.handleListIncomingRequests)
			authenticated.PATCH("/incoming-requests/:id/status", s.handleUpdateIncomingRequestStatus)
			authenticated.POST("/sessions/:id/send-as", s.handleSendMessageAsOperator)
			authenticated.GET("/messages/:id/media", s.handleGetMessageMedia)

			adminOnly := authenticated.Group("")
			adminOnly.Use(s.requireAdmin())
			{
				adminOnly.GET("/audit-log", s.handleListAuditLog)
				adminOnly.GET("/operators", s.handleListOperators)
				adminOnly.POST("/operators", s.handleCreateOperator)
				adminOnly.PATCH("/operators/:id", s.handleUpdateOperator)
				adminOnly.POST("/operators/:id/password/reset", s.handleResetOperatorPassword)
			}

		}
	}
}

func RegisterSwagger(r *gin.Engine) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
