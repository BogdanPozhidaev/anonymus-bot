package repository

import (
	"context"
	"fmt"
	"time"

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

func (r *MessageRepository) ListUndelivered(ctx context.Context, sessionID int64) ([]*models.Message, error) {
	query := `
		SELECT id, session_id, sender_user_id, sender_role, content_type,
			content, file_id, internal_file_path, sent_by_operator, impersonated_role, delivered, created_at
		FROM messages
		WHERE session_id = $1 AND delivered = false
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list undelivered messages: %w", err)
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
			&m.Delivered,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}
		messages = append(messages, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating undelivered message rows: %w", err)
	}

	return messages, nil
}

func (r *MessageRepository) MarkDelivered(ctx context.Context, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}

	query := `UPDATE messages SET delivered = true WHERE id = ANY($1)`

	_, err := r.pool.Exec(ctx, query, messageIDs)
	if err != nil {
		return fmt.Errorf("failed to mark messages delivered: %w", err)
	}

	return nil
}

func (r *MessageRepository) DeleteBySessionID(ctx context.Context, sessionID int64) error {
	query := `DELETE FROM messages WHERE session_id = $1`
	_, err := r.pool.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete messages for session %d: %w", sessionID, err)
	}
	return nil
}

// ListForDeletion возвращает сообщения сессий, закрытых раньше threshold —
// кандидаты на удаление согласно retention-политике (слой 2: сообщения/медиа).
func (r *MessageRepository) ListForDeletion(ctx context.Context, threshold time.Time) ([]*models.Message, error) {
	query := `
		SELECT m.id, m.session_id, m.sender_user_id, m.sender_role, m.content_type,
			m.content, m.file_id, m.internal_file_path, m.sent_by_operator,
			m.impersonated_role, m.delivered, m.created_at
		FROM messages m
		INNER JOIN sessions s ON s.id = m.session_id
		WHERE s.status = $1 AND s.closed_at IS NOT NULL AND s.closed_at < $2
		ORDER BY m.id ASC
	`

	rows, err := r.pool.Query(ctx, query, models.SessionStatusClosed, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages for deletion: %w", err)
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
			&m.Delivered,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}
		messages = append(messages, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows for deletion: %w", err)
	}

	return messages, nil
}

// DeleteByIDs необратимо удаляет записи сообщений по списку ID.
func (r *MessageRepository) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	query := `DELETE FROM messages WHERE id = ANY($1)`

	_, err := r.pool.Exec(ctx, query, ids)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	return nil
}
