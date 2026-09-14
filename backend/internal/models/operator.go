package models

import "time"

type Operator struct {
	ID           int64      `db:"id" json:"id"`
	Name         string     `db:"name" json:"name"`
	Login        string     `db:"login" json:"login"`
	Email        *string    `db:"email" json:"email"`
	TelegramID   *int64     `db:"telegram_id" json:"telegram_id"`
	Role         string     `db:"role" json:"role"`
	Status       string     `db:"status" json:"status"`
	PasswordHash *string    `db:"password_hash" json:"-"`
	TOTPSecret   *string    `db:"totp_secret" json:"-"`
	Language     string     `db:"language" json:"language"`
	CreatedBy    *int64     `db:"created_by" json:"created_by"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"last_login_at"`
}

const (
	OperatorRoleAdmin    = "admin"
	OperatorRoleOperator = "operator"

	OperatorStatusInvited   = "invited"
	OperatorStatusActive    = "active"
	OperatorStatusSuspended = "suspended"
	OperatorStatusDisabled  = "disabled"
)
