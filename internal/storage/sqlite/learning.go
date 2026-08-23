package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/learning"
)

func (q *queries) InsertEvidence(ctx context.Context, value learning.Evidence) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO evidence_submissions(
id, tenant_id, enrollment_id, learner_id, outcome_code, rubric_version, attempt,
artifact_uri, artifact_sha256, reflection, status, version, submitted_at, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID, value.EnrollmentID,
		value.LearnerID, value.OutcomeCode, value.RubricVersion, value.Attempt, value.ArtifactURI,
		value.ArtifactSHA256, value.Reflection, value.Status, value.Version, nullableTime(value.SubmittedAt),
		value.CreatedAt.Format(time.RFC3339Nano), value.UpdatedAt.Format(time.RFC3339Nano))
	return translateError("insert evidence", err)
}

func (q *queries) UpdateEvidence(ctx context.Context, value learning.Evidence, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE evidence_submissions SET artifact_uri=?,
artifact_sha256=?, reflection=?, status=?, version=?, submitted_at=?, updated_at=?
WHERE tenant_id=? AND id=? AND version=?`, value.ArtifactURI, value.ArtifactSHA256,
		value.Reflection, value.Status, value.Version, nullableTime(value.SubmittedAt),
		value.UpdatedAt.Format(time.RFC3339Nano), value.TenantID, value.ID, expectedVersion)
	return expectOne("update evidence", result, err)
}

func (q *queries) GetEvidence(ctx context.Context, tenantID, evidenceID common.ID) (learning.Evidence, error) {
	var value learning.Evidence
	var submittedAt, createdAt, updatedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, enrollment_id, learner_id,
outcome_code, rubric_version, attempt, artifact_uri, artifact_sha256, reflection, status,
version, submitted_at, created_at, updated_at FROM evidence_submissions
WHERE tenant_id=? AND id=?`, tenantID, evidenceID).Scan(&value.ID, &value.TenantID,
		&value.EnrollmentID, &value.LearnerID, &value.OutcomeCode, &value.RubricVersion,
		&value.Attempt, &value.ArtifactURI, &value.ArtifactSHA256, &value.Reflection,
		&value.Status, &value.Version, &submittedAt, &createdAt, &updatedAt)
	if err != nil {
		return learning.Evidence{}, translateError("get evidence", err)
	}
	var parseErr error
	if value.SubmittedAt, parseErr = parseNullableTime(submittedAt); parseErr != nil {
		return learning.Evidence{}, parseErr
	}
	if value.CreatedAt, parseErr = parseTime(createdAt.String); parseErr != nil {
		return learning.Evidence{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt.String); parseErr != nil {
		return learning.Evidence{}, parseErr
	}
	return value, nil
}

func (q *queries) InsertReviewDecision(ctx context.Context, value learning.ReviewDecision) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO review_decisions(
id, evidence_id, reviewer_id, outcome, score, feedback, created_at) VALUES(?,?,?,?,?,?,?)`,
		value.ID, value.EvidenceID, value.ReviewerID, value.Outcome, value.Score,
		value.Feedback, value.CreatedAt.Format(time.RFC3339Nano))
	return translateError("insert review decision", err)
}

func (q *queries) InsertCompetency(ctx context.Context, value learning.Competency) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO competencies(
id, tenant_id, learner_id, code, level, evidence_id, awarded_at, expires_at,
version, superseded, superseded_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID,
		value.LearnerID, value.Code, value.Level, value.EvidenceID, value.AwardedAt.Format(time.RFC3339Nano),
		nullableTime(value.ExpiresAt), value.Version, value.Superseded, nullableTime(value.SupersededAt))
	return translateError("insert competency", err)
}

func (q *queries) SupersedeCompetencies(ctx context.Context, tenantID, learnerID common.ID, code string, now time.Time) error {
	_, err := q.runner.ExecContext(ctx, `UPDATE competencies SET superseded=1, superseded_at=?,
version=version+1 WHERE tenant_id=? AND learner_id=? AND code=? AND superseded=0`,
		now.UTC().Format(time.RFC3339Nano), tenantID, learnerID, code)
	return translateError("supersede competencies", err)
}

func (q *queries) ListCompetencies(ctx context.Context, tenantID, learnerID common.ID) ([]learning.Competency, error) {
	rows, err := q.runner.QueryContext(ctx, `SELECT id, tenant_id, learner_id, code, level,
evidence_id, awarded_at, expires_at, version, superseded, superseded_at FROM competencies
WHERE tenant_id=? AND learner_id=? ORDER BY code, level DESC, awarded_at DESC`, tenantID, learnerID)
	if err != nil {
		return nil, translateError("list competencies", err)
	}
	defer rows.Close()
	items := make([]learning.Competency, 0)
	for rows.Next() {
		var value learning.Competency
		var awardedAt, expiresAt, supersededAt sql.NullString
		if err := rows.Scan(&value.ID, &value.TenantID, &value.LearnerID, &value.Code,
			&value.Level, &value.EvidenceID, &awardedAt, &expiresAt, &value.Version,
			&value.Superseded, &supersededAt); err != nil {
			return nil, fmt.Errorf("scan competency: %w", err)
		}
		var parseErr error
		if value.AwardedAt, parseErr = parseTime(awardedAt.String); parseErr != nil {
			return nil, parseErr
		}
		if value.ExpiresAt, parseErr = parseNullableTime(expiresAt); parseErr != nil {
			return nil, parseErr
		}
		if value.SupersededAt, parseErr = parseNullableTime(supersededAt); parseErr != nil {
			return nil, parseErr
		}
		items = append(items, value)
	}
	return items, rows.Err()
}
