package models

import "time"

type IncomingRequest struct {
	ID               int64     `db:"id"`
	TelegramID       int64     `db:"telegram_id"`
	Username         *string   `db:"username"`
	FirstName        *string   `db:"first_name"`
	FirstMessageText *string   `db:"first_message_text"`
	LanguageCode     *string   `db:"language_code"`
	Status           string    `db:"status"`
	CreatedAt        time.Time `db:"created_at"`
}

const (
	IncomingRequestStatusNew       = "new"
	IncomingRequestStatusProcessed = "processed"
)
