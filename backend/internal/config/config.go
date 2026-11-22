package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/redis/go-redis/v9"
)

func DatabaseURL() (string, error) {
	if raw := os.Getenv("DATABASE_URL"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" {
			return "", fmt.Errorf("DATABASE_URL must be a PostgreSQL URL with a host and database")
		}
		if u.Port() != "" && !validPort(u.Port()) {
			return "", fmt.Errorf("DATABASE_URL contains an invalid port")
		}
		return raw, nil
	}
	for _, name := range []string{"PGHOST", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return "", fmt.Errorf("%s is required when DATABASE_URL is unset", name)
		}
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(os.Getenv("PGHOST"), "5432"), User: url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")), Path: "/" + os.Getenv("POSTGRES_DB"), RawQuery: "sslmode=disable"}
	return u.String(), nil
}

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func MediaSettings() (photos, voice string, err error) {
	photos, voice = os.Getenv("PHOTOS_DIR"), os.Getenv("VOICE_DIR")
	if strings.TrimSpace(photos) == "" {
		return "", "", fmt.Errorf("PHOTOS_DIR is required")
	}
	if strings.TrimSpace(voice) == "" {
		return "", "", fmt.Errorf("VOICE_DIR is required")
	}
	return photos, voice, nil
}

type API struct {
	DatabaseURL, PhotosDir, VoiceDir, BotURL, Port, TrustedProxyIP string
	Encryption                                                     Encryption
	Redis                                                          *redis.Options
	Swagger                                                        bool
	Moderation                                                     moderation.Config
	OIDC                                                           *OIDC
}

func LoadAPI() (API, error) {
	var cfg API
	var err error
	cfg.DatabaseURL, err = DatabaseURL()
	if err != nil {
		return cfg, err
	}
	cfg.PhotosDir, cfg.VoiceDir, err = MediaSettings()
	if err != nil {
		return cfg, err
	}
	cfg.Encryption, err = LoadEncryption()
	if err != nil {
		return cfg, err
	}
	cfg.BotURL = os.Getenv("BOT_INTERNAL_URL")
	endpoint, err := url.Parse(cfg.BotURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return cfg, fmt.Errorf("BOT_INTERNAL_URL must be an HTTP(S) URL without credentials")
	}
	rawRedis := os.Getenv("REDIS_URL")
	redisURL, err := url.Parse(rawRedis)
	if err != nil || ((redisURL.Scheme == "redis" || redisURL.Scheme == "rediss") && redisURL.Hostname() == "") {
		return cfg, fmt.Errorf("REDIS_URL must be a valid Redis connection URL")
	}
	cfg.Redis, err = redis.ParseURL(rawRedis)
	if err != nil {
		return cfg, fmt.Errorf("REDIS_URL must be a valid Redis connection URL")
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		cfg.Redis.Password = password
	}
	cfg.Port = os.Getenv("BACKEND_PORT")
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if !validPort(cfg.Port) {
		return cfg, fmt.Errorf("BACKEND_PORT must be between 1 and 65535")
	}
	cfg.TrustedProxyIP = os.Getenv("TRUSTED_PROXY_IP")
	if cfg.TrustedProxyIP != "" && net.ParseIP(cfg.TrustedProxyIP) == nil {
		return cfg, fmt.Errorf("TRUSTED_PROXY_IP must be a single IP address")
	}
	switch os.Getenv("ENABLE_SWAGGER") {
	case "true":
		cfg.Swagger = true
	case "", "false":
	default:
		return cfg, fmt.Errorf("ENABLE_SWAGGER must be true or false")
	}
	cfg.Moderation, err = moderation.ParseConfig(os.Getenv("MODERATION_DISABLED_RULES"), os.Getenv("MODERATION_AUTO_PAUSE"))
	if err != nil {
		return cfg, err
	}
	cfg.OIDC, err = loadOIDC()
	return cfg, err
}
