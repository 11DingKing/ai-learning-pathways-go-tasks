package repository

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
)

type Page struct {
	Limit  int
	Offset int
}

func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

type CurriculumFilter struct {
	Statuses []curriculum.Status
	Stage    string
	Search   string
	Sort     string
}

func (f CurriculumFilter) Normalize() CurriculumFilter {
	f.Stage = strings.TrimSpace(f.Stage)
	f.Search = strings.TrimSpace(f.Search)
	switch f.Sort {
	case "created_at", "title", "code", "updated_at":
	default:
		f.Sort = "updated_at"
	}
	return f
}

type CurriculumPage struct {
	Items []curriculum.Curriculum
	Total int
	Page  Page
}

type OfferingFilter struct {
	Statuses   []offering.Status
	ActiveAt   *time.Time
	EducatorID string
	Search     string
	Sort       string
}

func (f OfferingFilter) Normalize() OfferingFilter {
	f.EducatorID = strings.TrimSpace(f.EducatorID)
	f.Search = strings.TrimSpace(f.Search)
	switch f.Sort {
	case "starts_at", "ends_at", "title", "available_seats":
	default:
		f.Sort = "starts_at"
	}
	return f
}

type OfferingPage struct {
	Items []offering.Offering
	Total int
	Page  Page
}

type AuditFilter struct {
	ActorID    string
	Action     string
	ObjectType string
	ObjectID   string
	From       *time.Time
	To         *time.Time
}

type AuditPage struct {
	Items []AuditEvent
	Total int
	Page  Page
}
