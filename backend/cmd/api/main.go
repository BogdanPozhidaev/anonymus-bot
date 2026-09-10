package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
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

	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		logger.Error("failed to create postgres pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		logger.Error("failed to ping postgres", "error", err)
		os.Exit(1)
	}
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
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		healthCtx, healthCancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer healthCancel()

		dbStatus := "ok"
		if err := dbPool.Ping(healthCtx); err != nil {
			dbStatus = "error: " + err.Error()
		}

		redisStatus := "ok"
		if err := redisClient.Ping(healthCtx).Err(); err != nil {
			redisStatus = "error: " + err.Error()
		}

		overallStatus := http.StatusOK
		if dbStatus != "ok" || redisStatus != "ok" {
			overallStatus = http.StatusServiceUnavailable
		}

		c.JSON(overallStatus, gin.H{
			"status":   "ok",
			"postgres": dbStatus,
			"redis":    redisStatus,
		})
	})

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
