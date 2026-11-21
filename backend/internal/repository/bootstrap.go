package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrBootstrapInitialized = errors.New("operators already exist; first-admin bootstrap is closed")

// BootstrapAdmin serializes the empty-table check with every operator write.
func (r *OperatorRepository) BootstrapAdmin(ctx context.Context, login, name, hash string) (bool, error) {
	created := false
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "LOCK TABLE operators IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		var occupied, same bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM operators) OR EXISTS (SELECT 1 FROM admin_bootstrap),
   EXISTS (SELECT 1 FROM operators WHERE login=$1 AND role='admin')`, login).Scan(&occupied, &same); err != nil {
			return err
		}
		if occupied {
			if same {
				return nil
			}
			return ErrBootstrapInitialized
		}
		_, err := tx.Exec(ctx, `INSERT INTO operators (name,login,role,status,password_hash,language)
   VALUES ($1,$2,'admin','active',$3,'ru')`, name, login, hash)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO admin_bootstrap DEFAULT VALUES")
		created = err == nil
		return err
	})
	return created && err == nil, err
}
