package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/resource"
)

func (q *queries) InsertResourcePack(ctx context.Context, value resource.ResourcePack) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO resource_packs(
id, tenant_id, title, license_code, content_uri, content_sha256, minimum_stage,
maximum_stage, status, approved_by, version, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID, value.Title, value.LicenseCode,
		value.ContentURI, value.ContentSHA256, value.MinimumStage, value.MaximumStage, value.Status,
		nullableID(value.ApprovedBy), value.Version, value.CreatedAt.Format(time.RFC3339Nano),
		value.UpdatedAt.Format(time.RFC3339Nano))
	return translateError("insert resource pack", err)
}

func (q *queries) UpdateResourcePack(ctx context.Context, value resource.ResourcePack, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE resource_packs SET title=?, license_code=?,
content_uri=?, content_sha256=?, minimum_stage=?, maximum_stage=?, status=?, approved_by=?,
version=?, updated_at=? WHERE tenant_id=? AND id=? AND version=?`, value.Title, value.LicenseCode,
		value.ContentURI, value.ContentSHA256, value.MinimumStage, value.MaximumStage, value.Status,
		nullableID(value.ApprovedBy), value.Version, value.UpdatedAt.Format(time.RFC3339Nano),
		value.TenantID, value.ID, expectedVersion)
	return expectOne("update resource pack", result, err)
}

func (q *queries) GetResourcePack(ctx context.Context, tenantID, packID common.ID) (resource.ResourcePack, error) {
	var value resource.ResourcePack
	var approvedBy, createdAt, updatedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, title, license_code, content_uri,
content_sha256, minimum_stage, maximum_stage, status, approved_by, version, created_at, updated_at
FROM resource_packs WHERE tenant_id=? AND id=?`, tenantID, packID).Scan(&value.ID, &value.TenantID,
		&value.Title, &value.LicenseCode, &value.ContentURI, &value.ContentSHA256, &value.MinimumStage,
		&value.MaximumStage, &value.Status, &approvedBy, &value.Version, &createdAt, &updatedAt)
	if err != nil {
		return resource.ResourcePack{}, translateError("get resource pack", err)
	}
	if approvedBy.Valid {
		id := common.ID(approvedBy.String)
		value.ApprovedBy = &id
	}
	if value.CreatedAt, err = parseTime(createdAt.String); err != nil {
		return resource.ResourcePack{}, err
	}
	if value.UpdatedAt, err = parseTime(updatedAt.String); err != nil {
		return resource.ResourcePack{}, err
	}
	return value, nil
}

func (q *queries) InsertDelivery(ctx context.Context, value resource.Delivery) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO resource_deliveries(
id, tenant_id, pack_id, offering_id, recipient_id, status, entitlement_key, lease_owner,
lease_until, attempts, last_error, delivered_at, acknowledged_at, version, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID, value.PackID, value.OfferingID,
		value.RecipientID, value.Status, value.EntitlementKey, value.LeaseOwner, nullableTime(value.LeaseUntil),
		value.Attempts, value.LastError, nullableTime(value.DeliveredAt), nullableTime(value.AcknowledgedAt),
		value.Version, value.CreatedAt.Format(time.RFC3339Nano), value.UpdatedAt.Format(time.RFC3339Nano))
	return translateError("insert resource delivery", err)
}

func (q *queries) UpdateDelivery(ctx context.Context, value resource.Delivery, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE resource_deliveries SET status=?, entitlement_key=?,
lease_owner=?, lease_until=?, attempts=?, last_error=?, delivered_at=?, acknowledged_at=?, version=?,
updated_at=? WHERE tenant_id=? AND id=? AND version=?`, value.Status, value.EntitlementKey,
		value.LeaseOwner, nullableTime(value.LeaseUntil), value.Attempts, value.LastError,
		nullableTime(value.DeliveredAt), nullableTime(value.AcknowledgedAt), value.Version,
		value.UpdatedAt.Format(time.RFC3339Nano), value.TenantID, value.ID, expectedVersion)
	return expectOne("update resource delivery", result, err)
}

func (q *queries) GetDelivery(ctx context.Context, tenantID, deliveryID common.ID) (resource.Delivery, error) {
	return q.scanDelivery(q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, pack_id, offering_id,
recipient_id, status, entitlement_key, lease_owner, lease_until, attempts, last_error, delivered_at,
acknowledged_at, version, created_at, updated_at FROM resource_deliveries
WHERE tenant_id=? AND id=?`, tenantID, deliveryID))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (q *queries) scanDelivery(row rowScanner) (resource.Delivery, error) {
	var value resource.Delivery
	var leaseUntil, deliveredAt, acknowledgedAt, createdAt, updatedAt sql.NullString
	err := row.Scan(&value.ID, &value.TenantID, &value.PackID, &value.OfferingID, &value.RecipientID,
		&value.Status, &value.EntitlementKey, &value.LeaseOwner, &leaseUntil, &value.Attempts,
		&value.LastError, &deliveredAt, &acknowledgedAt, &value.Version, &createdAt, &updatedAt)
	if err != nil {
		return resource.Delivery{}, translateError("scan resource delivery", err)
	}
	if value.LeaseUntil, err = parseNullableTime(leaseUntil); err != nil {
		return resource.Delivery{}, err
	}
	if value.DeliveredAt, err = parseNullableTime(deliveredAt); err != nil {
		return resource.Delivery{}, err
	}
	if value.AcknowledgedAt, err = parseNullableTime(acknowledgedAt); err != nil {
		return resource.Delivery{}, err
	}
	if value.CreatedAt, err = parseTime(createdAt.String); err != nil {
		return resource.Delivery{}, err
	}
	if value.UpdatedAt, err = parseTime(updatedAt.String); err != nil {
		return resource.Delivery{}, err
	}
	return value, nil
}

func (q *queries) ClaimDeliveries(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration, limit int) ([]resource.Delivery, error) {
	if owner == "" || leaseDuration <= 0 || limit <= 0 {
		return nil, common.FieldError{Field: "delivery_claim", Message: "owner, lease duration, and limit are required"}
	}
	rows, err := q.runner.QueryContext(ctx, `SELECT id, version FROM resource_deliveries
WHERE status IN ('pending','failed','leased') AND (status <> 'leased' OR lease_until <= ?)
ORDER BY updated_at, id LIMIT ?`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, translateError("select delivery claims", err)
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
			return nil, fmt.Errorf("scan delivery candidate: %w", err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close delivery candidates: %w", err)
	}
	claimed := make([]resource.Delivery, 0, len(candidates))
	until := now.UTC().Add(leaseDuration)
	for _, item := range candidates {
		result, err := q.runner.ExecContext(ctx, `UPDATE resource_deliveries SET status='leased', lease_owner=?,
lease_until=?, attempts=attempts+1, last_error='', version=version+1, updated_at=?
WHERE id=? AND version=? AND status IN ('pending','failed','leased')
AND (status <> 'leased' OR lease_until <= ?)`, owner, until.Format(time.RFC3339Nano),
			now.UTC().Format(time.RFC3339Nano), item.id, item.version, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return nil, translateError("claim resource delivery", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("claim delivery rows: %w", err)
		}
		if count == 0 {
			continue
		}
		value, err := q.scanDelivery(q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, pack_id, offering_id,
recipient_id, status, entitlement_key, lease_owner, lease_until, attempts, last_error, delivered_at,
acknowledged_at, version, created_at, updated_at FROM resource_deliveries WHERE id=?`, item.id))
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, value)
	}
	return claimed, nil
}
