package offering

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type Status string

const (
	StatusPlanned   Status = "planned"
	StatusActive    Status = "active"
	StatusClosed    Status = "closed"
	StatusCancelled Status = "cancelled"
)

type Offering struct {
	ID                common.ID
	TenantID          common.ID
	CurriculumID      common.ID
	CurriculumVersion int64
	Code              string
	Title             string
	Status            Status
	Window            common.Window
	Capacity          int
	ConfirmedSeats    int
	WaitlistedSeats   int
	EducatorID        common.ID
	MinimumAge        int
	RequiresGuardian  bool
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func New(id, tenantID, curriculumID, educatorID common.ID, code, title string, curriculumVersion int64, window common.Window, capacity int, now time.Time) (Offering, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	title = strings.TrimSpace(title)
	if !id.Valid() || !tenantID.Valid() || !curriculumID.Valid() || !educatorID.Valid() {
		return Offering{}, common.FieldError{Field: "offering", Message: "ids are required"}
	}
	if code == "" || title == "" {
		return Offering{}, common.FieldError{Field: "offering", Message: "code and title are required"}
	}
	if curriculumVersion <= 0 {
		return Offering{}, common.FieldError{Field: "curriculum_version", Message: "must be positive"}
	}
	if capacity < 1 || capacity > 1000 {
		return Offering{}, common.FieldError{Field: "capacity", Message: "must be between 1 and 1000"}
	}
	now = now.UTC()
	return Offering{
		ID: id, TenantID: tenantID, CurriculumID: curriculumID, CurriculumVersion: curriculumVersion,
		Code: code, Title: title, Status: StatusPlanned, Window: window, Capacity: capacity,
		EducatorID: educatorID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (o *Offering) Activate(now time.Time) error {
	if o.Status != StatusPlanned {
		return common.StateError{Entity: "offering", ID: o.ID.String(), From: string(o.Status), To: string(StatusActive)}
	}
	if !now.UTC().Before(o.Window.EndsAt) {
		return common.FieldError{Field: "window", Message: "cannot activate an ended offering"}
	}
	o.Status = StatusActive
	o.Version++
	o.UpdatedAt = now.UTC()
	return nil
}

func (o *Offering) Close(now time.Time) error {
	if o.Status != StatusActive {
		return common.StateError{Entity: "offering", ID: o.ID.String(), From: string(o.Status), To: string(StatusClosed)}
	}
	if now.UTC().Before(o.Window.EndsAt) {
		return common.FieldError{Field: "window", Message: "offering cannot close before its end"}
	}
	o.Status = StatusClosed
	o.Version++
	o.UpdatedAt = now.UTC()
	return nil
}

func (o *Offering) Cancel(now time.Time) error {
	if o.Status != StatusPlanned && o.Status != StatusActive {
		return common.StateError{Entity: "offering", ID: o.ID.String(), From: string(o.Status), To: string(StatusCancelled)}
	}
	o.Status = StatusCancelled
	o.Version++
	o.UpdatedAt = now.UTC()
	return nil
}

func (o Offering) AvailableSeats() int {
	available := o.Capacity - o.ConfirmedSeats
	if available < 0 {
		return 0
	}
	return available
}

func (o Offering) AcceptingEnrollments(now time.Time) bool {
	return o.Status == StatusActive && now.UTC().Before(o.Window.EndsAt)
}
