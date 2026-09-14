package models

import "time"

type Message struct {
	ID               int64     `db:"id" json:"id"`
	SessionID        int64     `db:"session_id" json:"session_id"`
	SenderUserID     *int64    `db:"sender_user_id" json:"sender_user_id"`
	SenderRole       string    `db:"sender_role" json:"sender_role"`
	ContentType      string    `db:"content_type" json:"content_type"`
	Content          *string   `db:"content" json:"content"`
	FileID           *string   `db:"file_id" json:"file_id"`
	InternalFilePath *string   `db:"internal_file_path" json:"-"`
	SentByOperator   bool      `db:"sent_by_operator" json:"sent_by_operator"`
	ImpersonatedRole *string   `db:"impersonated_role" json:"impersonated_role"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	Delivered        bool      `db:"delivered" json:"delivered"`
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
