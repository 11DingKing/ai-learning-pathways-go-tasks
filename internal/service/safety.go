package service

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/safety"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type ToolGrantInput struct {
	LearnerID, PolicyID, ConsentID     common.ID
	Purpose                            string
	LearnerAge                         int
	EducatorApproved, GuardianApproved bool
	ExpiresAt                          time.Time
}

func (s *Service) IssueToolGrant(ctx context.Context, actor Actor, input ToolGrantInput) (safety.ToolGrant, error) {
	now := s.clock.Now()
	id, err := newID("grant")
	if err != nil {
		return safety.ToolGrant{}, err
	}
	var value safety.ToolGrant
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "offering.manage"); err != nil {
			return err
		}
		policy, err := tx.GetToolPolicy(ctx, actor.TenantID, input.PolicyID)
		if err != nil {
			return err
		}
		consent, err := tx.GetConsent(ctx, actor.TenantID, input.ConsentID)
		if err != nil {
			return err
		}
		created, err := safety.NewToolGrant(id, actor.TenantID, input.LearnerID, policy, consent, input.Purpose, input.LearnerAge, input.EducatorApproved, input.GuardianApproved, now, input.ExpiresAt)
		if err != nil {
			return err
		}
		if err := tx.InsertToolGrant(ctx, created); err != nil {
			return err
		}
		if err := audit(ctx, tx, actor, "tool_grant.issued", "tool_grant", created.ID, "success", map[string]any{"learner_id": created.LearnerID, "purpose": created.Purpose}, now); err != nil {
			return err
		}
		value = created
		return nil
	})
	return value, err
}

type IncidentInput struct {
	LearnerID, ToolGrantID common.ID
	Severity               safety.IncidentSeverity
	Summary                string
	RequiredTasks          []string
	RemediationDueAt       time.Time
}

func (s *Service) ReportIncident(ctx context.Context, actor Actor, input IncidentInput) (safety.Incident, safety.Remediation, error) {
	now := s.clock.Now()
	incidentID, err := newID("incident")
	if err != nil {
		return safety.Incident{}, safety.Remediation{}, err
	}
	remediationID, err := newID("remediation")
	if err != nil {
		return safety.Incident{}, safety.Remediation{}, err
	}
	incident, err := safety.NewIncident(incidentID, actor.TenantID, input.LearnerID, input.ToolGrantID, actor.UserID, input.Severity, input.Summary, now)
	if err != nil {
		return safety.Incident{}, safety.Remediation{}, err
	}
	remediation := safety.Remediation{ID: remediationID, TenantID: actor.TenantID, IncidentID: incident.ID, AssignedTo: input.LearnerID, RequiredTasks: input.RequiredTasks, Completed: map[string]time.Time{}, DueAt: input.RemediationDueAt.UTC(), Version: 1}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "incident.review"); err != nil {
			return err
		}
		grant, err := tx.GetToolGrant(ctx, actor.TenantID, input.ToolGrantID)
		if err != nil {
			return err
		}
		if grant.LearnerID != input.LearnerID {
			return common.ErrForbidden
		}
		grantVersion := grant.Version
		if grant.ValidAt(now) {
			if err := grant.Revoke(now); err != nil {
				return err
			}
			if err := tx.UpdateToolGrant(ctx, grant, grantVersion); err != nil {
				return fmt.Errorf("restrict tool access: %w", err)
			}
		}
		if err := incident.Restrict(now); err != nil {
			return err
		}
		if err := incident.BeginRemediation(now); err != nil {
			return err
		}
		if err := tx.InsertIncident(ctx, incident); err != nil {
			return err
		}
		if err := tx.InsertRemediation(ctx, remediation); err != nil {
			return err
		}
		if err := outbox(ctx, tx, actor.TenantID, "incident", incident.ID, "safety.remediation_assigned", map[string]any{"learner_id": input.LearnerID, "due_at": remediation.DueAt}, now); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "incident.restricted", "incident", incident.ID, "success", map[string]any{"severity": incident.Severity}, now)
	})
	return incident, remediation, err
}
