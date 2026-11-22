package repository_test

import (
	"bytes"
	"encoding/csv"
	"fmt"
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

func TestSessionTranscriptAccessContentAndLimits(t *testing.T) {
	ctx := t.Context()
	pool := setupTestDB(t)
	redis := setupThrottleRedis(t)
	sessions := repository.NewSessionRepository(pool)
	operators := repository.NewOperatorRepository(pool)
	messages := repository.NewMessageRepository(pool)
	audit := repository.NewAuditLogRepository(pool)
	authSessions := auth.NewSessionService(redis)
	operator := func(login, role string) (int64, string) {
		t.Helper()
		op := &models.Operator{Name: login, Login: login, Role: role, Status: "active", Language: "en"}
		if err := operators.Create(ctx, op); err != nil {
			t.Fatal(err)
		}
		token, err := authSessions.Create(ctx, auth.SessionData{OperatorID: op.ID, AuthVersion: op.AuthVersion, Role: role})
		if err != nil {
			t.Fatal(err)
		}
		return op.ID, token
	}
	ownerID, owner := operator("transcript-owner", "operator")
	_, other := operator("transcript-other", "operator")
	_, admin := operator("transcript-admin", "admin")
	session := &models.Session{Title: "Private fixture title", Status: "active", Language: "ru", OwnerOperatorID: &ownerID}
	if err := sessions.Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	private := &models.Session{Title: "Other fixture", Status: "active", Language: "en"}
	if err := sessions.Create(ctx, private); err != nil {
		t.Fatal(err)
	}
	body := "Привет, мир"
	caption := "  =2+2"
	otherText := "fixture-from-another-session"
	fileID := "38c45a07-ff42-40ce-92f5-4f6e03dfaf22"
	role := "executor"
	user := &models.User{TelegramID: 99881501, LanguageCode: "ru"}
	if err := repository.NewUserRepository(pool, nil).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	for _, message := range []*models.Message{
		{SessionID: session.ID, SenderUserID: &user.ID, SenderRole: "client", ContentType: "text", Content: &body},
		{SessionID: session.ID, SenderRole: "operator", ContentType: "photo", Content: &caption, FileID: &fileID, SentByOperator: true, ImpersonatedRole: &role},
		{SessionID: private.ID, SenderRole: "client", ContentType: "text", Content: &otherText},
	} {
		if err := messages.Enqueue(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE messages SET created_at='2026-01-02 00:00:00+00' WHERE session_id=$1", session.ID); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, sessions, nil, operators, messages, audit,
		nil, authSessions, nil, nil, "", "").RegisterRoutes(router, strings.Repeat("a", 64))
	base := fmt.Sprintf("/api/sessions/%d/transcript", session.ID)
	request := func(path, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("GET %s returned %d, want %d: %s", path, response.Code, status, response.Body.String())
		}
		if status != http.StatusOK && response.Header().Get("Content-Disposition") != "" {
			t.Fatal("failed export sent an attachment")
		}
		return response
	}
	for _, format := range []string{"csv", "pdf"} {
		request(base+"?format="+format, "", http.StatusUnauthorized)
		request(base+"?format="+format, other, http.StatusForbidden)
	}
	request(base+"?format=html", owner, http.StatusBadRequest)
	request("/api/sessions/999999/transcript?format=csv", admin, http.StatusNotFound)
	csvResponse := request(base+"?format=csv", owner, http.StatusOK)
	if csvResponse.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		csvResponse.Header().Get("Content-Disposition") != fmt.Sprintf("attachment; filename=session-%d.csv", session.ID) ||
		csvResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("CSV response headers do not force a private download")
	}
	csvBody := csvResponse.Body.String()
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(csvBody, "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 3 || rows[1][6] != body || rows[1][3] != "queued" ||
		rows[2][6] != "'"+caption || rows[2][2] != "photo" || rows[2][4] != "true" || rows[2][5] != role {
		t.Fatalf("unexpected CSV fields or order: %v", err)
	}
	for _, excluded := range []string{otherText, fileID, session.Title, "telegram_id", "sender_user_id", strconv.FormatInt(user.TelegramID, 10)} {
		if strings.Contains(csvBody, excluded) {
			t.Fatalf("unexpected field in CSV: %q", excluded)
		}
	}
	pdfResponse := request(base+"?format=pdf", admin, http.StatusOK)
	if pdfResponse.Header().Get("Content-Disposition") != fmt.Sprintf("attachment; filename=session-%d.pdf", session.ID) ||
		pdfResponse.Header().Get("Content-Type") != "application/pdf" || pdfResponse.Header().Get("Cache-Control") != "no-store" ||
		!bytes.HasPrefix(pdfResponse.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("PDF response is not a private PDF attachment")
	}
	action := "session_transcript_exported"
	audits, err := audit.List(ctx, repository.AuditLogFilter{Action: &action})
	if err != nil || len(audits) != 2 || !strings.Contains(string(audits[0].Payload), `"pdf"`) ||
		!strings.Contains(string(audits[1].Payload), `"csv"`) || strings.Contains(string(audits[1].Payload), body) {
		t.Fatalf("unexpected audit events: %v", err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("ALTER TABLE audit_log ADD CONSTRAINT fixture_no_export_audit CHECK (action <> 'session_transcript_exported') NOT VALID")
	request(base+"?format=csv", owner, http.StatusServiceUnavailable)
	exec("UPDATE messages SET content=repeat('x', 1048577) WHERE session_id=$1 AND content_type='text'", session.ID)
	request(base+"?format=csv", owner, http.StatusUnprocessableEntity)
	exec("UPDATE messages SET content=NULL WHERE session_id=$1", session.ID)
	exec("INSERT INTO messages (session_id, sender_role, content_type) SELECT $1, 'client', 'text' FROM generate_series(1,1999)", session.ID)
	request(base+"?format=csv", owner, http.StatusUnprocessableEntity)
}
