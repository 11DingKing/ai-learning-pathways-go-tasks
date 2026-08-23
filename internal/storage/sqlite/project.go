package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/project"
)

func (q *queries) InsertProject(ctx context.Context, value project.Project) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO projects(
id, tenant_id, offering_id, title, problem_statement, status, mentor_id, approved_by,
version, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID,
		value.OfferingID, value.Title, value.ProblemStatement, value.Status, nullableID(value.MentorID),
		nullableID(value.ApprovedBy), value.Version, value.CreatedAt.Format(time.RFC3339Nano),
		value.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return translateError("insert project", err)
	}
	if err := q.ReplaceProjectMembers(ctx, value.ID, value.MemberIDs); err != nil {
		return err
	}
	return q.ReplaceProjectToolGrants(ctx, value.ID, value.ToolGrantIDs)
}

func (q *queries) UpdateProject(ctx context.Context, value project.Project, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE projects SET title=?, problem_statement=?, status=?,
mentor_id=?, approved_by=?, version=?, updated_at=? WHERE tenant_id=? AND id=? AND version=?`,
		value.Title, value.ProblemStatement, value.Status, nullableID(value.MentorID), nullableID(value.ApprovedBy),
		value.Version, value.UpdatedAt.Format(time.RFC3339Nano), value.TenantID, value.ID, expectedVersion)
	return expectOne("update project", result, err)
}

func (q *queries) ReplaceProjectMembers(ctx context.Context, projectID common.ID, members []common.ID) error {
	if _, err := q.runner.ExecContext(ctx, "DELETE FROM project_members WHERE project_id=?", projectID); err != nil {
		return translateError("clear project members", err)
	}
	for _, member := range members {
		if _, err := q.runner.ExecContext(ctx, "INSERT INTO project_members(project_id, learner_id) VALUES(?,?)", projectID, member); err != nil {
			return translateError("insert project member", err)
		}
	}
	return nil
}

func (q *queries) ReplaceProjectToolGrants(ctx context.Context, projectID common.ID, grants []common.ID) error {
	if _, err := q.runner.ExecContext(ctx, "DELETE FROM project_tool_grants WHERE project_id=?", projectID); err != nil {
		return translateError("clear project tool grants", err)
	}
	for _, grant := range grants {
		if _, err := q.runner.ExecContext(ctx, "INSERT INTO project_tool_grants(project_id, grant_id) VALUES(?,?)", projectID, grant); err != nil {
			return translateError("insert project tool grant", err)
		}
	}
	return nil
}

func (q *queries) GetProject(ctx context.Context, tenantID, projectID common.ID) (project.Project, error) {
	var value project.Project
	var mentorID, approvedBy, createdAt, updatedAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, offering_id, title, problem_statement,
status, mentor_id, approved_by, version, created_at, updated_at FROM projects
WHERE tenant_id=? AND id=?`, tenantID, projectID).Scan(&value.ID, &value.TenantID, &value.OfferingID,
		&value.Title, &value.ProblemStatement, &value.Status, &mentorID, &approvedBy, &value.Version,
		&createdAt, &updatedAt)
	if err != nil {
		return project.Project{}, translateError("get project", err)
	}
	if mentorID.Valid {
		id := common.ID(mentorID.String)
		value.MentorID = &id
	}
	if approvedBy.Valid {
		id := common.ID(approvedBy.String)
		value.ApprovedBy = &id
	}
	var parseErr error
	if value.CreatedAt, parseErr = parseTime(createdAt.String); parseErr != nil {
		return project.Project{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt.String); parseErr != nil {
		return project.Project{}, parseErr
	}
	value.MemberIDs = []common.ID{}
	value.ToolGrantIDs = []common.ID{}
	members, err := q.runner.QueryContext(ctx, "SELECT learner_id FROM project_members WHERE project_id=? ORDER BY learner_id", value.ID)
	if err != nil {
		return project.Project{}, translateError("list project members", err)
	}
	defer members.Close()
	for members.Next() {
		var id common.ID
		if err := members.Scan(&id); err != nil {
			return project.Project{}, fmt.Errorf("scan project member: %w", err)
		}
		value.MemberIDs = append(value.MemberIDs, id)
	}
	grants, err := q.runner.QueryContext(ctx, "SELECT grant_id FROM project_tool_grants WHERE project_id=? ORDER BY grant_id", value.ID)
	if err != nil {
		return project.Project{}, translateError("list project grants", err)
	}
	defer grants.Close()
	for grants.Next() {
		var id common.ID
		if err := grants.Scan(&id); err != nil {
			return project.Project{}, fmt.Errorf("scan project grant: %w", err)
		}
		value.ToolGrantIDs = append(value.ToolGrantIDs, id)
	}
	return value, grants.Err()
}

func (q *queries) ReserveMentorSlot(ctx context.Context, tenantID, mentorID common.ID, limit int) error {
	if limit <= 0 {
		return common.FieldError{Field: "mentor_limit", Message: "must be positive"}
	}
	if _, err := q.runner.ExecContext(ctx, `INSERT INTO mentor_loads(tenant_id, mentor_id, active_projects, version)
VALUES(?,?,0,1) ON CONFLICT(tenant_id, mentor_id) DO NOTHING`, tenantID, mentorID); err != nil {
		return translateError("ensure mentor load", err)
	}
	result, err := q.runner.ExecContext(ctx, `UPDATE mentor_loads SET active_projects=active_projects+1,
version=version+1 WHERE tenant_id=? AND mentor_id=? AND active_projects < ?`, tenantID, mentorID, limit)
	if err := expectOne("reserve mentor slot", result, err); err != nil {
		return common.CapacityError{Resource: "mentor", Requested: 1, Available: 0}
	}
	return nil
}

func (q *queries) ReleaseMentorSlot(ctx context.Context, tenantID, mentorID common.ID) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE mentor_loads SET active_projects=active_projects-1,
version=version+1 WHERE tenant_id=? AND mentor_id=? AND active_projects > 0`, tenantID, mentorID)
	return expectOne("release mentor slot", result, err)
}

func (q *queries) AssertLabCapacity(ctx context.Context, tenantID, resourceID common.ID, window common.Window, units, capacity int) error {
	var reserved int
	err := q.runner.QueryRowContext(ctx, `SELECT COALESCE(SUM(units),0) FROM lab_allocations
WHERE tenant_id=? AND resource_id=? AND status IN ('reserved','checked_in') AND starts_at < ? AND ends_at > ?`,
		tenantID, resourceID, window.EndsAt.Format(time.RFC3339Nano), window.StartsAt.Format(time.RFC3339Nano)).Scan(&reserved)
	if err != nil {
		return translateError("calculate lab capacity", err)
	}
	if reserved+units > capacity {
		return common.CapacityError{Resource: "lab_resource", Requested: units, Available: capacity - reserved}
	}
	return nil
}

func (q *queries) InsertLabAllocation(ctx context.Context, value project.LabAllocation) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO lab_allocations(
id, tenant_id, project_id, resource_id, owner_id, starts_at, ends_at, units, status,
version, created_at, released_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TenantID,
		value.ProjectID, value.ResourceID, value.OwnerID, value.Window.StartsAt.Format(time.RFC3339Nano),
		value.Window.EndsAt.Format(time.RFC3339Nano), value.Units, value.Status, value.Version,
		value.CreatedAt.Format(time.RFC3339Nano), nullableTime(value.ReleasedAt))
	return translateError("insert lab allocation", err)
}

func (q *queries) UpdateLabAllocation(ctx context.Context, value project.LabAllocation, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE lab_allocations SET owner_id=?, starts_at=?, ends_at=?,
units=?, status=?, version=?, released_at=? WHERE tenant_id=? AND id=? AND version=?`, value.OwnerID,
		value.Window.StartsAt.Format(time.RFC3339Nano), value.Window.EndsAt.Format(time.RFC3339Nano),
		value.Units, value.Status, value.Version, nullableTime(value.ReleasedAt), value.TenantID, value.ID, expectedVersion)
	return expectOne("update lab allocation", result, err)
}
