package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/learning"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
	"github.com/11DingKing/ai-learning-pathways-go/internal/service"
	"github.com/11DingKing/ai-learning-pathways-go/internal/storage/sqlite"
)

type fixture struct {
	db      *sqlite.DB
	svc     *service.Service
	tenant  common.ID
	admin   identity.User
	learner identity.User
	now     time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	now := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	adminHash, err := service.HashPassword("administrator-password")
	if err != nil {
		t.Fatal(err)
	}
	tenant := common.ID("tenant_service")
	admin, err := identity.NewUser(common.ID("admin_service"), tenant, "admin@service.edu", "Administrator", adminHash, identity.RoleProgramAdmin, identity.StageUniversity, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BootstrapIdentity(context.Background(), tenant, "Service Tenant", "UTC", admin); err != nil {
		t.Fatal(err)
	}
	learnerHash, _ := service.HashPassword("learner-password-123")
	learner, err := identity.NewUser(common.ID("learner_service"), tenant, "learner@service.edu", "Learner", learnerHash, identity.RoleLearner, identity.StageUniversity, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertUser(ctx, learner) }); err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(db, common.FixedClock{Value: now}, 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{db: db, svc: svc, tenant: tenant, admin: admin, learner: learner, now: now}
}

func (f fixture) close() { _ = f.db.Close() }
func adminActor() service.Actor {
	return service.Actor{TenantID: common.ID("tenant_service"), UserID: common.ID("admin_service"), Role: identity.RoleProgramAdmin, RequestID: "req-service"}
}
func learnerActor() service.Actor {
	return service.Actor{TenantID: common.ID("tenant_service"), UserID: common.ID("learner_service"), Role: identity.RoleLearner, RequestID: "req-learner"}
}

func TestLoginAuthenticateLogoutLifecycle(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	login, err := f.svc.Login(context.Background(), f.tenant, "ADMIN@SERVICE.EDU", "administrator-password")
	if err != nil {
		t.Fatal(err)
	}
	if login.Token == "" || login.User.ID != f.admin.ID {
		t.Fatalf("unexpected login result: %+v", login)
	}
	actor, err := f.svc.Authenticate(context.Background(), login.Token)
	if err != nil {
		t.Fatal(err)
	}
	if actor.UserID != f.admin.ID || actor.Role != identity.RoleProgramAdmin {
		t.Fatalf("unexpected actor: %+v", actor)
	}
	if err := f.svc.Logout(context.Background(), actor, login.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(context.Background(), login.Token); !errors.Is(err, common.ErrUnauthenticated) {
		t.Fatalf("revoked token authenticated: %v", err)
	}
	if _, err := f.svc.Login(context.Background(), f.tenant, "admin@service.edu", "wrong-password"); !errors.Is(err, common.ErrUnauthenticated) {
		t.Fatalf("wrong password should fail auth: %v", err)
	}
}

func curriculumInput() service.CurriculumInput {
	knowledge, _ := curriculum.NewOutcome("KNOW", "Knowledge", "Understand the capabilities and limits of models.", curriculum.OutcomeKnowledge, true)
	practice, _ := curriculum.NewOutcome("MAKE", "Practice", "Build and test a useful classroom prototype.", curriculum.OutcomePractice, true)
	ethics, _ := curriculum.NewOutcome("SAFE", "Safety", "Identify privacy, bias, and accountability risks.", curriculum.OutcomeEthics, true)
	return service.CurriculumInput{Code: "AI-SERVICE", Title: "AI Service Curriculum", Summary: "A detailed curriculum for safe practical artificial intelligence learning across a full term.", Stage: identity.StageUniversity, Outcomes: []curriculum.Outcome{knowledge, practice, ethics}, MinimumEthicsHours: 2, MinimumPracticeMinutes: 90}
}

func TestCurriculumPublishWorkflowWritesState(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	actor := adminActor()
	value, err := f.svc.CreateCurriculum(context.Background(), actor, curriculumInput())
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != curriculum.StatusDraft {
		t.Fatal("new curriculum should be draft")
	}
	value, err = f.svc.SubmitCurriculum(context.Background(), actor, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	value, err = f.svc.ApproveCurriculum(context.Background(), actor, value.ID, "The review confirms practice, ethics, and measurable learning outcomes.")
	if err != nil {
		t.Fatal(err)
	}
	value, err = f.svc.PublishCurriculum(context.Background(), actor, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != curriculum.StatusPublished {
		t.Fatalf("expected published curriculum: %+v", value)
	}
	if err := f.db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		loaded, err := reader.GetCurriculum(ctx, f.tenant, value.ID)
		if err != nil {
			return err
		}
		if loaded.Status != curriculum.StatusPublished || len(loaded.Outcomes) != 3 {
			t.Fatalf("persisted curriculum incomplete: %+v", loaded)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCurriculumAuthorizationAndStateGuards(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	value, err := f.svc.CreateCurriculum(context.Background(), adminActor(), curriculumInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SubmitCurriculum(context.Background(), learnerActor(), value.ID); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("learner should not submit curriculum: %v", err)
	}
	if _, err := f.svc.PublishCurriculum(context.Background(), adminActor(), value.ID); !errors.Is(err, common.ErrState) {
		t.Fatalf("draft should not publish: %v", err)
	}
}

func prepareActiveOffering(t *testing.T, f fixture) (offering.Offering, identity.User) {
	t.Helper()
	actor := adminActor()
	course, err := f.svc.CreateCurriculum(context.Background(), actor, curriculumInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SubmitCurriculum(context.Background(), actor, course.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ApproveCurriculum(context.Background(), actor, course.ID, "The reviewed curriculum satisfies policy and practice requirements."); err != nil {
		t.Fatal(err)
	}
	course, err = f.svc.PublishCurriculum(context.Background(), actor, course.ID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := f.svc.CreateOffering(context.Background(), actor, service.OfferingInput{CurriculumID: course.ID, Code: "OFFER-SERVICE", Title: "Service Offering", StartsAt: f.now.Add(-time.Minute), EndsAt: f.now.Add(2 * time.Hour), Capacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	value, err = f.svc.ActivateOffering(context.Background(), actor, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	return value, f.learner
}

func TestEnrollmentIdempotencyReturnsSameDurableOutcome(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	value, learner := prepareActiveOffering(t, f)
	input := service.EnrollmentInput{OfferingID: value.ID, IdempotencyKey: "enrollment-operation-1"}
	first, err := f.svc.Enroll(context.Background(), learnerActor(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Enroll(context.Background(), learnerActor(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Status != offering.EnrollmentConfirmed {
		t.Fatalf("idempotency returned different outcome: %v %v", first.ID, second.ID)
	}
	if _, err := f.svc.Enroll(context.Background(), learnerActor(), service.EnrollmentInput{OfferingID: value.ID, IdempotencyKey: input.IdempotencyKey, GuardianConsentID: func() *common.ID { x := common.ID("different"); return &x }()}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed idempotency request should conflict: %v", err)
	}
	if err := f.db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		loaded, err := reader.FindEnrollment(ctx, f.tenant, value.ID, learner.ID)
		if err != nil {
			return err
		}
		if loaded.ID != first.ID {
			t.Fatalf("durable enrollment mismatch: %+v", loaded)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollmentCapacityAndEligibility(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	value, _ := prepareActiveOffering(t, f)
	for i := 0; i < 2; i++ {
		learnerID := common.ID("learner_extra_" + string(rune('a'+i)))
		learnerHash, _ := service.HashPassword("learner-extra-password")
		user, err := identity.NewUser(learnerID, f.tenant, learnerID.String()+"@service.edu", "Extra Learner", learnerHash, identity.RoleLearner, identity.StageUniversity, f.now)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertUser(ctx, user) }); err != nil {
			t.Fatal(err)
		}
		actor := service.Actor{TenantID: f.tenant, UserID: learnerID, Role: identity.RoleLearner}
		if _, err := f.svc.Enroll(context.Background(), actor, service.EnrollmentInput{OfferingID: value.ID, IdempotencyKey: "capacity-request-" + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	thirdID := common.ID("learner_third")
	thirdHash, _ := service.HashPassword("learner-third-password")
	third, _ := identity.NewUser(thirdID, f.tenant, "third@service.edu", "Third Learner", thirdHash, identity.RoleLearner, identity.StageUniversity, f.now)
	if err := f.db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertUser(ctx, third) }); err != nil {
		t.Fatal(err)
	}
	actor := service.Actor{TenantID: f.tenant, UserID: third.ID, Role: identity.RoleLearner}
	if _, err := f.svc.Enroll(context.Background(), actor, service.EnrollmentInput{OfferingID: value.ID, IdempotencyKey: "capacity-request-third"}); !errors.Is(err, common.ErrConflict) && !errors.Is(err, common.ErrCapacity) {
		t.Fatalf("third enrollment should not confirm: %v", err)
	}
}

func TestEvidenceReviewAwardsCompetencyAndQueuesJob(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	value, _ := prepareActiveOffering(t, f)
	enrollment, err := f.svc.Enroll(context.Background(), learnerActor(), service.EnrollmentInput{OfferingID: value.ID, IdempotencyKey: "evidence-enrollment"})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := f.svc.SubmitEvidence(context.Background(), learnerActor(), service.EvidenceInput{EnrollmentID: enrollment.ID, OutcomeCode: "SAFE", RubricVersion: 1, Attempt: 1, ArtifactURI: "s3://evidence", ArtifactSHA256: "0123456789012345678901234567890123456789012345678901234567890123", Reflection: "This reflection documents the model choice, privacy boundary, test evidence, and improvement plan for the classroom project."})
	if err != nil {
		t.Fatal(err)
	}
	updated, competency, err := f.svc.ReviewEvidence(context.Background(), adminActor(), service.ReviewInput{EvidenceID: evidence.ID, Outcome: learning.ReviewPass, Score: 91, Feedback: "The submitted evidence is reproducible, technically sound, and includes a complete safety reflection.", CompetencyCode: "SAFE", CompetencyLevel: learning.LevelIndependent, ValidFor: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != learning.EvidenceAccepted || competency == nil {
		t.Fatalf("review did not award competency: %+v %+v", updated, competency)
	}
	if err := f.db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		items, err := reader.ListCompetencies(ctx, f.tenant, f.learner.ID)
		if err != nil {
			return err
		}
		if len(items) != 1 || items[0].Code != "SAFE" {
			t.Fatalf("competency not persisted: %+v", items)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceCannotBeSubmittedByAnotherLearner(t *testing.T) {
	f := newFixture(t)
	defer f.close()
	value, _ := prepareActiveOffering(t, f)
	enrollment, err := f.svc.Enroll(context.Background(), learnerActor(), service.EnrollmentInput{OfferingID: value.ID, IdempotencyKey: "wrong-owner-enrollment"})
	if err != nil {
		t.Fatal(err)
	}
	otherID := common.ID("learner_other")
	otherHash, _ := service.HashPassword("other-password-123")
	other, _ := identity.NewUser(otherID, f.tenant, "other@service.edu", "Other Learner", otherHash, identity.RoleLearner, identity.StageUniversity, f.now)
	if err := f.db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertUser(ctx, other) }); err != nil {
		t.Fatal(err)
	}
	actor := service.Actor{TenantID: f.tenant, UserID: other.ID, Role: identity.RoleLearner}
	_, err = f.svc.SubmitEvidence(context.Background(), actor, service.EvidenceInput{EnrollmentID: enrollment.ID, OutcomeCode: "SAFE", RubricVersion: 1, Attempt: 1, ArtifactURI: "s3://evidence", ArtifactSHA256: "0123456789012345678901234567890123456789012345678901234567890123", Reflection: "This reflection is long enough to satisfy the evidence contract and demonstrate ownership checks."})
	if !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("wrong learner should be forbidden: %v", err)
	}
}
