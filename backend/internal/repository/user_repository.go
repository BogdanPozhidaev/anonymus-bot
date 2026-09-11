package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type UserRepository struct {
	pool      *pgxpool.Pool
	encryptor *crypto.Encryptor
}

func NewUserRepository(pool *pgxpool.Pool, encryptor *crypto.Encryptor) *UserRepository {
	return &UserRepository{pool: pool, encryptor: encryptor}
}

func (r *UserRepository) encryptField(field *string) (*string, error) {
	if field == nil {
		return nil, nil
	}
	encrypted, err := r.encryptor.Encrypt(*field)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt field: %w", err)
	}
	return &encrypted, nil
}

func (r *UserRepository) decryptField(field *string) (*string, error) {
	if field == nil {
		return nil, nil
	}
	decrypted, err := r.encryptor.Decrypt(*field)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt field: %w", err)
	}
	return &decrypted, nil
}

func (r *UserRepository) Create(ctx context.Context, u *models.User) error {
	encryptedUsername, err := r.encryptField(u.Username)
	if err != nil {
		return err
	}
	encryptedFirstName, err := r.encryptField(u.FirstName)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO users (telegram_id, username, first_name, language_code)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	err = r.pool.QueryRow(ctx, query,
		u.TelegramID,
		encryptedUsername,
		encryptedFirstName,
		u.LanguageCode,
	).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, language_code, active_session_id, created_at
		FROM users
		WHERE id = $1
	`

	return r.scanAndDecrypt(ctx, query, id)
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, language_code, active_session_id, created_at
		FROM users
		WHERE telegram_id = $1
	`

	return r.scanAndDecrypt(ctx, query, telegramID)
}

func (r *UserRepository) scanAndDecrypt(ctx context.Context, query string, arg interface{}) (*models.User, error) {
	var u models.User
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&u.ID,
		&u.TelegramID,
		&u.Username,
		&u.FirstName,
		&u.LanguageCode,
		&u.ActiveSessionID,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	u.Username, err = r.decryptField(u.Username)
	if err != nil {
		return nil, err
	}
	u.FirstName, err = r.decryptField(u.FirstName)
	if err != nil {
		return nil, err
	}

	return &u, nil
}

func (r *UserRepository) Update(ctx context.Context, u *models.User) error {
	encryptedUsername, err := r.encryptField(u.Username)
	if err != nil {
		return err
	}
	encryptedFirstName, err := r.encryptField(u.FirstName)
	if err != nil {
		return err
	}

	query := `
		UPDATE users
		SET username = $1, first_name = $2, language_code = $3, active_session_id = $4
		WHERE id = $5
	`

	cmdTag, err := r.pool.Exec(ctx, query,
		encryptedUsername,
		encryptedFirstName,
		u.LanguageCode,
		u.ActiveSessionID,
		u.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *UserRepository) SetActiveSession(ctx context.Context, userID, sessionID int64) error {
	query := `UPDATE users SET active_session_id = $1 WHERE id = $2`

	cmdTag, err := r.pool.Exec(ctx, query, sessionID, userID)
	if err != nil {
		return fmt.Errorf("failed to set active session: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
