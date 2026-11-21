package repository

import (
	"context"
	"errors"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

var ErrInviteUnavailable = errors.New("invitation unavailable")
var ErrInviteForbidden = errors.New("invitation management forbidden")

func (r *SessionRepository) ManageInvite(ctx context.Context, sessionID int64, role string, operatorID int64, admin bool, hash []byte) (time.Time, error) {
	var expiry time.Time
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var status string
		var owner, client, executor *int64
		err := tx.QueryRow(ctx, `SELECT status, owner_operator_id, client_user_id, executor_user_id FROM sessions WHERE id=$1 FOR UPDATE`, sessionID).Scan(&status, &owner, &client, &executor)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !admin && (owner == nil || *owner != operatorID) {
			return ErrInviteForbidden
		}
		if hash == nil {
			_, err = tx.Exec(ctx, `DELETE FROM session_invites WHERE session_id=$1 AND role=$2 AND used_at IS NULL`, sessionID, role)
			return err
		}
		if status != models.SessionStatusActive || (role == "client" && client != nil) || (role == "executor" && executor != nil) {
			return ErrInviteUnavailable
		}
		return tx.QueryRow(ctx, `INSERT INTO session_invites (session_id,role,token_hash,expires_at)
   VALUES ($1,$2,$3,clock_timestamp()+interval '24 hours')
   ON CONFLICT (session_id,role) DO UPDATE SET token_hash=EXCLUDED.token_hash, expires_at=EXCLUDED.expires_at, used_at=NULL
   RETURNING expires_at`, sessionID, role, hash).Scan(&expiry)
	})
	return expiry, err
}

func (r *SessionRepository) BindInvite(ctx context.Context, users *UserRepository, hash []byte, user *models.User) (models.Session, string, error) {
	var session models.Session
	var role string
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "LOCK TABLE sessions IN ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT session_id,role FROM session_invites WHERE token_hash=$1`, hash).Scan(&session.ID, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInviteUnavailable
		}
		if err != nil {
			return err
		}
		// Serialize redemption, issuance, revocation, and session status changes.
		err = tx.QueryRow(ctx, `SELECT title,status,client_user_id,executor_user_id FROM sessions WHERE id=$1 FOR NO KEY UPDATE`, session.ID).Scan(&session.Title, &session.Status, &session.ClientUserID, &session.ExecutorUserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInviteUnavailable
		}
		if err != nil {
			return err
		}
		if session.Status != models.SessionStatusActive || (role == "client" && session.ClientUserID != nil) || (role == "executor" && session.ExecutorUserID != nil) {
			return ErrInviteUnavailable
		}
		result, err := tx.Exec(ctx, `UPDATE session_invites SET used_at=clock_timestamp()
   WHERE session_id=$1 AND role=$2 AND token_hash=$3 AND used_at IS NULL AND expires_at>clock_timestamp()`, session.ID, role, hash)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrInviteUnavailable
		}

		username, err := users.encryptField(user.Username)
		if err != nil {
			return err
		}
		firstName, err := users.encryptField(user.FirstName)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO users (telegram_id,username,first_name,language_code)
   VALUES ($1,$2,$3,$4) ON CONFLICT (telegram_id) DO NOTHING`, user.TelegramID, username, firstName, user.LanguageCode)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT id FROM users WHERE telegram_id=$1 FOR UPDATE`, user.TelegramID).Scan(&user.ID)
		if err != nil {
			return err
		}
		if (session.ClientUserID != nil && *session.ClientUserID == user.ID) || (session.ExecutorUserID != nil && *session.ExecutorUserID == user.ID) {
			return ErrInviteUnavailable
		}
		_, err = tx.Exec(ctx, `UPDATE sessions SET
   client_user_id=CASE WHEN $2='client' THEN $3 ELSE client_user_id END,
   executor_user_id=CASE WHEN $2='executor' THEN $3 ELSE executor_user_id END WHERE id=$1`, session.ID, role, user.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE users SET active_session_id=$2 WHERE id=$1`, user.ID, session.ID)
		return err
	})
	return session, role, err
}
