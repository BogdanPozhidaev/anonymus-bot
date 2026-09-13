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

func NewOperatorRepository(pool *pgxpool.Pool) *OperatorRepository {
	return &OperatorRepository{pool: pool}
}

func (r *OperatorRepository) Create(ctx context.Context, o *models.Operator) error {
	query := `
		INSERT INTO operators (name, login, email, telegram_id, role, status, password_hash, language, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
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
	).Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create operator: %w", err)
	}

	return nil
}

func (r *OperatorRepository) GetByID(ctx context.Context, id int64) (*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at
		FROM operators
		WHERE id = $1
	`

	return r.scanOne(ctx, query, id)
}

func (r *OperatorRepository) GetByLogin(ctx context.Context, login string) (*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at
		FROM operators
		WHERE login = $1
	`

	return r.scanOne(ctx, query, login)
}

func (r *OperatorRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get operator: %w", err)
	}

	return &o, nil
}

func (r *OperatorRepository) Update(ctx context.Context, o *models.Operator) error {
	query := `
		UPDATE operators
		SET name = $1, email = $2, telegram_id = $3, role = $4, status = $5,
			password_hash = $6, totp_secret = $7, language = $8, last_login_at = $9
		WHERE id = $10
	`

	cmdTag, err := r.pool.Exec(ctx, query,
		o.Name,
		o.Email,
		o.TelegramID,
		o.Role,
		o.Status,
		o.PasswordHash,
		o.TOTPSecret,
		o.Language,
		o.LastLoginAt,
		o.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update operator: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *OperatorRepository) ListAll(ctx context.Context) ([]*models.Operator, error) {
	query := `
		SELECT id, name, login, email, telegram_id, role, status,
			password_hash, totp_secret, language, created_by, created_at, last_login_at
		FROM operators
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
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
