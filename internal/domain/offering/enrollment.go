package offering

import (
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type EnrollmentStatus string

const (
	EnrollmentPending   EnrollmentStatus = "pending"
	EnrollmentConfirmed EnrollmentStatus = "confirmed"
	EnrollmentWaitlist  EnrollmentStatus = "waitlisted"
	EnrollmentCompleted EnrollmentStatus = "completed"
	EnrollmentWithdrawn EnrollmentStatus = "withdrawn"
)

type Enrollment struct {
	ID                common.ID
	TenantID          common.ID
	OfferingID        common.ID
	LearnerID         common.ID
	Status            EnrollmentStatus
	GuardianConsentID *common.ID
	IdempotencyKey    string
	CompletedAt       *time.Time
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewEnrollment(id, tenantID, offeringID, learnerID common.ID, idempotencyKey string, now time.Time) (Enrollment, error) {
	if !id.Valid() || !tenantID.Valid() || !offeringID.Valid() || !learnerID.Valid() {
		return Enrollment{}, common.FieldError{Field: "enrollment", Message: "ids are required"}
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 160 {
		return Enrollment{}, common.FieldError{Field: "idempotency_key", Message: "must contain 8 to 160 characters"}
	}
	now = now.UTC()
	return Enrollment{
		ID: id, TenantID: tenantID, OfferingID: offeringID, LearnerID: learnerID,
		Status: EnrollmentPending, IdempotencyKey: idempotencyKey, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (e *Enrollment) Confirm(now time.Time) error {
	if e.Status != EnrollmentPending && e.Status != EnrollmentWaitlist {
		return common.StateError{Entity: "enrollment", ID: e.ID.String(), From: string(e.Status), To: string(EnrollmentConfirmed)}
	}
	e.Status = EnrollmentConfirmed
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *Enrollment) Waitlist(now time.Time) error {
	if e.Status != EnrollmentPending {
		return common.StateError{Entity: "enrollment", ID: e.ID.String(), From: string(e.Status), To: string(EnrollmentWaitlist)}
	}
	e.Status = EnrollmentWaitlist
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *Enrollment) Complete(now time.Time) error {
	if e.Status != EnrollmentConfirmed {
		return common.StateError{Entity: "enrollment", ID: e.ID.String(), From: string(e.Status), To: string(EnrollmentCompleted)}
	}
	value := now.UTC()
	e.Status = EnrollmentCompleted
	e.CompletedAt = &value
	e.Version++
	e.UpdatedAt = value
	return nil
}

func (e *Enrollment) Withdraw(now time.Time) error {
	if e.Status == EnrollmentCompleted || e.Status == EnrollmentWithdrawn {
		return common.StateError{Entity: "enrollment", ID: e.ID.String(), From: string(e.Status), To: string(EnrollmentWithdrawn)}
	}
	e.Status = EnrollmentWithdrawn
	e.Version++
	e.UpdatedAt = now.UTC()
	return nil
}
