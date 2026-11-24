package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/delivery"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestServerWiringWithFakeBot(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	redis := setupThrottleRedis(t)
	users := repository.NewUserRepository(pool, nil)
	sessions := repository.NewSessionRepository(pool)
	operators := repository.NewOperatorRepository(pool)
	messages := repository.NewMessageRepository(pool)
	audit := repository.NewAuditLogRepository(pool)
	incoming := repository.NewIncomingRequestRepository(pool)
	access := auth.NewSessionService(redis)
	operator := models.Operator{Name: "Fixture", Login: "fixture-wiring", Role: "admin", Status: "active", Language: "ru"}
	if err := operators.Create(ctx, &operator); err != nil {
		t.Fatal(err)
	}
	bearer, err := access.Create(ctx, auth.SessionData{OperatorID: operator.ID, Role: operator.Role, AuthVersion: operator.AuthVersion})
	if err != nil {
		t.Fatal(err)
	}
	client := models.User{TelegramID: 601, LanguageCode: "ru"}
	executor := models.User{TelegramID: 602, LanguageCode: "ru"}
	for _, user := range []*models.User{&client, &executor} {
		if err := users.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	session := models.Session{Title: "Fixture", Status: "active", Language: "ru", ClientUserID: &client.ID, ExecutorUserID: &executor.ID}
	if err := sessions.Create(ctx, &session); err != nil {
		t.Fatal(err)
	}
	if err := users.SetActiveSession(ctx, client.ID, session.ID); err != nil {
		t.Fatal(err)
	}

	var alerts, sends atomic.Int32
	var lastDelivery atomic.Pointer[botclient.SendMessageRequest]
	notices := make(chan botclient.SendMessageRequest, 8)
	botToken := strings.Repeat("b", 64)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != botToken {
			t.Error("missing bot service credential")
		}
		switch r.URL.Path {
		case "/internal/send-alert":
			alerts.Add(1)
		case "/internal/send-message":
			sends.Add(1)
			var req botclient.SendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error("invalid request sent to the bot")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.SessionID == 0 {
				notices <- req
			} else {
				lastDelivery.Store(&req)
			}
		default:
			t.Error("unexpected bot route")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()
	botClient := botclient.New(fake.URL, botToken)
	worker := delivery.New(messages, botClient, slog.Default(), nil)
	send := func() {
		t.Helper()
		if ok, err := worker.ProcessNext(ctx); !ok || err != nil {
			t.Fatalf("process delivery: %v", err)
		}
	}
	serviceToken := strings.Repeat("a", 64)
	router := gin.New()
	photosDir, voiceDir := t.TempDir(), t.TempDir()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), users, sessions, incoming, operators, messages, audit,
		moderation.NewService(redis, moderation.Config{}), access, nil, botClient, photosDir, voiceDir).RegisterRoutes(router, serviceToken)
	request := func(t *testing.T, method, path, body, token string, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", serviceToken)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d want %d", method, path, res.Code, want)
		}
		return res.Body.String()
	}
	call := func(path, body string, want int) {
		t.Helper()
		request(t, http.MethodPost, path, body, bearer, want)
	}
	call("/internal/incoming-requests", `{"telegram_id":603}`, http.StatusCreated)
	call("/internal/incoming-requests", `{"telegram_id":603,"first_message_text":"Fixture first message"}`, http.StatusOK)
	call("/internal/messages/relay", `{"sender_telegram_id":601,"content_type":"photo","file_id":"invalid"}`, http.StatusBadRequest)
	const fileID = "38c45a07-ff42-40ce-92f5-4f6e03dfaf22"
	var mediaPaths []string
	for _, media := range []struct{ kind, ext, dir, mime string }{
		{"photo", ".jpg", photosDir, "image/jpeg"}, {"voice", ".ogg", voiceDir, "audio/ogg"},
	} {
		if err := os.WriteFile(filepath.Join(media.dir, fileID+media.ext), []byte(media.kind), 0600); err != nil {
			t.Fatal(err)
		}
		call("/internal/messages/relay", fmt.Sprintf(`{"sender_telegram_id":601,"content_type":%q,"file_id":%q}`, media.kind, fileID), http.StatusOK)
		send()
		var id int64
		if err := pool.QueryRow(ctx, "SELECT id FROM messages WHERE session_id=$1 AND content_type=$2 AND delivered=true", session.ID, media.kind).Scan(&id); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/messages/%d/media", id), nil)
		mediaPaths = append(mediaPaths, req.URL.Path)
		req.Header.Set("Authorization", "Bearer "+bearer)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK || res.Body.String() != media.kind || res.Header().Get("Content-Type") != media.mime {
			t.Fatalf("preview %s returned status %d", media.kind, res.Code)
		}
	}
	call(fmt.Sprintf("/api/sessions/%d/send-as", session.ID), `{"impersonated_role":"client","content_type":"text","content":"hello"}`, http.StatusOK)
	send()
	call("/internal/sessions/stop", `{"telegram_id":601}`, http.StatusOK)
	if sends.Load() != 3 || alerts.Load() != 2 {
		t.Fatalf("fake bot received %d sends and %d alerts", sends.Load(), alerts.Load())
	}
	entries, err := audit.List(ctx, repository.AuditLogFilter{})
	if err != nil || len(entries) != 4 || entries[0].ActorType != "user" || entries[1].ActorType != "operator" {
		t.Fatalf("unexpected audit actor typing, err=%v", err)
	}

	t.Run("session access and archive", func(t *testing.T) {
		telegramID := int64(604)
		owner := models.Operator{Name: "Owner", Login: "fixture-owner", Role: "operator", Status: "active", Language: "ru", TelegramID: &telegramID}
		other := models.Operator{Name: "Other", Login: "fixture-other", Role: "operator", Status: "active", Language: "ru"}
		operatorToken := func(op *models.Operator) string {
			t.Helper()
			if err := operators.Create(ctx, op); err != nil {
				t.Fatal(err)
			}
			token, err := access.Create(ctx, auth.SessionData{OperatorID: op.ID, Role: op.Role, AuthVersion: op.AuthVersion})
			if err != nil {
				t.Fatal(err)
			}
			return token
		}
		ownerToken, otherToken := operatorToken(&owner), operatorToken(&other)
		sessionPath := fmt.Sprintf("/api/sessions/%d", session.ID)
		request(t, http.MethodPatch, sessionPath+"/owner", fmt.Sprintf(`{"operator_id":%d}`, owner.ID), bearer, http.StatusOK)
		for _, path := range append([]string{sessionPath, sessionPath + "/messages"}, mediaPaths...) {
			request(t, http.MethodGet, path, "", otherToken, http.StatusForbidden)
			request(t, http.MethodGet, path, "", "", http.StatusUnauthorized)
			request(t, http.MethodGet, path, "", ownerToken, http.StatusOK)
		}
		if body := request(t, http.MethodGet, sessionPath+"/messages", "", ownerToken, http.StatusOK); strings.Count(body, `"delivered":true`) != 3 {
			t.Fatal("panel history did not expose the three delivered messages")
		}
		first := request(t, http.MethodGet, sessionPath+"/messages?limit=1", "", ownerToken, http.StatusOK)
		last := request(t, http.MethodGet, sessionPath+"/messages?limit=1&offset=2", "", ownerToken, http.StatusOK)
		if !strings.Contains(first, `"has_more":true`) || !strings.Contains(first, `"content_type":"text"`) ||
			!strings.Contains(last, `"has_more":false`) || !strings.Contains(last, `"content_type":"photo"`) {
			t.Fatal("history pagination lost its newest-first page order")
		}
		request(t, http.MethodGet, sessionPath+"/messages?limit=201", "", ownerToken, http.StatusBadRequest)
		for _, test := range []struct{ token, present string }{{ownerToken, `"title":"Fixture"`}, {otherToken, `"sessions":[]`}, {bearer, `"title":"Fixture"`}} {
			if body := request(t, http.MethodGet, "/api/sessions", "", test.token, http.StatusOK); !strings.Contains(body, test.present) {
				t.Fatal("session list is inconsistent with the operator's scope")
			}
		}
		request(t, http.MethodPost, sessionPath+"/send-as", `{"impersonated_role":"client","content_type":"text","content":"denied"}`, otherToken, http.StatusForbidden)
		request(t, http.MethodPost, sessionPath+"/invites", `{"role":"client"}`, otherToken, http.StatusForbidden)
		request(t, http.MethodPost, sessionPath+"/invites", `{"role":"client"}`, ownerToken, http.StatusConflict)
		request(t, http.MethodGet, "/api/operators", "", ownerToken, http.StatusForbidden)
		if body := request(t, http.MethodGet, "/api/operators", "", bearer, http.StatusOK); !strings.Contains(body, "fixture-owner") {
			t.Fatal("admin operator list did not include the owner")
		}
		request(t, http.MethodGet, "/internal/operators/by-telegram/604", "", "", http.StatusOK)
		request(t, http.MethodGet, "/internal/operators/by-telegram/604/sessions", "", "", http.StatusOK)
		request(t, http.MethodGet, "/api/audit-log", "", bearer, http.StatusOK)
		if csv := request(t, http.MethodGet, "/api/audit-log?format=csv", "", bearer, http.StatusOK); !strings.Contains(csv, "operator") {
			t.Fatal("audit CSV missing operator records")
		}
		request(t, http.MethodPost, "/api/auth/logout", "", otherToken, http.StatusOK)
		request(t, http.MethodGet, "/api/sessions", "", otherToken, http.StatusUnauthorized)
	})

	t.Run("bot routing and moderation", func(t *testing.T) {
		service := moderation.NewService(redis, moderation.Config{})
		userPath := "/internal/users/by-telegram/601"
		sessionPath := fmt.Sprintf("/api/sessions/%d", session.ID)
		request(t, http.MethodGet, userPath, "", "", http.StatusOK)
		request(t, http.MethodPatch, userPath+"/language", `{"language_code":"en"}`, "", http.StatusOK)
		request(t, http.MethodGet, userPath+"/sessions", "", "", http.StatusOK)
		request(t, http.MethodPatch, sessionPath+"/status", `{"status":"active"}`, bearer, http.StatusOK)
		request(t, http.MethodPatch, userPath+"/active-session", fmt.Sprintf(`{"target_session_id":%d}`, session.ID), "", http.StatusOK)
		for _, kind := range []string{"text", "photo", "voice"} {
			mediaRef := fileID
			if kind == "text" {
				mediaRef = ""
			}
			body := request(t, http.MethodPost, "/internal/messages/relay", fmt.Sprintf(`{"sender_telegram_id":601,"content_type":%q,"content":"fixture@example.invalid","file_id":%q}`, kind, mediaRef), "", http.StatusOK)
			if !strings.Contains(body, `"blocked":true`) || strings.Contains(body, `"message_id"`) {
				t.Fatal("blocked message produced a delivery record or went unreported")
			}
		}
		current, err := sessions.GetByID(ctx, session.ID)
		if err != nil || current.Status != "active" || alerts.Load() != 5 {
			t.Fatalf("default moderation should leave the session active and send three alerts: %v", err)
		}
		violationKey := fmt.Sprintf("moderation:session:%d:violations", session.ID)
		ttl, err := redis.TTL(ctx, violationKey).Result()
		if err != nil || ttl <= 0 || ttl > 24*time.Hour {
			t.Fatalf("invalid moderation TTL: %s, %v", ttl, err)
		}
		reqs, err := incoming.ListByStatus(ctx, "new")
		if err != nil || len(reqs) != 1 {
			t.Fatalf("expected one new request: %v", err)
		}
		request(t, http.MethodGet, "/api/incoming-requests", "", bearer, http.StatusOK)
		request(t, http.MethodGet, "/internal/incoming-requests", "", "", http.StatusOK)
		request(t, http.MethodPost, fmt.Sprintf("/internal/incoming-requests/%d/mark-processed", reqs[0].ID), "", "", http.StatusOK)
		request(t, http.MethodPatch, fmt.Sprintf("/api/incoming-requests/%d/status", reqs[0].ID), `{"status":"new"}`, bearer, http.StatusOK)
		if body := request(t, http.MethodGet, "/api/incoming-requests", "", bearer, http.StatusOK); !strings.Contains(body, `"status":"new"`) ||
			!strings.Contains(body, `"first_message_text":"Fixture first message"`) {
			t.Fatal("request did not return to new status")
		}
		if result, err := service.Check(ctx, session.ID+1, "user", client.ID, "fixture@example.invalid"); err != nil || !result.Blocked || result.ViolationCount != 1 || result.ShouldPause {
			t.Fatalf("counter was not isolated per session: %+v, %v", result, err)
		}
		if result, err := service.Check(ctx, session.ID, "operator", client.ID, "fixture@example.invalid"); err != nil || !result.Blocked || result.ViolationCount != 1 || result.ShouldPause {
			t.Fatalf("counter was not isolated per user: %+v, %v", result, err)
		}
		cfg, err := moderation.ParseConfig("", "true")
		if err != nil {
			t.Fatal(err)
		}
		autoPause := moderation.NewService(redis, cfg)
		for i := 1; i <= 3; i++ {
			result, err := autoPause.Check(ctx, session.ID, "user", executor.ID, "contact me directly")
			if err != nil || result.ShouldPause != (i == 3) || result.Reason != "suspicious_phrase" {
				t.Fatalf("unexpected opt-in auto-pause decision at attempt %d: %+v, %v", i, result, err)
			}
		}
		if err := sessions.UpdateStatus(ctx, session.ID, "paused"); err != nil {
			t.Fatal(err)
		}
		request(t, http.MethodPatch, sessionPath+"/status", `{"status":"active"}`, bearer, http.StatusOK)
		if n, err := redis.Exists(ctx, violationKey).Result(); err != nil || n != 0 {
			t.Fatal("resuming did not clear moderation counters for the session")
		}
	})

	t.Run("management operations and refusals", func(t *testing.T) {
		created := request(t, http.MethodPost, "/api/operators", `{"name":"New operator","role":"operator","login":"fixture-invited"}`, bearer, http.StatusCreated)
		var invited struct {
			ID                int64  `json:"id"`
			TemporaryPassword string `json:"temporary_password"`
		}
		if err := json.Unmarshal([]byte(created), &invited); err != nil || invited.ID == 0 || invited.TemporaryPassword == "" {
			t.Fatal("operator response omitted the new account or temporary password")
		}
		op, err := operators.GetByID(ctx, invited.ID)
		if err != nil || op.Status != "invited" || op.PasswordHash == nil ||
			*op.PasswordHash == invited.TemporaryPassword || !op.MustChangePassword {
			t.Fatal("invited operator was not saved with a hashed temporary password requiring rotation")
		}
		operatorPath := fmt.Sprintf("/api/operators/%d", invited.ID)
		request(t, http.MethodPatch, operatorPath, `{"name":"Renamed operator"}`, bearer, http.StatusOK)
		op, err = operators.GetByID(ctx, invited.ID)
		if err != nil || op.Name != "Renamed operator" {
			t.Fatal("operator name change was not persisted")
		}
		created = request(t, http.MethodPost, "/api/sessions", `{"title":"Fresh fixture"}`, bearer, http.StatusCreated)
		var fresh models.Session
		if err := json.Unmarshal([]byte(created), &fresh); err != nil || fresh.ID == 0 || fresh.Status != "active" {
			t.Fatal("created session is missing or inactive")
		}
		freshPath := fmt.Sprintf("/api/sessions/%d", fresh.ID)
		issued := request(t, http.MethodPost, freshPath+"/invites", `{"role":"executor"}`, bearer, http.StatusCreated)
		var invitation struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal([]byte(issued), &invitation); err != nil || len(invitation.Token) != 43 {
			t.Fatal("server failed to issue an opaque session invitation")
		}
		if snapshot := request(t, http.MethodGet, freshPath, "", bearer, http.StatusOK); strings.Contains(snapshot, invitation.Token) {
			t.Fatal("session details exposed a previously issued invitation token")
		}
		request(t, http.MethodDelete, freshPath+"/invites/executor", "", bearer, http.StatusNoContent)
		// The earlier stop request leaves the original session pending until it is resumed.
		// Set up both active fixtures here so this subtest also works on its own.
		if err := sessions.UpdateStatus(ctx, session.ID, models.SessionStatusActive); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE sessions SET created_at=$1 WHERE id IN ($2, $3)", session.CreatedAt, session.ID, fresh.ID); err != nil {
			t.Fatal(err)
		}
		page := request(t, http.MethodGet, "/api/sessions?q=fixture&status=active&limit=1", "", bearer, http.StatusOK)
		second := request(t, http.MethodGet, "/api/sessions?q=fixture&status=active&limit=1&offset=1", "", bearer, http.StatusOK)
		assertSessionPage := func(body string, wantID int64, hasMore bool) {
			t.Helper()
			var got struct {
				Sessions []struct {
					ID int64 `json:"id"`
				} `json:"sessions"`
				HasMore bool `json:"has_more"`
			}
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("decode session page: %v", err)
			}
			if len(got.Sessions) != 1 || got.Sessions[0].ID != wantID || got.HasMore != hasMore {
				t.Fatalf("expected session %d with has_more=%t, got IDs %+v with has_more=%t", wantID, hasMore, got.Sessions, got.HasMore)
			}
		}
		assertSessionPage(page, fresh.ID, true)
		assertSessionPage(second, session.ID, false)
		if body := request(t, http.MethodGet, "/api/sessions?status=closed", "", bearer, http.StatusOK); !strings.Contains(body, `"sessions":[]`) {
			t.Fatal("status filter returned an active session")
		}
		for _, test := range []struct {
			method, path, body string
			status             int
		}{
			{http.MethodGet, "/api/sessions/oops", "", http.StatusBadRequest},
			{http.MethodGet, "/api/sessions?status=unknown", "", http.StatusBadRequest},
			{http.MethodGet, "/api/sessions?q=" + strings.Repeat("a", 241), "", http.StatusBadRequest},
			{http.MethodGet, "/api/sessions/9999", "", http.StatusNotFound},
			{http.MethodGet, "/api/sessions/oops/messages", "", http.StatusBadRequest},
			{http.MethodGet, "/api/sessions/9999/messages", "", http.StatusNotFound},
			{http.MethodGet, "/api/messages/oops/media", "", http.StatusBadRequest},
			{http.MethodGet, "/api/messages/9999/media", "", http.StatusNotFound},
			{http.MethodPost, "/api/sessions", `{}`, http.StatusBadRequest},
			{http.MethodPost, freshPath + "/invites", `{"role":"admin"}`, http.StatusBadRequest},
			{http.MethodDelete, freshPath + "/invites/admin", "", http.StatusBadRequest},
			{http.MethodDelete, "/api/sessions/9999/invites/client", "", http.StatusNotFound},
			{http.MethodPost, "/api/operators", `{"role":"unknown"}`, http.StatusBadRequest},
			{http.MethodPatch, operatorPath, `{"role":"unknown"}`, http.StatusBadRequest},
			{http.MethodPatch, "/api/operators/9999", `{"status":"disabled"}`, http.StatusNotFound},
			{http.MethodPatch, "/api/operators/oops", `{}`, http.StatusBadRequest},
			{http.MethodPatch, "/api/sessions/oops/owner", `{}`, http.StatusBadRequest},
			{http.MethodPatch, "/api/sessions/9999/owner", `{"operator_id":1}`, http.StatusNotFound},
			{http.MethodPatch, "/api/sessions/oops/status", `{}`, http.StatusBadRequest},
			{http.MethodPatch, "/api/sessions/9999/status", `{"status":"paused"}`, http.StatusNotFound},
			{http.MethodPost, "/api/sessions/oops/send-as", `{}`, http.StatusBadRequest},
			{http.MethodPost, "/api/sessions/9999/send-as", `{"impersonated_role":"client","content_type":"text","content":"fixture"}`, http.StatusNotFound},
			{http.MethodGet, "/api/audit-log?date_from=invalid", "", http.StatusBadRequest},
			{http.MethodGet, "/api/audit-log?format=xml", "", http.StatusBadRequest},
			{http.MethodPatch, "/api/incoming-requests/oops/status", `{}`, http.StatusBadRequest},
			{http.MethodPatch, "/api/incoming-requests/1/status", `{"status":"unknown"}`, http.StatusBadRequest},
			{http.MethodGet, "/internal/users/by-telegram/oops", "", http.StatusBadRequest},
			{http.MethodGet, "/internal/users/by-telegram/9999", "", http.StatusNotFound},
			{http.MethodGet, "/internal/users/by-telegram/oops/sessions", "", http.StatusBadRequest},
			{http.MethodGet, "/internal/users/by-telegram/9999/sessions", "", http.StatusNotFound},
			{http.MethodPatch, "/internal/users/by-telegram/601/language", `{"language_code":"unknown"}`, http.StatusBadRequest},
			{http.MethodPatch, "/internal/users/by-telegram/601/active-session", `{"target_session_id":9999}`, http.StatusForbidden},
			{http.MethodGet, "/internal/operators/by-telegram/oops", "", http.StatusBadRequest},
			{http.MethodGet, "/internal/operators/by-telegram/9999", "", http.StatusNotFound},
			{http.MethodGet, "/internal/operators/by-telegram/9999/sessions", "", http.StatusNotFound},
		} {
			t.Run(test.method+" "+test.path, func(t *testing.T) {
				request(t, test.method, test.path, test.body, bearer, test.status)
			})
		}
	})

	t.Run("audit has one entry per successful action", func(t *testing.T) {
		targets := []struct {
			action, target string
			count          int
		}{
			{"operator_logout", "operator", 1},
			{"session_created", "session", 1},
			{"incoming_request_status_updated", "incoming_request", 2},
			{"message_media_accessed", "message", 4},
			{"session_stop_requested", "session", 1},
			{"session_switched", "session", 1},
		}
		for _, action := range targets {
			entries, err := audit.List(ctx, repository.AuditLogFilter{Action: &action.action})
			if err != nil || len(entries) != action.count {
				t.Fatalf("%s: got %d audit records, want %d: %v", action.action, len(entries), action.count, err)
			}
			for _, entry := range entries {
				if entry.TargetType == nil || *entry.TargetType != action.target || entry.TargetID == nil {
					t.Fatalf("%s: missing target", action.action)
				}
			}
		}
	})

	t.Run("remaining list routes enforce bounded pages", func(t *testing.T) {
		ownerID := int64(604)
		owner, err := operators.GetByTelegramID(ctx, ownerID)
		if err != nil {
			t.Fatal(err)
		}
		newer := models.Session{Title: "Second page fixture", Status: "active", Language: "ru", ClientUserID: &client.ID}
		if err := sessions.Create(ctx, &newer); err != nil {
			t.Fatal(err)
		}
		if err := sessions.AssignOwner(ctx, newer.ID, owner.ID); err != nil {
			t.Fatal(err)
		}
		if err := incoming.Create(ctx, &models.IncomingRequest{TelegramID: 605}); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ path, key, token string }{
			{"/api/operators", "operators", bearer},
			{"/api/incoming-requests", "requests", bearer},
			{"/internal/incoming-requests", "requests", ""},
			{"/internal/operators/by-telegram/604/sessions", "sessions", ""},
			{"/internal/users/by-telegram/601/sessions", "sessions", ""},
		} {
			t.Run(tc.path, func(t *testing.T) {
				type item struct {
					ID        int64 `json:"id"`
					SessionID int64 `json:"session_id"`
					IsCurrent bool  `json:"is_current"`
				}
				get := func(offset int) (item, bool) {
					t.Helper()
					body := request(t, http.MethodGet, fmt.Sprintf("%s?limit=1&offset=%d", tc.path, offset), "", tc.token, http.StatusOK)
					var result map[string]json.RawMessage
					var items []item
					var more bool
					if json.Unmarshal([]byte(body), &result) != nil || json.Unmarshal(result[tc.key], &items) != nil ||
						json.Unmarshal(result["has_more"], &more) != nil || len(items) != 1 {
						t.Fatal("unexpected page envelope")
					}
					return items[0], more
				}
				first, more := get(0)
				second, _ := get(1)
				if !more || first == second {
					t.Fatal("second page did not advance")
				}
				if first.SessionID != 0 && (!first.IsCurrent || first.SessionID != session.ID || second.SessionID != newer.ID) {
					t.Fatal("current participant session did not remain on the first page")
				}
				request(t, http.MethodGet, tc.path+"?limit=201", "", tc.token, http.StatusBadRequest)
				request(t, http.MethodGet, tc.path+"?offset=-1", "", tc.token, http.StatusBadRequest)
			})
		}
	})

	t.Run("pause and close notify both participants once", func(t *testing.T) {
		path := fmt.Sprintf("/api/sessions/%d/status", session.ID)
		request(t, http.MethodPatch, path, `{"status":"active"}`, bearer, http.StatusOK)
		for _, test := range []struct{ status, english, russian string }{
			{"paused", "The session has been paused.", "Сессия приостановлена."},
			{"closed", "The session has been closed.", "Сессия закрыта."},
		} {
			body := fmt.Sprintf(`{"status":%q}`, test.status)
			request(t, http.MethodPatch, path, body, bearer, http.StatusOK)
			request(t, http.MethodPatch, path, body, bearer, http.StatusOK)
			want := map[int64]string{client.TelegramID: test.english, executor.TelegramID: test.russian}
			for len(notices) > 0 {
				notice := <-notices
				if want[notice.RecipientTelegramID] != notice.Text || notice.Text == "" || notice.SenderLabel != "" || notice.FileID != "" {
					t.Fatal("unexpected or repeated notice to a participant")
				}
				delete(want, notice.RecipientTelegramID)
			}
			if len(want) != 0 {
				t.Fatal("both participants must receive the status notice")
			}
		}
		if current, err := sessions.GetByID(ctx, session.ID); err != nil || current.Status != "closed" || current.ClosedAt == nil {
			t.Fatalf("closing the session did not persist the status and timestamp: %v", err)
		}
	})

	t.Run("opt-in moderation pause", func(t *testing.T) {
		cfg, err := moderation.ParseConfig("", "true")
		if err != nil {
			t.Fatal(err)
		}
		previous := router
		router = gin.New()
		t.Cleanup(func() { router = previous })
		api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), users, sessions, incoming, operators, messages, audit,
			moderation.NewService(redis, cfg), access, nil, botClient, photosDir, voiceDir).RegisterRoutes(router, serviceToken)
		path := fmt.Sprintf("/api/sessions/%d/status", session.ID)
		request(t, http.MethodPatch, path, `{"status":"active"}`, bearer, http.StatusOK)
		for i := 1; i <= 3; i++ {
			request(t, http.MethodPost, "/internal/messages/relay", `{"sender_telegram_id":601,"content_type":"text","content":"fixture@example.invalid"}`, "", http.StatusOK)
			got, err := sessions.GetByID(ctx, session.ID)
			if err != nil || (got.Status == "paused") != (i == 3) {
				t.Fatalf("unexpected session status after violation %d: %v", i, err)
			}
		}
		if len(notices) != 2 {
			t.Fatal("opt-in automatic pause did not notify both participants")
		}
	})

	t.Run("relay decisions preserve queue boundaries", func(t *testing.T) {
		user := models.User{TelegramID: 605, LanguageCode: "en"}
		if err := users.Create(ctx, &user); err != nil {
			t.Fatal(err)
		}
		waiting := models.Session{Title: "Waiting fixture", Status: models.SessionStatusActive, Language: "en", ClientUserID: &user.ID}
		if err := sessions.Create(ctx, &waiting); err != nil {
			t.Fatal(err)
		}
		relay := func(reason string, queued bool) {
			t.Helper()
			body := request(t, http.MethodPost, "/internal/messages/relay", `{"sender_telegram_id":605,"content_type":"text","content":"fixture"}`, "", http.StatusOK)
			var got struct {
				Blocked   bool   `json:"blocked"`
				Reason    string `json:"block_reason"`
				MessageID int64  `json:"message_id"`
			}
			if err := json.Unmarshal([]byte(body), &got); err != nil || !got.Blocked || got.Reason != reason || (got.MessageID > 0) != queued {
				t.Fatalf("unexpected relay result: %+v, %v", got, err)
			}
		}
		relay("no_active_session", false)
		if err := users.SetActiveSession(ctx, user.ID, session.ID); err != nil {
			t.Fatal(err)
		}
		relay("session_not_active", false)
		if err := sessions.UpdateStatus(ctx, session.ID, models.SessionStatusActive); err != nil {
			t.Fatal(err)
		}
		relay("sender_not_in_session", false)
		if err := users.SetActiveSession(ctx, user.ID, waiting.ID); err != nil {
			t.Fatal(err)
		}
		relay("counterparty_not_bound_yet", true)
		var queued int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM message_outbox JOIN messages ON messages.id=message_outbox.message_id WHERE session_id=$1", waiting.ID).Scan(&queued); err != nil || queued != 1 {
			t.Fatalf("expected exactly one waiting message: %d, %v", queued, err)
		}
	})

	for _, path := range []string{"/health", "/api/health"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"status":"ok"`) {
			t.Fatalf("health %s returned %d", path, res.Code)
		}
	}
	request(t, http.MethodPatch, fmt.Sprintf("/api/sessions/%d/status", session.ID), `{"status":"active"}`, bearer, http.StatusOK)
	t.Run("operator reply targets the session in the request", func(t *testing.T) {
		secondClient := models.User{TelegramID: 606, LanguageCode: "ru"}
		if err := users.Create(ctx, &secondClient); err != nil {
			t.Fatal(err)
		}
		owner := models.Operator{Name: "Reply operator", Login: "fixture-reply-owner", TelegramID: &executor.TelegramID,
			Role: models.OperatorRoleOperator, Status: models.OperatorStatusActive, Language: "ru"}
		if err := operators.Create(ctx, &owner); err != nil {
			t.Fatal(err)
		}
		ownerBearer, err := access.Create(ctx, auth.SessionData{OperatorID: owner.ID, Role: owner.Role, AuthVersion: owner.AuthVersion})
		if err != nil {
			t.Fatal(err)
		}
		if err := users.SetActiveSession(ctx, executor.ID, session.ID); err != nil {
			t.Fatal(err)
		}
		for _, user := range []models.User{client, secondClient} {
			body := fmt.Sprintf(`{"telegram_id":%d}`, user.TelegramID)
			request(t, http.MethodPost, "/internal/incoming-requests", body, "", http.StatusCreated)
		}
		second := models.Session{Title: "Second client", Status: models.SessionStatusActive,
			Language: "ru", ClientUserID: &secondClient.ID, ExecutorUserID: &executor.ID, OwnerOperatorID: &owner.ID}
		if err := sessions.Create(ctx, &second); err != nil {
			t.Fatal(err)
		}
		before := sends.Load()
		path := fmt.Sprintf("/api/sessions/%d/send-as", second.ID)
		reply := func(recipient int64, key, content string) string {
			return fmt.Sprintf(`{"impersonated_role":"executor","content_type":"text","recipient_role":"client","recipient_user_id":%d,"request_key":%q,"content":%q}`,
				recipient, key, content)
		}
		const key = "10000000-0000-4000-8000-000000000001"
		request(t, http.MethodPost, path, reply(secondClient.ID, "invalid", "Invalid key"), ownerBearer, http.StatusBadRequest)
		request(t, http.MethodPost, path, reply(client.ID, key, "Wrong recipient"), ownerBearer, http.StatusConflict)
		body := reply(secondClient.ID, key, "Second request reply")
		first := request(t, http.MethodPost, path, body, ownerBearer, http.StatusOK)
		if strings.Contains(first, "recipient_telegram_id") {
			t.Fatal("explicit reply response must not expose the Telegram recipient ID")
		}
		if retry := request(t, http.MethodPost, path, body, ownerBearer, http.StatusOK); retry != first {
			t.Fatal("repeated reply returned a different message")
		}
		request(t, http.MethodPost, path, reply(secondClient.ID, key, "Different request"), ownerBearer, http.StatusConflict)
		if ok, err := worker.ProcessNext(ctx); !ok || err != nil {
			t.Fatalf("process operator reply: processed=%t, err=%v", ok, err)
		}
		got := lastDelivery.Load()
		if got == nil || got.SessionID != second.ID || got.RecipientTelegramID != secondClient.TelegramID {
			t.Fatal("operator reply was not delivered to the second session's client")
		}
		if ok, err := worker.ProcessNext(ctx); ok || err != nil || sends.Load() != before+1 {
			t.Fatalf("expected only one bot send: processed=%t, err=%v, sends=%d", ok, err, sends.Load()-before)
		}
		for _, tc := range []struct {
			name, update string
			value, prior any
			status       int
		}{
			{"participant replaced", "UPDATE sessions SET client_user_id=$1 WHERE id=$2", client.ID, secondClient.ID, http.StatusConflict},
			{"session closed", "UPDATE sessions SET status=$1 WHERE id=$2", models.SessionStatusClosed, models.SessionStatusActive, http.StatusConflict},
			{"operator access changed", "UPDATE sessions SET owner_operator_id=$1 WHERE id=$2", operator.ID, owner.ID, http.StatusForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				message := request(t, http.MethodPost, path, `{"impersonated_role":"executor","content_type":"text","content":"Queued fixture"}`, ownerBearer, http.StatusOK)
				var result struct {
					ID int64 `json:"message_id"`
				}
				if err := json.Unmarshal([]byte(message), &result); err != nil || result.ID == 0 {
					t.Fatalf("invalid queued message: %v", err)
				}
				item, err := messages.ClaimDelivery(ctx, time.Minute)
				if err != nil || item == nil || item.MessageID != result.ID || item.Cancelled {
					t.Fatalf("could not lease expected message: %v", err)
				}
				if err := messages.CompleteDelivery(ctx, item, false, 0, 5); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, tc.update, tc.value, second.ID); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := pool.Exec(ctx, tc.update, tc.prior, second.ID); err != nil {
						t.Error(err)
					}
				}()
				request(t, http.MethodPost, path, body, ownerBearer, tc.status)
				if ok, err := worker.ProcessNext(ctx); !ok || err != nil || sends.Load() != before+1 {
					t.Fatalf("invalidated retry must not reach the bot: processed=%t, err=%v", ok, err)
				}
				var status string
				if err := pool.QueryRow(ctx, "SELECT status FROM message_outbox WHERE message_id=$1", result.ID).Scan(&status); err != nil || status != "failed" {
					t.Fatalf("expected terminal failure, got %q: %v", status, err)
				}
			})
		}
		var queued int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM message_outbox o JOIN messages m ON m.id=o.message_id WHERE m.session_id=$1", second.ID).Scan(&queued); err != nil || queued != 4 {
			t.Fatalf("unexpected number of queue records: %d, %v", queued, err)
		}
		role, content := models.SenderRoleExecutor, "Legacy queued reply"
		legacy := &models.Message{SessionID: second.ID, SenderRole: models.SenderRoleOperator, SentByOperator: true,
			ImpersonatedRole: &role, ContentType: models.ContentTypeText, Content: &content}
		if err := messages.Enqueue(ctx, legacy); err != nil {
			t.Fatal(err)
		}
		if ok, err := worker.ProcessNext(ctx); !ok || err != nil || sends.Load() != before+1 {
			t.Fatalf("legacy reply without a fixed recipient must not reach the bot: processed=%t, err=%v", ok, err)
		}
		var legacyStatus string
		if err := pool.QueryRow(ctx, "SELECT status FROM message_outbox WHERE message_id=$1", legacy.ID).Scan(&legacyStatus); err != nil || legacyStatus != "failed" {
			t.Fatalf("expected legacy reply to fail safely: %q, %v", legacyStatus, err)
		}
	})

	t.Run("incoming request claim and linked session", func(t *testing.T) {
		body := request(t, http.MethodPost, "/internal/incoming-requests", `{"telegram_id":607,"first_message_text":"Fixture request details"}`, "", http.StatusCreated)
		var created struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal([]byte(body), &created); err != nil || created.ID == 0 {
			t.Fatalf("invalid created request: %v", err)
		}
		path := fmt.Sprintf("/api/incoming-requests/%d", created.ID)
		request(t, http.MethodGet, path, "", "", http.StatusUnauthorized)
		request(t, http.MethodPost, path+"/session", `{"title":"Queue fixture"}`, bearer, http.StatusConflict)
		request(t, http.MethodPost, path+"/claim", "", bearer, http.StatusNoContent)
		request(t, http.MethodPost, path+"/claim", "", bearer, http.StatusNoContent)
		reused := request(t, http.MethodPost, "/internal/incoming-requests", `{"telegram_id":607}`, "", http.StatusOK)
		var returned struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(reused), &returned); err != nil ||
			returned.ID != created.ID || returned.Status != models.IncomingRequestStatusInProgress {
			t.Fatalf("repeat request did not return the assigned card: %+v, %v", returned, err)
		}
		other := models.Operator{Name: "Queue operator", Login: "fixture-queue-other",
			Role: models.OperatorRoleOperator, Status: models.OperatorStatusActive, Language: "en"}
		if err := operators.Create(ctx, &other); err != nil {
			t.Fatal(err)
		}
		otherToken, err := access.Create(ctx, auth.SessionData{OperatorID: other.ID, Role: other.Role, AuthVersion: other.AuthVersion})
		if err != nil {
			t.Fatal(err)
		}
		request(t, http.MethodGet, path, "", otherToken, http.StatusForbidden)
		request(t, http.MethodPost, path+"/claim", "", otherToken, http.StatusConflict)
		first := request(t, http.MethodPost, path+"/session", `{"title":"Queue fixture"}`, bearer, http.StatusCreated)
		repeat := request(t, http.MethodPost, path+"/session", `{"title":"Queue fixture"}`, bearer, http.StatusOK)
		if first != repeat {
			t.Fatal("session retry changed the linked session")
		}
		request(t, http.MethodPatch, path+"/status", `{"status":"processed"}`, bearer, http.StatusConflict)
		detail := request(t, http.MethodGet, path, "", bearer, http.StatusOK)
		var item models.IncomingRequest
		if err := json.Unmarshal([]byte(detail), &item); err != nil || item.Status != models.IncomingRequestStatusInProgress ||
			item.OwnerOperatorID == nil || *item.OwnerOperatorID != operator.ID || item.SessionID == nil {
			t.Fatalf("request detail omitted its assignment or session: %v", err)
		}
		request(t, http.MethodGet, "/api/incoming-requests?status=in_progress", "", bearer, http.StatusOK)
		request(t, http.MethodGet, "/api/incoming-requests?status=in_progress&scope=all", "", otherToken, http.StatusForbidden)
		request(t, http.MethodGet, "/api/incoming-requests?scope=all", "", bearer, http.StatusBadRequest)
		request(t, http.MethodGet, "/api/incoming-requests?status=in_progress&scope=all", "", bearer, http.StatusOK)
		transferBody := func(expected, next int64) string {
			return fmt.Sprintf(`{"expected_owner_id":%d,"operator_id":%d}`, expected, next)
		}
		request(t, http.MethodPost, path+"/transfer", transferBody(operator.ID, other.ID), otherToken, http.StatusForbidden)
		request(t, http.MethodPost, path+"/transfer", transferBody(operator.ID, other.ID), bearer, http.StatusNoContent)
		request(t, http.MethodPost, path+"/transfer", transferBody(operator.ID, other.ID), bearer, http.StatusNoContent)
		request(t, http.MethodGet, path, "", otherToken, http.StatusOK)
		sessionPath := fmt.Sprintf("/api/sessions/%d", *item.SessionID)
		request(t, http.MethodGet, sessionPath, "", otherToken, http.StatusOK)
		for _, tc := range []struct {
			role, want string
			telegramID int64
		}{
			{"client", "", 607},
			{"executor", "Fixture request details", 6080},
		} {
			issued := request(t, http.MethodPost, sessionPath+"/invites", fmt.Sprintf(`{"role":%q}`, tc.role), otherToken, http.StatusCreated)
			var invitation struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal([]byte(issued), &invitation); err != nil || invitation.Token == "" {
				t.Fatal("failed to issue test invitation")
			}
			bound := request(t, http.MethodPost, "/internal/sessions/bind", fmt.Sprintf(`{"telegram_id":%d,"token":%q}`, tc.telegramID, invitation.Token), "", http.StatusOK)
			var result struct {
				Role        string `json:"role"`
				RequestText string `json:"request_text"`
			}
			if err := json.Unmarshal([]byte(bound), &result); err != nil || result.Role != tc.role || result.RequestText != tc.want {
				t.Fatalf("unexpected invitation response for role %s", tc.role)
			}
		}

		next := models.Operator{Name: "Next operator", Login: "fixture-queue-next",
			Role: models.OperatorRoleOperator, Status: models.OperatorStatusActive, Language: "en"}
		if err := operators.Create(ctx, &next); err != nil {
			t.Fatal(err)
		}
		nextToken, err := access.Create(ctx, auth.SessionData{OperatorID: next.ID, Role: next.Role, AuthVersion: next.AuthVersion})
		if err != nil {
			t.Fatal(err)
		}
		request(t, http.MethodPost, path+"/transfer", transferBody(operator.ID, next.ID), bearer, http.StatusConflict)
		request(t, http.MethodPost, path+"/transfer", transferBody(other.ID, next.ID), bearer, http.StatusNoContent)
		request(t, http.MethodGet, path, "", otherToken, http.StatusForbidden)
		request(t, http.MethodGet, sessionPath, "", otherToken, http.StatusForbidden)
		request(t, http.MethodGet, path, "", nextToken, http.StatusOK)
		request(t, http.MethodGet, sessionPath, "", nextToken, http.StatusOK)
		action := "incoming_request_transferred"
		entries, err := audit.List(ctx, repository.AuditLogFilter{Action: &action})
		if err != nil || len(entries) != 2 || entries[0].TargetID == nil || *entries[0].TargetID != created.ID {
			t.Fatalf("transfer audit should record only completed ownership changes: %v", err)
		}
		request(t, http.MethodPatch, fmt.Sprintf("/api/operators/%d", next.ID), `{"status":"suspended"}`, bearer, http.StatusOK)
		request(t, http.MethodGet, path, "", nextToken, http.StatusUnauthorized)
		request(t, http.MethodPost, path+"/transfer", transferBody(next.ID, next.ID), bearer, http.StatusConflict)
	})

	if err := redis.Close(); err != nil {
		t.Fatal(err)
	}
	body := request(t, http.MethodPost, "/internal/messages/relay", `{"sender_telegram_id":601,"content_type":"text","content":"fixture@example.invalid"}`, "", http.StatusOK)
	if !strings.Contains(body, `"block_reason":"moderation_unavailable"`) || strings.Contains(body, `"message_id"`) {
		t.Fatal("message with a contact bypassed moderation after a Redis error")
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if res.Code != http.StatusServiceUnavailable || res.Body.String() != `{"postgres":"ok","redis":"error","status":"error"}` {
		t.Fatalf("unhealthy dependencies returned %d or unexpected body", res.Code)
	}
}
