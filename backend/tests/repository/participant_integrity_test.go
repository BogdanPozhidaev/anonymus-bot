package repository_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestParticipantIntegrityMigration(t *testing.T) {
	ctx := t.Context()
	pool := setupTestDB(t)
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsPath()), pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Migrate(10); err != nil {
		t.Fatal(err)
	}
	exec := func(query string) {
		t.Helper()
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	expectDenied := func(query, code string) {
		t.Helper()
		_, err := pool.Exec(ctx, query)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("expected SQLSTATE %s, got %v", code, err)
		}
	}
	exec(`
INSERT INTO users (id, telegram_id) VALUES (101, 501), (102, 502), (103, 503);
INSERT INTO operators (id, name, login, role) VALUES
    (101, 'Fixture owner', 'fixture-owner', 'operator'),
    (102, 'Fixture creator', 'fixture-creator', 'admin');
INSERT INTO sessions (id, title, client_user_id, executor_user_id, owner_operator_id, created_by)
VALUES (101, 'legacy', 101, 101, 101, 102);`)
	if err := m.Steps(1); err == nil || !strings.Contains(err.Error(), "assigned to both roles") {
		t.Fatalf("legacy conflict must block the migration: %v", err)
	}
	var executorID int64
	if err := pool.QueryRow(ctx, "SELECT executor_user_id FROM sessions WHERE id=101").Scan(&executorID); err != nil || executorID != 101 {
		t.Fatalf("legacy conflict must remain available for review: %v", err)
	}
	version, dirty, err := m.Version()
	if err != nil || version != 11 || !dirty {
		t.Fatalf("unexpected migration failure state: %d dirty=%v: %v", version, dirty, err)
	}
	exec("UPDATE sessions SET executor_user_id=102 WHERE id=101")
	if err := m.Force(10); err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(1); err != nil {
		t.Fatal(err)
	}
	expectDenied("UPDATE sessions SET executor_user_id=101 WHERE id=101", "23514")
	expectDenied("INSERT INTO sessions (title, client_user_id, executor_user_id) VALUES ('duplicate',101,101)", "23514")
	exec(`
INSERT INTO sessions (id, title, executor_user_id) VALUES (102, 'another session', 101);
UPDATE users SET active_session_id=102 WHERE id=101;
INSERT INTO session_invites (session_id, role, token_hash, expires_at)
VALUES (102, 'client', decode(repeat('01',32), 'hex'), now());
INSERT INTO messages (id, session_id, sender_user_id, sender_role, content_type)
VALUES (101, 101, 103, 'client', 'text');
INSERT INTO message_outbox (message_id) VALUES (101);`)
	expectDenied("DELETE FROM users WHERE id=101", "23503")
	expectDenied("DELETE FROM users WHERE id=103", "23503")
	expectDenied("DELETE FROM operators WHERE id=102", "23503")
	expectDenied("DELETE FROM sessions WHERE id=101", "23503")
	exec(`
DELETE FROM operators WHERE id=101;
DELETE FROM sessions WHERE id=102;
DELETE FROM messages WHERE id=101;`)
	var ownershipCleared, activeCleared, invitesRemoved, outboxRemoved bool
	err = pool.QueryRow(ctx, `SELECT (SELECT owner_operator_id IS NULL FROM sessions WHERE id=101),
    (SELECT active_session_id IS NULL FROM users WHERE id=101),
    NOT EXISTS (SELECT 1 FROM session_invites WHERE session_id=102),
    NOT EXISTS (SELECT 1 FROM message_outbox WHERE message_id=101)`).
		Scan(&ownershipCleared, &activeCleared, &invitesRemoved, &outboxRemoved)
	if err != nil || !ownershipCleared || !activeCleared || !invitesRemoved || !outboxRemoved {
		t.Fatalf("unexpected FK cleanup behavior: %v", err)
	}
	var indexes int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE schemaname='public' AND (
    indexname IN ('idx_users_active_session_id','idx_operators_created_by','idx_sessions_created_by',
                  'idx_sessions_close_requested_by','idx_messages_sender_user_id')
    OR (indexname='idx_audit_log_action_order' AND replace(indexdef, '"', '') LIKE '%(action, timestamp DESC, id DESC)%')
)`).Scan(&indexes)
	if err != nil || indexes != 6 {
		t.Fatalf("unexpected supporting indexes: %d: %v", indexes, err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE sessions SET executor_user_id=101 WHERE id=101")
}
