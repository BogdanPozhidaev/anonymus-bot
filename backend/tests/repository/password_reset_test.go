package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

func TestAdministratorPasswordResetRequiresRotationAndPreservesTOTP(t *testing.T) {
	ctx := context.Background()
	pool, redis := setupTestDB(t), setupThrottleRedis(t)
	operators := repository.NewOperatorRepository(pool)
	audit := repository.NewAuditLogRepository(pool)
	sessions := auth.NewSessionService(redis)
	pending := auth.NewPendingAuthService(redis, testEncryptor(t))
	const oldPassword = "synthetic-previous-password"
	hash, err := auth.HashPassword(oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	create := func(login, role, status string) models.Operator {
		t.Helper()
		op := models.Operator{Name: login, Login: login, Role: role, Status: status, Language: "en", PasswordHash: &hash}
		if err := operators.Create(ctx, &op); err != nil {
			t.Fatal(err)
		}
		return op
	}
	admin := create("fixture-reset-admin", "admin", "active")
	target := create("fixture-reset-target", "operator", "active")
	suspended := create("fixture-reset-suspended", "operator", "suspended")
	secret, _, err := auth.GenerateTOTPSecret(target.Login, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := pending.EncodeTOTP(secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := operators.RecordVerifiedLogin(ctx, &target, &encrypted, false); err != nil {
		t.Fatal(err)
	}
	issue := func(op models.Operator) string {
		t.Helper()
		token, err := sessions.Create(ctx, auth.SessionData{OperatorID: op.ID, AuthVersion: op.AuthVersion, Role: op.Role})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	adminToken, oldToken := issue(admin), issue(target)
	oldChallenge, err := pending.CreatePending(ctx, target.ID, target.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, repository.NewSessionRepository(pool), nil,
		operators, nil, audit, nil, sessions, pending, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	call := func(method, path, token string, body map[string]string, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, strings.NewReader(string(payload)))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s status=%d, want %d", path, res.Code, want)
		}
		return res
	}
	reset := func(id int64, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		return call("POST", fmt.Sprintf("/api/operators/%d/password/reset", id), token, nil, want)
	}
	reset(target.ID, "", 401)
	reset(target.ID, oldToken, 403)
	reset(admin.ID, adminToken, 403)
	reset(suspended.ID, adminToken, 409)
	reset(0, adminToken, 400)
	reset(9999, adminToken, 404)

	res := reset(target.ID, adminToken, 200)
	var issued struct {
		Password string `json:"temporary_password"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &issued); err != nil || auth.ValidateNewPassword(issued.Password) != nil ||
		res.Header().Get("Cache-Control") != "no-store" || issued.Password == oldPassword {
		t.Fatal("missing fresh temporary password or cache control")
	}
	stored, err := operators.GetByID(ctx, target.ID)
	if err != nil || stored.TOTPSecret == nil || *stored.TOTPSecret != encrypted || stored.PasswordHash == nil ||
		*stored.PasswordHash == issued.Password || !auth.VerifyPassword(*stored.PasswordHash, issued.Password) ||
		stored.AuthVersion != target.AuthVersion+1 || !stored.MustChangePassword || stored.Status != target.Status {
		t.Fatal("password reset did not atomically rotate credentials and preserve TOTP")
	}
	if applied, err := operators.ResetPassword(ctx, admin.ID, admin.AuthVersion, target.ID, target.AuthVersion, hash); err != nil || applied {
		t.Fatal("stale reset replaced the new password")
	}
	if applied, err := operators.ResetPassword(ctx, admin.ID, admin.AuthVersion+1, target.ID, stored.AuthVersion, hash); err != nil || applied {
		t.Fatal("a stale administrator session reset the password")
	}
	list := call("GET", "/api/operators", adminToken, nil, 200)
	if strings.Contains(list.Body.String(), issued.Password) {
		t.Fatal("operator list exposed the temporary password")
	}
	action := "operator_password_reset"
	entries, err := audit.List(ctx, repository.AuditLogFilter{Action: &action})
	if err != nil || len(entries) != 1 || entries[0].TargetID == nil || *entries[0].TargetID != target.ID ||
		entries[0].ActorID == nil || *entries[0].ActorID != admin.ID || strings.Contains(string(entries[0].Payload), issued.Password) {
		t.Fatal("unexpected administrator reset audit")
	}
	call("GET", "/api/sessions", oldToken, nil, 401)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/auth/totp/verify", "", map[string]string{"pending_token": oldChallenge, "code": code}, 401)
	call("POST", "/api/auth/login", "", map[string]string{"login": target.Login, "password": oldPassword}, 401)
	login := call("POST", "/api/auth/login", "", map[string]string{"login": target.Login, "password": issued.Password}, 200)
	var challenge struct {
		Token  string `json:"pending_token"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil || challenge.Token == "" || challenge.Status != "totp_required" {
		t.Fatal("temporary password did not require the existing TOTP")
	}
	verified := call("POST", "/api/auth/totp/verify", "", map[string]string{"pending_token": challenge.Token, "code": code}, 200)
	var limited struct {
		Token      string `json:"session_token"`
		MustChange bool   `json:"password_change_required"`
	}
	if err := json.Unmarshal(verified.Body.Bytes(), &limited); err != nil || !limited.MustChange || limited.Token == "" {
		t.Fatal("temporary password yielded unrestricted access")
	}
	call("GET", "/api/sessions", limited.Token, nil, 401)
	call("POST", "/api/auth/password", limited.Token, map[string]string{
		"current_password": issued.Password, "new_password": "synthetic-permanent-replacement",
	}, 204)
}
