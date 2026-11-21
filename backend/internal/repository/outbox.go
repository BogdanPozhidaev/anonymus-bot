package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type Delivery struct {
	MessageID, SessionID, RecipientTelegramID       int64
	SenderRole, ContentType, Content, FileID, Lease string
	Attempts                                        int
	Notice, CanNotify                               bool
}

func (r *MessageRepository) Enqueue(ctx context.Context, m *models.Message) error {
	m.Delivered = false
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := insertMessage(ctx, tx, m); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO message_outbox (message_id) VALUES ($1)", m.ID)
		return err
	})
}

// ClaimDelivery leases one item before network I/O. Unclaimed work remains
// available, and a crashed worker's lease expires.
func (r *MessageRepository) ClaimDelivery(ctx context.Context, lease time.Duration) (*Delivery, error) {
	const query = `
WITH candidate AS (
 SELECT o.message_id, m.session_id,
        CASE WHEN o.notice_pending THEN sender.telegram_id ELSE recipient.telegram_id END AS telegram_id,
        COALESCE(m.impersonated_role,m.sender_role) AS sender_role, m.content_type,
        COALESCE(m.content,'') AS content, COALESCE(m.file_id,'') AS file_id,
        CASE WHEN o.notice_pending THEN o.notice_attempts ELSE o.attempts END AS attempts,
        o.notice_pending, COALESCE(sender.telegram_id > 0,false) AS can_notify
 FROM message_outbox o JOIN messages m ON m.id=o.message_id
 JOIN sessions s ON s.id=m.session_id
 LEFT JOIN users sender ON sender.id=m.sender_user_id
 LEFT JOIN users recipient ON recipient.id=CASE COALESCE(m.impersonated_role,m.sender_role)
   WHEN 'client' THEN s.executor_user_id WHEN 'executor' THEN s.client_user_id END
 WHERE o.available_at <= clock_timestamp()
 AND ((o.notice_pending AND sender.telegram_id > 0)
 OR (o.status IN ('queued','retrying') AND s.status='active' AND recipient.telegram_id > 0
 AND (m.sent_by_operator OR (m.sender_user_id = CASE m.sender_role WHEN 'client' THEN s.client_user_id WHEN 'executor' THEN s.executor_user_id END))))
 ORDER BY o.available_at, o.message_id
 FOR UPDATE OF o SKIP LOCKED LIMIT 1
)
UPDATE message_outbox o SET lease_token=$1, available_at=clock_timestamp()+($2::bigint * interval '1 millisecond')
FROM candidate c WHERE o.message_id=c.message_id
RETURNING c.message_id,c.session_id,c.telegram_id,c.sender_role,c.content_type,c.content,c.file_id,c.attempts,c.notice_pending,c.can_notify`
	d := &Delivery{Lease: uuid.NewString()}
	err := r.pool.QueryRow(ctx, query, d.Lease, lease.Milliseconds()).Scan(&d.MessageID, &d.SessionID, &d.RecipientTelegramID, &d.SenderRole, &d.ContentType, &d.Content, &d.FileID, &d.Attempts, &d.Notice, &d.CanNotify)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *MessageRepository) CompleteDelivery(ctx context.Context, d *Delivery, success bool, retry time.Duration, maxAttempts int) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		attempts := d.Attempts + 1
		terminal := attempts >= maxAttempts
		if d.Notice {
			_, err := tx.Exec(ctx, `UPDATE message_outbox SET notice_attempts=$3, notice_pending=$4, lease_token=NULL,
     available_at=clock_timestamp()+($5::bigint * interval '1 millisecond') WHERE message_id=$1 AND lease_token=$2`,
				d.MessageID, d.Lease, attempts, !success && !terminal, retry.Milliseconds())
			return err
		}
		status := "retrying"
		if success {
			status = "delivered"
		} else if terminal {
			status = "failed"
			retry = 0
		}
		result, err := tx.Exec(ctx, `UPDATE message_outbox SET status=$3, attempts=$4, notice_pending=$5, lease_token=NULL,
     available_at=clock_timestamp()+($6::bigint * interval '1 millisecond') WHERE message_id=$1 AND lease_token=$2`,
			d.MessageID, d.Lease, status, attempts, !success && terminal && d.CanNotify, retry.Milliseconds())
		if err != nil || !success || result.RowsAffected() == 0 {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE messages SET delivered=true WHERE id=$1", d.MessageID)
		return err
	})
}
