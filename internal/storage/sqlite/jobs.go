package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
)

func (q *queries) InsertJob(ctx context.Context, value job.Job) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO jobs(id, tenant_id, kind, aggregate_type,
aggregate_id, payload, status, available_at, lease_owner, lease_until, attempt, max_attempts,
last_error, completed_at, version, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TenantID, value.Kind, value.AggregateType, value.AggregateID, value.Payload,
		value.Status, value.AvailableAt.Format(time.RFC3339Nano), value.LeaseOwner, nullableTime(value.LeaseUntil),
		value.Attempt, value.MaxAttempts, value.LastError, nullableTime(value.CompletedAt), value.Version,
		value.CreatedAt.Format(time.RFC3339Nano), value.UpdatedAt.Format(time.RFC3339Nano))
	return translateError("insert job", err)
}

func (q *queries) UpdateJob(ctx context.Context, value job.Job, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE jobs SET payload=?, status=?, available_at=?,
lease_owner=?, lease_until=?, attempt=?, max_attempts=?, last_error=?, completed_at=?, version=?,
updated_at=? WHERE tenant_id=? AND id=? AND version=?`, value.Payload, value.Status,
		value.AvailableAt.Format(time.RFC3339Nano), value.LeaseOwner, nullableTime(value.LeaseUntil),
		value.Attempt, value.MaxAttempts, value.LastError, nullableTime(value.CompletedAt), value.Version,
		value.UpdatedAt.Format(time.RFC3339Nano), value.TenantID, value.ID, expectedVersion)
	return expectOne("update job", result, err)
}

func (q *queries) GetJob(ctx context.Context, tenantID, jobID common.ID) (job.Job, error) {
	return q.scanJob(q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, kind, aggregate_type,
aggregate_id, payload, status, available_at, lease_owner, lease_until, attempt, max_attempts,
last_error, completed_at, version, created_at, updated_at FROM jobs WHERE tenant_id=? AND id=?`, tenantID, jobID))
}

func (q *queries) scanJob(row rowScanner) (job.Job, error) {
	var value job.Job
	var payload []byte
	var availableAt, leaseUntil, completedAt, createdAt, updatedAt sql.NullString
	err := row.Scan(&value.ID, &value.TenantID, &value.Kind, &value.AggregateType, &value.AggregateID,
		&payload, &value.Status, &availableAt, &value.LeaseOwner, &leaseUntil, &value.Attempt,
		&value.MaxAttempts, &value.LastError, &completedAt, &value.Version, &createdAt, &updatedAt)
	if err != nil {
		return job.Job{}, translateError("scan job", err)
	}
	value.Payload = json.RawMessage(append([]byte(nil), payload...))
	if value.AvailableAt, err = parseTime(availableAt.String); err != nil {
		return job.Job{}, err
	}
	if value.LeaseUntil, err = parseNullableTime(leaseUntil); err != nil {
		return job.Job{}, err
	}
	if value.CompletedAt, err = parseNullableTime(completedAt); err != nil {
		return job.Job{}, err
	}
	if value.CreatedAt, err = parseTime(createdAt.String); err != nil {
		return job.Job{}, err
	}
	if value.UpdatedAt, err = parseTime(updatedAt.String); err != nil {
		return job.Job{}, err
	}
	return value, nil
}

func (q *queries) ClaimJobs(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration, kinds []string, limit int) ([]job.Job, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" || leaseDuration <= 0 || limit <= 0 || len(kinds) == 0 {
		return nil, common.FieldError{Field: "job_claim", Message: "owner, lease duration, kinds, and limit are required"}
	}
	placeholders := make([]string, len(kinds))
	args := make([]any, 0, len(kinds)+3)
	for i, kind := range kinds {
		placeholders[i] = "?"
		args = append(args, strings.TrimSpace(kind))
	}
	nowText := now.UTC().Format(time.RFC3339Nano)
	args = append(args, nowText, nowText, limit)
	query := `SELECT id, version FROM jobs WHERE kind IN (` + strings.Join(placeholders, ",") + `)
AND status IN ('pending','retry','leased') AND available_at <= ?
AND (status <> 'leased' OR lease_until <= ?) ORDER BY available_at, created_at, id LIMIT ?`
	rows, err := q.runner.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, translateError("select job claims", err)
	}
	type candidate struct {
		id      common.ID
		version int64
	}
	candidates := make([]candidate, 0, limit)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.version); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan job candidate: %w", err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close job candidates: %w", err)
	}
	claimed := make([]job.Job, 0, len(candidates))
	until := now.UTC().Add(leaseDuration).Format(time.RFC3339Nano)
	for _, item := range candidates {
		result, err := q.runner.ExecContext(ctx, `UPDATE jobs SET status='leased', lease_owner=?, lease_until=?,
attempt=attempt+1, version=version+1, updated_at=? WHERE id=? AND version=?
AND status IN ('pending','retry','leased') AND available_at <= ?
AND (status <> 'leased' OR lease_until <= ?)`, owner, until, nowText, item.id, item.version, nowText, nowText)
		if err != nil {
			return nil, translateError("claim job", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("claim job rows: %w", err)
		}
		if count == 0 {
			continue
		}
		value, err := q.scanJob(q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, kind, aggregate_type,
aggregate_id, payload, status, available_at, lease_owner, lease_until, attempt, max_attempts,
last_error, completed_at, version, created_at, updated_at FROM jobs WHERE id=?`, item.id))
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, value)
	}
	return claimed, nil
}
