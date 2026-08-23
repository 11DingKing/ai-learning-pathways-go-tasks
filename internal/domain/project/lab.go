package project

import (
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type LabAllocationStatus string

const (
	LabReserved  LabAllocationStatus = "reserved"
	LabCheckedIn LabAllocationStatus = "checked_in"
	LabReleased  LabAllocationStatus = "released"
	LabCancelled LabAllocationStatus = "cancelled"
)

type LabAllocation struct {
	ID         common.ID
	TenantID   common.ID
	ProjectID  common.ID
	ResourceID common.ID
	OwnerID    common.ID
	Window     common.Window
	Units      int
	Status     LabAllocationStatus
	Version    int64
	CreatedAt  time.Time
	ReleasedAt *time.Time
}

func NewLabAllocation(id, tenantID, projectID, resourceID, ownerID common.ID, window common.Window, units int, now time.Time) (LabAllocation, error) {
	if !id.Valid() || !tenantID.Valid() || !projectID.Valid() || !resourceID.Valid() || !ownerID.Valid() {
		return LabAllocation{}, common.FieldError{Field: "lab_allocation", Message: "ids are required"}
	}
	if units <= 0 || units > 50 {
		return LabAllocation{}, common.FieldError{Field: "units", Message: "must be between 1 and 50"}
	}
	return LabAllocation{
		ID: id, TenantID: tenantID, ProjectID: projectID, ResourceID: resourceID,
		OwnerID: ownerID, Window: window, Units: units, Status: LabReserved,
		Version: 1, CreatedAt: now.UTC(),
	}, nil
}

func (a *LabAllocation) CheckIn(actorID common.ID, now time.Time) error {
	if a.Status != LabReserved {
		return common.StateError{Entity: "lab_allocation", ID: a.ID.String(), From: string(a.Status), To: string(LabCheckedIn)}
	}
	if actorID != a.OwnerID {
		return common.ErrForbidden
	}
	if !a.Window.Contains(now) {
		return common.FieldError{Field: "window", Message: "check-in must occur during the reservation"}
	}
	a.Status = LabCheckedIn
	a.Version++
	return nil
}

func (a *LabAllocation) Release(actorID common.ID, now time.Time) error {
	if a.Status != LabReserved && a.Status != LabCheckedIn {
		return common.StateError{Entity: "lab_allocation", ID: a.ID.String(), From: string(a.Status), To: string(LabReleased)}
	}
	if actorID != a.OwnerID {
		return common.ErrForbidden
	}
	value := now.UTC()
	a.Status = LabReleased
	a.ReleasedAt = &value
	a.Version++
	return nil
}

func (a *LabAllocation) Cancel(actorID common.ID, now time.Time) error {
	if a.Status != LabReserved {
		return common.StateError{Entity: "lab_allocation", ID: a.ID.String(), From: string(a.Status), To: string(LabCancelled)}
	}
	if actorID != a.OwnerID {
		return common.ErrForbidden
	}
	value := now.UTC()
	a.Status = LabCancelled
	a.ReleasedAt = &value
	a.Version++
	return nil
}
