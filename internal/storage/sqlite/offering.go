package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

func (q *queries) InsertOffering(ctx context.Context, value offering.Offering) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO offerings(
id, tenant_id, curriculum_id, curriculum_version, code, title, status, starts_at, ends_at,
capacity, confirmed_seats, waitlisted_seats, educator_id, minimum_age, requires_guardian,
version, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TenantID, value.CurriculumID, value.CurriculumVersion, value.Code,
		value.Title, value.Status, value.Window.StartsAt.Format(time.RFC3339Nano),
		value.Window.EndsAt.Format(time.RFC3339Nano), value.Capacity, value.ConfirmedSeats,
		value.WaitlistedSeats, value.EducatorID, value.MinimumAge, value.RequiresGuardian,
		value.Version, value.CreatedAt.Format(time.RFC3339Nano), value.UpdatedAt.Format(time.RFC3339Nano))
	return translateError("insert offering", err)
}

func (q *queries) UpdateOffering(ctx context.Context, value offering.Offering, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE offerings SET code=?, title=?, status=?, starts_at=?, ends_at=?,
capacity=?, confirmed_seats=?, waitlisted_seats=?, educator_id=?, minimum_age=?, requires_guardian=?,
version=?, updated_at=? WHERE tenant_id=? AND id=? AND version=?`, value.Code, value.Title,
		value.Status, value.Window.StartsAt.Format(time.RFC3339Nano), value.Window.EndsAt.Format(time.RFC3339Nano),
		value.Capacity, value.ConfirmedSeats, value.WaitlistedSeats, value.EducatorID, value.MinimumAge,
		value.RequiresGuardian, value.Version, value.UpdatedAt.Format(time.RFC3339Nano), value.TenantID,
		value.ID, expectedVersion)
	return expectOne("update offering", result, err)
}

const selectOffering = `SELECT id, tenant_id, curriculum_id, curriculum_version, code, title,
status, starts_at, ends_at, capacity, confirmed_seats, waitlisted_seats, educator_id,
minimum_age, requires_guardian, version, created_at, updated_at FROM offerings`

func scanOffering(scanner interface{ Scan(...any) error }) (offering.Offering, error) {
	var value offering.Offering
	var startsAt, endsAt, createdAt, updatedAt string
	err := scanner.Scan(&value.ID, &value.TenantID, &value.CurriculumID, &value.CurriculumVersion,
		&value.Code, &value.Title, &value.Status, &startsAt, &endsAt, &value.Capacity,
		&value.ConfirmedSeats, &value.WaitlistedSeats, &value.EducatorID, &value.MinimumAge,
		&value.RequiresGuardian, &value.Version, &createdAt, &updatedAt)
	if err != nil {
		return offering.Offering{}, err
	}
	var parseErr error
	if value.Window.StartsAt, parseErr = parseTime(startsAt); parseErr != nil {
		return offering.Offering{}, parseErr
	}
	if value.Window.EndsAt, parseErr = parseTime(endsAt); parseErr != nil {
		return offering.Offering{}, parseErr
	}
	if value.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return offering.Offering{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return offering.Offering{}, parseErr
	}
	return value, nil
}

func (q *queries) GetOffering(ctx context.Context, tenantID, offeringID common.ID) (offering.Offering, error) {
	value, err := scanOffering(q.runner.QueryRowContext(ctx, selectOffering+" WHERE tenant_id=? AND id=?", tenantID, offeringID))
	return value, translateError("get offering", err)
}

func (q *queries) ListOfferings(ctx context.Context, tenantID common.ID, filter repository.OfferingFilter, page repository.Page) (repository.OfferingPage, error) {
	filter = filter.Normalize()
	page = page.Normalize()
	where := []string{"tenant_id=?"}
	args := []any{tenantID}
	if filter.EducatorID != "" {
		where = append(where, "educator_id=?")
		args = append(args, filter.EducatorID)
	}
	if filter.Search != "" {
		where = append(where, "(code LIKE ? OR title LIKE ?)")
		pattern := "%" + filter.Search + "%"
		args = append(args, pattern, pattern)
	}
	if filter.ActiveAt != nil {
		where = append(where, "starts_at<=? AND ends_at>?")
		value := filter.ActiveAt.UTC().Format(time.RFC3339Nano)
		args = append(args, value, value)
	}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for index, status := range filter.Statuses {
			placeholders[index] = "?"
			args = append(args, status)
		}
		where = append(where, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	predicate := strings.Join(where, " AND ")
	var total int
	if err := q.runner.QueryRowContext(ctx, "SELECT COUNT(*) FROM offerings WHERE "+predicate, args...).Scan(&total); err != nil {
		return repository.OfferingPage{}, translateError("count offerings", err)
	}
	order := filter.Sort
	if order == "available_seats" {
		order = "(capacity-confirmed_seats)"
	}
	queryArgs := append(append([]any{}, args...), page.Limit, page.Offset)
	rows, err := q.runner.QueryContext(ctx, selectOffering+" WHERE "+predicate+" ORDER BY "+order+", id LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return repository.OfferingPage{}, translateError("list offerings", err)
	}
	defer rows.Close()
	items := make([]offering.Offering, 0)
	for rows.Next() {
		value, err := scanOffering(rows)
		if err != nil {
			return repository.OfferingPage{}, fmt.Errorf("scan offering list: %w", err)
		}
		items = append(items, value)
	}
	return repository.OfferingPage{Items: items, Total: total, Page: page}, rows.Err()
}

func (q *queries) ReserveOfferingSeat(ctx context.Context, tenantID, offeringID common.ID, expectedVersion int64) (offering.Offering, error) {
	result, err := q.runner.ExecContext(ctx, `UPDATE offerings SET confirmed_seats=confirmed_seats+1,
version=version+1, updated_at=? WHERE tenant_id=? AND id=? AND version=? AND status='active'
AND confirmed_seats < capacity`, time.Now().UTC().Format(time.RFC3339Nano), tenantID, offeringID, expectedVersion)
	if err := expectOne("reserve offering seat", result, err); err != nil {
		return offering.Offering{}, err
	}
	return q.GetOffering(ctx, tenantID, offeringID)
}

func (q *queries) ReleaseOfferingSeat(ctx context.Context, tenantID, offeringID common.ID) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE offerings SET confirmed_seats=confirmed_seats-1,
version=version+1, updated_at=? WHERE tenant_id=? AND id=? AND confirmed_seats > 0`,
		time.Now().UTC().Format(time.RFC3339Nano), tenantID, offeringID)
	return expectOne("release offering seat", result, err)
}

func (q *queries) InsertEnrollment(ctx context.Context, value offering.Enrollment) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO enrollments(
id, tenant_id, offering_id, learner_id, status, guardian_consent_id, idempotency_key,
completed_at, version, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TenantID, value.OfferingID, value.LearnerID, value.Status,
		nullableID(value.GuardianConsentID), value.IdempotencyKey, nullableTime(value.CompletedAt),
		value.Version, value.CreatedAt.Format(time.RFC3339Nano), value.UpdatedAt.Format(time.RFC3339Nano))
	return translateError("insert enrollment", err)
}

func (q *queries) UpdateEnrollment(ctx context.Context, value offering.Enrollment, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE enrollments SET status=?, guardian_consent_id=?,
completed_at=?, version=?, updated_at=? WHERE tenant_id=? AND id=? AND version=?`, value.Status,
		nullableID(value.GuardianConsentID), nullableTime(value.CompletedAt), value.Version,
		value.UpdatedAt.Format(time.RFC3339Nano), value.TenantID, value.ID, expectedVersion)
	return expectOne("update enrollment", result, err)
}

const selectEnrollment = `SELECT id, tenant_id, offering_id, learner_id, status,
guardian_consent_id, idempotency_key, completed_at, version, created_at, updated_at FROM enrollments`

func scanEnrollment(scanner interface{ Scan(...any) error }) (offering.Enrollment, error) {
	var value offering.Enrollment
	var guardian, completedAt, createdAt, updatedAt sql.NullString
	err := scanner.Scan(&value.ID, &value.TenantID, &value.OfferingID, &value.LearnerID,
		&value.Status, &guardian, &value.IdempotencyKey, &completedAt, &value.Version, &createdAt, &updatedAt)
	if err != nil {
		return offering.Enrollment{}, err
	}
	if guardian.Valid {
		id := common.ID(guardian.String)
		value.GuardianConsentID = &id
	}
	var parseErr error
	if value.CompletedAt, parseErr = parseNullableTime(completedAt); parseErr != nil {
		return offering.Enrollment{}, parseErr
	}
	if value.CreatedAt, parseErr = parseTime(createdAt.String); parseErr != nil {
		return offering.Enrollment{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt.String); parseErr != nil {
		return offering.Enrollment{}, parseErr
	}
	return value, nil
}

func (q *queries) GetEnrollment(ctx context.Context, tenantID, enrollmentID common.ID) (offering.Enrollment, error) {
	value, err := scanEnrollment(q.runner.QueryRowContext(ctx, selectEnrollment+" WHERE tenant_id=? AND id=?", tenantID, enrollmentID))
	return value, translateError("get enrollment", err)
}

func (q *queries) FindEnrollment(ctx context.Context, tenantID, offeringID, learnerID common.ID) (offering.Enrollment, error) {
	value, err := scanEnrollment(q.runner.QueryRowContext(ctx, selectEnrollment+" WHERE tenant_id=? AND offering_id=? AND learner_id=?", tenantID, offeringID, learnerID))
	return value, translateError("find enrollment", err)
}
