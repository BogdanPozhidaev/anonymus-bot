package models

import "time"

type Operator struct {
	ID           int64      `db:"id"`
	Name         string     `db:"name"`
	Login        string     `db:"login"`
	Email        *string    `db:"email"`
	TelegramID   *int64     `db:"telegram_id"`
	Role         string     `db:"role"`
	Status       string     `db:"status"`
	PasswordHash *string    `db:"password_hash"`
	TOTPSecret   *string    `db:"totp_secret"`
	Language     string     `db:"language"`
	CreatedBy    *int64     `db:"created_by"`
	CreatedAt    time.Time  `db:"created_at"`
	LastLoginAt  *time.Time `db:"last_login_at"`
}

const (
	OperatorRoleAdmin    = "admin"
	OperatorRoleOperator = "operator"

	OperatorStatusInvited   = "invited"
	OperatorStatusActive    = "active"
	OperatorStatusSuspended = "suspended"
	OperatorStatusDisabled  = "disabled"
)
