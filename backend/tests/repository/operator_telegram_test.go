package repository_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestOperatorTelegramLink(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	redis := setupThrottleRedis(t)
	ops := repository.NewOperatorRepository(pool)
	users := repository.NewUserRepository(pool, nil)
	sessions := auth.NewSessionService(redis)
	makeOperator := func(login string, changePassword bool) (int64, string) {
		t.Helper()
		op := models.Operator{Name: login, Login: login, Role: "operator", Status: "active", Language: "ru", MustChangePassword: changePassword}
		if err := ops.Create(ctx, &op); err != nil {
			t.Fatal(err)
		}
		token, err := sessions.Create(ctx, auth.SessionData{OperatorID: op.ID, Role: op.Role, AuthVersion: op.AuthVersion})
		if err != nil {
			t.Fatal(err)
		}
		return op.ID, token
	}
	firstID, first := makeOperator("telegram-first", false)
	secondID, second := makeOperator("telegram-second", false)
	_, temporary := makeOperator("telegram-temporary", true)
	const path = "/api/account/telegram-link"
	const claim = "/internal/operators/telegram-link/claim"
	const linkedOperator = "/internal/operators/by-telegram/801"
	const language = "/internal/users/by-telegram/801/language"
	service := strings.Repeat("a", 64)
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), users, nil, nil, ops, nil,
		repository.NewAuditLogRepository(pool), nil, sessions, nil, nil, "", "").RegisterRoutes(router, service)
	call := func(method, route, bearer, internal string, input any, want int) map[string]any {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, route, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if internal != "" {
			req.Header.Set("X-Internal-Token", internal)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d want %d", method, route, res.Code, want)
		}
		result := map[string]any{}
		if res.Body.Len() > 0 {
			if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
				t.Fatal("expected JSON response")
			}
		}
		if (route == path || route == claim) && (want == 200 || want == 201) && res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("link responses must not be cached")
		}
		return result
	}
	issue := func(token string) string {
		t.Helper()
		result := call("POST", path, token, "", nil, 201)
		code, _ := result["code"].(string)
		if len(code) < 26 {
			t.Fatal("missing initial challenge")
		}
		return code
	}
	claimCode := func(code string, id int64, token string, want int) string {
		t.Helper()
		result := call("POST", claim, "", token, map[string]any{"code": code, "telegram_id": id}, want)
		confirmation, _ := result["confirmation_code"].(string)
		if want == 200 && (len(confirmation) < 26 || confirmation == code) {
			t.Fatal("missing independent confirmation challenge")
		}
		return confirmation
	}
	confirm := func(token, code string, want int) {
		t.Helper()
		call("POST", path+"/confirm", token, "", map[string]string{"code": code}, want)
	}
	call("POST", path, "", "", nil, 401)
	call("POST", path, temporary, "", nil, 401)
	if got := call("GET", path, first, "", nil, 200)["linked"]; got != false {
		t.Fatal("operator should have no binding initially")
	}
	invalidated := issue(first)
	firstCode := issue(first)
	claimCode(invalidated, 801, service, 404)
	claimCode(firstCode, 801, "", 401)
	confirm(first, firstCode, 404)
	firstConfirmation := claimCode(strings.ToLower(firstCode), 801, service, 200)
	claimCode(firstCode, 802, service, 404)
	call("GET", linkedOperator, "", service, nil, 404)
	confirm(second, firstConfirmation, 404)
	confirm(first, firstConfirmation, 204)
	confirm(first, firstConfirmation, 404)
	if got := call("GET", path, first, "", nil, 200)["linked"]; got != true {
		t.Fatal("operator binding is missing")
	}
	if got := call("GET", linkedOperator, "", service, nil, 200)["id"]; got != float64(firstID) {
		t.Fatal("existing bot lookup did not accept the linked account")
	}

	call("PATCH", language, "", service, map[string]string{"language_code": "en"}, 200)
	if got := call("GET", linkedOperator, "", service, nil, 200)["language"]; got != "en" {
		t.Fatal("updated operator language was not read back")
	}
	u := models.User{TelegramID: 801, LanguageCode: "ru"}
	if err := users.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	call("PATCH", language, "", service, map[string]string{"language_code": "de"}, 400)
	call("PATCH", language, "", service, map[string]string{"language_code": "en"}, 200)
	if got := call("GET", "/internal/users/by-telegram/801", "", service, nil, 200)["language_code"]; got != "en" {
		t.Fatal("updated participant language was not read back")
	}

	confirm(second, claimCode(issue(second), 801, service, 200), 409)
	secondConfirmation := claimCode(issue(second), 802, service, 200)
	if _, err := pool.Exec(ctx, "UPDATE operator_telegram_links SET expires_at=now()-interval '1 second' WHERE operator_id=$1", secondID); err != nil {
		t.Fatal(err)
	}
	confirm(second, secondConfirmation, 404)
	for _, dryRun := range []bool{true, true, false} {
		if n, err := ops.PurgeExpiredTelegramLinks(ctx, dryRun); err != nil || n != 1 {
			t.Fatalf("expired links (dry run %t): count %d, err %v", dryRun, n, err)
		}
	}
	if n, err := ops.PurgeExpiredTelegramLinks(ctx, false); err != nil || n != 0 {
		t.Fatalf("expired links retry: count %d, err %v", n, err)
	}
	confirm(first, claimCode(issue(first), 803, service, 200), 204)
	newName := "Renamed fixture operator"
	firstOperator, err := ops.GetByID(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ops.UpdateProfile(ctx, firstID, firstOperator.AuthVersion, &newName, nil, nil); err != nil {
		t.Fatal(err)
	}
	call("GET", linkedOperator, "", service, nil, 404)
	call("GET", "/internal/operators/by-telegram/803", "", service, nil, 200)

	revokedCode := issue(first)
	suspended := "suspended"
	if err := ops.UpdateProfile(ctx, firstID, firstOperator.AuthVersion, nil, nil, &suspended); err != nil {
		t.Fatal(err)
	}
	claimCode(revokedCode, 804, service, 404)
	call("GET", path, first, "", nil, 401)
}
