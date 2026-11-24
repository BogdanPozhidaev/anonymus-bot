package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

var (
	ErrIncomingRequestConflict  = errors.New("incoming request state conflict")
	ErrIncomingRequestForbidden = errors.New("incoming request access denied")
)

func (r *IncomingRequestRepository) Claim(ctx context.Context, id int64, actorID int64, role string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		state, owner, _, err := lockIncomingRequest(ctx, tx, id)
		if err != nil {
			return err
		}
		if state == models.IncomingRequestStatusInProgress && owner != nil && *owner == actorID {
			return nil
		}
		if state != models.IncomingRequestStatusNew {
			return ErrIncomingRequestConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE incoming_requests SET status='in_progress', owner_operator_id=$2 WHERE id=$1`, id, actorID); err != nil {
			return err
		}
		return auditIncomingRequest(ctx, tx, id, actorID, role, "incoming_request_claimed", nil)
	})
}

func (r *IncomingRequestRepository) OpenSession(ctx context.Context, id, actorID int64, role string, session *models.Session) (int64, bool, error) {
	var sessionID int64
	var created bool
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		state, owner, linked, err := lockIncomingRequest(ctx, tx, id)
		if err != nil {
			return err
		}
		if state != models.IncomingRequestStatusInProgress || owner == nil {
			return ErrIncomingRequestConflict
		}
		if role != models.OperatorRoleAdmin && *owner != actorID {
			return ErrIncomingRequestForbidden
		}
		if linked != nil {
			var sessionOwner *int64
			if err := tx.QueryRow(ctx, "SELECT owner_operator_id FROM sessions WHERE id=$1", *linked).Scan(&sessionOwner); err != nil {
				return err
			}
			if role != models.OperatorRoleAdmin && (sessionOwner == nil || *sessionOwner != actorID) {
				return ErrIncomingRequestForbidden
			}
			sessionID = *linked
			return nil
		}
		session.OwnerOperatorID = owner
		session.CreatedBy = &actorID
		if err := insertSession(ctx, tx.QueryRow, session); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE incoming_requests SET session_id=$2 WHERE id=$1", id, session.ID); err != nil {
			return err
		}
		sessionID, created = session.ID, true
		return auditIncomingRequest(ctx, tx, id, actorID, role, "incoming_request_session_created", nil)
	})
	return sessionID, created && err == nil, err
}

func (r *IncomingRequestRepository) Transfer(ctx context.Context, id, adminID, authVersion, expectedOwner, nextOwner int64) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		state, owner, linked, err := lockIncomingRequest(ctx, tx, id)
		if err != nil {
			return err
		}
		if state != models.IncomingRequestStatusInProgress || owner == nil || (*owner != expectedOwner && *owner != nextOwner) {
			return ErrIncomingRequestConflict
		}
		if linked != nil {
			var sessionOwner *int64
			if err := tx.QueryRow(ctx, "SELECT owner_operator_id FROM sessions WHERE id=$1 FOR UPDATE", *linked).Scan(&sessionOwner); err != nil {
				return err
			}
			if sessionOwner == nil || *sessionOwner != *owner {
				return ErrIncomingRequestConflict
			}
		}
		var operatorID int64
		err = tx.QueryRow(ctx, `SELECT id FROM operators
			WHERE id=$1 AND status='active' AND role='admin' AND auth_version=$2 FOR SHARE`,
			adminID, authVersion).Scan(&operatorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIncomingRequestForbidden
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, "SELECT id FROM operators WHERE id=$1 AND status='active' FOR SHARE", nextOwner).Scan(&operatorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIncomingRequestConflict
		}
		if err != nil {
			return err
		}
		if *owner == nextOwner {
			return nil
		}
		if linked != nil {
			if _, err := tx.Exec(ctx, "UPDATE sessions SET owner_operator_id=$2 WHERE id=$1", *linked, nextOwner); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE incoming_requests SET owner_operator_id=$2 WHERE id=$1", id, nextOwner); err != nil {
			return err
		}
		payload := fmt.Appendf(nil, `{"previous_owner_id":%d,"new_owner_id":%d}`, *owner, nextOwner)
		return auditIncomingRequest(ctx, tx, id, adminID, models.OperatorRoleAdmin, "incoming_request_transferred", payload)
	})
}

func lockIncomingRequest(ctx context.Context, tx pgx.Tx, id int64) (state string, owner, linked *int64, err error) {
	err = tx.QueryRow(ctx, `SELECT status, owner_operator_id, session_id FROM incoming_requests WHERE id=$1 FOR UPDATE`, id).
		Scan(&state, &owner, &linked)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

func auditIncomingRequest(ctx context.Context, tx pgx.Tx, id, actorID int64, role, action string, payload []byte) error {
	target := "incoming_request"
	return NewAuditLogRepository(tx).Create(ctx, &models.AuditLog{
		ActorID: &actorID, ActorType: "operator", ActorRole: &role,
		Action: action, TargetType: &target, TargetID: &id, Payload: payload,
	})
}
