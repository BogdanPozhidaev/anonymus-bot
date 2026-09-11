package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type AuditLogRepository struct {
	pool *pgxpool.Pool
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{pool: pool}
}

func (r *AuditLogRepository) Create(ctx context.Context, a *models.AuditLog) error {
	query := `
		INSERT INTO audit_log (actor_id, actor_role, action, target_type, target_id, payload, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, timestamp
	`

	err := r.pool.QueryRow(ctx, query,
		a.ActorID,
		a.ActorRole,
		a.Action,
		a.TargetType,
		a.TargetID,
		a.Payload,
		a.IPAddress,
		a.UserAgent,
	).Scan(&a.ID, &a.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to create audit log entry: %w", err)
	}

	return nil
}

type AuditLogFilter struct {
	ActorID    *int64
	Action     *string
	TargetType *string
	DateFrom   *string
	DateTo     *string
}

func (r *AuditLogRepository) List(ctx context.Context, filter AuditLogFilter) ([]*models.AuditLog, error) {
	query := `
		SELECT id, timestamp, actor_id, actor_role, action, target_type, target_id, payload, ip_address, user_agent
		FROM audit_log
		WHERE ($1::bigint IS NULL OR actor_id = $1)
			AND ($2::text IS NULL OR action = $2)
			AND ($3::text IS NULL OR target_type = $3)
			AND ($4::text IS NULL OR timestamp >= $4::timestamptz)
			AND ($5::text IS NULL OR timestamp <= $5::timestamptz)
		ORDER BY timestamp DESC
	`

	rows, err := r.pool.Query(ctx, query,
		filter.ActorID,
		filter.Action,
		filter.TargetType,
		filter.DateFrom,
		filter.DateTo,
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
