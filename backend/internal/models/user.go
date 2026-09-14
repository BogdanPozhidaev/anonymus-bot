package models

import "time"

type User struct {
	ID              int64     `db:"id" json:"id"`
	TelegramID      int64     `db:"telegram_id" json:"telegram_id"`
	Username        *string   `db:"username" json:"username"`
	FirstName       *string   `db:"first_name" json:"first_name"`
	LanguageCode    string    `db:"language_code" json:"language_code"`
	ActiveSessionID *int64    `db:"active_session_id" json:"active_session_id"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
}
