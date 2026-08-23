package curriculum

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusReview    Status = "review"
	StatusApproved  Status = "approved"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

type OutcomeKind string

const (
	OutcomeKnowledge OutcomeKind = "knowledge"
	OutcomePractice  OutcomeKind = "practice"
	OutcomeEthics    OutcomeKind = "ethics"
	OutcomeInquiry   OutcomeKind = "inquiry"
)

type Outcome struct {
	Code        string
	Title       string
	Description string
	Kind        OutcomeKind
	Required    bool
}

func NewOutcome(code, title, description string, kind OutcomeKind, required bool) (Outcome, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if code == "" || len(code) > 40 {
		return Outcome{}, common.FieldError{Field: "outcome.code", Message: "is required and must fit 40 characters"}
	}
	if title == "" || description == "" {
		return Outcome{}, common.FieldError{Field: "outcome", Message: "title and description are required"}
	}
	switch kind {
	case OutcomeKnowledge, OutcomePractice, OutcomeEthics, OutcomeInquiry:
	default:
		return Outcome{}, common.FieldError{Field: "outcome.kind", Message: "is unsupported"}
	}
	return Outcome{Code: code, Title: title, Description: description, Kind: kind, Required: required}, nil
}

type Curriculum struct {
	ID                  common.ID
	TenantID            common.ID
	Code                string
	Title               string
	Summary             string
	Stage               identity.EducationStage
	Status              Status
	Version             int64
	Revision            int
	CreatedBy           common.ID
	ReviewedBy          *common.ID
	ReviewComment       string
	Outcomes            []Outcome
	PrerequisiteIDs     []common.ID
	MinimumEthicsHours  int
	MinimumPracticeMins int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func New(id, tenantID, creatorID common.ID, code, title, summary string, stage identity.EducationStage, now time.Time) (Curriculum, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if !id.Valid() || !tenantID.Valid() || !creatorID.Valid() {
		return Curriculum{}, common.FieldError{Field: "curriculum", Message: "ids are required"}
	}
	if code == "" || len(code) > 50 {
		return Curriculum{}, common.FieldError{Field: "code", Message: "is required and must fit 50 characters"}
	}
	if title == "" || len(title) > 180 {
		return Curriculum{}, common.FieldError{Field: "title", Message: "is required and must fit 180 characters"}
	}
	if len(summary) < 30 || len(summary) > 2000 {
		return Curriculum{}, common.FieldError{Field: "summary", Message: "must contain 30 to 2000 characters"}
	}
	if _, err := identity.ParseEducationStage(string(stage)); err != nil {
		return Curriculum{}, err
	}
	now = now.UTC()
	return Curriculum{
		ID: id, TenantID: tenantID, Code: code, Title: title, Summary: summary,
		Stage: stage, Status: StatusDraft, Version: 1, Revision: 1,
		CreatedBy: creatorID, Outcomes: []Outcome{}, PrerequisiteIDs: []common.ID{},
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (c *Curriculum) AddOutcome(outcome Outcome, now time.Time) error {
	if c.Status != StatusDraft {
		return common.StateError{Entity: "curriculum", ID: c.ID.String(), From: string(c.Status), To: string(c.Status), Reason: "outcomes are editable only in draft"}
	}
	if slices.ContainsFunc(c.Outcomes, func(existing Outcome) bool { return existing.Code == outcome.Code }) {
		return fmt.Errorf("outcome %s already exists: %w", outcome.Code, common.ErrConflict)
	}
	c.Outcomes = append(c.Outcomes, outcome)
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *Curriculum) SetPrerequisites(ids []common.ID, now time.Time) error {
	if c.Status != StatusDraft {
		return common.StateError{Entity: "curriculum", ID: c.ID.String(), From: string(c.Status), To: string(c.Status), Reason: "prerequisites are editable only in draft"}
	}
	seen := make(map[common.ID]struct{}, len(ids))
	clean := make([]common.ID, 0, len(ids))
	for _, id := range ids {
		if !id.Valid() || id == c.ID {
			return common.FieldError{Field: "prerequisites", Message: "contains an invalid or self reference"}
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	slices.Sort(clean)
	c.PrerequisiteIDs = clean
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c Curriculum) ValidateForReview() error {
	if c.Status != StatusDraft {
		return common.StateError{Entity: "curriculum", ID: c.ID.String(), From: string(c.Status), To: string(StatusReview)}
	}
	if len(c.Outcomes) < 3 {
		return common.FieldError{Field: "outcomes", Message: "at least three outcomes are required"}
	}
	hasEthics := slices.ContainsFunc(c.Outcomes, func(outcome Outcome) bool {
		return outcome.Kind == OutcomeEthics && outcome.Required
	})
	hasPractice := slices.ContainsFunc(c.Outcomes, func(outcome Outcome) bool {
		return outcome.Kind == OutcomePractice && outcome.Required
	})
	if !hasEthics || !hasPractice {
		return common.FieldError{Field: "outcomes", Message: "required ethics and practice outcomes are mandatory"}
	}
	if c.MinimumEthicsHours <= 0 || c.MinimumPracticeMins < 45 {
		return common.FieldError{Field: "learning_time", Message: "ethics hours and practice minutes are below policy"}
	}
	return nil
}

func (c *Curriculum) SubmitForReview(now time.Time) error {
	if err := c.ValidateForReview(); err != nil {
		return err
	}
	c.Status = StatusReview
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *Curriculum) Approve(reviewerID common.ID, comment string, now time.Time) error {
	if c.Status != StatusReview {
		return common.StateError{Entity: "curriculum", ID: c.ID.String(), From: string(c.Status), To: string(StatusApproved)}
	}
	if !reviewerID.Valid() {
		return common.FieldError{Field: "reviewer_id", Message: "is required"}
	}
	comment = strings.TrimSpace(comment)
	if len(comment) < 10 {
		return common.FieldError{Field: "review_comment", Message: "must explain the decision"}
	}
	c.Status = StatusApproved
	c.ReviewedBy = &reviewerID
	c.ReviewComment = comment
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *Curriculum) Publish(now time.Time) error {
	if c.Status != StatusApproved {
		return common.StateError{Entity: "curriculum", ID: c.ID.String(), From: string(c.Status), To: string(StatusPublished)}
	}
	c.Status = StatusPublished
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *Curriculum) Retire(now time.Time) error {
	if c.Status != StatusPublished {
		return common.StateError{Entity: "curriculum", ID: c.ID.String(), From: string(c.Status), To: string(StatusRetired)}
	}
	c.Status = StatusRetired
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}
