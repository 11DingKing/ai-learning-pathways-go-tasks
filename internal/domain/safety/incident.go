package safety

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type IncidentSeverity string

const (
	SeverityLow      IncidentSeverity = "low"
	SeverityModerate IncidentSeverity = "moderate"
	SeverityHigh     IncidentSeverity = "high"
	SeverityCritical IncidentSeverity = "critical"
)

type IncidentStatus string

const (
	IncidentOpen        IncidentStatus = "open"
	IncidentRestricted  IncidentStatus = "restricted"
	IncidentRemediating IncidentStatus = "remediating"
	IncidentResolved    IncidentStatus = "resolved"
)

type Incident struct {
	ID          common.ID
	TenantID    common.ID
	LearnerID   common.ID
	ToolGrantID common.ID
	Severity    IncidentSeverity
	Summary     string
	Status      IncidentStatus
	ReportedBy  common.ID
	ReportedAt  time.Time
	ResolvedAt  *time.Time
	Version     int64
}

func NewIncident(id, tenantID, learnerID, grantID, reporterID common.ID, severity IncidentSeverity, summary string, now time.Time) (Incident, error) {
	if !id.Valid() || !tenantID.Valid() || !learnerID.Valid() || !grantID.Valid() || !reporterID.Valid() {
		return Incident{}, common.FieldError{Field: "incident", Message: "ids are required"}
	}
	summary = strings.TrimSpace(summary)
	if len(summary) < 20 || len(summary) > 2000 {
		return Incident{}, common.FieldError{Field: "summary", Message: "must contain 20 to 2000 characters"}
	}
	switch severity {
	case SeverityLow, SeverityModerate, SeverityHigh, SeverityCritical:
	default:
		return Incident{}, common.FieldError{Field: "severity", Message: "is unsupported"}
	}
	return Incident{
		ID: id, TenantID: tenantID, LearnerID: learnerID, ToolGrantID: grantID,
		Severity: severity, Summary: summary, Status: IncidentOpen, ReportedBy: reporterID,
		ReportedAt: now.UTC(), Version: 1,
	}, nil
}

func (i *Incident) Restrict(now time.Time) error {
	if i.Status != IncidentOpen {
		return common.StateError{Entity: "incident", ID: i.ID.String(), From: string(i.Status), To: string(IncidentRestricted)}
	}
	i.Status = IncidentRestricted
	i.Version++
	return nil
}

func (i *Incident) BeginRemediation(now time.Time) error {
	if i.Status != IncidentRestricted {
		return common.StateError{Entity: "incident", ID: i.ID.String(), From: string(i.Status), To: string(IncidentRemediating)}
	}
	i.Status = IncidentRemediating
	i.Version++
	return nil
}

func (i *Incident) Resolve(now time.Time) error {
	if i.Status != IncidentRemediating {
		return common.StateError{Entity: "incident", ID: i.ID.String(), From: string(i.Status), To: string(IncidentResolved)}
	}
	value := now.UTC()
	i.Status = IncidentResolved
	i.ResolvedAt = &value
	i.Version++
	return nil
}

type Remediation struct {
	ID            common.ID
	TenantID      common.ID
	IncidentID    common.ID
	AssignedTo    common.ID
	RequiredTasks []string
	Completed     map[string]time.Time
	DueAt         time.Time
	Version       int64
}

func (r Remediation) Complete() bool {
	if len(r.RequiredTasks) == 0 {
		return false
	}
	for _, task := range r.RequiredTasks {
		if _, ok := r.Completed[task]; !ok {
			return false
		}
	}
	return true
}
