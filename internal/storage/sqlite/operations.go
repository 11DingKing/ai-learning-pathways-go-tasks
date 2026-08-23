package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

func (q *queries) InsertAudit(ctx context.Context, value repository.AuditEvent) error {
	metadata := value.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	_, err := q.runner.ExecContext(ctx, `INSERT INTO audit_events(id, tenant_id, actor_id,
request_id, action, object_type, object_id, result, metadata, occurred_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TenantID, value.ActorID, value.RequestID, value.Action, value.ObjectType,
		value.ObjectID, value.Result, metadata, value.OccurredAt.Format(time.RFC3339Nano))
	return translateError("insert audit event", err)
}

func (q *queries) ListAudit(ctx context.Context, tenantID common.ID, filter repository.AuditFilter, page repository.Page) (repository.AuditPage, error) {
	page = page.Normalize()
	conditions := []string{"tenant_id=?"}
	args := []any{tenantID}
	appendText := func(column, value string) {
		if value != "" {
			conditions = append(conditions, column+"=?")
			args = append(args, value)
		}
	}
	appendText("actor_id", strings.TrimSpace(filter.ActorID))
	appendText("action", strings.TrimSpace(filter.Action))
	appendText("object_type", strings.TrimSpace(filter.ObjectType))
	appendText("object_id", strings.TrimSpace(filter.ObjectID))
	if filter.From != nil {
		conditions = append(conditions, "occurred_at>=?")
		args = append(args, filter.From.UTC().Format(time.RFC3339Nano))
	}
	if filter.To != nil {
		conditions = append(conditions, "occurred_at<?")
		args = append(args, filter.To.UTC().Format(time.RFC3339Nano))
	}
	where := strings.Join(conditions, " AND ")
	var total int
	if err := q.runner.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE "+where, args...).Scan(&total); err != nil {
		return repository.AuditPage{}, translateError("count audit events", err)
	}
	queryArgs := append(append([]any(nil), args...), page.Limit, page.Offset)
	rows, err := q.runner.QueryContext(ctx, `SELECT id, tenant_id, actor_id, request_id, action,
object_type, object_id, result, metadata, occurred_at FROM audit_events WHERE `+where+
		` ORDER BY occurred_at DESC, id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return repository.AuditPage{}, translateError("list audit events", err)
	}
	defer rows.Close()
	items := make([]repository.AuditEvent, 0, page.Limit)
	for rows.Next() {
		var value repository.AuditEvent
		var metadata []byte
		var occurredAt string
		if err := rows.Scan(&value.ID, &value.TenantID, &value.ActorID, &value.RequestID, &value.Action,
			&value.ObjectType, &value.ObjectID, &value.Result, &metadata, &occurredAt); err != nil {
			return repository.AuditPage{}, fmt.Errorf("scan audit event: %w", err)
		}
		value.Metadata = json.RawMessage(append([]byte(nil), metadata...))
		if value.OccurredAt, err = parseTime(occurredAt); err != nil {
			return repository.AuditPage{}, err
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return repository.AuditPage{}, fmt.Errorf("iterate audit events: %w", err)
	}
	return repository.AuditPage{Items: items, Total: total, Page: page}, nil
}

func (q *queries) InsertOutbox(ctx context.Context, value repository.OutboxEvent) error {
	payload := value.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	_, err := q.runner.ExecContext(ctx, `INSERT INTO outbox_events(id, tenant_id, aggregate_type,
aggregate_id, kind, payload, created_at, published_at, attempts, last_error) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TenantID, value.AggregateType, value.AggregateID, value.Kind, payload,
		value.CreatedAt.Format(time.RFC3339Nano), nullableTime(value.PublishedAt), value.Attempts, value.LastError)
	return translateError("insert outbox event", err)
}

func (q *queries) InsertIdempotency(ctx context.Context, value repository.IdempotencyRecord) error {
	response := value.Response
	if len(response) == 0 {
		response = json.RawMessage(`{}`)
	}
	_, err := q.runner.ExecContext(ctx, `INSERT INTO idempotency_records(scope, key, request_hash,
state, status_code, response, created_at, completed_at) VALUES(?,?,?,?,?,?,?,?)`, value.Scope,
		value.Key, value.RequestHash, value.State, value.StatusCode, response,
		value.CreatedAt.Format(time.RFC3339Nano), nullableTime(value.CompletedAt))
	return translateError("insert idempotency record", err)
}

func (q *queries) CompleteIdempotency(ctx context.Context, scope, key string, status int, body json.RawMessage, now time.Time) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE idempotency_records SET state='completed',
status_code=?, response=?, completed_at=? WHERE scope=? AND key=? AND state='processing'`, status,
		body, now.UTC().Format(time.RFC3339Nano), scope, key)
	return expectOne("complete idempotency record", result, err)
}

func (q *queries) GetIdempotency(ctx context.Context, scope, key string) (repository.IdempotencyRecord, error) {
	var value repository.IdempotencyRecord
	var response []byte
	var createdAt string
	var completedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT scope, key, request_hash, state, status_code,
response, created_at, completed_at FROM idempotency_records WHERE scope=? AND key=?`, scope, key).
		Scan(&value.Scope, &value.Key, &value.RequestHash, &value.State, &value.StatusCode,
			&response, &createdAt, &completedAt)
	if err != nil {
		return repository.IdempotencyRecord{}, translateError("get idempotency record", err)
	}
	value.Response = json.RawMessage(append([]byte(nil), response...))
	if value.CreatedAt, err = parseTime(createdAt); err != nil {
		return repository.IdempotencyRecord{}, err
	}
	if value.CompletedAt, err = parseNullableTime(completedAt); err != nil {
		return repository.IdempotencyRecord{}, err
	}
	return value, nil
}
