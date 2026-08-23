package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type OfferingInput struct {
	CurriculumID         common.ID
	Code, Title          string
	StartsAt, EndsAt     time.Time
	Capacity, MinimumAge int
	RequiresGuardian     bool
}

func (s *Service) CreateOffering(ctx context.Context, actor Actor, input OfferingInput) (offering.Offering, error) {
	window, err := common.NewWindow(input.StartsAt, input.EndsAt)
	if err != nil {
		return offering.Offering{}, err
	}
	id, err := newID("offering")
	if err != nil {
		return offering.Offering{}, err
	}
	now := s.clock.Now()
	var value offering.Offering
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "offering.manage"); err != nil {
			return err
		}
		course, err := tx.GetCurriculum(ctx, actor.TenantID, input.CurriculumID)
		if err != nil {
			return err
		}
		if course.Status != curriculum.StatusPublished {
			return common.StateError{Entity: "curriculum", ID: course.ID.String(), From: string(course.Status), To: string(curriculum.StatusPublished)}
		}
		created, err := offering.New(id, actor.TenantID, course.ID, actor.UserID, input.Code, input.Title, course.Version, window, input.Capacity, now)
		if err != nil {
			return err
		}
		created.MinimumAge = input.MinimumAge
		created.RequiresGuardian = input.RequiresGuardian
		if err := tx.InsertOffering(ctx, created); err != nil {
			return err
		}
		if err := audit(ctx, tx, actor, "offering.created", "offering", created.ID, "success", map[string]any{"curriculum_version": course.Version}, now); err != nil {
			return err
		}
		value = created
		return nil
	})
	return value, err
}

func (s *Service) ActivateOffering(ctx context.Context, actor Actor, id common.ID) (offering.Offering, error) {
	now := s.clock.Now()
	var value offering.Offering
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "offering.manage"); err != nil {
			return err
		}
		current, err := tx.GetOffering(ctx, actor.TenantID, id)
		if err != nil {
			return err
		}
		course, err := tx.GetCurriculum(ctx, actor.TenantID, current.CurriculumID)
		if err != nil {
			return err
		}
		if course.Status != curriculum.StatusPublished || course.Version != current.CurriculumVersion {
			return fmt.Errorf("offering curriculum snapshot is no longer publishable: %w", common.ErrConflict)
		}
		expected := current.Version
		if err := current.Activate(now); err != nil {
			return err
		}
		if err := tx.UpdateOffering(ctx, current, expected); err != nil {
			return err
		}
		if err := outbox(ctx, tx, actor.TenantID, "offering", current.ID, "offering.activated", map[string]any{"starts_at": current.Window.StartsAt}, now); err != nil {
			return err
		}
		value = current
		return audit(ctx, tx, actor, "offering.activated", "offering", current.ID, "success", nil, now)
	})
	return value, err
}

type EnrollmentInput struct {
	OfferingID        common.ID
	IdempotencyKey    string
	GuardianConsentID *common.ID
}

func (s *Service) Enroll(ctx context.Context, actor Actor, input EnrollmentInput) (offering.Enrollment, error) {
	now := s.clock.Now()
	id, err := newID("enrollment")
	if err != nil {
		return offering.Enrollment{}, err
	}
	requestBytes, _ := json.Marshal(input)
	sum := sha256.Sum256(requestBytes)
	requestHash := hex.EncodeToString(sum[:])
	scope := repository.EnrollmentIdempotencyScope(actor.TenantID, input.OfferingID)
	var value offering.Enrollment
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		learner, err := s.authorizedUser(ctx, tx, actor, "learning.participate")
		if err != nil {
			return err
		}
		if prior, err := tx.GetIdempotency(ctx, scope, input.IdempotencyKey); err == nil {
			if prior.RequestHash != requestHash {
				return fmt.Errorf("idempotency key reused with different request: %w", common.ErrConflict)
			}
			if prior.State != "completed" {
				return fmt.Errorf("enrollment request is still processing: %w", common.ErrConflict)
			}
			var saved offering.Enrollment
			if err := json.Unmarshal(prior.Response, &saved); err != nil {
				return fmt.Errorf("decode enrollment outcome: %w", err)
			}
			value = saved
			return nil
		}
		if err := tx.InsertIdempotency(ctx, repository.IdempotencyRecord{Scope: scope, Key: input.IdempotencyKey, RequestHash: requestHash, State: "processing", Response: json.RawMessage(`{}`), CreatedAt: now}); err != nil {
			return err
		}
		courseOffering, err := tx.GetOffering(ctx, actor.TenantID, input.OfferingID)
		if err != nil {
			return err
		}
		if !courseOffering.AcceptingEnrollments(now) {
			return common.StateError{Entity: "offering", ID: courseOffering.ID.String(), From: string(courseOffering.Status), To: "accepting_enrollments"}
		}
		if learner.BirthDate != nil {
			age := now.Year() - learner.BirthDate.Year()
			anniversary := time.Date(now.Year(), learner.BirthDate.Month(), learner.BirthDate.Day(), 0, 0, 0, 0, time.UTC)
			if now.Before(anniversary) {
				age--
			}
			if age < courseOffering.MinimumAge {
				return common.ErrForbidden
			}
		}
		if courseOffering.RequiresGuardian && input.GuardianConsentID == nil {
			return common.FieldError{Field: "guardian_consent_id", Message: "is required for this offering"}
		}
		enrollment, err := offering.NewEnrollment(id, actor.TenantID, courseOffering.ID, actor.UserID, input.IdempotencyKey, now)
		if err != nil {
			return err
		}
		enrollment.GuardianConsentID = input.GuardianConsentID
		reserved, err := tx.ReserveOfferingSeat(ctx, actor.TenantID, courseOffering.ID, courseOffering.Version)
		if err != nil {
			return fmt.Errorf("reserve enrollment seat: %w", err)
		}
		if err := enrollment.Confirm(now); err != nil {
			return err
		}
		if err := tx.InsertEnrollment(ctx, enrollment); err != nil {
			return err
		}
		body, err := json.Marshal(enrollment)
		if err != nil {
			return err
		}
		if err := tx.CompleteIdempotency(ctx, scope, input.IdempotencyKey, 201, body, now); err != nil {
			return err
		}
		if err := audit(ctx, tx, actor, "enrollment.confirmed", "enrollment", enrollment.ID, "success", map[string]any{"offering_version": reserved.Version}, now); err != nil {
			return err
		}
		if err := outbox(ctx, tx, actor.TenantID, "enrollment", enrollment.ID, "enrollment.confirmed", map[string]any{"learner_id": actor.UserID}, now); err != nil {
			return err
		}
		value = enrollment
		return nil
	})
	return value, err
}
