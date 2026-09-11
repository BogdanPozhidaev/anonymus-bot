package repository

import (
	"context"
	"errors"
	"fmt"

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
	query := `
		INSERT INTO sessions (
			title, user_visible_name, client_user_id, executor_user_id,
			client_display_name, executor_display_name, owner_operator_id,
			status, language, payment_status, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
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
			close_reason, payment_status, created_by, created_at
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
			close_reason, payment_status, created_by, created_at
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
			close_reason, payment_status, created_by, created_at
		FROM sessions
		ORDER BY created_at DESC
	`

	return r.queryList(ctx, query)
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
	query := `UPDATE sessions SET status = $1 WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
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
