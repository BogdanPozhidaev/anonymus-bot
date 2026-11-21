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

type IncomingRequestRepository struct {
	pool *pgxpool.Pool
}

func (r *IncomingRequestRepository) PurgeBefore(ctx context.Context, threshold time.Time, dryRun bool) (int64, error) {
	if dryRun {
		var count int64
		err := r.pool.QueryRow(ctx, "SELECT count(*) FROM incoming_requests WHERE created_at < $1", threshold).Scan(&count)
		return count, err
	}
	result, err := r.pool.Exec(ctx, "DELETE FROM incoming_requests WHERE created_at < $1", threshold)
	return result.RowsAffected(), err
}

func NewIncomingRequestRepository(pool *pgxpool.Pool) *IncomingRequestRepository {
	return &IncomingRequestRepository{pool: pool}
}

func (r *IncomingRequestRepository) Create(ctx context.Context, ir *models.IncomingRequest) error {
	return insertIncomingRequest(ctx, r.pool.QueryRow, ir)
}

func insertIncomingRequest(ctx context.Context, queryRow func(context.Context, string, ...any) pgx.Row, ir *models.IncomingRequest) error {
	if ir.Status == "" {
		ir.Status = models.IncomingRequestStatusNew
	}

	query := `
		INSERT INTO incoming_requests (telegram_id, username, first_name, first_message_text, language_code, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err := queryRow(ctx, query,
		ir.TelegramID,
		ir.Username,
		ir.FirstName,
		ir.FirstMessageText,
		ir.LanguageCode,
		ir.Status,
	).Scan(&ir.ID, &ir.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create incoming request: %w", err)
	}

	return nil
}

func (r *IncomingRequestRepository) CreateOrReuse(ctx context.Context, ir *models.IncomingRequest) (bool, error) {
	created := false
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('incoming:' || $1::bigint::text, 0))`, ir.TelegramID); err != nil {
			return err
		}
		var status string
		err := tx.QueryRow(ctx, `
			SELECT id, status FROM incoming_requests
			WHERE telegram_id=$1 AND (status='new' OR created_at > now() - interval '10 minutes')
			ORDER BY (status='new') DESC, created_at DESC, id DESC LIMIT 1 FOR UPDATE
		`, ir.TelegramID).Scan(&ir.ID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			created = true
			return insertIncomingRequest(ctx, tx.QueryRow, ir)
		}
		if err != nil {
			return err
		}
		if status == models.IncomingRequestStatusNew && ir.FirstMessageText != nil {
			_, err = tx.Exec(ctx, `UPDATE incoming_requests SET first_message_text=$2 WHERE id=$1 AND first_message_text IS NULL`, ir.ID, ir.FirstMessageText)
		}
		return err
	})
	return created && err == nil, err
}

func (r *IncomingRequestRepository) GetByID(ctx context.Context, id int64) (*models.IncomingRequest, error) {
	query := `
		SELECT id, telegram_id, username, first_name, first_message_text, language_code, status, created_at
		FROM incoming_requests
		WHERE id = $1
	`

	var ir models.IncomingRequest
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&ir.ID,
		&ir.TelegramID,
		&ir.Username,
		&ir.FirstName,
		&ir.FirstMessageText,
		&ir.LanguageCode,
		&ir.Status,
		&ir.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get incoming request by id: %w", err)
	}

	return &ir, nil
}

func (r *IncomingRequestRepository) ListByStatus(ctx context.Context, status string) ([]*models.IncomingRequest, error) {
	return r.listByStatus(ctx, status, nil, 0)
}

func (r *IncomingRequestRepository) ListByStatusPage(ctx context.Context, status string, limit, offset int) ([]*models.IncomingRequest, error) {
	return r.listByStatus(ctx, status, &limit, offset)
}

func (r *IncomingRequestRepository) listByStatus(ctx context.Context, status string, limit *int, offset int) ([]*models.IncomingRequest, error) {
	query := `
		SELECT id, telegram_id, username, first_name, first_message_text, language_code, status, created_at
		FROM incoming_requests
		WHERE status = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list incoming requests: %w", err)
	}
	defer rows.Close()

	var requests []*models.IncomingRequest
	for rows.Next() {
		var ir models.IncomingRequest
		err := rows.Scan(
			&ir.ID,
			&ir.TelegramID,
			&ir.Username,
			&ir.FirstName,
			&ir.FirstMessageText,
			&ir.LanguageCode,
			&ir.Status,
			&ir.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan incoming request row: %w", err)
		}
		requests = append(requests, &ir)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating incoming request rows: %w", err)
	}

	return requests, nil
}

func (r *IncomingRequestRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	query := `UPDATE incoming_requests SET status = $1 WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update incoming request status: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
