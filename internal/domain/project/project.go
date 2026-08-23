package project

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type Status string

const (
	StatusProposed Status = "proposed"
	StatusApproved Status = "approved"
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusComplete Status = "complete"
	StatusRejected Status = "rejected"
)

type Project struct {
	ID               common.ID
	TenantID         common.ID
	OfferingID       common.ID
	Title            string
	ProblemStatement string
	Status           Status
	MentorID         *common.ID
	MemberIDs        []common.ID
	ToolGrantIDs     []common.ID
	ApprovedBy       *common.ID
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func New(id, tenantID, offeringID common.ID, title, problemStatement string, members []common.ID, now time.Time) (Project, error) {
	if !id.Valid() || !tenantID.Valid() || !offeringID.Valid() {
		return Project{}, common.FieldError{Field: "project", Message: "ids are required"}
	}
	title = strings.TrimSpace(title)
	problemStatement = strings.TrimSpace(problemStatement)
	if len(title) < 5 || len(title) > 180 {
		return Project{}, common.FieldError{Field: "title", Message: "must contain 5 to 180 characters"}
	}
	if len(problemStatement) < 80 || len(problemStatement) > 5000 {
		return Project{}, common.FieldError{Field: "problem_statement", Message: "must contain 80 to 5000 characters"}
	}
	if len(members) < 2 || len(members) > 12 {
		return Project{}, common.FieldError{Field: "members", Message: "a project requires 2 to 12 learners"}
	}
	seen := make(map[common.ID]struct{}, len(members))
	clean := make([]common.ID, 0, len(members))
	for _, member := range members {
		if !member.Valid() {
			return Project{}, common.FieldError{Field: "members", Message: "contains an invalid learner"}
		}
		if _, exists := seen[member]; exists {
			continue
		}
		seen[member] = struct{}{}
		clean = append(clean, member)
	}
	if len(clean) < 2 {
		return Project{}, common.FieldError{Field: "members", Message: "requires two distinct learners"}
	}
	now = now.UTC()
	return Project{
		ID: id, TenantID: tenantID, OfferingID: offeringID, Title: title,
		ProblemStatement: problemStatement, Status: StatusProposed, MemberIDs: clean,
		ToolGrantIDs: []common.ID{}, Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (p *Project) Approve(reviewerID common.ID, now time.Time) error {
	if p.Status != StatusProposed {
		return common.StateError{Entity: "project", ID: p.ID.String(), From: string(p.Status), To: string(StatusApproved)}
	}
	if !reviewerID.Valid() {
		return common.FieldError{Field: "reviewer_id", Message: "is required"}
	}
	p.Status = StatusApproved
	p.ApprovedBy = &reviewerID
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *Project) AssignMentor(mentorID common.ID, now time.Time) error {
	if p.Status != StatusApproved && p.Status != StatusPaused {
		return common.StateError{Entity: "project", ID: p.ID.String(), From: string(p.Status), To: string(p.Status), Reason: "mentor assignment is unavailable"}
	}
	if !mentorID.Valid() {
		return common.FieldError{Field: "mentor_id", Message: "is required"}
	}
	p.MentorID = &mentorID
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *Project) SetToolGrants(grantIDs []common.ID, now time.Time) error {
	if p.Status != StatusApproved && p.Status != StatusPaused {
		return common.StateError{Entity: "project", ID: p.ID.String(), From: string(p.Status), To: string(p.Status), Reason: "tool grants are frozen while active"}
	}
	seen := make(map[common.ID]struct{}, len(grantIDs))
	clean := make([]common.ID, 0, len(grantIDs))
	for _, id := range grantIDs {
		if !id.Valid() {
			return common.FieldError{Field: "tool_grants", Message: "contains an invalid id"}
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	p.ToolGrantIDs = clean
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *Project) Launch(now time.Time) error {
	if p.Status != StatusApproved {
		return common.StateError{Entity: "project", ID: p.ID.String(), From: string(p.Status), To: string(StatusActive)}
	}
	if p.MentorID == nil || len(p.ToolGrantIDs) == 0 {
		return common.FieldError{Field: "project", Message: "mentor and tool grants are required before launch"}
	}
	p.Status = StatusActive
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *Project) Pause(now time.Time) error {
	if p.Status != StatusActive {
		return common.StateError{Entity: "project", ID: p.ID.String(), From: string(p.Status), To: string(StatusPaused)}
	}
	p.Status = StatusPaused
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *Project) Complete(now time.Time) error {
	if p.Status != StatusActive {
		return common.StateError{Entity: "project", ID: p.ID.String(), From: string(p.Status), To: string(StatusComplete)}
	}
	p.Status = StatusComplete
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}
