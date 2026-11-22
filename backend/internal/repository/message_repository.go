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

type MessageRepository struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

func (r *MessageRepository) Create(ctx context.Context, m *models.Message) error {
	return insertMessage(ctx, r.pool, m)
}

type messageInserter interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertMessage(ctx context.Context, db messageInserter, m *models.Message) error {
	query := `
		INSERT INTO messages (
			session_id, sender_user_id, sender_role, content_type,
			content, file_id, sent_by_operator, impersonated_role, delivered
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`

	err := db.QueryRow(ctx, query,
		m.SessionID,
		m.SenderUserID,
		m.SenderRole,
		m.ContentType,
		m.Content,
		m.FileID,
		m.SentByOperator,
		m.ImpersonatedRole,
		m.Delivered,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	return nil
}

func (r *MessageRepository) ListBySessionID(ctx context.Context, sessionID int64) ([]*models.Message, error) {
	return r.listBySessionID(ctx, "ORDER BY m.created_at ASC, m.id ASC", sessionID)
}

func (r *MessageRepository) ListRecentBySessionID(ctx context.Context, sessionID int64, limit, offset int) ([]*models.Message, error) {
	return r.listBySessionID(ctx, "ORDER BY m.created_at DESC, m.id DESC LIMIT $2 OFFSET $3", sessionID, limit, offset)
}

var ErrTranscriptTooLarge = errors.New("transcript exceeds export limits")

func (r *MessageRepository) ListTranscript(ctx context.Context, sessionID int64, maxMessages, maxTextBytes int) ([]models.TranscriptEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.created_at, m.sender_role, m.content_type, COALESCE(o.status, 'legacy_unknown'),
			m.sent_by_operator, COALESCE(m.impersonated_role, ''),
			COALESCE(CASE WHEN octet_length(m.content) <= $3 THEN m.content END, ''),
			COALESCE(octet_length(m.content), 0)
		FROM messages m LEFT JOIN message_outbox o ON o.message_id=m.id
		WHERE m.session_id=$1 ORDER BY m.created_at ASC, m.id ASC LIMIT $2`,
		sessionID, maxMessages+1, maxTextBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.TranscriptEntry
	textBytes := 0
	for rows.Next() {
		var entry models.TranscriptEntry
		var size int
		if err := rows.Scan(&entry.CreatedAt, &entry.SenderRole, &entry.ContentType, &entry.DeliveryStatus,
			&entry.SentByOperator, &entry.ImpersonatedRole, &entry.Content, &size); err != nil {
			return nil, err
		}
		if len(entries) == maxMessages || size > maxTextBytes-textBytes {
			return nil, ErrTranscriptTooLarge
		}
		textBytes += size
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *MessageRepository) listBySessionID(ctx context.Context, order string, args ...any) ([]*models.Message, error) {
	query := `
		SELECT m.id, m.session_id, m.sender_user_id, m.sender_role, m.content_type,
			m.content, m.file_id, m.sent_by_operator, m.impersonated_role, m.created_at, m.delivered,
            COALESCE(o.status, 'legacy_unknown')
		FROM messages m LEFT JOIN message_outbox o ON o.message_id=m.id
		WHERE m.session_id = $1
	` + order

	rows, err := r.pool.Query(ctx, query, args...)
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
			&m.SentByOperator,
			&m.ImpersonatedRole,
			&m.CreatedAt,
			&m.Delivered,
			&m.DeliveryStatus,
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

// ListForDeletion returns retention candidates from sessions closed before threshold.
func (r *MessageRepository) ListForDeletion(ctx context.Context, threshold time.Time) ([]*models.Message, error) {
	query := `
		SELECT m.id, m.session_id, m.sender_user_id, m.sender_role, m.content_type,
			m.content, m.file_id, m.sent_by_operator,
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

// DeleteExpired locks the session through file and row cleanup so a concurrent
// reopen cannot make a selected message ineligible while it is being removed.
func (r *MessageRepository) DeleteExpired(ctx context.Context, message *models.Message, threshold time.Time, beforeDelete func() error) (bool, error) {
	deleted := false
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, `SELECT id FROM sessions
			WHERE id=$1 AND status='closed' AND closed_at < $2 FOR NO KEY UPDATE`, message.SessionID, threshold).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, "SELECT id FROM messages WHERE id=$1 AND session_id=$2 FOR UPDATE", message.ID, message.SessionID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		shared := false
		if message.FileID != nil {
			// Legacy messages can share a reference. Serialize cleanup per file
			// and unlink only after all other references have been removed.
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",
				message.ContentType+":"+*message.FileID); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM messages WHERE content_type = $1 AND file_id = $2 AND id <> $3
			)`, message.ContentType, *message.FileID, message.ID).Scan(&shared); err != nil {
				return err
			}
		}
		if !shared {
			if err := beforeDelete(); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "DELETE FROM messages WHERE id=$1", message.ID)
		deleted = err == nil
		return err
	})
	return deleted && err == nil, err
}

func (r *MessageRepository) GetByID(ctx context.Context, id int64) (*models.Message, error) {
	query := `
		SELECT id, session_id, sender_user_id, sender_role, content_type,
			content, file_id, sent_by_operator,
			impersonated_role, delivered, created_at
		FROM messages
		WHERE id = $1
	`

	var m models.Message
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&m.ID,
		&m.SessionID,
		&m.SenderUserID,
		&m.SenderRole,
		&m.ContentType,
		&m.Content,
		&m.FileID,
		&m.SentByOperator,
		&m.ImpersonatedRole,
		&m.Delivered,
		&m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get message by id: %w", err)
	}

	return &m, nil
}
