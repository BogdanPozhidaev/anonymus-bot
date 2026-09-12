package models

import "time"

type Message struct {
	ID               int64     `db:"id"`
	SessionID        int64     `db:"session_id"`
	SenderUserID     *int64    `db:"sender_user_id"`
	SenderRole       string    `db:"sender_role"`
	ContentType      string    `db:"content_type"`
	Content          *string   `db:"content"`
	FileID           *string   `db:"file_id"`
	InternalFilePath *string   `db:"internal_file_path"`
	SentByOperator   bool      `db:"sent_by_operator"`
	ImpersonatedRole *string   `db:"impersonated_role"`
	CreatedAt        time.Time `db:"created_at"`
	Delivered        bool      `db:"delivered"`
}

const (
	SenderRoleClient   = "client"
	SenderRoleExecutor = "executor"
	SenderRoleOperator = "operator"

	ContentTypeText   = "text"
	ContentTypePhoto  = "photo"
	ContentTypeVoice  = "voice"
	ContentTypeSystem = "system"
)
