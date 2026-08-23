package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

func (q *queries) InsertCurriculum(ctx context.Context, value curriculum.Curriculum) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO curricula(
id, tenant_id, code, title, summary, stage, status, version, revision, created_by,
reviewed_by, review_comment, minimum_ethics_hours, minimum_practice_mins, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID, value.Code, value.Title,
		value.Summary, value.Stage, value.Status, value.Version, value.Revision, value.CreatedBy,
		nullableID(value.ReviewedBy), value.ReviewComment, value.MinimumEthicsHours,
		value.MinimumPracticeMins, value.CreatedAt.Format(time.RFC3339Nano), value.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return translateError("insert curriculum", err)
	}
	if err := q.ReplaceCurriculumOutcomes(ctx, value.ID, value.Outcomes); err != nil {
		return err
	}
	return q.ReplaceCurriculumPrerequisites(ctx, value.ID, value.PrerequisiteIDs)
}

func (q *queries) UpdateCurriculum(ctx context.Context, value curriculum.Curriculum, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE curricula SET code=?, title=?, summary=?, stage=?, status=?,
version=?, revision=?, reviewed_by=?, review_comment=?, minimum_ethics_hours=?, minimum_practice_mins=?, updated_at=?
WHERE tenant_id=? AND id=? AND version=?`, value.Code, value.Title, value.Summary, value.Stage,
		value.Status, value.Version, value.Revision, nullableID(value.ReviewedBy), value.ReviewComment,
		value.MinimumEthicsHours, value.MinimumPracticeMins, value.UpdatedAt.Format(time.RFC3339Nano),
		value.TenantID, value.ID, expectedVersion)
	return expectOne("update curriculum", result, err)
}

func (q *queries) ReplaceCurriculumOutcomes(ctx context.Context, curriculumID common.ID, outcomes []curriculum.Outcome) error {
	if _, err := q.runner.ExecContext(ctx, "DELETE FROM curriculum_outcomes WHERE curriculum_id=?", curriculumID); err != nil {
		return translateError("clear curriculum outcomes", err)
	}
	for _, outcome := range outcomes {
		if _, err := q.runner.ExecContext(ctx, `INSERT INTO curriculum_outcomes(
curriculum_id, code, title, description, kind, required) VALUES(?,?,?,?,?,?)`,
			curriculumID, outcome.Code, outcome.Title, outcome.Description, outcome.Kind, outcome.Required,
		); err != nil {
			return translateError("insert curriculum outcome", err)
		}
	}
	return nil
}

func (q *queries) ReplaceCurriculumPrerequisites(ctx context.Context, curriculumID common.ID, ids []common.ID) error {
	if _, err := q.runner.ExecContext(ctx, "DELETE FROM curriculum_prerequisites WHERE curriculum_id=?", curriculumID); err != nil {
		return translateError("clear curriculum prerequisites", err)
	}
	for _, id := range ids {
		if _, err := q.runner.ExecContext(ctx, `INSERT INTO curriculum_prerequisites(
curriculum_id, prerequisite_id) VALUES(?,?)`, curriculumID, id); err != nil {
			return translateError("insert curriculum prerequisite", err)
		}
	}
	return nil
}

const selectCurriculum = `SELECT id, tenant_id, code, title, summary, stage, status, version,
revision, created_by, reviewed_by, review_comment, minimum_ethics_hours, minimum_practice_mins,
created_at, updated_at FROM curricula`

func scanCurriculum(scanner interface{ Scan(...any) error }) (curriculum.Curriculum, error) {
	var value curriculum.Curriculum
	var reviewedBy, createdAt, updatedAt sql.NullString
	err := scanner.Scan(&value.ID, &value.TenantID, &value.Code, &value.Title, &value.Summary,
		&value.Stage, &value.Status, &value.Version, &value.Revision, &value.CreatedBy,
		&reviewedBy, &value.ReviewComment, &value.MinimumEthicsHours, &value.MinimumPracticeMins,
		&createdAt, &updatedAt)
	if err != nil {
		return curriculum.Curriculum{}, err
	}
	if reviewedBy.Valid {
		id := common.ID(reviewedBy.String)
		value.ReviewedBy = &id
	}
	var parseErr error
	if value.CreatedAt, parseErr = parseTime(createdAt.String); parseErr != nil {
		return curriculum.Curriculum{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt.String); parseErr != nil {
		return curriculum.Curriculum{}, parseErr
	}
	value.Outcomes = []curriculum.Outcome{}
	value.PrerequisiteIDs = []common.ID{}
	return value, nil
}

func (q *queries) loadCurriculumRelations(ctx context.Context, value *curriculum.Curriculum) error {
	rows, err := q.runner.QueryContext(ctx, `SELECT code, title, description, kind, required
FROM curriculum_outcomes WHERE curriculum_id=? ORDER BY code`, value.ID)
	if err != nil {
		return translateError("list curriculum outcomes", err)
	}
	defer rows.Close()
	for rows.Next() {
		var outcome curriculum.Outcome
		if err := rows.Scan(&outcome.Code, &outcome.Title, &outcome.Description, &outcome.Kind, &outcome.Required); err != nil {
			return fmt.Errorf("scan curriculum outcome: %w", err)
		}
		value.Outcomes = append(value.Outcomes, outcome)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate curriculum outcomes: %w", err)
	}
	prerequisites, err := q.runner.QueryContext(ctx, `SELECT prerequisite_id
FROM curriculum_prerequisites WHERE curriculum_id=? ORDER BY prerequisite_id`, value.ID)
	if err != nil {
		return translateError("list curriculum prerequisites", err)
	}
	defer prerequisites.Close()
	for prerequisites.Next() {
		var id common.ID
		if err := prerequisites.Scan(&id); err != nil {
			return fmt.Errorf("scan curriculum prerequisite: %w", err)
		}
		value.PrerequisiteIDs = append(value.PrerequisiteIDs, id)
	}
	return prerequisites.Err()
}

func (q *queries) GetCurriculum(ctx context.Context, tenantID, curriculumID common.ID) (curriculum.Curriculum, error) {
	value, err := scanCurriculum(q.runner.QueryRowContext(ctx, selectCurriculum+" WHERE tenant_id=? AND id=?", tenantID, curriculumID))
	if err != nil {
		return curriculum.Curriculum{}, translateError("get curriculum", err)
	}
	if err := q.loadCurriculumRelations(ctx, &value); err != nil {
		return curriculum.Curriculum{}, err
	}
	return value, nil
}

func (q *queries) ListCurricula(ctx context.Context, tenantID common.ID, filter repository.CurriculumFilter, page repository.Page) (repository.CurriculumPage, error) {
	filter = filter.Normalize()
	page = page.Normalize()
	where := []string{"tenant_id=?"}
	args := []any{tenantID}
	if filter.Stage != "" {
		where = append(where, "stage=?")
		args = append(args, filter.Stage)
	}
	if filter.Search != "" {
		where = append(where, "(code LIKE ? OR title LIKE ?)")
		pattern := "%" + filter.Search + "%"
		args = append(args, pattern, pattern)
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
	if err := q.runner.QueryRowContext(ctx, "SELECT COUNT(*) FROM curricula WHERE "+predicate, args...).Scan(&total); err != nil {
		return repository.CurriculumPage{}, translateError("count curricula", err)
	}
	queryArgs := append(append([]any{}, args...), page.Limit, page.Offset)
	rows, err := q.runner.QueryContext(ctx, selectCurriculum+" WHERE "+predicate+" ORDER BY "+filter.Sort+" DESC, id LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return repository.CurriculumPage{}, translateError("list curricula", err)
	}
	defer rows.Close()
	items := make([]curriculum.Curriculum, 0)
	for rows.Next() {
		value, err := scanCurriculum(rows)
		if err != nil {
			return repository.CurriculumPage{}, fmt.Errorf("scan curriculum list: %w", err)
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return repository.CurriculumPage{}, fmt.Errorf("iterate curriculum list: %w", err)
	}
	return repository.CurriculumPage{Items: items, Total: total, Page: page}, nil
}
