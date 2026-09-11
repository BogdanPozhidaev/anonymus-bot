package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type MessageRepository struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

func (r *MessageRepository) Create(ctx context.Context, m *models.Message) error {
	query := `
		INSERT INTO messages (
			session_id, sender_user_id, sender_role, content_type,
			content, file_id, internal_file_path, sent_by_operator, impersonated_role
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
		m.SessionID,
		m.SenderUserID,
		m.SenderRole,
		m.ContentType,
		m.Content,
		m.FileID,
		m.InternalFilePath,
		m.SentByOperator,
		m.ImpersonatedRole,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	return nil
}

func (r *MessageRepository) ListBySessionID(ctx context.Context, sessionID int64) ([]*models.Message, error) {
	query := `
		SELECT id, session_id, sender_user_id, sender_role, content_type,
			content, file_id, internal_file_path, sent_by_operator, impersonated_role, created_at
		FROM messages
		WHERE session_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}
	defer rows.Close()

	var messages []*models.Message
	for rows.Next() {
		var m models.Message
		err := rows.Scan(
			&m.ID,
			&m.SessionID,
			&m.SenderUserID,
			&m.SenderRole,
			&m.ContentType,
			&m.Content,
			&m.FileID,
			&m.InternalFilePath,
			&m.SentByOperator,
			&m.ImpersonatedRole,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}
		messages = append(messages, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows: %w", err)
	}

	return messages, nil
}
