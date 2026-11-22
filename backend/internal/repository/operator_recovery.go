package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (r *OperatorRepository) CompleteTOTPRecovery(ctx context.Context, id, version int64, secret string) (int64, error) {
	var nextVersion int64
	err := r.pool.QueryRow(ctx, `
		WITH replaced AS (
			UPDATE operators
			SET totp_secret=$3, auth_version=auth_version+1, last_login_at=now()
			WHERE id=$1 AND auth_version=$2 AND status='active' AND totp_secret IS NOT NULL
			RETURNING id, auth_version
		), expired_codes AS (
			DELETE FROM operator_recovery_codes AS code USING replaced
			WHERE code.operator_id=replaced.id
		)
		SELECT auth_version FROM replaced
	`, id, version, secret).Scan(&nextVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return nextVersion, err
}

func (r *OperatorRepository) ReplaceRecoveryCodes(ctx context.Context, id, version int64, digests [][]byte) (bool, error) {
	changed := false
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE operators SET auth_version=auth_version+1
			WHERE id=$1 AND auth_version=$2 AND status='active'
				AND totp_secret IS NOT NULL AND NOT must_change_password
		`, id, version)
		if err != nil || result.RowsAffected() != 1 {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM operator_recovery_codes WHERE operator_id=$1", id); err != nil {
			return err
		}
		rows := make([][]any, 0, len(digests))
		for _, digest := range digests {
			rows = append(rows, []any{id, digest})
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"operator_recovery_codes"}, []string{"operator_id", "digest"}, pgx.CopyFromRows(rows))
		changed = err == nil
		return err
	})
	return changed && err == nil, err
}

func (r *OperatorRepository) ConsumeRecoveryCode(ctx context.Context, id, version int64, digest []byte) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		WITH eligible AS (
			SELECT id FROM operators WHERE id=$1 AND auth_version=$2
				AND status='active' AND totp_secret IS NOT NULL FOR UPDATE
		)
		DELETE FROM operator_recovery_codes AS code USING eligible
		WHERE code.operator_id=eligible.id AND code.digest=$3
	`, id, version, digest)
	return err == nil && result.RowsAffected() == 1, err
}
