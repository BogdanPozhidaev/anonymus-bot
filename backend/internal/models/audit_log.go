package models

import (
	"encoding/json"
	"time"
)

type AuditLog struct {
	ID         int64           `db:"id"`
	Timestamp  time.Time       `db:"timestamp"`
	ActorID    *int64          `db:"actor_id"`
	ActorRole  *string         `db:"actor_role"`
	Action     string          `db:"action"`
	TargetType *string         `db:"target_type"`
	TargetID   *int64          `db:"target_id"`
	Payload    json.RawMessage `db:"payload"`
	IPAddress  *string         `db:"ip_address"`
	UserAgent  *string         `db:"user_agent"`
}
