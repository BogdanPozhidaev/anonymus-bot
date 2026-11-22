package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestTOTPEnrollmentRequiresConfirmationAndEncryptsSecret(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	client := setupThrottleRedis(t)
	operators := repository.NewOperatorRepository(pool)
	pending := auth.NewPendingAuthService(client, testEncryptor(t))
	const password = "synthetic-enrollment-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	operator := models.Operator{Name: "Enrollment fixture", Login: "fixture-enrollment", Role: "operator", Status: "invited", Language: "en", PasswordHash: &hash, MustChangePassword: true}
	if err := operators.Create(ctx, &operator); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.NewServer(pool, client, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, repository.NewSessionRepository(pool), nil, operators,
		nil, repository.NewAuditLogRepository(pool), nil, auth.NewSessionService(client), pending, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	call := func(method, route, token string, payload map[string]string, want int) []byte {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, route, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s returned %d, want %d", route, response.Code, want)
		}
		return response.Body.Bytes()
	}
	type challenge struct {
		Token  string `json:"pending_token"`
		Secret string `json:"totp_secret"`
		Status string `json:"status"`
	}
	login := func(password string) challenge {
		t.Helper()
		var result challenge
		if err := json.Unmarshal(call(http.MethodPost, "/api/auth/login", "", map[string]string{"login": operator.Login, "password": password}, http.StatusOK), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first, second := login(password), login(password)
	if first.Status != "totp_setup_required" || first.Secret == "" || first.Secret == second.Secret {
		t.Fatal("enrollment attempts did not receive independent secrets")
	}
	if stored, err := pending.Enrollment(ctx, first.Token); err != nil || strings.Contains(stored, first.Secret) {
		t.Fatal("temporary enrollment secret is missing or present in plaintext")
	}
	type issuedSession struct {
		Token      string `json:"session_token"`
		Restricted bool   `json:"password_change_required"`
	}
	verify := func(c challenge, code string, want int) issuedSession {
		t.Helper()
		result := call(http.MethodPost, "/api/auth/totp/verify", "", map[string]string{"pending_token": c.Token, "code": code}, want)
		var issued issuedSession
		if want == http.StatusOK && (json.Unmarshal(result, &issued) != nil || issued.Token == "") {
			t.Fatal("verified sign-in did not receive a session token")
		}
		return issued
	}
	verify(first, "invalid", http.StatusUnauthorized)
	get := func() *models.Operator {
		t.Helper()
		op, err := operators.GetByID(ctx, operator.ID)
		if err != nil {
			t.Fatal(err)
		}
		return op
	}
	if op := get(); op.TOTPSecret != nil || op.LastLoginAt != nil || op.Status != "invited" {
		t.Fatal("unconfirmed enrollment modified the operator")
	}
	generate := func(secret string) string {
		t.Helper()
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return code
	}
	limited := verify(first, generate(first.Secret), http.StatusOK)
	if !limited.Restricted {
		t.Fatal("temporary password received a full session")
	}
	call(http.MethodGet, "/api/sessions", limited.Token, nil, http.StatusUnauthorized)
	call(http.MethodGet, "/api/operators", limited.Token, nil, http.StatusUnauthorized)
	if ttl := client.TTL(ctx, "password_change_session:"+limited.Token).Val(); ttl <= 0 || ttl > 5*time.Minute {
		t.Fatalf("unexpected restricted session TTL: %s", ttl)
	}
	op := get()
	if op.Status != "active" || op.TOTPSecret == nil || op.LastLoginAt == nil || op.AuthVersion == operator.AuthVersion {
		t.Fatal("confirmed enrollment did not activate and timestamp the account")
	}
	secret, legacy, err := pending.DecodeTOTP(*op.TOTPSecret)
	if err != nil || legacy || secret != first.Secret || strings.Contains(*op.TOTPSecret, first.Secret) {
		t.Fatal("stored TOTP does not match the verified encrypted secret")
	}
	if _, err := pending.Enrollment(ctx, first.Token); !errors.Is(err, redis.Nil) {
		t.Fatal("completed enrollment still has temporary secret material")
	}
	verify(second, generate(second.Secret), http.StatusUnauthorized)
	ordinary := login(password)
	if ordinary.Status != "totp_required" || ordinary.Secret != "" {
		t.Fatal("returning operator was prompted to enroll or received a secret")
	}
	secondLimited := verify(ordinary, generate(first.Secret), http.StatusOK)
	if !secondLimited.Restricted {
		t.Fatal("repeated sign-in bypassed mandatory password change")
	}
	const replacement = "synthetic-replacement-password"
	change := map[string]string{"current_password": password, "new_password": replacement}
	call(http.MethodPost, "/api/auth/password", limited.Token, change, http.StatusNoContent)
	if got := get(); got.MustChangePassword {
		t.Fatal("changing the password did not clear the stored requirement")
	}
	for _, token := range []string{limited.Token, secondLimited.Token} {
		call(http.MethodPost, "/api/auth/password", token, change, http.StatusUnauthorized)
	}
	call(http.MethodPost, "/api/auth/login", "", map[string]string{"login": operator.Login, "password": password}, http.StatusUnauthorized)
	full := verify(login(replacement), generate(first.Secret), http.StatusOK)
	if full.Restricted {
		t.Fatal("new permanent password received a restricted session")
	}
	call(http.MethodGet, "/api/sessions", full.Token, nil, http.StatusOK)
}

func setupThrottleRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	instance, err := testcontainers.Run(ctx, "redis:7-alpine", testcontainers.WithExposedPorts("6379/tcp"), testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := instance.Terminate(ctx); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := instance.PortEndpoint(ctx, "6379/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Addr: endpoint})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestRedisAuthAttemptLimits(t *testing.T) {
	client := setupThrottleRedis(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for range 12 {
		wg.Go(func() {
			retry, err := auth.ReserveLoginAttempt(ctx, client, "fixture", "192.0.2.1")
			if err != nil {
				t.Error(err)
				return
			}
			if retry == 0 {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 5 {
		t.Fatalf("allowed %d concurrent attempts, want 5", allowed.Load())
	}
	for i := range 26 {
		retry, err := auth.ReserveLoginAttempt(ctx, client, string(rune('a'+i)), "192.0.2.1")
		if err != nil || (retry > 0) != (i == 25) {
			t.Fatalf("IP reservation %d: %v, %v", i, retry, err)
		}
	}
	keys, err := client.Keys(ctx, "auth_attempt:login_*").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if ttl := client.PTTL(ctx, key).Val(); ttl <= 0 || ttl > 5*time.Minute {
			t.Fatalf("unexpected login TTL %v", ttl)
		}
		if err := client.PExpire(ctx, key, time.Millisecond).Err(); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond)
	if retry, err := auth.ReserveLoginAttempt(ctx, client, "fixture", "192.0.2.1"); err != nil || retry != 0 {
		t.Fatalf("login after expiry: %v, %v", retry, err)
	}

	pending := auth.NewPendingAuthService(client, testEncryptor(t))
	for i := range 3 {
		token, err := pending.CreatePending(ctx, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		for j := range 6 {
			retry, err := pending.ReserveAttempt(ctx, token, 1, 1, false)
			if err != nil || (retry > 0) != (i == 2 || j == 5) {
				t.Fatalf("TOTP reservation %d/%d: %v, %v", i, j, retry, err)
			}
		}
		if err := pending.Consume(ctx, token); err != nil {
			t.Fatal(err)
		}
		if _, err := pending.ReserveAttempt(ctx, token, 1, 1, false); !errors.Is(err, redis.Nil) {
			t.Fatalf("consumed token: %v", err)
		}
	}
}

func TestAuthThrottleHandlers(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	client := setupThrottleRedis(t)
	pending := auth.NewPendingAuthService(client, testEncryptor(t))
	sessions := auth.NewSessionService(client)
	const password = "fixture-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := auth.GenerateTOTPSecret("fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	var operatorID int64
	if err := pool.QueryRow(ctx, `INSERT INTO operators (name,login,password_hash,totp_secret,status,role)
  VALUES ('active','active',$1,$2,'active','operator') RETURNING id`, hash, secret).Scan(&operatorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO operators (name,login,password_hash,status,role)
  VALUES ('disabled','disabled',$1,'disabled','operator')`, hash); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	audit := repository.NewAuditLogRepository(pool)
	api.NewServer(pool, client, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, repository.NewOperatorRepository(pool),
		nil, audit, nil, sessions, pending, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	request := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	login := func(login, password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"login": login, "password": password})
		return request("/api/auth/login", string(body))
	}
	for _, response := range []*httptest.ResponseRecorder{login("missing", password), login("disabled", password), login("active", "wrong")} {
		if response.Code != 401 || response.Body.String() != `{"error":"invalid credentials"}` {
			t.Fatalf("unexpected rejection: status %d", response.Code)
		}
	}
	success := login("active", password)
	if success.Code != 200 || !strings.Contains(success.Body.String(), "totp_required") {
		t.Fatalf("login status %d", success.Code)
	}
	for range 3 {
		if response := login("active", "wrong"); response.Code != 401 {
			t.Fatalf("wrong password status %d", response.Code)
		}
	}
	if response := login("active", password); response.Code != 429 || response.Header().Get("Retry-After") == "" {
		t.Fatalf("login limit status %d", response.Code)
	}
	verify := func(token, code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"pending_token": token, "code": code})
		return request("/api/auth/totp/verify", string(body))
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	token, err := pending.CreatePending(ctx, operatorID, 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if response := verify(token, "invalid"); response.Code != 401 {
			t.Fatalf("wrong TOTP status %d", response.Code)
		}
	}
	if response := verify(token, code); response.Code != 429 || response.Header().Get("Retry-After") == "" {
		t.Fatalf("TOTP limit status %d", response.Code)
	}
	token, err = pending.CreatePending(ctx, operatorID, 1)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan int, 2)
	for range 2 {
		go func() { outcomes <- verify(token, code).Code }()
	}
	a, b := <-outcomes, <-outcomes
	if (a != 200 || b != 401) && (a != 401 || b != 200) {
		t.Fatalf("concurrent verification statuses %d, %d", a, b)
	}
	if response := verify(token, code); response.Code != 401 {
		t.Fatalf("replay status %d", response.Code)
	}
	token, err = pending.CreatePending(ctx, operatorID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if response := login("active", password); response.Code != 503 {
		t.Fatalf("login without Redis status %d", response.Code)
	}
	if response := verify(token, code); response.Code != 503 {
		t.Fatalf("TOTP without Redis status %d", response.Code)
	}
	want := map[string]int{"operator_login_failed": 11, "operator_login": 1}
	entries, err := audit.List(ctx, repository.AuditLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		want[entry.Action]--
		if string(entry.Payload) != "null" || entry.IPAddress != nil || entry.UserAgent != nil {
			t.Fatal("authentication audit unexpectedly included input or client metadata")
		}
		if entry.ActorType == "unknown" && (entry.ActorID != nil || entry.TargetID != nil) {
			t.Fatal("unknown login was associated with an operator")
		}
	}
	for action, difference := range want {
		if difference != 0 {
			t.Errorf("%s audit count differs by %d", action, difference)
		}
	}
}
