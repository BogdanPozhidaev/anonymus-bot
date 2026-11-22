package repository_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

func TestTOTPRecoveryRequiresPasswordAndNewFactor(t *testing.T) {
	ctx := context.Background()
	pool, redis := setupTestDB(t), setupThrottleRedis(t)
	operators := repository.NewOperatorRepository(pool)
	sessions := auth.NewSessionService(redis)
	pending := auth.NewPendingAuthService(redis, testEncryptor(t))
	const password = "synthetic-recovery-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	op := models.Operator{Name: "Recovery fixture", Login: "fixture-recovery", Role: "operator", Status: "active", Language: "en", PasswordHash: &hash}
	if err := operators.Create(ctx, &op); err != nil {
		t.Fatal(err)
	}
	oldSecret, _, err := auth.GenerateTOTPSecret(op.Login, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := pending.EncodeTOTP(oldSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := operators.RecordVerifiedLogin(ctx, &op, &encrypted, false); err != nil {
		t.Fatal(err)
	}
	oldSession, err := sessions.Create(ctx, auth.SessionData{OperatorID: op.ID, AuthVersion: op.AuthVersion, Role: op.Role})
	if err != nil {
		t.Fatal(err)
	}
	audit := repository.NewAuditLogRepository(pool)
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, repository.NewSessionRepository(pool), nil,
		operators, nil, audit, nil, sessions, pending, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	call := func(method, path, bearer string, payload map[string]string, want int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s returned %d, want %d", path, res.Code, want)
		}
		return res
	}
	generate := func(secret string) string {
		t.Helper()
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return code
	}
	const issuePath = "/api/auth/totp/recovery-codes"
	const recoverPath = "/api/auth/totp/recover"
	const verifyPath = "/api/auth/totp/verify"
	params := map[string]string{"password": password, "code": generate(oldSecret)}
	call("POST", issuePath, "", params, 401)
	call("POST", issuePath, oldSession, map[string]string{"password": "synthetic-incorrect", "code": params["code"]}, 403)
	call("POST", issuePath, oldSession, map[string]string{"password": password, "code": "invalid"}, 403)
	response := call("POST", issuePath, oldSession, params, 200)
	var issued struct {
		Codes []string `json:"codes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || len(issued.Codes) != 8 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("eight private recovery codes were not issued")
	}
	var digestCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM operator_recovery_codes WHERE operator_id=$1 AND octet_length(digest)=32", op.ID).Scan(&digestCount); err != nil || digestCount != len(issued.Codes) {
		t.Fatal("database did not store the recovery-code digests")
	}
	call("GET", "/api/sessions", oldSession, nil, 401)

	type challenge struct {
		Token  string `json:"pending_token"`
		Secret string `json:"totp_secret"`
	}
	login := func() challenge {
		t.Helper()
		var item challenge
		res := call("POST", "/api/auth/login", "", map[string]string{"login": op.Login, "password": password}, 200)
		if json.Unmarshal(res.Body.Bytes(), &item) != nil || item.Token == "" || item.Secret != "" {
			t.Fatal("login omitted a password challenge or revealed an enrolled secret")
		}
		return item
	}
	recover := func(token, code string, want int) *httptest.ResponseRecorder {
		t.Helper()
		return call("POST", recoverPath, "", map[string]string{"pending_token": token, "recovery_code": code}, want)
	}
	recover("invalid", issued.Codes[0], 401)
	recover(login().Token, "synthetic-incorrect", 401)
	before := login()
	started := recover(before.Token, strings.ToLower(issued.Codes[0]), 200)
	var enrollment challenge
	if json.Unmarshal(started.Body.Bytes(), &enrollment) != nil || enrollment.Token == "" || enrollment.Secret == "" ||
		enrollment.Secret == oldSecret || strings.Contains(started.Body.String(), "session_token") {
		t.Fatal("recovery did not provide a fresh enrollment challenge")
	}
	recover(login().Token, issued.Codes[0], 401)
	if got, err := operators.GetByID(ctx, op.ID); err != nil || got.TOTPSecret == nil || *got.TOTPSecret != encrypted {
		t.Fatal("unconfirmed recovery replaced the persistent TOTP")
	}
	// An already-issued normal challenge and a verified session must be revoked
	// when the new factor is confirmed, rather than when recovery starts.
	stale := login()
	oldVerified := call("POST", verifyPath, "", map[string]string{"pending_token": stale.Token, "code": generate(oldSecret)}, 200)
	var previous struct {
		Token string `json:"session_token"`
	}
	if json.Unmarshal(oldVerified.Body.Bytes(), &previous) != nil || previous.Token == "" {
		t.Fatal("old factor stopped working before recovery confirmation")
	}
	call("GET", "/api/sessions", enrollment.Token, nil, 401)
	call("POST", verifyPath, "", map[string]string{"pending_token": enrollment.Token, "code": "invalid"}, 401)
	stale = login()
	confirmed := call("POST", verifyPath, "", map[string]string{"pending_token": enrollment.Token, "code": generate(enrollment.Secret)}, 200)
	var full struct {
		Token string `json:"session_token"`
	}
	if json.Unmarshal(confirmed.Body.Bytes(), &full) != nil || full.Token == "" {
		t.Fatal("new authenticator did not yield a full session")
	}
	call("GET", "/api/sessions", full.Token, nil, 200)
	call("GET", "/api/sessions", previous.Token, nil, 401)
	call("POST", verifyPath, "", map[string]string{"pending_token": stale.Token, "code": generate(oldSecret)}, 401)
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM operator_recovery_codes WHERE operator_id=$1", op.ID).Scan(&digestCount); err != nil || digestCount != 0 {
		t.Fatal("unused recovery codes survived the factor replacement")
	}
	action := "operator_totp_recovered"
	entries, err := audit.List(ctx, repository.AuditLogFilter{Action: &action})
	if err != nil || len(entries) != 1 || string(entries[0].Payload) != "null" {
		t.Fatal("recovery confirmation was not recorded without secret material")
	}
}

func TestRecoveryCodeConsumptionIsAtomic(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	repo := repository.NewOperatorRepository(pool)
	op := models.Operator{Name: "Concurrent recovery", Login: "fixture-concurrent-recovery", Role: "operator", Status: "active", Language: "en"}
	if err := repo.Create(ctx, &op); err != nil {
		t.Fatal(err)
	}
	fixtureSecret := "fixture-secret-present"
	if err := repo.RecordVerifiedLogin(ctx, &op, &fixtureSecret, false); err != nil {
		t.Fatal(err)
	}
	_, digests := auth.GenerateRecoveryCodes()
	if changed, err := repo.ReplaceRecoveryCodes(ctx, op.ID, op.AuthVersion, digests); err != nil || !changed {
		t.Fatalf("could not issue fixture recovery codes: %v", err)
	}
	var successes atomic.Int32
	var group sync.WaitGroup
	for range 6 {
		group.Go(func() {
			used, err := repo.ConsumeRecoveryCode(ctx, op.ID, op.AuthVersion+1, digests[0])
			if err != nil {
				t.Error(err)
			}
			if used {
				successes.Add(1)
			}
		})
	}
	group.Wait()
	if successes.Load() != 1 {
		t.Fatalf("one code authorized %d attempts", successes.Load())
	}
	_, next := auth.GenerateRecoveryCodes()
	if changed, err := repo.ReplaceRecoveryCodes(ctx, op.ID, op.AuthVersion+1, next); err != nil || !changed {
		t.Fatalf("could not replace the recovery codes: %v", err)
	}
	if used, err := repo.ConsumeRecoveryCode(ctx, op.ID, op.AuthVersion+2, digests[1]); err != nil || used {
		t.Fatal("unused code from a superseded batch authorized recovery")
	}
	if used, err := repo.ConsumeRecoveryCode(ctx, op.ID, op.AuthVersion+2, next[0]); err != nil || !used {
		t.Fatalf("newly issued code was rejected: %v", err)
	}
}
