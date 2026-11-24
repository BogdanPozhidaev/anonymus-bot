package repository

import (
	"context"
	"errors"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrOperatorDeliveryForbidden = errors.New("operator delivery access denied")
	ErrOperatorDeliveryConflict  = errors.New("operator delivery context changed")
)

type OperatorDeliveryTarget struct {
	OperatorID      int64
	AuthVersion     int64
	RecipientUserID int64
	RequestKey      *uuid.UUID
}

// EnqueueOperatorDelivery checks the current session and participant under
// locks, then saves the proposed recipient and the message in one transaction.
func (r *MessageRepository) EnqueueOperatorDelivery(ctx context.Context, m *models.Message, target OperatorDeliveryTarget) (bool, error) {
	saved := false
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if target.RequestKey != nil {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('operator-delivery:' || $1::bigint::text || ':' || $2::uuid::text, 0))`,
				target.OperatorID, target.RequestKey); err != nil {
				return err
			}
		}

		var status string
		var owner, recipient *int64
		err := tx.QueryRow(ctx, `SELECT status, owner_operator_id,
			CASE WHEN $2='client' THEN executor_user_id ELSE client_user_id END
			FROM sessions WHERE id=$1 FOR UPDATE`, m.SessionID, *m.ImpersonatedRole).Scan(&status, &owner, &recipient)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var role string
		err = tx.QueryRow(ctx, `SELECT role FROM operators
			WHERE id=$1 AND status='active' AND auth_version=$2 FOR SHARE`,
			target.OperatorID, target.AuthVersion).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOperatorDeliveryForbidden
		}
		if err != nil {
			return err
		}
		if role != models.OperatorRoleAdmin && (owner == nil || *owner != target.OperatorID) {
			return ErrOperatorDeliveryForbidden
		}
		if recipient == nil || *recipient != target.RecipientUserID || status != models.SessionStatusActive {
			return ErrOperatorDeliveryConflict
		}

		if target.RequestKey != nil {
			var priorID, priorSession, priorRecipient int64
			var priorRole, priorContent string
			err := tx.QueryRow(ctx, `SELECT m.id, m.session_id, o.recipient_user_id, m.impersonated_role, m.content
				FROM message_outbox o JOIN messages m ON m.id=o.message_id
				WHERE o.operator_id=$1 AND o.request_key=$2`, target.OperatorID, target.RequestKey).
				Scan(&priorID, &priorSession, &priorRecipient, &priorRole, &priorContent)
			if err == nil {
				if priorSession != m.SessionID || priorRecipient != target.RecipientUserID ||
					priorRole != *m.ImpersonatedRole || priorContent != *m.Content {
					return ErrOperatorDeliveryConflict
				}
				m.ID = priorID
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if err := insertMessage(ctx, tx, m); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO message_outbox
			(message_id, recipient_user_id, operator_id, operator_auth_version, request_key) VALUES ($1,$2,$3,$4,$5)`,
			m.ID, target.RecipientUserID, target.OperatorID, target.AuthVersion, target.RequestKey)
		saved = err == nil
		return err
	})
	return saved && err == nil, err
}
