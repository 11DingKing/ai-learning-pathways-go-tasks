package domain_test

import (
	"errors"
	"testing"
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

var testNow = time.Date(2026, 8, 23, 8, 0, 0, 0, time.UTC)

func id(prefix string) common.ID { return common.ID(prefix + "_test") }

func TestWindowBoundaries(t *testing.T) {
	window, err := common.NewWindow(testNow, testNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !window.Contains(testNow) {
		t.Fatal("start should be included")
	}
	if window.Contains(testNow.Add(time.Hour)) {
		t.Fatal("end should be excluded")
	}
	if !window.Overlaps(common.Window{StartsAt: testNow.Add(30 * time.Minute), EndsAt: testNow.Add(2 * time.Hour)}) {
		t.Fatal("windows should overlap")
	}
	if window.Overlaps(common.Window{StartsAt: testNow.Add(time.Hour), EndsAt: testNow.Add(2 * time.Hour)}) {
		t.Fatal("adjacent windows should not overlap")
	}
	if _, err := common.NewWindow(testNow, testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("expected invalid window, got %v", err)
	}
}

func TestCurriculumLifecycleRequiresEthicsAndPractice(t *testing.T) {
	value, err := curriculum.New(id("cur"), id("tenant"), id("creator"), "ai-101", "AI Foundations", "A curriculum that teaches safe, practical, and reflective use of artificial intelligence.", identity.StageUniversity, testNow)
	if err != nil {
		t.Fatal(err)
	}
	value.MinimumEthicsHours = 2
	value.MinimumPracticeMins = 60
	knowledge, _ := curriculum.NewOutcome("KNOW", "Knowledge", "Understand core models and limitations.", curriculum.OutcomeKnowledge, true)
	practice, _ := curriculum.NewOutcome("MAKE", "Practice", "Build and evaluate a small useful artifact.", curriculum.OutcomePractice, true)
	ethics, _ := curriculum.NewOutcome("SAFE", "Safety", "Reason about privacy, bias, and accountability.", curriculum.OutcomeEthics, true)
	for _, outcome := range []curriculum.Outcome{knowledge, practice, ethics} {
		if err := value.AddOutcome(outcome, testNow); err != nil {
			t.Fatal(err)
		}
	}
	if err := value.SubmitForReview(testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Approve(id("reviewer"), "The curriculum includes measurable practice and required ethics outcomes.", testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Publish(testNow); err != nil {
		t.Fatal(err)
	}
	if value.Status != curriculum.StatusPublished || value.Version <= 1 {
		t.Fatalf("unexpected published value: %+v", value)
	}
	if err := value.AddOutcome(knowledge, testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("expected frozen outcomes, got %v", err)
	}
}

func TestCurriculumReviewRejectsMissingPolicyOutcomes(t *testing.T) {
	value, err := curriculum.New(id("cur"), id("tenant"), id("creator"), "ai-101", "AI Foundations", "A curriculum that teaches safe, practical, and reflective use of artificial intelligence.", identity.StageUniversity, testNow)
	if err != nil {
		t.Fatal(err)
	}
	value.MinimumEthicsHours = 1
	value.MinimumPracticeMins = 60
	for i := 0; i < 3; i++ {
		outcome, _ := curriculum.NewOutcome("K"+string(rune('A'+i)), "Knowledge", "Understand a concept.", curriculum.OutcomeKnowledge, true)
		if err := value.AddOutcome(outcome, testNow); err != nil {
			t.Fatal(err)
		}
	}
	if err := value.SubmitForReview(testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("expected policy validation, got %v", err)
	}
}

func TestPrerequisiteGraph(t *testing.T) {
	a, b, c := id("a"), id("b"), id("c")
	if err := (curriculum.PrerequisiteGraph{a: {b}, b: {c}, c: {}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (curriculum.PrerequisiteGraph{a: {b}, b: {a}}).Validate(); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected cycle conflict, got %v", err)
	}
	if err := (curriculum.PrerequisiteGraph{a: {id("missing")}}).Validate(); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected missing prerequisite, got %v", err)
	}
	missing := curriculum.PrerequisiteGraph{a: {b, c}, b: {c}, c: {}}.Missing(map[common.ID]bool{c: true}, a)
	if len(missing) != 1 || missing[0] != b {
		t.Fatalf("unexpected missing prerequisites: %v", missing)
	}
}

func TestSessionLifecycle(t *testing.T) {
	session, err := identity.NewSession(id("session"), id("tenant"), id("user"), "abcdefghijklmnopqrstuvwxyz123456", testNow, testNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Validate("abcdefghijklmnopqrstuvwxyz123456", testNow.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := session.Validate("wrong-token-abcdefghijklmnopqrstuvwxyz", testNow); !errors.Is(err, common.ErrUnauthenticated) {
		t.Fatalf("expected auth failure, got %v", err)
	}
	if err := session.Revoke(testNow.Add(31 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := session.Validate("abcdefghijklmnopqrstuvwxyz123456", testNow.Add(31*time.Minute)); !errors.Is(err, common.ErrUnauthenticated) {
		t.Fatalf("expected revoked session, got %v", err)
	}
	if err := session.Revoke(testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if session.Version != 2 {
		t.Fatalf("revoke should be idempotent, version=%d", session.Version)
	}
}

func TestOfferingAndEnrollmentTransitions(t *testing.T) {
	window, _ := common.NewWindow(testNow, testNow.Add(2*time.Hour))
	value, err := offering.New(id("offering"), id("tenant"), id("cur"), id("educator"), "AI-101", "AI Foundations", 3, window, 2, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if !value.AcceptingEnrollments(testNow) || value.AvailableSeats() != 2 {
		t.Fatal("offering should accept enrollment")
	}
	enrollment, err := offering.NewEnrollment(id("enrollment"), id("tenant"), value.ID, id("learner"), "request-1234", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := enrollment.Confirm(testNow); err != nil {
		t.Fatal(err)
	}
	if err := enrollment.Complete(testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := enrollment.Withdraw(testNow.Add(2 * time.Hour)); !errors.Is(err, common.ErrState) {
		t.Fatalf("completed enrollment should not withdraw: %v", err)
	}
}

func TestProjectRequiresMentorAndGrants(t *testing.T) {
	value, err := project.New(id("project"), id("tenant"), id("offering"), "Campus AI Lab", "Students investigate a real school problem and build a safe, transparent prototype with evidence and reflection.", []common.ID{id("learner1"), id("learner2")}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Launch(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("expected launch guard, got %v", err)
	}
	if err := value.Approve(id("reviewer"), testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.AssignMentor(id("mentor"), testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.SetToolGrants([]common.ID{id("grant1")}, testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Launch(testNow); err != nil {
		t.Fatal(err)
	}
	if value.Status != project.StatusActive {
		t.Fatal("project should be active")
	}
	if err := value.SetToolGrants([]common.ID{id("grant2")}, testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("active project grants should be frozen: %v", err)
	}
}

func TestLabAllocationOwnershipAndWindow(t *testing.T) {
	window, _ := common.NewWindow(testNow, testNow.Add(time.Hour))
	allocation, err := project.NewLabAllocation(id("lab"), id("tenant"), id("project"), id("resource"), id("owner"), window, 2, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := allocation.CheckIn(id("other"), testNow.Add(10*time.Minute)); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("expected owner check, got %v", err)
	}
	if err := allocation.CheckIn(id("owner"), testNow.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := allocation.Release(id("owner"), testNow.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if allocation.ReleasedAt == nil || allocation.Status != project.LabReleased {
		t.Fatalf("unexpected allocation: %+v", allocation)
	}
}

func TestEvidenceReviewAndCompetency(t *testing.T) {
	evidence, err := learning.NewEvidence(id("evidence"), id("tenant"), id("enrollment"), id("learner"), "SAFE-101", 2, 1, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Attach("s3://artifact", "0123456789012345678901234567890123456789012345678901234567890123", "This reflection explains the design choices, limitations, privacy concerns, and next iteration for the project.", testNow); err != nil {
		t.Fatal(err)
	}
	if err := evidence.Submit(testNow); err != nil {
		t.Fatal(err)
	}
	if err := evidence.StartReview(testNow); err != nil {
		t.Fatal(err)
	}
	decision, err := learning.NewReviewDecision(id("review"), evidence.ID, id("reviewer"), learning.ReviewPass, 88, "The evidence demonstrates the outcome with a clear safety analysis and reproducible practice.", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.ApplyDecision(decision, testNow); err != nil {
		t.Fatal(err)
	}
	competency, err := learning.NewCompetency(id("competency"), id("tenant"), evidence.LearnerID, evidence.ID, "SAFE-101", learning.LevelIndependent, testNow, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !competency.ActiveAt(testNow.Add(time.Hour)) || competency.ActiveAt(testNow.Add(25*time.Hour)) {
		t.Fatal("competency expiry boundary is wrong")
	}
	if selected, ok := learning.HighestActive([]learning.Competency{competency}, "safe-101", testNow); !ok || selected.ID != competency.ID {
		t.Fatal("highest active competency not selected")
	}
}

func TestSafetyGrantAndIncident(t *testing.T) {
	policy := safety.ToolPolicy{ID: id("policy"), TenantID: id("tenant"), ToolCode: "IMAGE", DisplayName: "Image Tool", AllowedDataClasses: []safety.DataClass{safety.DataEducational}, MinimumAge: 12, RequiresEducator: true, Active: true, Version: 1}
	consent := safety.Consent{ID: id("consent"), TenantID: id("tenant"), LearnerID: id("learner"), Purpose: "class-project", DataClasses: []safety.DataClass{safety.DataEducational}, GrantedBy: id("guardian"), GrantedAt: testNow, ExpiresAt: testNow.Add(48 * time.Hour), Version: 1}
	grant, err := safety.NewToolGrant(id("grant"), id("tenant"), id("learner"), policy, consent, "class-project", 14, true, false, testNow, testNow.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !grant.ValidAt(testNow.Add(time.Hour)) {
		t.Fatal("grant should be valid")
	}
	if err := grant.Revoke(testNow.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if grant.ValidAt(testNow.Add(3 * time.Hour)) {
		t.Fatal("revoked grant should be invalid")
	}
	incident, err := safety.NewIncident(id("incident"), id("tenant"), id("learner"), grant.ID, id("reporter"), safety.SeverityHigh, "The learner accidentally included private data in a generated artifact and needs guided remediation.", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := incident.Restrict(testNow); err != nil {
		t.Fatal(err)
	}
	if err := incident.BeginRemediation(testNow); err != nil {
		t.Fatal(err)
	}
	if incident.Status != safety.IncidentRemediating {
		t.Fatal("incident should be remediating")
	}
}

func TestResourceDeliveryLease(t *testing.T) {
	delivery, err := resource.NewDelivery(id("delivery"), id("tenant"), id("pack"), id("offering"), id("recipient"), "pack:offering:recipient", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.Lease("worker-a", testNow, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := delivery.MarkDelivered("worker-b", testNow.Add(10*time.Second)); !errors.Is(err, common.ErrLeaseLost) {
		t.Fatalf("wrong owner should lose lease: %v", err)
	}
	if err := delivery.MarkDelivered("worker-a", testNow.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := delivery.Acknowledge(testNow.Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if delivery.Status != resource.DeliveryAcked {
		t.Fatal("delivery should be acknowledged")
	}
}

func TestJobRetriesAndDeadLetter(t *testing.T) {
	value, err := job.New(id("job"), id("tenant"), "delivery", "delivery", id("delivery"), map[string]string{"id": "delivery"}, testNow, 2, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Lease("worker-a", testNow, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := value.Fail("worker-a", errors.New("temporary provider failure"), testNow, time.Second); err != nil {
		t.Fatal(err)
	}
	if value.Status != job.StatusRetry || !value.AvailableAt.After(testNow) {
		t.Fatalf("expected retry state: %+v", value)
	}
	if err := value.Lease("worker-a", value.AvailableAt, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := value.Fail("worker-a", errors.New("permanent provider failure"), value.AvailableAt, time.Second); err != nil {
		t.Fatal(err)
	}
	if value.Status != job.StatusDead {
		t.Fatalf("expected dead letter: %+v", value)
	}
}

func TestRoleCapabilities(t *testing.T) {
	roles := []identity.Role{identity.RoleLearner, identity.RoleEducator, identity.RoleReviewer, identity.RoleProgramAdmin}
	for _, role := range roles {
		user, err := identity.NewUser(id(string(role)), id("tenant"), string(role)+"@example.edu", "Test User", "hash", role, identity.StageUniversity, testNow)
		if err != nil {
			t.Fatal(err)
		}
		if role == identity.RoleLearner && !user.Can("evidence.submit") {
			t.Fatal("learner should submit evidence")
		}
		if role != identity.RoleLearner && user.Can("evidence.submit") {
			t.Fatalf("%s should not submit learner evidence", role)
		}
		if role == identity.RoleReviewer && !user.Can("curriculum.approve") {
			t.Fatal("reviewer should approve")
		}
	}
}
