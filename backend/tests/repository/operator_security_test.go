package repository_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestOperatorSessionRevocationAndAuditAccess(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	redis := setupThrottleRedis(t)
	operators := repository.NewOperatorRepository(pool)
	audit := repository.NewAuditLogRepository(pool)
	sessions := auth.NewSessionService(redis)
	pending := auth.NewPendingAuthService(redis, testEncryptor(t))
	makeOperator := func(login, role string) *models.Operator {
		t.Helper()
		operator := &models.Operator{Name: login, Login: login, Role: role, Status: models.OperatorStatusActive, Language: "ru"}
		if err := operators.Create(ctx, operator); err != nil {
			t.Fatal(err)
		}
		return operator
	}
	admin := makeOperator("fixture-admin", "admin")
	target := makeOperator("fixture-target", "admin")
	issue := func(o *models.Operator) string {
		t.Helper()
		token, err := sessions.Create(ctx, auth.SessionData{OperatorID: o.ID, Role: o.Role, AuthVersion: o.AuthVersion})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	first, second := issue(admin), issue(target)
	challenge, err := pending.CreatePending(ctx, target.ID, target.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, repository.NewSessionRepository(pool), nil,
		operators, nil, audit, nil, sessions, pending, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	call := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: status %d, want %d", method, path, w.Code, want)
		}
		return w
	}
	call(http.MethodGet, "/api/audit-log", second, "", 200)
	call(http.MethodGet, "/swagger/index.html", first, "", 404)
	path := "/api/operators/" + strconv.FormatInt(target.ID, 10)
	call(http.MethodPatch, path, first, `{"role":"operator"}`, 200)
	call(http.MethodGet, "/api/audit-log", second, "", 401)
	target, err = operators.GetByID(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	current := issue(target)
	call(http.MethodGet, "/api/audit-log", current, "", 403)
	call(http.MethodGet, "/api/sessions", current, "", 200)
	call(http.MethodPatch, path, first, `{"status":"suspended"}`, 200)
	call(http.MethodGet, "/api/sessions", current, "", 401)
	call(http.MethodPatch, path, first, `{"status":"active"}`, 200)
	call(http.MethodGet, "/api/sessions", current, "", 401)
	challengeBody, err := json.Marshal(map[string]string{"pending_token": challenge, "code": "123456"})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPost, "/api/auth/totp/verify", "", string(challengeBody), 401)

	call(http.MethodGet, "/api/audit-log?date_from=invalid", first, "", 400)
	call(http.MethodGet, "/api/audit-log?date_from=2026-01-02&date_to=2026-01-01", first, "", 400)
	call(http.MethodGet, "/api/audit-log?limit=201", first, "", 400)
	var lastID int64
	for offset := range 3 {
		w := call(http.MethodGet, "/api/audit-log?limit=1&offset="+strconv.Itoa(offset), first, "", 200)
		var page struct {
			Entries []models.AuditLog
			HasMore bool `json:"has_more"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Entries) != 1 || page.HasMore != (offset < 2) || page.Entries[0].ActorType != "operator" ||
			page.Entries[0].ActorRole == nil || *page.Entries[0].ActorRole != "admin" || (offset > 0 && page.Entries[0].ID >= lastID) {
			t.Fatal("unexpected audit page")
		}
		lastID = page.Entries[0].ID
	}
	w := call(http.MethodGet, "/api/audit-log?format=csv&limit=1", first, "", 200)
	if !strings.Contains(w.Body.String(), "actor_type") || strings.Count(w.Body.String(), "\n") != 2 {
		t.Fatal("expected bounded CSV with actor type")
	}
}
