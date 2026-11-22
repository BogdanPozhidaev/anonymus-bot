package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KeyRotationResult struct {
	Users     int
	Operators int
}

// RotateEncryptionKeys verifies every stored ciphertext and optionally
// rewrites outdated values in one transaction.
func RotateEncryptionKeys(ctx context.Context, pool *pgxpool.Pool, pii, totp *crypto.Encryptor, apply bool) (KeyRotationResult, error) {
	var result KeyRotationResult
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		// Serialize rotation and keep concurrent writes from invalidating the scan.
		if _, err := tx.Exec(ctx, "LOCK TABLE users, operators IN SHARE ROW EXCLUSIVE MODE NOWAIT"); err != nil {
			return fmt.Errorf("cannot lock users and operators; stop writers and retry")
		}
		var err error
		result.Users, err = rotateEncryptedRows(ctx, tx, "users", []string{"username", "first_name"}, pii.Reencrypt, apply)
		if err != nil {
			return err
		}
		result.Operators, err = rotateEncryptedRows(ctx, tx, "operators", []string{"totp_secret"}, totp.ReencryptTOTP, apply)
		return err
	})
	if err != nil {
		return KeyRotationResult{}, err
	}
	return result, nil
}

func rotateEncryptedRows(ctx context.Context, tx pgx.Tx, table string, columns []string, reencrypt func(string) (string, error), apply bool) (int, error) {
	// Table and column names are fixed by the caller. Values are query parameters.
	var assignments, populated []string
	for i, col := range columns {
		assignments = append(assignments, fmt.Sprintf("%s=$%d", col, i+2))
		populated = append(populated, col+" IS NOT NULL")
	}
	query := "SELECT id, " + strings.Join(columns, ", ") + " FROM " + table +
		" WHERE ($1::bigint IS NULL OR id > $1) AND (" + strings.Join(populated, " OR ") + ") ORDER BY id LIMIT 256"
	update := "UPDATE " + table + " SET " + strings.Join(assignments, ", ") + " WHERE id=$1"
	type row struct {
		id     int64
		fields []*string
	}
	var cursor *int64
	count := 0
	for {
		rows, err := tx.Query(ctx, query, cursor)
		if err != nil {
			return 0, fmt.Errorf("cannot scan %s for key rotation", table)
		}
		batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			item := row{fields: make([]*string, len(columns))}
			dest := []any{&item.id}
			for i := range item.fields {
				dest = append(dest, &item.fields[i])
			}
			err := r.Scan(dest...)
			return item, err
		})
		if err != nil {
			return 0, fmt.Errorf("cannot read %s for key rotation", table)
		}
		if len(batch) == 0 {
			return count, nil
		}
		for _, item := range batch {
			args := []any{item.id}
			changed := false
			for i, field := range item.fields {
				if field != nil {
					value, err := reencrypt(*field)
					if err != nil {
						return 0, fmt.Errorf("cannot decrypt %s.%s at row %d; check key versions and data integrity", table, columns[i], item.id)
					}
					changed = changed || value != *field
					field = &value
				}
				args = append(args, field)
			}
			if !changed {
				continue
			}
			count++
			if apply {
				if _, err := tx.Exec(ctx, update, args...); err != nil {
					return 0, fmt.Errorf("cannot update %s row %d during key rotation", table, item.id)
				}
			}
		}
		cursor = &batch[len(batch)-1].id
	}
}
