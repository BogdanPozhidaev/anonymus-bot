package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *models.Session) error {
	return insertSession(ctx, r.pool.QueryRow, s)
}

func insertSession(ctx context.Context, queryRow func(context.Context, string, ...any) pgx.Row, s *models.Session) error {
	query := `
		INSERT INTO sessions (
			title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, payment_status, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	err := queryRow(ctx, query,
		s.Title,
		s.UserVisibleName,
		s.ClientUserID,
		s.ExecutorUserID,
		s.ClientDisplayName,
		s.ExecutorDisplayName,
		s.OwnerOperatorID,
		s.Status,
		s.Language,
		s.PaymentStatus,
		s.CreatedBy,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

func (r *SessionRepository) GetByID(ctx context.Context, id int64) (*models.Session, error) {
	query := `
		SELECT id, title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, close_requested_by, close_requested_at,
			close_reason, closed_at, payment_status, created_by, created_at
		FROM sessions
		WHERE id = $1
	`

	var s models.Session
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID,
		&s.Title,
		&s.UserVisibleName,
		&s.ClientUserID,
		&s.ExecutorUserID,
		&s.ClientDisplayName,
		&s.ExecutorDisplayName,
		&s.OwnerOperatorID,
		&s.Status,
		&s.Language,
		&s.CloseRequestedBy,
		&s.CloseRequestedAt,
		&s.CloseReason,
		&s.ClosedAt,
		&s.PaymentStatus,
		&s.CreatedBy,
		&s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get session by id: %w", err)
	}

	return &s, nil
}

func (r *SessionRepository) ListByOperator(ctx context.Context, operatorID int64) ([]*models.Session, error) {
	query := `
		SELECT id, title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, close_requested_by, close_requested_at,
			close_reason, closed_at, payment_status, created_by, created_at
		FROM sessions
		WHERE owner_operator_id = $1
		ORDER BY created_at DESC
	`

	return r.queryList(ctx, query, operatorID)
}

func (r *SessionRepository) ListAll(ctx context.Context) ([]*models.Session, error) {
	query := `
		SELECT id, title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, close_requested_by, close_requested_at,
			close_reason, closed_at, payment_status, created_by, created_at
		FROM sessions
		ORDER BY created_at DESC
	`

	return r.queryList(ctx, query)
}

func (r *SessionRepository) ListPanelPage(ctx context.Context, ownerID *int64, status, query string, limit, offset int) ([]*models.Session, error) {
	return r.queryList(ctx, `
		SELECT id, title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, close_requested_by, close_requested_at,
			close_reason, closed_at, payment_status, created_by, created_at
		FROM sessions
		WHERE ($1::bigint IS NULL OR owner_operator_id=$1)
			AND ($2='' OR status=$2)
			AND ($3='' OR strpos(lower(title), lower($3)) > 0 OR id::text=$3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5
	`, ownerID, status, query, limit, offset)
}

func (r *SessionRepository) queryList(ctx context.Context, query string, args ...interface{}) ([]*models.Session, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		var s models.Session
		err := rows.Scan(
			&s.ID,
			&s.Title,
			&s.UserVisibleName,
			&s.ClientUserID,
			&s.ExecutorUserID,
			&s.ClientDisplayName,
			&s.ExecutorDisplayName,
			&s.OwnerOperatorID,
			&s.Status,
			&s.Language,
			&s.CloseRequestedBy,
			&s.CloseRequestedAt,
			&s.CloseReason,
			&s.ClosedAt,
			&s.PaymentStatus,
			&s.CreatedBy,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session row: %w", err)
		}
		sessions = append(sessions, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating session rows: %w", err)
	}

	return sessions, nil
}

func (r *SessionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	var query string
	var args []interface{}

	if status == models.SessionStatusClosed {
		query = `UPDATE sessions SET status = $1, closed_at = now() WHERE id = $2`
		args = []interface{}{status, id}
	} else {
		query = `UPDATE sessions SET status = $1 WHERE id = $2`
		args = []interface{}{status, id}
	}

	cmdTag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *SessionRepository) TransitionStatus(ctx context.Context, id int64, previous, next string) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE sessions SET status = $3, closed_at = CASE WHEN $3 = 'closed' THEN now() ELSE closed_at END
		WHERE id = $1 AND status = $2
	`, id, previous, next)
	if err != nil {
		return false, fmt.Errorf("failed to transition session status: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *SessionRepository) AssignOwner(ctx context.Context, sessionID, operatorID int64) error {
	query := `UPDATE sessions SET owner_operator_id = $1 WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, operatorID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to assign session owner: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *SessionRepository) SetClientUser(ctx context.Context, sessionID, userID int64) error {
	query := `UPDATE sessions SET client_user_id = $1 WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, userID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to set client user: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *SessionRepository) SetExecutorUser(ctx context.Context, sessionID, userID int64) error {
	query := `UPDATE sessions SET executor_user_id = $1 WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, userID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to set executor user: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *SessionRepository) ListByParticipant(ctx context.Context, userID int64) ([]*models.Session, error) {
	return r.listByParticipant(ctx, userID, nil, nil, 0)
}

func (r *SessionRepository) ListByParticipantPage(ctx context.Context, userID int64, current *int64, limit, offset int) ([]*models.Session, error) {
	return r.listByParticipant(ctx, userID, current, &limit, offset)
}

func (r *SessionRepository) listByParticipant(ctx context.Context, userID int64, current *int64, limit *int, offset int) ([]*models.Session, error) {
	query := `
		SELECT id, title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, close_requested_by, close_requested_at,
			close_reason, closed_at, payment_status, created_by, created_at
		FROM sessions
		WHERE (client_user_id = $1 OR executor_user_id = $1)
			AND status != $2
		ORDER BY (id = $3) DESC NULLS LAST, created_at DESC, id DESC LIMIT $4 OFFSET $5
	`

	return r.queryList(ctx, query, userID, models.SessionStatusClosed, current, limit, offset)
}

func (r *SessionRepository) SetCloseRequestedBy(ctx context.Context, sessionID, userID int64) error {
	query := `UPDATE sessions SET close_requested_by = $1, close_requested_at = now() WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, userID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to set close_requested_by: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ListClosedBefore returns sessions that were closed before the cutoff.
func (r *SessionRepository) ListClosedBefore(ctx context.Context, threshold time.Time) ([]*models.Session, error) {
	query := `
		SELECT id, title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, close_requested_by, close_requested_at,
			close_reason, closed_at, payment_status, created_by, created_at
		FROM sessions
		WHERE status = $1 AND closed_at IS NOT NULL AND closed_at < $2
		ORDER BY closed_at ASC
	`

	return r.queryList(ctx, query, models.SessionStatusClosed, threshold)
}
