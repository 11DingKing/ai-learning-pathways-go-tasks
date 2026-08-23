package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/safety"
)

func (q *queries) InsertToolPolicy(ctx context.Context, value safety.ToolPolicy) error {
	classes, err := json.Marshal(value.AllowedDataClasses)
	if err != nil {
		return fmt.Errorf("encode tool policy data classes: %w", err)
	}
	_, err = q.runner.ExecContext(ctx, `INSERT INTO tool_policies(
id, tenant_id, tool_code, display_name, allowed_data_classes, minimum_age,
requires_guardian, requires_educator, version, active) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TenantID, value.ToolCode, value.DisplayName, classes, value.MinimumAge,
		value.RequiresGuardian, value.RequiresEducator, value.Version, value.Active)
	return translateError("insert tool policy", err)
}

func (q *queries) GetToolPolicy(ctx context.Context, tenantID, policyID common.ID) (safety.ToolPolicy, error) {
	var value safety.ToolPolicy
	var classes []byte
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, tool_code, display_name,
allowed_data_classes, minimum_age, requires_guardian, requires_educator, version, active
FROM tool_policies WHERE tenant_id=? AND id=?`, tenantID, policyID).Scan(&value.ID, &value.TenantID,
		&value.ToolCode, &value.DisplayName, &classes, &value.MinimumAge, &value.RequiresGuardian,
		&value.RequiresEducator, &value.Version, &value.Active)
	if err != nil {
		return safety.ToolPolicy{}, translateError("get tool policy", err)
	}
	if err := json.Unmarshal(classes, &value.AllowedDataClasses); err != nil {
		return safety.ToolPolicy{}, fmt.Errorf("decode tool policy classes: %w", err)
	}
	return value, nil
}

func (q *queries) InsertConsent(ctx context.Context, value safety.Consent) error {
	classes, err := json.Marshal(value.DataClasses)
	if err != nil {
		return fmt.Errorf("encode consent data classes: %w", err)
	}
	_, err = q.runner.ExecContext(ctx, `INSERT INTO data_consents(
id, tenant_id, learner_id, purpose, data_classes, granted_by, granted_at,
expires_at, revoked_at, version) VALUES(?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID,
		value.LearnerID, value.Purpose, classes, value.GrantedBy, value.GrantedAt.Format(time.RFC3339Nano),
		value.ExpiresAt.Format(time.RFC3339Nano), nullableTime(value.RevokedAt), value.Version)
	return translateError("insert consent", err)
}

func (q *queries) UpdateConsent(ctx context.Context, value safety.Consent, expectedVersion int64) error {
	classes, err := json.Marshal(value.DataClasses)
	if err != nil {
		return fmt.Errorf("encode consent data classes: %w", err)
	}
	result, err := q.runner.ExecContext(ctx, `UPDATE data_consents SET purpose=?, data_classes=?,
expires_at=?, revoked_at=?, version=? WHERE tenant_id=? AND id=? AND version=?`, value.Purpose,
		classes, value.ExpiresAt.Format(time.RFC3339Nano), nullableTime(value.RevokedAt), value.Version,
		value.TenantID, value.ID, expectedVersion)
	return expectOne("update consent", result, err)
}

func (q *queries) GetConsent(ctx context.Context, tenantID, consentID common.ID) (safety.Consent, error) {
	var value safety.Consent
	var classes []byte
	var grantedAt, expiresAt, revokedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, learner_id, purpose,
data_classes, granted_by, granted_at, expires_at, revoked_at, version FROM data_consents
WHERE tenant_id=? AND id=?`, tenantID, consentID).Scan(&value.ID, &value.TenantID,
		&value.LearnerID, &value.Purpose, &classes, &value.GrantedBy, &grantedAt,
		&expiresAt, &revokedAt, &value.Version)
	if err != nil {
		return safety.Consent{}, translateError("get consent", err)
	}
	if err := json.Unmarshal(classes, &value.DataClasses); err != nil {
		return safety.Consent{}, fmt.Errorf("decode consent classes: %w", err)
	}
	var parseErr error
	if value.GrantedAt, parseErr = parseTime(grantedAt.String); parseErr != nil {
		return safety.Consent{}, parseErr
	}
	if value.ExpiresAt, parseErr = parseTime(expiresAt.String); parseErr != nil {
		return safety.Consent{}, parseErr
	}
	if value.RevokedAt, parseErr = parseNullableTime(revokedAt); parseErr != nil {
		return safety.Consent{}, parseErr
	}
	return value, nil
}

func (q *queries) InsertToolGrant(ctx context.Context, value safety.ToolGrant) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO tool_grants(
id, tenant_id, learner_id, tool_policy_id, consent_id, purpose, status, issued_at,
expires_at, revoked_at, version) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID,
		value.LearnerID, value.ToolPolicyID, value.ConsentID, value.Purpose, value.Status,
		value.IssuedAt.Format(time.RFC3339Nano), value.ExpiresAt.Format(time.RFC3339Nano),
		nullableTime(value.RevokedAt), value.Version)
	return translateError("insert tool grant", err)
}

func (q *queries) UpdateToolGrant(ctx context.Context, value safety.ToolGrant, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE tool_grants SET purpose=?, status=?,
expires_at=?, revoked_at=?, version=? WHERE tenant_id=? AND id=? AND version=?`, value.Purpose,
		value.Status, value.ExpiresAt.Format(time.RFC3339Nano), nullableTime(value.RevokedAt),
		value.Version, value.TenantID, value.ID, expectedVersion)
	return expectOne("update tool grant", result, err)
}

func (q *queries) GetToolGrant(ctx context.Context, tenantID, grantID common.ID) (safety.ToolGrant, error) {
	var value safety.ToolGrant
	var issuedAt, expiresAt, revokedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, learner_id, tool_policy_id,
consent_id, purpose, status, issued_at, expires_at, revoked_at, version FROM tool_grants
WHERE tenant_id=? AND id=?`, tenantID, grantID).Scan(&value.ID, &value.TenantID, &value.LearnerID,
		&value.ToolPolicyID, &value.ConsentID, &value.Purpose, &value.Status, &issuedAt,
		&expiresAt, &revokedAt, &value.Version)
	if err != nil {
		return safety.ToolGrant{}, translateError("get tool grant", err)
	}
	var parseErr error
	if value.IssuedAt, parseErr = parseTime(issuedAt.String); parseErr != nil {
		return safety.ToolGrant{}, parseErr
	}
	if value.ExpiresAt, parseErr = parseTime(expiresAt.String); parseErr != nil {
		return safety.ToolGrant{}, parseErr
	}
	if value.RevokedAt, parseErr = parseNullableTime(revokedAt); parseErr != nil {
		return safety.ToolGrant{}, parseErr
	}
	return value, nil
}

func (q *queries) InsertIncident(ctx context.Context, value safety.Incident) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO safety_incidents(
id, tenant_id, learner_id, tool_grant_id, severity, summary, status, reported_by,
reported_at, resolved_at, version) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID,
		value.LearnerID, value.ToolGrantID, value.Severity, value.Summary, value.Status,
		value.ReportedBy, value.ReportedAt.Format(time.RFC3339Nano), nullableTime(value.ResolvedAt), value.Version)
	return translateError("insert incident", err)
}

func (q *queries) UpdateIncident(ctx context.Context, value safety.Incident, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE safety_incidents SET severity=?, summary=?,
status=?, resolved_at=?, version=? WHERE tenant_id=? AND id=? AND version=?`, value.Severity,
		value.Summary, value.Status, nullableTime(value.ResolvedAt), value.Version,
		value.TenantID, value.ID, expectedVersion)
	return expectOne("update incident", result, err)
}

func (q *queries) GetIncident(ctx context.Context, tenantID, incidentID common.ID) (safety.Incident, error) {
	var value safety.Incident
	var reportedAt, resolvedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, learner_id, tool_grant_id,
severity, summary, status, reported_by, reported_at, resolved_at, version FROM safety_incidents
WHERE tenant_id=? AND id=?`, tenantID, incidentID).Scan(&value.ID, &value.TenantID, &value.LearnerID,
		&value.ToolGrantID, &value.Severity, &value.Summary, &value.Status, &value.ReportedBy,
		&reportedAt, &resolvedAt, &value.Version)
	if err != nil {
		return safety.Incident{}, translateError("get incident", err)
	}
	var parseErr error
	if value.ReportedAt, parseErr = parseTime(reportedAt.String); parseErr != nil {
		return safety.Incident{}, parseErr
	}
	if value.ResolvedAt, parseErr = parseNullableTime(resolvedAt); parseErr != nil {
		return safety.Incident{}, parseErr
	}
	return value, nil
}

func (q *queries) InsertRemediation(ctx context.Context, value safety.Remediation) error {
	required, err := json.Marshal(value.RequiredTasks)
	if err != nil {
		return fmt.Errorf("encode remediation requirements: %w", err)
	}
	completed, err := json.Marshal(value.Completed)
	if err != nil {
		return fmt.Errorf("encode remediation completion: %w", err)
	}
	_, err = q.runner.ExecContext(ctx, `INSERT INTO remediations(
id, tenant_id, incident_id, assigned_to, required_tasks, completed_tasks, due_at, version)
VALUES(?,?,?,?,?,?,?,?)`, value.ID, value.TenantID, value.IncidentID, value.AssignedTo,
		required, completed, value.DueAt.Format(time.RFC3339Nano), value.Version)
	return translateError("insert remediation", err)
}
