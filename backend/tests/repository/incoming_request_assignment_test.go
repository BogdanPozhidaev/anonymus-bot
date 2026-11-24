package repository_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/golang-migrate/migrate/v4"
)

func TestIncomingRequestAssignmentMigrationPreservesLegacyRows(t *testing.T) {
	ctx := t.Context()
	pool := setupTestDB(t)
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsPath()), pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Migrate(11); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO incoming_requests (telegram_id, status) VALUES (9101, 'new'), (9102, 'processed')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(1); err != nil {
		t.Fatal(err)
	}
	var statuses string
	var assigned int
	err = pool.QueryRow(ctx, `SELECT string_agg(status, ',' ORDER BY telegram_id),
		count(*) FILTER (WHERE owner_operator_id IS NOT NULL OR session_id IS NOT NULL)
		FROM incoming_requests`).Scan(&statuses, &assigned)
	if err != nil || statuses != "new,processed" || assigned != 0 {
		t.Fatalf("legacy requests changed: statuses=%q assigned=%d err=%v", statuses, assigned, err)
	}
}

func TestIncomingRequestAssignment(t *testing.T) {
	ctx := t.Context()
	pool := setupTestDB(t)
	requests := repository.NewIncomingRequestRepository(pool)
	operators := repository.NewOperatorRepository(pool)
	first := models.Operator{Name: "First", Login: "assignment-first", Role: models.OperatorRoleOperator, Status: models.OperatorStatusActive}
	second := models.Operator{Name: "Second", Login: "assignment-second", Role: models.OperatorRoleOperator, Status: models.OperatorStatusActive}
	for _, operator := range []*models.Operator{&first, &second} {
		operator.Language = "en"
		if err := operators.Create(ctx, operator); err != nil {
			t.Fatal(err)
		}
	}
	incoming := models.IncomingRequest{TelegramID: 9001}
	if err := requests.Create(ctx, &incoming); err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		actorID int64
		err     error
	}
	claims := make(chan claimResult, 2)
	start := make(chan struct{})
	for _, actor := range []int64{first.ID, second.ID} {
		go func() {
			<-start
			claims <- claimResult{actor, requests.Claim(ctx, incoming.ID, actor, models.OperatorRoleOperator)}
		}()
	}
	close(start)
	winner, loser := <-claims, <-claims
	if winner.err != nil {
		winner, loser = loser, winner
	}
	if winner.err != nil || !errors.Is(loser.err, repository.ErrIncomingRequestConflict) {
		t.Fatalf("expected one assignment and one conflict: %v, %v", winner.err, loser.err)
	}
	if err := requests.Claim(ctx, incoming.ID, winner.actorID, models.OperatorRoleOperator); err != nil {
		t.Fatal(err)
	}
	if err := requests.UpdateStatus(ctx, incoming.ID, models.IncomingRequestStatusProcessed); !errors.Is(err, repository.ErrIncomingRequestConflict) {
		t.Fatalf("legacy status update should reject assignment: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE incoming_requests SET created_at=now()-interval '1 day' WHERE id=$1", incoming.ID); err != nil {
		t.Fatal(err)
	}
	message := "Fixture first message"
	repeat := models.IncomingRequest{TelegramID: incoming.TelegramID, FirstMessageText: &message}
	if created, err := requests.CreateOrReuse(ctx, &repeat); err != nil || created || repeat.ID != incoming.ID {
		t.Fatalf("expected assigned request reuse: created=%t, err=%v", created, err)
	}
	mine, err := requests.ListAssignedPage(ctx, winner.actorID, 10, 0)
	if err != nil || len(mine) != 1 || mine[0].FirstMessageText == nil || *mine[0].FirstMessageText != message {
		t.Fatalf("assigned request or first message missing: %v", err)
	}
	other, err := requests.ListAssignedPage(ctx, loser.actorID, 10, 0)
	if err != nil || len(other) != 0 {
		t.Fatalf("another operator could list the assigned request: %v", err)
	}
	fixtureSession := func() *models.Session {
		return &models.Session{Title: "Fixture assignment", Status: models.SessionStatusActive, Language: "en"}
	}
	if _, _, err := requests.OpenSession(ctx, incoming.ID, loser.actorID, models.OperatorRoleOperator, fixtureSession()); !errors.Is(err, repository.ErrIncomingRequestForbidden) {
		t.Fatalf("another operator could create a session: %v", err)
	}

	if _, err := pool.Exec(ctx, `ALTER TABLE audit_log ADD CONSTRAINT fixture_reject_audit CHECK (action <> 'incoming_request_session_created')`); err != nil {
		t.Fatal(err)
	}
	if _, created, err := requests.OpenSession(ctx, incoming.ID, winner.actorID, models.OperatorRoleOperator, fixtureSession()); err == nil || created {
		t.Fatal("audit rejection should roll back session creation")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatalf("audit rejection left a session: count=%d, err=%v", count, err)
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE audit_log DROP CONSTRAINT fixture_reject_audit"); err != nil {
		t.Fatal(err)
	}

	type sessionResult struct {
		id      int64
		created bool
		err     error
	}
	results := make(chan sessionResult, 2)
	for range 2 {
		go func() {
			id, created, err := requests.OpenSession(ctx, incoming.ID, winner.actorID, models.OperatorRoleOperator, fixtureSession())
			results <- sessionResult{id, created, err}
		}()
	}
	left, right := <-results, <-results
	if left.err != nil || right.err != nil || left.id == 0 || left.id != right.id || left.created == right.created {
		t.Fatalf("expected one session and one repeat: %+v, %+v", left, right)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE target_type='incoming_request' AND target_id=$1", incoming.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("expected one audit event per mutation: count=%d, err=%v", count, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM sessions WHERE id=$1", left.id); err == nil {
		t.Fatal("linked session should be protected from deletion")
	}
	if count, err := requests.PurgeBefore(ctx, time.Now().Add(-time.Hour), false); err != nil || count != 1 {
		t.Fatalf("expected request to expire: count=%d, err=%v", count, err)
	}
	if _, err := repository.NewSessionRepository(pool).GetByID(ctx, left.id); err != nil {
		t.Fatalf("request retention removed the linked session: %v", err)
	}
}
