package safety

import (
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type DataClass string

const (
	DataPublic      DataClass = "public"
	DataEducational DataClass = "educational"
	DataPersonal    DataClass = "personal"
	DataSensitive   DataClass = "sensitive"
)

type ToolPolicy struct {
	ID                 common.ID
	TenantID           common.ID
	ToolCode           string
	DisplayName        string
	AllowedDataClasses []DataClass
	MinimumAge         int
	RequiresGuardian   bool
	RequiresEducator   bool
	Version            int64
	Active             bool
}

type Consent struct {
	ID          common.ID
	TenantID    common.ID
	LearnerID   common.ID
	Purpose     string
	DataClasses []DataClass
	GrantedBy   common.ID
	GrantedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	Version     int64
}

func (c Consent) ActiveFor(purpose string, required []DataClass, now time.Time) bool {
	if c.RevokedAt != nil || !now.UTC().Before(c.ExpiresAt) || c.Purpose != purpose {
		return false
	}
	allowed := make(map[DataClass]bool, len(c.DataClasses))
	for _, class := range c.DataClasses {
		allowed[class] = true
	}
	for _, class := range required {
		if !allowed[class] {
			return false
		}
	}
	return true
}

type ToolGrantStatus string

const (
	GrantActive  ToolGrantStatus = "active"
	GrantRevoked ToolGrantStatus = "revoked"
	GrantExpired ToolGrantStatus = "expired"
)

type ToolGrant struct {
	ID           common.ID
	TenantID     common.ID
	LearnerID    common.ID
	ToolPolicyID common.ID
	ConsentID    common.ID
	Purpose      string
	Status       ToolGrantStatus
	IssuedAt     time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	Version      int64
}

func NewToolGrant(id, tenantID, learnerID common.ID, policy ToolPolicy, consent Consent, purpose string, learnerAge int, educatorApproved, guardianApproved bool, now, expiresAt time.Time) (ToolGrant, error) {
	purpose = strings.TrimSpace(purpose)
	if !id.Valid() || !tenantID.Valid() || !learnerID.Valid() || !policy.ID.Valid() || !consent.ID.Valid() {
		return ToolGrant{}, common.FieldError{Field: "tool_grant", Message: "ids are required"}
	}
	if !policy.Active || policy.TenantID != tenantID || consent.TenantID != tenantID || consent.LearnerID != learnerID {
		return ToolGrant{}, common.ErrForbidden
	}
	if purpose == "" {
		return ToolGrant{}, common.FieldError{Field: "purpose", Message: "is required"}
	}
	if learnerAge < policy.MinimumAge || (policy.RequiresEducator && !educatorApproved) || (policy.RequiresGuardian && !guardianApproved) {
		return ToolGrant{}, common.ErrForbidden
	}
	if !consent.ActiveFor(purpose, policy.AllowedDataClasses, now) {
		return ToolGrant{}, common.ErrExpired
	}
	if !expiresAt.After(now) || expiresAt.After(consent.ExpiresAt) {
		return ToolGrant{}, common.FieldError{Field: "expires_at", Message: "must be inside the active consent window"}
	}
	return ToolGrant{
		ID: id, TenantID: tenantID, LearnerID: learnerID, ToolPolicyID: policy.ID,
		ConsentID: consent.ID, Purpose: purpose, Status: GrantActive,
		IssuedAt: now.UTC(), ExpiresAt: expiresAt.UTC(), Version: 1,
	}, nil
}

func (g ToolGrant) ValidAt(now time.Time) bool {
	return g.Status == GrantActive && g.RevokedAt == nil && now.UTC().Before(g.ExpiresAt)
}

func (g *ToolGrant) Revoke(now time.Time) error {
	if g.Status != GrantActive {
		return common.StateError{Entity: "tool_grant", ID: g.ID.String(), From: string(g.Status), To: string(GrantRevoked)}
	}
	value := now.UTC()
	g.Status = GrantRevoked
	g.RevokedAt = &value
	g.Version++
	return nil
}
