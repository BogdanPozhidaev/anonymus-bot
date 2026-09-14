package models

import (
	"encoding/json"
	"time"
)

type AuditLog struct {
	ID         int64           `db:"id" json:"id"`
	Timestamp  time.Time       `db:"timestamp" json:"timestamp"`
	ActorID    *int64          `db:"actor_id" json:"actor_id"`
	ActorRole  *string         `db:"actor_role" json:"actor_role"`
	Action     string          `db:"action" json:"action"`
	TargetType *string         `db:"target_type" json:"target_type"`
	TargetID   *int64          `db:"target_id" json:"target_id"`
	Payload    json.RawMessage `db:"payload" json:"payload"`
	IPAddress  *string         `db:"ip_address" json:"ip_address"`
	UserAgent  *string         `db:"user_agent" json:"user_agent"`
}
