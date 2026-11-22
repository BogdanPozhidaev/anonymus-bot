package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type auditLogDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type AuditLogRepository struct {
	db auditLogDB
}

func NewAuditLogRepository(db auditLogDB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(ctx context.Context, a *models.AuditLog) error {
	if a.ActorType == "" {
		a.ActorType = "unknown"
	}
	query := `
		INSERT INTO audit_log (actor_id, actor_role, action, target_type, target_id, payload, ip_address, user_agent, actor_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, timestamp
	`

	err := r.db.QueryRow(ctx, query,
		a.ActorID,
		a.ActorRole,
		a.Action,
		a.TargetType,
		a.TargetID,
		a.Payload,
		a.IPAddress,
		a.UserAgent,
		a.ActorType,
	).Scan(&a.ID, &a.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to create audit log entry: %w", err)
	}

	return nil
}

type AuditLogFilter struct {
	ActorID       *int64
	Action        *string
	TargetType    *string
	DateFrom      *string
	DateTo        *string
	Limit         *int
	Offset        int
	RelatedUserID *int64
}

func (r *AuditLogRepository) List(ctx context.Context, filter AuditLogFilter) ([]*models.AuditLog, error) {
	query := `
		SELECT id, timestamp, actor_id, actor_role, action, target_type, target_id, payload, ip_address, user_agent, actor_type
		FROM audit_log
		WHERE ($1::bigint IS NULL OR actor_id = $1)
			AND ($2::text IS NULL OR action = $2)
			AND ($3::text IS NULL OR target_type = $3)
			AND ($4::text IS NULL OR timestamp >= $4::timestamptz)
			AND ($5::text IS NULL OR timestamp <= $5::timestamptz)
		AND ($8::bigint IS NULL OR (actor_type = 'user' AND actor_id = $8) OR (target_type = 'user' AND target_id = $8))
		ORDER BY timestamp DESC, id DESC
		LIMIT $6 OFFSET $7
	`

	rows, err := r.db.Query(ctx, query,
		filter.ActorID,
		filter.Action,
		filter.TargetType,
		filter.DateFrom,
		filter.DateTo,
		filter.Limit, filter.Offset, filter.RelatedUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit log: %w", err)
	}
	defer rows.Close()

	var logs []*models.AuditLog
	for rows.Next() {
		var a models.AuditLog
		err := rows.Scan(
			&a.ID,
			&a.Timestamp,
			&a.ActorID,
			&a.ActorRole,
			&a.Action,
			&a.TargetType,
			&a.TargetID,
			&a.Payload,
			&a.IPAddress,
			&a.UserAgent,
			&a.ActorType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log row: %w", err)
		}
		logs = append(logs, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log rows: %w", err)
	}

	return logs, nil
}

func (r *AuditLogRepository) UpdatePayload(ctx context.Context, id int64, payload []byte) error {
	query := `UPDATE audit_log SET payload = $1 WHERE id = $2`

	_, err := r.db.Exec(ctx, query, payload, id)
	if err != nil {
		return fmt.Errorf("failed to update audit log payload: %w", err)
	}

	return nil
}
