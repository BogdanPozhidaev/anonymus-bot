package repository_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

func TestPasswordChangeRevokesSessionsAndPendingChallenges(t *testing.T) {
	ctx := context.Background()
	pool, redis := setupTestDB(t), setupThrottleRedis(t)
	operators := repository.NewOperatorRepository(pool)
	const oldPassword = "fixture-old-password"
	const newPassword = "fixture-new-password"
	hash, err := auth.HashPassword(oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	operator := models.Operator{Name: "Fixture", Login: "fixture-password-change", Role: "admin", Status: "active", Language: "en", PasswordHash: &hash}
	if err := operators.Create(ctx, &operator); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessionService(redis)
	pending := auth.NewPendingAuthService(redis, testEncryptor(t))
	secret, _, err := auth.GenerateTOTPSecret(operator.Login, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := pending.EncodeTOTP(secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := operators.RecordVerifiedLogin(ctx, &operator, &encrypted, false); err != nil {
		t.Fatal(err)
	}
	issue := func(version int64) string {
		t.Helper()
		token, err := sessions.Create(ctx, auth.SessionData{OperatorID: operator.ID, AuthVersion: version, Role: operator.Role})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	first, second := issue(operator.AuthVersion), issue(operator.AuthVersion)
	challenge, err := pending.CreatePending(ctx, operator.ID, operator.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	audit := repository.NewAuditLogRepository(pool)
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, operators,
		nil, audit, nil, sessions, pending, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	call := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s status: %d, want %d", path, res.Code, want)
		}
		return res
	}
	const path = "/api/auth/password"
	good := `{"current_password":"` + oldPassword + `","new_password":"` + newPassword + `"}`
	wrong := `{"current_password":"fixture-incorrect","new_password":"` + newPassword + `"}`
	call("POST", path, "", good, http.StatusUnauthorized)
	call("POST", path, first, `{"current_password":"fixture-old-password","new_password":"short"}`, http.StatusBadRequest)
	call("POST", path, first, wrong, http.StatusForbidden)
	call("POST", path, first, good, http.StatusNoContent)
	for _, token := range []string{first, second} {
		call("GET", "/api/audit-log", token, "", http.StatusUnauthorized)
	}
	call("POST", "/api/auth/totp/verify", "", `{"pending_token":"`+challenge+`","code":"123456"}`, http.StatusUnauthorized)
	current, err := operators.GetByID(ctx, operator.ID)
	if err != nil || current.PasswordHash == nil || !auth.VerifyPassword(*current.PasswordHash, newPassword) ||
		auth.VerifyPassword(*current.PasswordHash, oldPassword) || current.AuthVersion != operator.AuthVersion+1 {
		t.Fatal("password hash or authentication version did not change correctly")
	}
	if changed, err := operators.ChangePassword(ctx, operator.ID, operator.AuthVersion, hash); err != nil || changed {
		t.Fatal("stale write replaced the new password")
	}
	entries, err := audit.List(ctx, repository.AuditLogFilter{})
	if err != nil || len(entries) != 1 || entries[0].Action != "operator_password_changed" ||
		strings.Contains(string(entries[0].Payload), "fixture-") {
		t.Fatal("unexpected password change audit")
	}
	loginBody := func(password string) string {
		return `{"login":"fixture-password-change","password":"` + password + `"}`
	}
	call("POST", "/api/auth/login", "", loginBody(oldPassword), http.StatusUnauthorized)
	next := call("POST", "/api/auth/login", "", loginBody(newPassword), http.StatusOK)
	var login struct {
		Status       string `json:"status"`
		PendingToken string `json:"pending_token"`
	}
	if err := json.Unmarshal(next.Body.Bytes(), &login); err != nil || login.Status != "totp_required" || login.PendingToken == "" {
		t.Fatal("new password did not lead to the existing TOTP challenge")
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	verified := call("POST", "/api/auth/totp/verify", "", `{"pending_token":"`+login.PendingToken+`","code":"`+code+`"}`, http.StatusOK)
	var approved struct {
		Token string `json:"session_token"`
	}
	if err := json.Unmarshal(verified.Body.Bytes(), &approved); err != nil || approved.Token == "" {
		t.Fatal("existing authenticator could not complete sign-in after password change")
	}
	currentToken := approved.Token
	for range 3 {
		call("POST", path, currentToken, wrong, http.StatusForbidden)
	}
	if res := call("POST", path, currentToken, wrong, http.StatusTooManyRequests); res.Header().Get("Retry-After") == "" {
		t.Fatal("password change throttle omitted Retry-After")
	}
}
