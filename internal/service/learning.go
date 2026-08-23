package service

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/learning"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type EvidenceInput struct {
	EnrollmentID                            common.ID
	OutcomeCode                             string
	RubricVersion, Attempt                  int
	ArtifactURI, ArtifactSHA256, Reflection string
}

func (s *Service) SubmitEvidence(ctx context.Context, actor Actor, input EvidenceInput) (learning.Evidence, error) {
	now := s.clock.Now()
	id, err := newID("evidence")
	if err != nil {
		return learning.Evidence{}, err
	}
	value, err := learning.NewEvidence(id, actor.TenantID, input.EnrollmentID, actor.UserID, input.OutcomeCode, input.RubricVersion, input.Attempt, now)
	if err != nil {
		return learning.Evidence{}, err
	}
	if err := value.Attach(input.ArtifactURI, input.ArtifactSHA256, input.Reflection, now); err != nil {
		return learning.Evidence{}, err
	}
	if err := value.Submit(now); err != nil {
		return learning.Evidence{}, err
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "evidence.submit"); err != nil {
			return err
		}
		enrollment, err := tx.GetEnrollment(ctx, actor.TenantID, input.EnrollmentID)
		if err != nil {
			return err
		}
		if enrollment.LearnerID != actor.UserID || enrollment.Status != offering.EnrollmentConfirmed {
			return common.ErrForbidden
		}
		if err := tx.InsertEvidence(ctx, value); err != nil {
			return err
		}
		jobID, err := newID("job")
		if err != nil {
			return err
		}
		reviewJob, err := job.New(jobID, actor.TenantID, "evidence.review_requested", "evidence", value.ID, map[string]any{"evidence_id": value.ID, "rubric_version": value.RubricVersion}, now, 8, now)
		if err != nil {
			return err
		}
		if err := tx.InsertJob(ctx, reviewJob); err != nil {
			return err
		}
		if err := outbox(ctx, tx, actor.TenantID, "evidence", value.ID, "evidence.submitted", map[string]any{"job_id": jobID}, now); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "evidence.submitted", "evidence", value.ID, "success", map[string]any{"attempt": value.Attempt}, now)
	})
	return value, err
}

type ReviewInput struct {
	EvidenceID      common.ID
	Outcome         learning.ReviewOutcome
	Score           int
	Feedback        string
	CompetencyCode  string
	CompetencyLevel learning.CompetencyLevel
	ValidFor        time.Duration
}

func (s *Service) ReviewEvidence(ctx context.Context, actor Actor, input ReviewInput) (learning.Evidence, *learning.Competency, error) {
	now := s.clock.Now()
	decisionID, err := newID("review")
	if err != nil {
		return learning.Evidence{}, nil, err
	}
	decision, err := learning.NewReviewDecision(decisionID, input.EvidenceID, actor.UserID, input.Outcome, input.Score, input.Feedback, now)
	if err != nil {
		return learning.Evidence{}, nil, err
	}
	var value learning.Evidence
	var awarded *learning.Competency
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "evidence.review"); err != nil {
			return err
		}
		current, err := tx.GetEvidence(ctx, actor.TenantID, input.EvidenceID)
		if err != nil {
			return err
		}
		expected := current.Version
		if current.Status == learning.EvidenceSubmitted {
			if err := current.StartReview(now); err != nil {
				return err
			}
		}
		if err := current.ApplyDecision(decision, now); err != nil {
			return err
		}
		if err := tx.UpdateEvidence(ctx, current, expected); err != nil {
			return fmt.Errorf("publish evidence decision: %w", err)
		}
		if err := tx.InsertReviewDecision(ctx, decision); err != nil {
			return err
		}
		if decision.Outcome == learning.ReviewPass {
			competencyID, err := newID("competency")
			if err != nil {
				return err
			}
			competency, err := learning.NewCompetency(competencyID, actor.TenantID, current.LearnerID, current.ID, input.CompetencyCode, input.CompetencyLevel, now, input.ValidFor)
			if err != nil {
				return err
			}
			if err := tx.SupersedeCompetencies(ctx, actor.TenantID, current.LearnerID, competency.Code, now); err != nil {
				return err
			}
			if err := tx.InsertCompetency(ctx, competency); err != nil {
				return err
			}
			if err := outbox(ctx, tx, actor.TenantID, "competency", competency.ID, "competency.awarded", map[string]any{"learner_id": competency.LearnerID, "level": competency.Level}, now); err != nil {
				return err
			}
			awarded = &competency
		}
		if err := audit(ctx, tx, actor, "evidence.reviewed", "evidence", current.ID, "success", map[string]any{"outcome": decision.Outcome, "score": decision.Score}, now); err != nil {
			return err
		}
		value = current
		return nil
	})
	return value, awarded, err
}
