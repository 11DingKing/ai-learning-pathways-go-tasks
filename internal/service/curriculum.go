package service

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type CurriculumInput struct {
	Code, Title, Summary                       string
	Stage                                      identity.EducationStage
	Outcomes                                   []curriculum.Outcome
	Prerequisites                              []common.ID
	MinimumEthicsHours, MinimumPracticeMinutes int
}

func (s *Service) CreateCurriculum(ctx context.Context, actor Actor, input CurriculumInput) (curriculum.Curriculum, error) {
	now := s.clock.Now()
	id, err := newID("curriculum")
	if err != nil {
		return curriculum.Curriculum{}, err
	}
	value, err := curriculum.New(id, actor.TenantID, actor.UserID, input.Code, input.Title, input.Summary, input.Stage, now)
	if err != nil {
		return curriculum.Curriculum{}, err
	}
	value.MinimumEthicsHours = input.MinimumEthicsHours
	value.MinimumPracticeMins = input.MinimumPracticeMinutes
	for _, outcome := range input.Outcomes {
		if err := value.AddOutcome(outcome, now); err != nil {
			return curriculum.Curriculum{}, err
		}
	}
	if err := value.SetPrerequisites(input.Prerequisites, now); err != nil {
		return curriculum.Curriculum{}, err
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "curriculum.propose"); err != nil {
			return err
		}
		for _, prerequisiteID := range value.PrerequisiteIDs {
			prerequisite, err := tx.GetCurriculum(ctx, actor.TenantID, prerequisiteID)
			if err != nil {
				return fmt.Errorf("load prerequisite %s: %w", prerequisiteID, err)
			}
			if prerequisite.Status != curriculum.StatusPublished {
				return common.StateError{Entity: "curriculum", ID: prerequisite.ID.String(), From: string(prerequisite.Status), To: string(curriculum.StatusPublished), Reason: "prerequisite must already be published"}
			}
		}
		if err := tx.InsertCurriculum(ctx, value); err != nil {
			return fmt.Errorf("create curriculum: %w", err)
		}
		return audit(ctx, tx, actor, "curriculum.created", "curriculum", value.ID, "success", map[string]any{"stage": value.Stage}, now)
	})
	return value, err
}

func (s *Service) SubmitCurriculum(ctx context.Context, actor Actor, curriculumID common.ID) (curriculum.Curriculum, error) {
	return s.changeCurriculum(ctx, actor, curriculumID, "curriculum.submitted", "curriculum.propose", func(value *curriculum.Curriculum, now time.Time) error { return value.SubmitForReview(now) })
}

func (s *Service) ApproveCurriculum(ctx context.Context, actor Actor, curriculumID common.ID, comment string) (curriculum.Curriculum, error) {
	return s.changeCurriculum(ctx, actor, curriculumID, "curriculum.approved", "curriculum.approve", func(value *curriculum.Curriculum, now time.Time) error {
		return value.Approve(actor.UserID, comment, now)
	})
}

func (s *Service) PublishCurriculum(ctx context.Context, actor Actor, curriculumID common.ID) (curriculum.Curriculum, error) {
	return s.changeCurriculum(ctx, actor, curriculumID, "curriculum.published", "curriculum.approve", func(value *curriculum.Curriculum, now time.Time) error { return value.Publish(now) })
}

func (s *Service) changeCurriculum(ctx context.Context, actor Actor, id common.ID, event, permission string, mutate func(*curriculum.Curriculum, time.Time) error) (curriculum.Curriculum, error) {
	now := s.clock.Now()
	var value curriculum.Curriculum
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, permission); err != nil {
			return err
		}
		current, err := tx.GetCurriculum(ctx, actor.TenantID, id)
		if err != nil {
			return err
		}
		expected := current.Version
		if err := mutate(&current, now); err != nil {
			return err
		}
		if err := tx.UpdateCurriculum(ctx, current, expected); err != nil {
			return fmt.Errorf("persist curriculum transition: %w", err)
		}
		if err := audit(ctx, tx, actor, event, "curriculum", current.ID, "success", map[string]any{"version": current.Version}, now); err != nil {
			return err
		}
		if err := outbox(ctx, tx, actor.TenantID, "curriculum", current.ID, event, map[string]any{"curriculum_id": current.ID, "status": current.Status}, now); err != nil {
			return err
		}
		value = current
		return nil
	})
	return value, err
}
