package identity

import (
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type Role string

const (
	RoleLearner      Role = "learner"
	RoleEducator     Role = "educator"
	RoleReviewer     Role = "reviewer"
	RoleProgramAdmin Role = "program_admin"
)

func ParseRole(value string) (Role, error) {
	role := Role(strings.TrimSpace(value))
	switch role {
	case RoleLearner, RoleEducator, RoleReviewer, RoleProgramAdmin:
		return role, nil
	default:
		return "", common.FieldError{Field: "role", Message: "is unsupported"}
	}
}

type EducationStage string

const (
	StagePrimary    EducationStage = "primary"
	StageMiddle     EducationStage = "middle"
	StageHigh       EducationStage = "high"
	StageUniversity EducationStage = "university"
	StageVocational EducationStage = "vocational"
	StageLifelong   EducationStage = "lifelong"
)

func ParseEducationStage(value string) (EducationStage, error) {
	stage := EducationStage(strings.TrimSpace(value))
	switch stage {
	case StagePrimary, StageMiddle, StageHigh, StageUniversity, StageVocational, StageLifelong:
		return stage, nil
	default:
		return "", common.FieldError{Field: "education_stage", Message: "is unsupported"}
	}
}

type User struct {
	ID             common.ID
	TenantID       common.ID
	Email          string
	DisplayName    string
	PasswordHash   string
	Role           Role
	EducationStage EducationStage
	BirthDate      *time.Time
	Active         bool
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewUser(id, tenantID common.ID, email, displayName, passwordHash string, role Role, stage EducationStage, now time.Time) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	if !id.Valid() || !tenantID.Valid() {
		return User{}, common.FieldError{Field: "identity", Message: "user and tenant ids are required"}
	}
	if !strings.Contains(email, "@") || len(email) > 254 {
		return User{}, common.FieldError{Field: "email", Message: "must be a valid address"}
	}
	if displayName == "" || len(displayName) > 120 {
		return User{}, common.FieldError{Field: "display_name", Message: "is required and must fit 120 characters"}
	}
	if passwordHash == "" {
		return User{}, common.FieldError{Field: "password", Message: "hash is required"}
	}
	if _, err := ParseRole(string(role)); err != nil {
		return User{}, err
	}
	if _, err := ParseEducationStage(string(stage)); err != nil {
		return User{}, err
	}
	now = now.UTC()
	return User{
		ID: id, TenantID: tenantID, Email: email, DisplayName: displayName,
		PasswordHash: passwordHash, Role: role, EducationStage: stage, Active: true,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (u User) Can(action string) bool {
	switch action {
	case "curriculum.propose", "offering.manage", "evidence.review":
		return u.Active && (u.Role == RoleEducator || u.Role == RoleProgramAdmin)
	case "curriculum.approve", "incident.review":
		return u.Active && (u.Role == RoleReviewer || u.Role == RoleProgramAdmin)
	case "program.admin":
		return u.Active && u.Role == RoleProgramAdmin
	case "learning.participate", "evidence.submit":
		return u.Active && u.Role == RoleLearner
	default:
		return false
	}
}

func (u User) Require(action string) error {
	if !u.Active {
		return fmt.Errorf("inactive user %s: %w", u.ID, common.ErrForbidden)
	}
	if !u.Can(action) {
		return fmt.Errorf("role %s cannot %s: %w", u.Role, action, common.ErrForbidden)
	}
	return nil
}
