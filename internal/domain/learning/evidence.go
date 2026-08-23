package learning

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type EvidenceStatus string

const (
	EvidenceDraft     EvidenceStatus = "draft"
	EvidenceSubmitted EvidenceStatus = "submitted"
	EvidenceReviewing EvidenceStatus = "reviewing"
	EvidenceAccepted  EvidenceStatus = "accepted"
	EvidenceRevision  EvidenceStatus = "revision_requested"
	EvidenceRejected  EvidenceStatus = "rejected"
)

type Evidence struct {
	ID             common.ID
	TenantID       common.ID
	EnrollmentID   common.ID
	LearnerID      common.ID
	OutcomeCode    string
	RubricVersion  int
	Attempt        int
	ArtifactURI    string
	ArtifactSHA256 string
	Reflection     string
	Status         EvidenceStatus
	Version        int64
	SubmittedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewEvidence(id, tenantID, enrollmentID, learnerID common.ID, outcomeCode string, rubricVersion, attempt int, now time.Time) (Evidence, error) {
	if !id.Valid() || !tenantID.Valid() || !enrollmentID.Valid() || !learnerID.Valid() {
		return Evidence{}, common.FieldError{Field: "evidence", Message: "ids are required"}
	}
	outcomeCode = strings.ToUpper(strings.TrimSpace(outcomeCode))
	if outcomeCode == "" {
		return Evidence{}, common.FieldError{Field: "outcome_code", Message: "is required"}
	}
	if rubricVersion <= 0 || attempt <= 0 {
		return Evidence{}, common.FieldError{Field: "evidence", Message: "rubric version and attempt must be positive"}
	}
	now = now.UTC()
	return Evidence{
		ID: id, TenantID: tenantID, EnrollmentID: enrollmentID, LearnerID: learnerID,
		OutcomeCode: outcomeCode, RubricVersion: rubricVersion, Attempt: attempt,
		Status: EvidenceDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (e *Evidence) Attach(uri, digest, reflection string, now time.Time) error {
	if e.Status != EvidenceDraft && e.Status != EvidenceRevision {
		return common.StateError{Entity: "evidence", ID: e.ID.String(), From: string(e.Status), To: string(e.Status), Reason: "artifact is not editable"}
	}
	uri = strings.TrimSpace(uri)
	digest = strings.ToLower(strings.TrimSpace(digest))
	reflection = strings.TrimSpace(reflection)
	if uri == "" || len(digest) != 64 {
		return common.FieldError{Field: "artifact", Message: "uri and SHA-256 digest are required"}
	}
	if len(reflection) < 30 || len(reflection) > 4000 {
		return common.FieldError{Field: "reflection", Message: "must contain 30 to 4000 characters"}
	}
	e.ArtifactURI = uri
	e.ArtifactSHA256 = digest
	e.Reflection = reflection
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *Evidence) Submit(now time.Time) error {
	if e.Status != EvidenceDraft && e.Status != EvidenceRevision {
		return common.StateError{Entity: "evidence", ID: e.ID.String(), From: string(e.Status), To: string(EvidenceSubmitted)}
	}
	if e.ArtifactURI == "" || e.ArtifactSHA256 == "" || e.Reflection == "" {
		return common.FieldError{Field: "evidence", Message: "artifact and reflection must be complete"}
	}
	value := now.UTC()
	e.Status = EvidenceSubmitted
	e.SubmittedAt = &value
	e.Version++
	e.UpdatedAt = value
	return nil
}

func (e *Evidence) StartReview(now time.Time) error {
	if e.Status != EvidenceSubmitted {
		return common.StateError{Entity: "evidence", ID: e.ID.String(), From: string(e.Status), To: string(EvidenceReviewing)}
	}
	e.Status = EvidenceReviewing
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *Evidence) ApplyDecision(decision ReviewDecision, now time.Time) error {
	if e.Status != EvidenceReviewing {
		return common.StateError{Entity: "evidence", ID: e.ID.String(), From: string(e.Status), To: string(decision.Outcome)}
	}
	switch decision.Outcome {
	case ReviewPass:
		e.Status = EvidenceAccepted
	case ReviewRevise:
		e.Status = EvidenceRevision
	case ReviewReject:
		e.Status = EvidenceRejected
	default:
		return common.FieldError{Field: "decision", Message: "outcome is unsupported"}
	}
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

type ReviewOutcome string

const (
	ReviewPass   ReviewOutcome = "pass"
	ReviewRevise ReviewOutcome = "revise"
	ReviewReject ReviewOutcome = "reject"
)

type ReviewDecision struct {
	ID         common.ID
	EvidenceID common.ID
	ReviewerID common.ID
	Outcome    ReviewOutcome
	Score      int
	Feedback   string
	CreatedAt  time.Time
}

func NewReviewDecision(id, evidenceID, reviewerID common.ID, outcome ReviewOutcome, score int, feedback string, now time.Time) (ReviewDecision, error) {
	if !id.Valid() || !evidenceID.Valid() || !reviewerID.Valid() {
		return ReviewDecision{}, common.FieldError{Field: "review", Message: "ids are required"}
	}
	if score < 0 || score > 100 {
		return ReviewDecision{}, common.FieldError{Field: "score", Message: "must be between 0 and 100"}
	}
	feedback = strings.TrimSpace(feedback)
	if len(feedback) < 15 {
		return ReviewDecision{}, common.FieldError{Field: "feedback", Message: "must explain the decision"}
	}
	switch outcome {
	case ReviewPass:
		if score < 60 {
			return ReviewDecision{}, common.FieldError{Field: "score", Message: "pass requires at least 60"}
		}
	case ReviewRevise, ReviewReject:
	default:
		return ReviewDecision{}, common.FieldError{Field: "outcome", Message: "is unsupported"}
	}
	return ReviewDecision{ID: id, EvidenceID: evidenceID, ReviewerID: reviewerID, Outcome: outcome, Score: score, Feedback: feedback, CreatedAt: now.UTC()}, nil
}
