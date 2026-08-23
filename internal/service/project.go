package service

import (
	"context"
	"fmt"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/project"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type ProjectInput struct {
	OfferingID              common.ID
	Title, ProblemStatement string
	MemberIDs               []common.ID
}

func (s *Service) CreateProject(ctx context.Context, actor Actor, input ProjectInput) (project.Project, error) {
	now := s.clock.Now()
	id, err := newID("project")
	if err != nil {
		return project.Project{}, err
	}
	value, err := project.New(id, actor.TenantID, input.OfferingID, input.Title, input.ProblemStatement, input.MemberIDs, now)
	if err != nil {
		return project.Project{}, err
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "learning.participate"); err != nil {
			return err
		}
		courseOffering, err := tx.GetOffering(ctx, actor.TenantID, input.OfferingID)
		if err != nil {
			return err
		}
		if courseOffering.Status != offering.StatusActive {
			return common.StateError{Entity: "offering", ID: courseOffering.ID.String(), From: string(courseOffering.Status), To: string(offering.StatusActive)}
		}
		actorIncluded := false
		for _, memberID := range value.MemberIDs {
			if memberID == actor.UserID {
				actorIncluded = true
			}
			enrollment, err := tx.FindEnrollment(ctx, actor.TenantID, courseOffering.ID, memberID)
			if err != nil {
				return fmt.Errorf("member %s enrollment: %w", memberID, err)
			}
			if enrollment.Status != offering.EnrollmentConfirmed {
				return fmt.Errorf("member %s is not actively enrolled: %w", memberID, common.ErrForbidden)
			}
		}
		if !actorIncluded {
			return fmt.Errorf("project creator must be a member: %w", common.ErrForbidden)
		}
		if err := tx.InsertProject(ctx, value); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "project.proposed", "project", value.ID, "success", map[string]any{"members": len(value.MemberIDs)}, now)
	})
	return value, err
}

func (s *Service) ApproveProject(ctx context.Context, actor Actor, projectID, mentorID common.ID, mentorLimit int, grantIDs []common.ID) (project.Project, error) {
	now := s.clock.Now()
	var value project.Project
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "curriculum.approve"); err != nil {
			return err
		}
		current, err := tx.GetProject(ctx, actor.TenantID, projectID)
		if err != nil {
			return err
		}
		expected := current.Version
		if err := current.Approve(actor.UserID, now); err != nil {
			return err
		}
		if err := tx.ReserveMentorSlot(ctx, actor.TenantID, mentorID, mentorLimit); err != nil {
			return err
		}
		if err := current.AssignMentor(mentorID, now); err != nil {
			return err
		}
		if err := current.SetToolGrants(grantIDs, now); err != nil {
			return err
		}
		covered := make(map[common.ID]bool, len(grantIDs))
		for _, grantID := range current.ToolGrantIDs {
			grant, err := tx.GetToolGrant(ctx, actor.TenantID, grantID)
			if err != nil {
				return fmt.Errorf("load project grant: %w", err)
			}
			if !grant.ValidAt(now) {
				return fmt.Errorf("project grant %s is inactive: %w", grant.ID, common.ErrExpired)
			}
			covered[grant.LearnerID] = true
		}
		for _, memberID := range current.MemberIDs {
			if !covered[memberID] {
				return fmt.Errorf("member %s has no active tool grant: %w", memberID, common.ErrForbidden)
			}
		}
		if err := tx.UpdateProject(ctx, current, expected); err != nil {
			return err
		}
		if err := tx.ReplaceProjectToolGrants(ctx, current.ID, current.ToolGrantIDs); err != nil {
			return err
		}
		if err := audit(ctx, tx, actor, "project.approved", "project", current.ID, "success", map[string]any{"mentor_id": mentorID}, now); err != nil {
			return err
		}
		value = current
		return nil
	})
	return value, err
}

func (s *Service) LaunchProject(ctx context.Context, actor Actor, projectID common.ID) (project.Project, error) {
	now := s.clock.Now()
	var value project.Project
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "offering.manage"); err != nil {
			return err
		}
		current, err := tx.GetProject(ctx, actor.TenantID, projectID)
		if err != nil {
			return err
		}
		for _, grantID := range current.ToolGrantIDs {
			grant, err := tx.GetToolGrant(ctx, actor.TenantID, grantID)
			if err != nil {
				return err
			}
			if !grant.ValidAt(now) {
				return common.ErrExpired
			}
		}
		expected := current.Version
		if err := current.Launch(now); err != nil {
			return err
		}
		if err := tx.UpdateProject(ctx, current, expected); err != nil {
			return err
		}
		if err := outbox(ctx, tx, actor.TenantID, "project", current.ID, "project.launched", map[string]any{"members": current.MemberIDs}, now); err != nil {
			return err
		}
		value = current
		return audit(ctx, tx, actor, "project.launched", "project", current.ID, "success", nil, now)
	})
	return value, err
}

func (s *Service) ReserveLab(ctx context.Context, actor Actor, projectID, resourceID common.ID, window common.Window, units, capacity int) (project.LabAllocation, error) {
	now := s.clock.Now()
	id, err := newID("lab")
	if err != nil {
		return project.LabAllocation{}, err
	}
	value, err := project.NewLabAllocation(id, actor.TenantID, projectID, resourceID, actor.UserID, window, units, now)
	if err != nil {
		return project.LabAllocation{}, err
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "learning.participate"); err != nil {
			return err
		}
		current, err := tx.GetProject(ctx, actor.TenantID, projectID)
		if err != nil {
			return err
		}
		member := false
		for _, id := range current.MemberIDs {
			member = member || id == actor.UserID
		}
		if !member || current.Status != project.StatusActive {
			return common.ErrForbidden
		}
		if err := tx.AssertLabCapacity(ctx, actor.TenantID, resourceID, window, units, capacity); err != nil {
			return err
		}
		if err := tx.InsertLabAllocation(ctx, value); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "lab.reserved", "lab_allocation", value.ID, "success", map[string]any{"units": units}, now)
	})
	return value, err
}
