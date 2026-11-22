package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type OperatorRepository struct {
	pool *pgxpool.Pool
}

var ErrLastActiveAdmin = errors.New("cannot remove the last active administrator")

func NewOperatorRepository(pool *pgxpool.Pool) *OperatorRepository {
	return &OperatorRepository{pool: pool}
}

func (r *OperatorRepository) Create(ctx context.Context, o *models.Operator) error {
	query := `
		INSERT INTO operators (name, login, email, telegram_id, role, status, password_hash, language, created_by, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, auth_version
	`

	err := r.pool.QueryRow(ctx, query,
		o.Name,
		o.Login,
		o.Email,
		o.TelegramID,
		o.Role,
		o.Status,
		o.PasswordHash,
		o.Language,
		o.CreatedBy,
		o.MustChangePassword,
	).Scan(&o.ID, &o.CreatedAt, &o.AuthVersion)
	if err != nil {
		return fmt.Errorf("failed to create operator: %w", err)
	}

	return nil
}

func (r *OperatorRepository) GetByID(ctx context.Context, id int64) (*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at, auth_version, must_change_password
		FROM operators
		WHERE id = $1
	`

	return r.scanOne(ctx, query, id)
}

func (r *OperatorRepository) GetByLogin(ctx context.Context, login string) (*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at, auth_version, must_change_password
		FROM operators
		WHERE login = $1
	`

	return r.scanOne(ctx, query, login)
}

func (r *OperatorRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at, auth_version, must_change_password
		FROM operators
		WHERE telegram_id = $1
	`

	return r.scanOne(ctx, query, telegramID)
}

// scanOne выполняет запрос с ровно одним параметром $1 и сканирует строку в Operator.
// Сам SQL-текст запроса всегда приходит из захардкоженных констант внутри этого файла,
// никогда не собирается из внешнего ввода — параметризуется только значение поиска.
func (r *OperatorRepository) scanOne(ctx context.Context, query string, arg interface{}) (*models.Operator, error) {
	var o models.Operator
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&o.ID,
		&o.Name,
		&o.Login,
		&o.Email,
		&o.TelegramID,
		&o.Role,
		&o.Status,
		&o.PasswordHash,
		&o.TOTPSecret,
		&o.Language,
		&o.CreatedBy,
		&o.CreatedAt,
		&o.LastLoginAt,
		&o.AuthVersion,
		&o.MustChangePassword,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get operator: %w", err)
	}

	return &o, nil
}

func (r *OperatorRepository) UpdateProfile(ctx context.Context, id, authVersion int64, name, role, status *string) error {
	query := `
		WITH updated AS (
			UPDATE operators SET name=COALESCE($3,name), role=COALESCE($4,role), status=COALESCE($5,status)
			WHERE id=$1 AND auth_version=$2 RETURNING id,status
		), revoked AS (
			DELETE FROM operator_telegram_links USING updated
			WHERE operator_id=updated.id AND updated.status <> 'active'
		)
		SELECT id FROM updated
	`
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		if role != nil || status != nil {
			// Serialize profile changes before reading the active administrator set.
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('operators:active-admin', 0))"); err != nil {
				return err
			}
			var removesLastAdmin bool
			err := tx.QueryRow(ctx, `
				SELECT role='admin' AND status='active'
					AND (COALESCE($3,role)<>'admin' OR COALESCE($4,status)<>'active')
					AND NOT EXISTS (
						SELECT 1 FROM operators other WHERE other.id<>$1 AND other.role='admin' AND other.status='active'
					)
				FROM operators WHERE id=$1 AND auth_version=$2 FOR UPDATE
			`, id, authVersion, role, status).Scan(&removesLastAdmin)
			if err != nil {
				return err
			}
			if removesLastAdmin {
				return ErrLastActiveAdmin
			}
		}
		var updatedID int64
		return tx.QueryRow(ctx, query, id, authVersion, name, role, status).Scan(&updatedID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if errors.Is(err, ErrLastActiveAdmin) {
		return ErrLastActiveAdmin
	}
	if err != nil {
		return fmt.Errorf("failed to update operator: %w", err)
	}

	return nil
}

func (r *OperatorRepository) ListAll(ctx context.Context) ([]*models.Operator, error) {
	return r.list(ctx, nil, 0)
}

func (r *OperatorRepository) ListPage(ctx context.Context, limit, offset int) ([]*models.Operator, error) {
	return r.list(ctx, &limit, offset)
}

func (r *OperatorRepository) list(ctx context.Context, limit *int, offset int) ([]*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at, auth_version
		FROM operators
		ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2
	`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list operators: %w", err)
	}
	defer rows.Close()

	var operators []*models.Operator
	for rows.Next() {
		var o models.Operator
		err := rows.Scan(
			&o.ID,
			&o.Name,
			&o.Login,
			&o.Email,
			&o.TelegramID,
			&o.Role,
			&o.Status,
			&o.PasswordHash,
			&o.TOTPSecret,
			&o.Language,
			&o.CreatedBy,
			&o.CreatedAt,
			&o.LastLoginAt,
			&o.AuthVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan operator row: %w", err)
		}
		operators = append(operators, &o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating operator rows: %w", err)
	}

	return operators, nil
}

func (r *OperatorRepository) GetAuthState(ctx context.Context, id int64) (role, status string, version int64, mustChange bool, err error) {
	err = r.pool.QueryRow(ctx, "SELECT role, status, auth_version, must_change_password FROM operators WHERE id = $1", id).
		Scan(&role, &status, &version, &mustChange)
	return
}

func (r *OperatorRepository) ChangePassword(ctx context.Context, operatorID, version int64, hash string) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE operators
		SET password_hash=$3, must_change_password=false, auth_version=auth_version+1
		WHERE id=$1 AND auth_version=$2 AND status='active'
	`, operatorID, version, hash)
	return err == nil && result.RowsAffected() == 1, err
}

func (r *OperatorRepository) ResetPassword(ctx context.Context, adminID, adminVersion, targetID, targetVersion int64, hash string) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE operators AS target
		SET password_hash=$5, must_change_password=true, auth_version=target.auth_version+1
		FROM operators AS actor
		WHERE actor.id=$1 AND actor.auth_version=$2 AND actor.role='admin'
			AND actor.status='active' AND NOT actor.must_change_password
			AND target.id=$3 AND target.id<>actor.id AND target.auth_version=$4
			AND target.status IN ('active','invited')
	`, adminID, adminVersion, targetID, targetVersion, hash)
	return err == nil && result.RowsAffected() == 1, err
}

func (r *OperatorRepository) RecordVerifiedLogin(ctx context.Context, operator *models.Operator, secret *string, enrollment bool) error {
	err := r.pool.QueryRow(ctx, `
		UPDATE operators SET last_login_at=now(), totp_secret=COALESCE($4,totp_secret),
			status=CASE WHEN status='invited' THEN 'active' ELSE status END,
			auth_version=auth_version+CASE WHEN $5 THEN 1 ELSE 0 END
		WHERE id=$1 AND auth_version=$2 AND status=$3
		RETURNING auth_version
	`, operator.ID, operator.AuthVersion, operator.Status, secret, enrollment).Scan(&operator.AuthVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
