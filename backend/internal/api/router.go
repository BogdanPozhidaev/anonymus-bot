package api

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
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
	moderationService   *moderation.Service
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
	moderationService *moderation.Service,
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
		moderationService:   moderationService,
	}
}

func (s *Server) pingDB(ctx context.Context) error {
	return s.dbPool.Ping(ctx)
}

func (s *Server) pingRedis(ctx context.Context) error {
	return s.redisClient.Ping(ctx).Err()
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", s.handleHealth)

	internal := r.Group("/internal")
	{
		internal.POST("/sessions/:id/bind", s.handleBindSession)
		internal.POST("/incoming-requests", s.handleCreateIncomingRequest)
		internal.GET("/users/by-telegram/:telegramID", s.handleGetUserByTelegramID)
		internal.PATCH("/users/by-telegram/:telegramID/language", s.handleUpdateUserLanguage)
		internal.GET("/operators/by-telegram/:telegramID", s.handleGetOperatorByTelegramID)
		internal.POST("/messages/relay", s.handleRelayMessage)
	}
}
