package models

import "time"

type IncomingRequest struct {
	ID               int64     `db:"id" json:"id"`
	TelegramID       int64     `db:"telegram_id" json:"telegram_id"`
	Username         *string   `db:"username" json:"username"`
	FirstName        *string   `db:"first_name" json:"first_name"`
	FirstMessageText *string   `db:"first_message_text" json:"first_message_text"`
	LanguageCode     *string   `db:"language_code" json:"language_code"`
	Status           string    `db:"status" json:"status"`
	OwnerOperatorID  *int64    `db:"owner_operator_id" json:"owner_operator_id"`
	SessionID        *int64    `db:"session_id" json:"session_id"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
}

const (
	IncomingRequestStatusNew        = "new"
	IncomingRequestStatusInProgress = "in_progress"
	IncomingRequestStatusProcessed  = "processed"
)
