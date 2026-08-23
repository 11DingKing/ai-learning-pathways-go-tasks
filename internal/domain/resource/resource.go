package resource

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type PackStatus string

const (
	PackDraft     PackStatus = "draft"
	PackApproved  PackStatus = "approved"
	PackPublished PackStatus = "published"
	PackRetired   PackStatus = "retired"
)

type ResourcePack struct {
	ID            common.ID
	TenantID      common.ID
	Title         string
	LicenseCode   string
	ContentURI    string
	ContentSHA256 string
	MinimumStage  string
	MaximumStage  string
	Status        PackStatus
	ApprovedBy    *common.ID
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewPack(id, tenantID common.ID, title, licenseCode, contentURI, digest, minimumStage, maximumStage string, now time.Time) (ResourcePack, error) {
	if !id.Valid() || !tenantID.Valid() {
		return ResourcePack{}, common.FieldError{Field: "resource_pack", Message: "ids are required"}
	}
	title = strings.TrimSpace(title)
	licenseCode = strings.ToUpper(strings.TrimSpace(licenseCode))
	contentURI = strings.TrimSpace(contentURI)
	digest = strings.ToLower(strings.TrimSpace(digest))
	if title == "" || len(title) > 180 {
		return ResourcePack{}, common.FieldError{Field: "title", Message: "is required and must fit 180 characters"}
	}
	if licenseCode == "" || contentURI == "" || len(digest) != 64 {
		return ResourcePack{}, common.FieldError{Field: "resource", Message: "license, content uri, and SHA-256 are required"}
	}
	if strings.TrimSpace(minimumStage) == "" || strings.TrimSpace(maximumStage) == "" {
		return ResourcePack{}, common.FieldError{Field: "stage_range", Message: "is required"}
	}
	now = now.UTC()
	return ResourcePack{
		ID: id, TenantID: tenantID, Title: title, LicenseCode: licenseCode,
		ContentURI: contentURI, ContentSHA256: digest, MinimumStage: minimumStage,
		MaximumStage: maximumStage, Status: PackDraft, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (p *ResourcePack) Approve(reviewerID common.ID, now time.Time) error {
	if p.Status != PackDraft {
		return common.StateError{Entity: "resource_pack", ID: p.ID.String(), From: string(p.Status), To: string(PackApproved)}
	}
	if !reviewerID.Valid() {
		return common.FieldError{Field: "reviewer_id", Message: "is required"}
	}
	p.Status = PackApproved
	p.ApprovedBy = &reviewerID
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *ResourcePack) Publish(now time.Time) error {
	if p.Status != PackApproved {
		return common.StateError{Entity: "resource_pack", ID: p.ID.String(), From: string(p.Status), To: string(PackPublished)}
	}
	p.Status = PackPublished
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryLeased    DeliveryStatus = "leased"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryAcked     DeliveryStatus = "acknowledged"
	DeliveryFailed    DeliveryStatus = "failed"
)

type Delivery struct {
	ID             common.ID
	TenantID       common.ID
	PackID         common.ID
	OfferingID     common.ID
	RecipientID    common.ID
	Status         DeliveryStatus
	EntitlementKey string
	LeaseOwner     string
	LeaseUntil     *time.Time
	Attempts       int
	LastError      string
	DeliveredAt    *time.Time
	AcknowledgedAt *time.Time
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewDelivery(id, tenantID, packID, offeringID, recipientID common.ID, entitlementKey string, now time.Time) (Delivery, error) {
	if !id.Valid() || !tenantID.Valid() || !packID.Valid() || !offeringID.Valid() || !recipientID.Valid() {
		return Delivery{}, common.FieldError{Field: "delivery", Message: "ids are required"}
	}
	entitlementKey = strings.TrimSpace(entitlementKey)
	if len(entitlementKey) < 8 || len(entitlementKey) > 180 {
		return Delivery{}, common.FieldError{Field: "entitlement_key", Message: "must contain 8 to 180 characters"}
	}
	now = now.UTC()
	return Delivery{
		ID: id, TenantID: tenantID, PackID: packID, OfferingID: offeringID,
		RecipientID: recipientID, Status: DeliveryPending, EntitlementKey: entitlementKey,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (d *Delivery) Lease(owner string, now time.Time, duration time.Duration) error {
	owner = strings.TrimSpace(owner)
	if owner == "" || duration <= 0 {
		return common.FieldError{Field: "lease", Message: "owner and positive duration are required"}
	}
	if d.Status != DeliveryPending && d.Status != DeliveryFailed {
		return common.StateError{Entity: "delivery", ID: d.ID.String(), From: string(d.Status), To: string(DeliveryLeased)}
	}
	until := now.UTC().Add(duration)
	d.Status = DeliveryLeased
	d.LeaseOwner = owner
	d.LeaseUntil = &until
	d.Attempts++
	d.LastError = ""
	d.Version++
	d.UpdatedAt = now.UTC()
	return nil
}

func (d *Delivery) MarkDelivered(owner string, now time.Time) error {
	if d.Status != DeliveryLeased || d.LeaseOwner != owner || d.LeaseUntil == nil || !now.UTC().Before(*d.LeaseUntil) {
		return common.ErrLeaseLost
	}
	value := now.UTC()
	d.Status = DeliveryDelivered
	d.DeliveredAt = &value
	d.LeaseOwner = ""
	d.LeaseUntil = nil
	d.Version++
	d.UpdatedAt = value
	return nil
}

func (d *Delivery) Fail(owner string, cause error, now time.Time) error {
	if d.Status != DeliveryLeased || d.LeaseOwner != owner {
		return common.ErrLeaseLost
	}
	if cause == nil {
		return common.FieldError{Field: "cause", Message: "is required"}
	}
	d.Status = DeliveryFailed
	d.LastError = cause.Error()
	d.LeaseOwner = ""
	d.LeaseUntil = nil
	d.Version++
	d.UpdatedAt = now.UTC()
	return nil
}

func (d *Delivery) Acknowledge(now time.Time) error {
	if d.Status != DeliveryDelivered {
		return common.StateError{Entity: "delivery", ID: d.ID.String(), From: string(d.Status), To: string(DeliveryAcked)}
	}
	value := now.UTC()
	d.Status = DeliveryAcked
	d.AcknowledgedAt = &value
	d.Version++
	d.UpdatedAt = value
	return nil
}
