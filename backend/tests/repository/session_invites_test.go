package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestSessionInvitationLifecycle(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	sessions := repository.NewSessionRepository(pool)
	users := repository.NewUserRepository(pool, nil)
	const adminID = 1
	var sessionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sessions (title,status) VALUES ('fixture','active') RETURNING id`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	token := func(value byte) []byte { return bytes.Repeat([]byte{value}, 32) }
	issue := func(role string, value byte) {
		t.Helper()
		expiry, err := sessions.ManageInvite(ctx, sessionID, role, adminID, true, token(value))
		if err != nil || time.Until(expiry) < 23*time.Hour {
			t.Fatalf("issue failed: %v", err)
		}
	}
	bind := func(value byte, id int64) error {
		_, _, err := sessions.BindInvite(ctx, users, token(value), &models.User{TelegramID: id, LanguageCode: "ru"})
		return err
	}
	reject := func(value byte, id int64) {
		t.Helper()
		if err := bind(value, id); !errors.Is(err, repository.ErrInviteUnavailable) {
			t.Fatalf("expected unavailable: %v", err)
		}
	}
	if _, err := sessions.ManageInvite(ctx, sessionID, "client", adminID, false, token(1)); !errors.Is(err, repository.ErrInviteForbidden) {
		t.Fatalf("expected forbidden: %v", err)
	}
	issue("client", 1)
	issue("client", 2)
	reject(1, 100)
	if _, err := sessions.ManageInvite(ctx, sessionID, "client", adminID, true, nil); err != nil {
		t.Fatal(err)
	}
	reject(2, 100)
	issue("client", 3)
	if _, err := pool.Exec(ctx, `UPDATE session_invites SET expires_at=clock_timestamp()-interval '1 second' WHERE session_id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	reject(3, 100)
	issue("client", 4)
	for _, status := range []string{"paused", "pending_close", "closed"} {
		if err := sessions.UpdateStatus(ctx, sessionID, status); err != nil {
			t.Fatal(err)
		}
		reject(4, 100)
	}
	if err := sessions.UpdateStatus(ctx, sessionID, "active"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []int64{101, 102} {
		wg.Add(1)
		go func() { defer wg.Done(); results <- bind(4, id) }()
	}
	wg.Wait()
	close(results)
	var accepted, rejected int
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, repository.ErrInviteUnavailable):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted/rejected = %d/%d", accepted, rejected)
	}
	var winner int64
	var active int64
	if err := pool.QueryRow(ctx, `SELECT telegram_id, active_session_id FROM users JOIN sessions ON users.id=sessions.client_user_id WHERE sessions.id=$1`, sessionID).Scan(&winner, &active); err != nil {
		t.Fatal(err)
	}
	if active != sessionID {
		t.Fatal("winner active session was not updated")
	}
	var userCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil || userCount != 1 {
		t.Fatalf("rejected bind changed users: count=%d err=%v", userCount, err)
	}
	reject(4, winner)
	if _, err := sessions.ManageInvite(ctx, sessionID, "client", adminID, true, token(5)); !errors.Is(err, repository.ErrInviteUnavailable) {
		t.Fatalf("expected occupied slot rejection: %v", err)
	}
	issue("executor", 6)
	reject(6, winner)
	if err := bind(6, 103); err != nil {
		t.Fatalf("dual-role rejection consumed the token: %v", err)
	}

	if err := pool.QueryRow(ctx, `INSERT INTO sessions (title) VALUES ('fixture') RETURNING id`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (session_id,sender_role,content_type,content,delivered)
		VALUES ($1,'executor','text','test pending message',false), ($1,'client','text','other direction',false)`, sessionID); err != nil {
		t.Fatal(err)
	}
	raw := bytes.Repeat([]byte{7}, 32)
	hash := sha256.Sum256(raw)
	if _, err := sessions.ManageInvite(ctx, sessionID, "client", adminID, true, hash[:]); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	server := api.NewServer(pool, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), users, sessions,
		nil, nil, repository.NewMessageRepository(pool), nil, nil, nil, nil, nil, "", "")
	serviceToken := strings.Repeat("a", 64)
	server.RegisterRoutes(router, serviceToken)
	body, err := json.Marshal(map[string]any{"telegram_id": 104, "token": base64.RawURLEncoding.EncodeToString(raw), "language_code": "en"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{http.StatusOK, http.StatusNotFound} {
		req := httptest.NewRequest(http.MethodPost, "/internal/sessions/bind", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", serviceToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != want || strings.Contains(response.Body.String(), "test pending message") ||
			strings.Contains(response.Body.String(), "other direction") {
			t.Fatalf("unexpected bind response, status = %d, want %d", response.Code, want)
		}
	}
	bound, err := users.GetByTelegramID(ctx, 104)
	if err != nil || bound.LanguageCode != "en" {
		t.Fatalf("bind did not persist the new user's language: %v", err)
	}
}
