package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrTelegramAlreadyLinked = errors.New("telegram account already linked")

func (r *OperatorRepository) BeginTelegramLink(ctx context.Context, id, version int64, digest []byte) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockTelegramOperator(ctx, tx, id, version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO operator_telegram_links (operator_id, auth_version, code_digest, expires_at)
			VALUES ($1,$2,$3,clock_timestamp()+interval '5 minutes')
			ON CONFLICT (operator_id) DO UPDATE SET auth_version=excluded.auth_version,
				code_digest=excluded.code_digest, expires_at=excluded.expires_at,
				candidate_telegram_id=NULL, confirmation_digest=NULL
		`, id, version, digest)
		return err
	})
}

func (r *OperatorRepository) ClaimTelegramLink(ctx context.Context, telegramID int64, digest, confirmation []byte) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE operator_telegram_links AS link
		SET candidate_telegram_id=$1, confirmation_digest=$3
		FROM operators AS op
		WHERE link.code_digest=$2 AND link.candidate_telegram_id IS NULL
			AND link.expires_at > clock_timestamp() AND op.id=link.operator_id
			AND op.auth_version=link.auth_version AND op.status='active' AND NOT op.must_change_password
	`, telegramID, digest, confirmation)
	return err == nil && result.RowsAffected() == 1, err
}

func (r *OperatorRepository) ConfirmTelegramLink(ctx context.Context, id, version int64, confirmation []byte) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockTelegramOperator(ctx, tx, id, version); err != nil {
			return err
		}
		var telegramID int64
		err := tx.QueryRow(ctx, `
			DELETE FROM operator_telegram_links
			WHERE operator_id=$1 AND auth_version=$2 AND confirmation_digest=$3
				AND expires_at > clock_timestamp()
			RETURNING candidate_telegram_id
		`, id, version, confirmation).Scan(&telegramID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE operators SET telegram_id=$2 WHERE id=$1", id, telegramID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrTelegramAlreadyLinked
		}
		return err
	})
}

func (r *OperatorRepository) PurgeExpiredTelegramLinks(ctx context.Context, dryRun bool) (int64, error) {
	if dryRun {
		var count int64
		err := r.pool.QueryRow(ctx, "SELECT count(*) FROM operator_telegram_links WHERE expires_at <= clock_timestamp()").Scan(&count)
		return count, err
	}
	result, err := r.pool.Exec(ctx, "DELETE FROM operator_telegram_links WHERE expires_at <= clock_timestamp()")
	return result.RowsAffected(), err
}

func lockTelegramOperator(ctx context.Context, tx pgx.Tx, id, version int64) error {
	var matched int64
	return tx.QueryRow(ctx, `SELECT id FROM operators WHERE id=$1 AND auth_version=$2
		AND status='active' AND NOT must_change_password FOR UPDATE`, id, version).Scan(&matched)
}
