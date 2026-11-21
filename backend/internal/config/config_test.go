package config

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestLoadAPIValidatesWithoutEchoingSecrets(t *testing.T) {
	valid := map[string]string{
		"DATABASE_URL": "postgres://db.example/app", "PHOTOS_DIR": "/photos", "VOICE_DIR": "/voice",
		"PII_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"BOT_INTERNAL_URL":   "http://bot:9090", "REDIS_URL": "redis://redis:6379", "REDIS_PASSWORD": "fixture",
		"BACKEND_PORT": "", "TRUSTED_PROXY_IP": "", "ENABLE_SWAGGER": "",
	}
	for k, v := range valid {
		t.Setenv(k, v)
	}
	cfg, err := LoadAPI()
	if err != nil || cfg.Port != "8080" || cfg.Redis.Password != "fixture" {
		t.Fatal("valid configuration rejected")
	}
	for _, name := range []string{"PHOTOS_DIR", "VOICE_DIR", "PII_ENCRYPTION_KEY", "BOT_INTERNAL_URL", "REDIS_URL", "DATABASE_URL", "BACKEND_PORT", "TRUSTED_PROXY_IP", "ENABLE_SWAGGER"} {
		t.Run(name, func(t *testing.T) {
			value := "fixture-secret"
			if name == "PHOTOS_DIR" || name == "VOICE_DIR" {
				value = ""
			}
			t.Setenv(name, value)
			_, err := LoadAPI()
			if err == nil || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatal("expected a diagnostic naming only the setting")
			}
		})
	}
}

func TestDatabaseURLFromComposeSettings(t *testing.T) {
	for k, v := range map[string]string{"DATABASE_URL": "", "PGHOST": "postgres", "POSTGRES_USER": "app", "POSTGRES_PASSWORD": "fixture:@/?#%", "POSTGRES_DB": "app"} {
		t.Setenv(k, v)
	}
	raw, err := DatabaseURL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal("DSN cannot be parsed")
	}
	password, _ := parsed.User.Password()
	if password != "fixture:@/?#%" {
		t.Fatal("DSN did not preserve password")
	}
	t.Setenv("POSTGRES_PASSWORD", "")
	if _, err := DatabaseURL(); err == nil || !strings.Contains(err.Error(), "POSTGRES_PASSWORD") {
		t.Fatal("missing credentials were accepted")
	}
}
