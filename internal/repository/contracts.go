package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/learning"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/project"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/resource"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/safety"
)

type Store interface {
	WithinTx(ctx context.Context, fn func(context.Context, Tx) error) error
	Read(ctx context.Context, fn func(context.Context, Reader) error) error
	Ping(ctx context.Context) error
	Close() error
}

type Reader interface {
	GetUser(ctx context.Context, tenantID, userID common.ID) (identity.User, error)
	GetUserByEmail(ctx context.Context, tenantID common.ID, email string) (identity.User, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (identity.Session, error)
	GetCurriculum(ctx context.Context, tenantID, curriculumID common.ID) (curriculum.Curriculum, error)
	ListCurricula(ctx context.Context, tenantID common.ID, filter CurriculumFilter, page Page) (CurriculumPage, error)
	GetOffering(ctx context.Context, tenantID, offeringID common.ID) (offering.Offering, error)
	ListOfferings(ctx context.Context, tenantID common.ID, filter OfferingFilter, page Page) (OfferingPage, error)
	GetEnrollment(ctx context.Context, tenantID, enrollmentID common.ID) (offering.Enrollment, error)
	FindEnrollment(ctx context.Context, tenantID, offeringID, learnerID common.ID) (offering.Enrollment, error)
	GetProject(ctx context.Context, tenantID, projectID common.ID) (project.Project, error)
	GetEvidence(ctx context.Context, tenantID, evidenceID common.ID) (learning.Evidence, error)
	ListCompetencies(ctx context.Context, tenantID, learnerID common.ID) ([]learning.Competency, error)
	GetToolPolicy(ctx context.Context, tenantID, policyID common.ID) (safety.ToolPolicy, error)
	GetConsent(ctx context.Context, tenantID, consentID common.ID) (safety.Consent, error)
	GetToolGrant(ctx context.Context, tenantID, grantID common.ID) (safety.ToolGrant, error)
	GetIncident(ctx context.Context, tenantID, incidentID common.ID) (safety.Incident, error)
	GetResourcePack(ctx context.Context, tenantID, packID common.ID) (resource.ResourcePack, error)
	GetDelivery(ctx context.Context, tenantID, deliveryID common.ID) (resource.Delivery, error)
	GetJob(ctx context.Context, tenantID, jobID common.ID) (job.Job, error)
	ListAudit(ctx context.Context, tenantID common.ID, filter AuditFilter, page Page) (AuditPage, error)
	GetIdempotency(ctx context.Context, scope, key string) (IdempotencyRecord, error)
}

type Tx interface {
	Reader
	InsertUser(ctx context.Context, user identity.User) error
	UpdateUser(ctx context.Context, user identity.User, expectedVersion int64) error
	InsertSession(ctx context.Context, session identity.Session) error
	UpdateSession(ctx context.Context, session identity.Session, expectedVersion int64) error
	RevokeSessionsForUser(ctx context.Context, tenantID, userID common.ID, now time.Time) (int64, error)
	InsertCurriculum(ctx context.Context, value curriculum.Curriculum) error
	UpdateCurriculum(ctx context.Context, value curriculum.Curriculum, expectedVersion int64) error
	ReplaceCurriculumOutcomes(ctx context.Context, curriculumID common.ID, outcomes []curriculum.Outcome) error
	ReplaceCurriculumPrerequisites(ctx context.Context, curriculumID common.ID, ids []common.ID) error
	InsertOffering(ctx context.Context, value offering.Offering) error
	UpdateOffering(ctx context.Context, value offering.Offering, expectedVersion int64) error
	ReserveOfferingSeat(ctx context.Context, tenantID, offeringID common.ID, expectedVersion int64) (offering.Offering, error)
	ReleaseOfferingSeat(ctx context.Context, tenantID, offeringID common.ID) error
	InsertEnrollment(ctx context.Context, value offering.Enrollment) error
	UpdateEnrollment(ctx context.Context, value offering.Enrollment, expectedVersion int64) error
	InsertProject(ctx context.Context, value project.Project) error
	UpdateProject(ctx context.Context, value project.Project, expectedVersion int64) error
	ReplaceProjectMembers(ctx context.Context, projectID common.ID, members []common.ID) error
	ReplaceProjectToolGrants(ctx context.Context, projectID common.ID, grants []common.ID) error
	ReserveMentorSlot(ctx context.Context, tenantID, mentorID common.ID, limit int) error
	ReleaseMentorSlot(ctx context.Context, tenantID, mentorID common.ID) error
	InsertLabAllocation(ctx context.Context, value project.LabAllocation) error
	UpdateLabAllocation(ctx context.Context, value project.LabAllocation, expectedVersion int64) error
	AssertLabCapacity(ctx context.Context, tenantID, resourceID common.ID, window common.Window, units, capacity int) error
	InsertEvidence(ctx context.Context, value learning.Evidence) error
	UpdateEvidence(ctx context.Context, value learning.Evidence, expectedVersion int64) error
	InsertReviewDecision(ctx context.Context, value learning.ReviewDecision) error
	InsertCompetency(ctx context.Context, value learning.Competency) error
	SupersedeCompetencies(ctx context.Context, tenantID, learnerID common.ID, code string, now time.Time) error
	InsertToolPolicy(ctx context.Context, value safety.ToolPolicy) error
	InsertConsent(ctx context.Context, value safety.Consent) error
	UpdateConsent(ctx context.Context, value safety.Consent, expectedVersion int64) error
	InsertToolGrant(ctx context.Context, value safety.ToolGrant) error
	UpdateToolGrant(ctx context.Context, value safety.ToolGrant, expectedVersion int64) error
	InsertIncident(ctx context.Context, value safety.Incident) error
	UpdateIncident(ctx context.Context, value safety.Incident, expectedVersion int64) error
	InsertRemediation(ctx context.Context, value safety.Remediation) error
	InsertResourcePack(ctx context.Context, value resource.ResourcePack) error
	UpdateResourcePack(ctx context.Context, value resource.ResourcePack, expectedVersion int64) error
	InsertDelivery(ctx context.Context, value resource.Delivery) error
	UpdateDelivery(ctx context.Context, value resource.Delivery, expectedVersion int64) error
	ClaimDeliveries(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration, limit int) ([]resource.Delivery, error)
	InsertJob(ctx context.Context, value job.Job) error
	UpdateJob(ctx context.Context, value job.Job, expectedVersion int64) error
	ClaimJobs(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration, kinds []string, limit int) ([]job.Job, error)
	InsertAudit(ctx context.Context, value AuditEvent) error
	InsertOutbox(ctx context.Context, value OutboxEvent) error
	InsertIdempotency(ctx context.Context, value IdempotencyRecord) error
	CompleteIdempotency(ctx context.Context, scope, key string, status int, body json.RawMessage, now time.Time) error
}

type AuditEvent struct {
	ID         common.ID
	TenantID   common.ID
	ActorID    common.ID
	RequestID  string
	Action     string
	ObjectType string
	ObjectID   common.ID
	Result     string
	Metadata   json.RawMessage
	OccurredAt time.Time
}

type OutboxEvent struct {
	ID            common.ID
	TenantID      common.ID
	AggregateType string
	AggregateID   common.ID
	Kind          string
	Payload       json.RawMessage
	CreatedAt     time.Time
	PublishedAt   *time.Time
	Attempts      int
	LastError     string
}

type IdempotencyRecord struct {
	Scope       string
	Key         string
	RequestHash string
	State       string
	StatusCode  int
	Response    json.RawMessage
	CreatedAt   time.Time
	CompletedAt *time.Time
}
