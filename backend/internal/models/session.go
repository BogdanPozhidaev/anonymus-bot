package models

import "time"

type Session struct {
	ID                  int64      `db:"id"`
	Title               string     `db:"title"`
	UserVisibleName     *string    `db:"user_visible_name"`
	ClientUserID        *int64     `db:"client_user_id"`
	ExecutorUserID      *int64     `db:"executor_user_id"`
	ClientDisplayName   *string    `db:"client_display_name"`
	ExecutorDisplayName *string    `db:"executor_display_name"`
	OwnerOperatorID     *int64     `db:"owner_operator_id"`
	Status              string     `db:"status"`
	Language            string     `db:"language"`
	CloseRequestedBy    *int64     `db:"close_requested_by"`
	CloseRequestedAt    *time.Time `db:"close_requested_at"`
	CloseReason         *string    `db:"close_reason"`
	ClosedAt            *time.Time `db:"closed_at"`
	PaymentStatus       *string    `db:"payment_status"`
	CreatedBy           *int64     `db:"created_by"`
	CreatedAt           time.Time  `db:"created_at"`
}

const (
	SessionStatusActive       = "active"
	SessionStatusPaused       = "paused"
	SessionStatusPendingClose = "pending_close"
	SessionStatusClosed       = "closed"
)
