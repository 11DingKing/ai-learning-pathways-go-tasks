package service

import (
	"context"
	"fmt"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/resource"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type ResourcePackInput struct{ Title, LicenseCode, ContentURI, ContentSHA256, MinimumStage, MaximumStage string }

func (s *Service) CreateResourcePack(ctx context.Context, actor Actor, input ResourcePackInput) (resource.ResourcePack, error) {
	now := s.clock.Now()
	id, err := newID("pack")
	if err != nil {
		return resource.ResourcePack{}, err
	}
	value, err := resource.NewPack(id, actor.TenantID, input.Title, input.LicenseCode, input.ContentURI, input.ContentSHA256, input.MinimumStage, input.MaximumStage, now)
	if err != nil {
		return resource.ResourcePack{}, err
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "offering.manage"); err != nil {
			return err
		}
		if err := tx.InsertResourcePack(ctx, value); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "resource_pack.created", "resource_pack", value.ID, "success", nil, now)
	})
	return value, err
}

func (s *Service) PublishResourcePack(ctx context.Context, actor Actor, packID common.ID) (resource.ResourcePack, error) {
	now := s.clock.Now()
	var value resource.ResourcePack
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "curriculum.approve"); err != nil {
			return err
		}
		current, err := tx.GetResourcePack(ctx, actor.TenantID, packID)
		if err != nil {
			return err
		}
		expected := current.Version
		if err := current.Approve(actor.UserID, now); err != nil {
			return err
		}
		if err := current.Publish(now); err != nil {
			return err
		}
		if err := tx.UpdateResourcePack(ctx, current, expected); err != nil {
			return err
		}
		value = current
		return audit(ctx, tx, actor, "resource_pack.published", "resource_pack", current.ID, "success", map[string]any{"license": current.LicenseCode}, now)
	})
	return value, err
}

func (s *Service) ReleaseResourcePack(ctx context.Context, actor Actor, packID, offeringID common.ID, recipientIDs []common.ID) ([]resource.Delivery, error) {
	now := s.clock.Now()
	deliveries := make([]resource.Delivery, 0, len(recipientIDs))
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "offering.manage"); err != nil {
			return err
		}
		pack, err := tx.GetResourcePack(ctx, actor.TenantID, packID)
		if err != nil {
			return err
		}
		if pack.Status != resource.PackPublished {
			return common.StateError{Entity: "resource_pack", ID: pack.ID.String(), From: string(pack.Status), To: string(resource.PackPublished)}
		}
		courseOffering, err := tx.GetOffering(ctx, actor.TenantID, offeringID)
		if err != nil {
			return err
		}
		if courseOffering.Status != offering.StatusActive {
			return common.ErrForbidden
		}
		seen := make(map[common.ID]bool, len(recipientIDs))
		for _, recipientID := range recipientIDs {
			if seen[recipientID] {
				continue
			}
			seen[recipientID] = true
			enrollment, err := tx.FindEnrollment(ctx, actor.TenantID, offeringID, recipientID)
			if err != nil {
				return fmt.Errorf("recipient enrollment: %w", err)
			}
			if enrollment.Status != offering.EnrollmentConfirmed {
				return common.ErrForbidden
			}
			deliveryID, err := newID("delivery")
			if err != nil {
				return err
			}
			entitlement := pack.ID.String() + ":" + offeringID.String() + ":" + recipientID.String()
			delivery, err := resource.NewDelivery(deliveryID, actor.TenantID, pack.ID, offeringID, recipientID, entitlement, now)
			if err != nil {
				return err
			}
			if err := tx.InsertDelivery(ctx, delivery); err != nil {
				return err
			}
			jobID, err := newID("job")
			if err != nil {
				return err
			}
			deliveryJob, err := job.New(jobID, actor.TenantID, "resource.deliver", "delivery", delivery.ID, map[string]any{"delivery_id": delivery.ID}, now, 10, now)
			if err != nil {
				return err
			}
			if err := tx.InsertJob(ctx, deliveryJob); err != nil {
				return err
			}
			deliveries = append(deliveries, delivery)
		}
		if err := outbox(ctx, tx, actor.TenantID, "resource_pack", pack.ID, "resource.release_requested", map[string]any{"offering_id": offeringID, "recipients": len(deliveries)}, now); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "resource_pack.released", "resource_pack", pack.ID, "success", map[string]any{"deliveries": len(deliveries)}, now)
	})
	return deliveries, err
}
