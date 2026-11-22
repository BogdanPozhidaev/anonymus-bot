package config

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestLoadAPIValidatesWithoutEchoingSecrets(t *testing.T) {
	valid := map[string]string{
		"DATABASE_URL": "postgres://db.example/app", "PHOTOS_DIR": "/photos", "VOICE_DIR": "/voice",
		"PII_ENCRYPTION_KEY":  base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"PII_ENCRYPTION_KEYS": "", "PII_ENCRYPTION_ACTIVE_VERSION": "",
		"BOT_INTERNAL_URL": "http://bot:9090", "REDIS_URL": "redis://redis:6379", "REDIS_PASSWORD": "fixture",
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

func TestLoadEncryptionKeyring(t *testing.T) {
	original := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	next := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	t.Setenv("PII_ENCRYPTION_KEY", original)
	t.Setenv("PII_ENCRYPTION_KEYS", "2:"+next)
	t.Setenv("PII_ENCRYPTION_ACTIVE_VERSION", "2")
	both, err := LoadEncryption()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PII_ENCRYPTION_KEY", "")
	current, err := LoadEncryption()
	if err != nil || !bytes.Equal(current.AuditKey, both.AuditKey) {
		t.Fatalf("loading only the active key failed: %v", err)
	}
	for _, tc := range []struct{ name, value string }{
		{"PII_ENCRYPTION_KEY", "fixture-secret"},
		{"PII_ENCRYPTION_KEYS", "2:fixture-secret"},
		{"PII_ENCRYPTION_KEYS", "2:" + next + ",2:" + next},
		{"PII_ENCRYPTION_KEYS", "2:" + next + ",3:" + next},
		{"PII_ENCRYPTION_KEYS", "1:" + next},
		{"PII_ENCRYPTION_ACTIVE_VERSION", "03"},
		{"PII_ENCRYPTION_ACTIVE_VERSION", "3"},
		{"PII_ENCRYPTION_ACTIVE_VERSION", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			_, err := LoadEncryption()
			if err == nil || !strings.Contains(err.Error(), tc.name) || strings.Contains(err.Error(), tc.value) && len(tc.value) > 2 {
				t.Fatalf("expected sanitized error for %s", tc.name)
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
