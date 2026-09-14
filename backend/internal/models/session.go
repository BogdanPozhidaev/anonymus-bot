package models

import "time"

type Session struct {
	ID                  int64      `db:"id" json:"id"`
	Title               string     `db:"title" json:"title"`
	UserVisibleName     *string    `db:"user_visible_name" json:"user_visible_name"`
	ClientUserID        *int64     `db:"client_user_id" json:"client_user_id"`
	ExecutorUserID      *int64     `db:"executor_user_id" json:"executor_user_id"`
	ClientDisplayName   *string    `db:"client_display_name" json:"client_display_name"`
	ExecutorDisplayName *string    `db:"executor_display_name" json:"executor_display_name"`
	OwnerOperatorID     *int64     `db:"owner_operator_id" json:"owner_operator_id"`
	Status              string     `db:"status" json:"status"`
	Language            string     `db:"language" json:"language"`
	CloseRequestedBy    *int64     `db:"close_requested_by" json:"close_requested_by"`
	CloseRequestedAt    *time.Time `db:"close_requested_at" json:"close_requested_at"`
	CloseReason         *string    `db:"close_reason" json:"close_reason"`
	ClosedAt            *time.Time `db:"closed_at" json:"closed_at"`
	PaymentStatus       *string    `db:"payment_status" json:"payment_status"`
	CreatedBy           *int64     `db:"created_by" json:"created_by"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
}

const (
	SessionStatusActive       = "active"
	SessionStatusPaused       = "paused"
	SessionStatusPendingClose = "pending_close"
	SessionStatusClosed       = "closed"
)
