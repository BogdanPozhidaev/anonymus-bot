package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOIDC(t *testing.T) {
	secret := "isolated-oidc-config-fixture"
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "client-secret")
	mapPath := filepath.Join(dir, "operators.json")
	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mapPath, []byte(`[{"subject":"fixture-subject","operator_id":42}]`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OIDC_ISSUER_URL", "https://identity.example.invalid/sso")
	t.Setenv("OIDC_REDIRECT_URL", "https://panel.example.invalid"+OIDCCallbackPath)
	t.Setenv("OIDC_CLIENT_ID", "fixture-panel")
	t.Setenv("OIDC_CLIENT_SECRET_FILE", secretPath)
	t.Setenv("OIDC_OPERATOR_MAP_FILE", mapPath)

	settings, err := loadOIDC()
	if err != nil || settings == nil || settings.ClientSecret != secret || settings.Operators["fixture-subject"] != 42 {
		t.Fatalf("valid settings rejected: %v", err)
	}

	t.Run("optional", func(t *testing.T) {
		t.Setenv("OIDC_ISSUER_URL", "")
		t.Setenv("OIDC_CLIENT_SECRET_FILE", "")
		got, err := loadOIDC()
		if got != nil || err != nil {
			t.Fatal("disabled OIDC required configuration")
		}
	})
	for _, tc := range []struct{ key, value string }{
		{"OIDC_ISSUER_URL", "http://identity.example.invalid"},
		{"OIDC_ISSUER_URL", "https://" + secret + "@identity.example.invalid"},
		{"OIDC_REDIRECT_URL", "https://panel.example.invalid" + OIDCCallbackPath + "?code=" + secret},
		{"OIDC_REDIRECT_URL", "https://panel.example.invalid" + OIDCCallbackPath + "/extra"},
		{"OIDC_CLIENT_ID", ""},
		{"OIDC_CLIENT_SECRET_FILE", filepath.Join(dir, secret+"-missing")},
		{"OIDC_OPERATOR_MAP_FILE", filepath.Join(dir, secret+"-missing")},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			got, err := loadOIDC()
			if got != nil || err == nil || !strings.Contains(err.Error(), tc.key) || strings.Contains(err.Error(), secret) {
				t.Fatalf("invalid settings were accepted or error was unsafe for %s", tc.key)
			}
		})
	}
	for _, operators := range []string{
		`[{"subject":"a","operator_id":1},{"subject":"a","operator_id":2}]`,
		`[{"subject":"a","operator_id":1},{"subject":"b","operator_id":1}]`,
		`[{"subject":"a","operator_id":0}]`,
	} {
		if err := os.WriteFile(mapPath, []byte(operators), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := loadOIDC(); got != nil || err == nil {
			t.Fatal("invalid operator map was accepted")
		}
	}
}
