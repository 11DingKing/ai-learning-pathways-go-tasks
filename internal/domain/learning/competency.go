package learning

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type CompetencyLevel int

const (
	LevelAware CompetencyLevel = iota + 1
	LevelGuided
	LevelIndependent
	LevelAdvanced
)

type Competency struct {
	ID           common.ID
	TenantID     common.ID
	LearnerID    common.ID
	Code         string
	Level        CompetencyLevel
	EvidenceID   common.ID
	AwardedAt    time.Time
	ExpiresAt    *time.Time
	Version      int64
	Superseded   bool
	SupersededAt *time.Time
}

func NewCompetency(id, tenantID, learnerID, evidenceID common.ID, code string, level CompetencyLevel, awardedAt time.Time, validFor time.Duration) (Competency, error) {
	if !id.Valid() || !tenantID.Valid() || !learnerID.Valid() || !evidenceID.Valid() {
		return Competency{}, common.FieldError{Field: "competency", Message: "ids are required"}
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return Competency{}, common.FieldError{Field: "code", Message: "is required"}
	}
	if level < LevelAware || level > LevelAdvanced {
		return Competency{}, common.FieldError{Field: "level", Message: "is unsupported"}
	}
	awardedAt = awardedAt.UTC()
	var expiresAt *time.Time
	if validFor > 0 {
		value := awardedAt.Add(validFor)
		expiresAt = &value
	}
	return Competency{
		ID: id, TenantID: tenantID, LearnerID: learnerID, EvidenceID: evidenceID,
		Code: code, Level: level, AwardedAt: awardedAt, ExpiresAt: expiresAt, Version: 1,
	}, nil
}

func (c Competency) ActiveAt(now time.Time) bool {
	if c.Superseded {
		return false
	}
	return c.ExpiresAt == nil || now.UTC().Before(*c.ExpiresAt)
}

func (c *Competency) Supersede(now time.Time) error {
	if c.Superseded {
		return nil
	}
	value := now.UTC()
	c.Superseded = true
	c.SupersededAt = &value
	c.Version++
	return nil
}

func HighestActive(items []Competency, code string, now time.Time) (Competency, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var selected Competency
	found := false
	for _, item := range items {
		if item.Code != code || !item.ActiveAt(now) {
			continue
		}
		if !found || item.Level > selected.Level || (item.Level == selected.Level && item.AwardedAt.After(selected.AwardedAt)) {
			selected = item
			found = true
		}
	}
	return selected, found
}
