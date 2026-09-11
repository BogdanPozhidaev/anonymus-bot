package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type IncomingRequestRepository struct {
	pool *pgxpool.Pool
}

func NewIncomingRequestRepository(pool *pgxpool.Pool) *IncomingRequestRepository {
	return &IncomingRequestRepository{pool: pool}
}

func (r *IncomingRequestRepository) Create(ctx context.Context, ir *models.IncomingRequest) error {
	if ir.Status == "" {
		ir.Status = models.IncomingRequestStatusNew
	}

	query := `
		INSERT INTO incoming_requests (telegram_id, username, first_name, first_message_text, language_code, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
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
	query := `
		SELECT id, telegram_id, username, first_name, first_message_text, language_code, status, created_at
		FROM incoming_requests
		WHERE status = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, status)
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
