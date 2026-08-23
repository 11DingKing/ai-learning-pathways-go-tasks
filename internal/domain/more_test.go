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

func TestIDValidationAndClockNormalization(t *testing.T) {
	if _, err := common.NewID(" "); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("empty id prefix should fail: %v", err)
	}
	if _, err := common.ParseID("id", ""); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("empty id should fail: %v", err)
	}
	if _, err := common.ParseID("id", "a__b"); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("malformed id should fail: %v", err)
	}
	fixed := common.FixedClock{Value: time.Date(2026, 8, 23, 1, 0, 0, 0, time.FixedZone("x", 3600))}
	if !fixed.Now().Equal(fixed.Value.UTC()) {
		t.Fatal("fixed clock should normalize to UTC")
	}
}

func TestCurriculumOutcomeAndPrerequisiteInputValidation(t *testing.T) {
	if _, err := curriculum.NewOutcome("", "Title", "Description", curriculum.OutcomeKnowledge, true); !errors.Is(err, common.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := curriculum.NewOutcome("CODE", "", "Description", curriculum.OutcomeKnowledge, true); !errors.Is(err, common.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := curriculum.NewOutcome("CODE", "Title", "Description", curriculum.OutcomeKind("unknown"), true); !errors.Is(err, common.ErrInvalid) {
		t.Fatal(err)
	}
	value, err := curriculum.New(common.ID("cur_more"), common.ID("tenant_more"), common.ID("creator_more"), "MORE", "More Curriculum", "This is a long enough curriculum summary for validation of prerequisite input.", identity.StageUniversity, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.SetPrerequisites([]common.ID{value.ID}, testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("self prerequisite should fail: %v", err)
	}
	if err := value.SetPrerequisites([]common.ID{common.ID("p1"), common.ID("p1")}, testNow); err != nil {
		t.Fatal(err)
	}
	if len(value.PrerequisiteIDs) != 1 {
		t.Fatal("duplicate prerequisites should be collapsed")
	}
}

func TestOfferingWindowAndStateErrors(t *testing.T) {
	window, _ := common.NewWindow(testNow, testNow.Add(time.Hour))
	value, err := offering.New(common.ID("offer_more"), common.ID("tenant_more"), common.ID("cur_more"), common.ID("educator_more"), "MORE", "More Offering", 1, window, 1, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Close(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("planned offering should not close: %v", err)
	}
	if err := value.Activate(testNow.Add(2 * time.Hour)); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("ended offering should not activate: %v", err)
	}
	if err := value.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Close(testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("early close should fail: %v", err)
	}
	if err := value.Cancel(testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Cancel(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("cancel should be terminal: %v", err)
	}
}

func TestEnrollmentTransitionsRejectInvalidEdges(t *testing.T) {
	e, err := offering.NewEnrollment(common.ID("enroll_more"), common.ID("tenant_more"), common.ID("offer_more"), common.ID("learner_more"), "idempotency-more", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Complete(testNow); !errors.Is(err, common.ErrState) {
		t.Fatal(err)
	}
	if err := e.Waitlist(testNow); err != nil {
		t.Fatal(err)
	}
	if err := e.Confirm(testNow); err != nil {
		t.Fatal(err)
	}
	if err := e.Withdraw(testNow); err != nil {
		t.Fatal(err)
	}
	if err := e.Withdraw(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("withdraw should be terminal: %v", err)
	}
}

func TestJobClaimHeartbeatAndLeaseLoss(t *testing.T) {
	value, err := job.New(common.ID("job_more"), common.ID("tenant_more"), "kind", "aggregate", common.ID("aggregate_more"), nil, testNow, 3, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Lease("owner-a", testNow, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := value.Heartbeat("owner-b", testNow.Add(time.Second), time.Minute); !errors.Is(err, common.ErrLeaseLost) {
		t.Fatalf("wrong heartbeat owner should fail: %v", err)
	}
	if err := value.Heartbeat("owner-a", testNow.Add(time.Second), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := value.Succeed("owner-a", testNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if value.Claimable(testNow.Add(3 * time.Second)) {
		t.Fatal("succeeded job should not be claimable")
	}
}

func TestResourcePackTransitionsAndDeliveryFailures(t *testing.T) {
	pack, err := resource.NewPack(common.ID("pack_more"), common.ID("tenant_more"), "Pack", "CC-BY", "https://example.edu", "0123456789012345678901234567890123456789012345678901234567890123", "primary", "high", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := pack.Publish(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("draft should not publish directly: %v", err)
	}
	if err := pack.Approve(common.ID("reviewer_more"), testNow); err != nil {
		t.Fatal(err)
	}
	if err := pack.Publish(testNow); err != nil {
		t.Fatal(err)
	}
	delivery, err := resource.NewDelivery(common.ID("delivery_more"), common.ID("tenant_more"), pack.ID, common.ID("offer_more"), common.ID("learner_more"), "pack:offer:learner", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.Fail("wrong-owner", errors.New("provider"), testNow); !errors.Is(err, common.ErrLeaseLost) {
		t.Fatalf("unleased delivery should reject failure: %v", err)
	}
	if err := delivery.Lease("owner", testNow, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := delivery.Fail("owner", errors.New("provider"), testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if delivery.Status != resource.DeliveryFailed || delivery.LastError == "" {
		t.Fatalf("delivery failure not recorded: %+v", delivery)
	}
}

func TestSafetyConsentAndRemediationCompleteness(t *testing.T) {
	consent := safety.Consent{Purpose: "lesson", DataClasses: []safety.DataClass{safety.DataEducational}, GrantedAt: testNow, ExpiresAt: testNow.Add(time.Hour)}
	if !consent.ActiveFor("lesson", []safety.DataClass{safety.DataEducational}, testNow) {
		t.Fatal("consent should be active")
	}
	if consent.ActiveFor("other", []safety.DataClass{safety.DataEducational}, testNow) {
		t.Fatal("purpose mismatch should fail")
	}
	remediation := safety.Remediation{RequiredTasks: []string{"read", "reflect"}, Completed: map[string]time.Time{"read": testNow}}
	if remediation.Complete() {
		t.Fatal("incomplete remediation reported complete")
	}
	remediation.Completed["reflect"] = testNow.Add(time.Minute)
	if !remediation.Complete() {
		t.Fatal("complete remediation reported incomplete")
	}
}

func TestCompetencySupersessionAndHighestSelection(t *testing.T) {
	first, _ := learning.NewCompetency(common.ID("comp_first"), common.ID("tenant_more"), common.ID("learner_more"), common.ID("evidence_first"), "SAFE", learning.LevelGuided, testNow, 0)
	second, _ := learning.NewCompetency(common.ID("comp_second"), common.ID("tenant_more"), common.ID("learner_more"), common.ID("evidence_second"), "SAFE", learning.LevelIndependent, testNow.Add(time.Minute), 0)
	if err := first.Supersede(testNow.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	selected, ok := learning.HighestActive([]learning.Competency{first, second}, "SAFE", testNow.Add(3*time.Minute))
	if !ok || selected.ID != second.ID {
		t.Fatalf("highest active selection wrong: %+v %v", selected, ok)
	}
	if err := first.Supersede(testNow.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceDecisionBoundaries(t *testing.T) {
	if _, err := learning.NewReviewDecision(common.ID("review_bad"), common.ID("evidence_bad"), common.ID("reviewer_bad"), learning.ReviewPass, 59, "Feedback is intentionally detailed enough for the validation contract.", testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("pass below threshold should fail: %v", err)
	}
	if _, err := learning.NewReviewDecision(common.ID("review_bad"), common.ID("evidence_bad"), common.ID("reviewer_bad"), learning.ReviewOutcome("unknown"), 50, "Feedback is intentionally detailed enough for the validation contract.", testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("unknown decision should fail: %v", err)
	}
	evidence, err := learning.NewEvidence(common.ID("evidence_bad"), common.ID("tenant_more"), common.ID("enrollment_more"), common.ID("learner_more"), "SAFE", 1, 1, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Attach("uri", "short", "too short", testNow); !errors.Is(err, common.ErrInvalid) {
		t.Fatalf("short artifact should fail: %v", err)
	}
}

func TestIncidentResolutionRequiresRemediationState(t *testing.T) {
	incident, err := safety.NewIncident(common.ID("incident_more"), common.ID("tenant_more"), common.ID("learner_more"), common.ID("grant_more"), common.ID("reporter_more"), safety.SeverityModerate, "A sufficiently detailed incident summary explains the observed issue and required response.", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := incident.Resolve(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("open incident should not resolve: %v", err)
	}
	if err := incident.Restrict(testNow); err != nil {
		t.Fatal(err)
	}
	if err := incident.BeginRemediation(testNow); err != nil {
		t.Fatal(err)
	}
	if err := incident.Resolve(testNow); err != nil {
		t.Fatal(err)
	}
	if incident.ResolvedAt == nil {
		t.Fatal("resolved incident should record time")
	}
}

func TestConsentExpiryBoundary(t *testing.T) {
	consent := safety.Consent{Purpose: "lesson", GrantedAt: testNow, ExpiresAt: testNow.Add(time.Hour)}
	if consent.ActiveFor("lesson", nil, testNow.Add(time.Hour)) {
		t.Fatal("consent expiry must be exclusive")
	}
}

func TestProjectMemberDeduplicationAndPauseResumeRules(t *testing.T) {
	value, err := project.New(common.ID("project_more"), common.ID("tenant_more"), common.ID("offer_more"), "Project", "Students investigate a real school problem and build a transparent prototype with evidence and reflection for review.", []common.ID{common.ID("learner_more"), common.ID("learner_more"), common.ID("learner_two")}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.MemberIDs) != 2 {
		t.Fatalf("members should be deduplicated: %v", value.MemberIDs)
	}
	if err := value.Approve(common.ID("reviewer_more"), testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.AssignMentor(common.ID("mentor_more"), testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.SetToolGrants([]common.ID{common.ID("grant_more")}, testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Launch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Pause(testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.SetToolGrants(nil, testNow); err != nil {
		t.Fatal(err)
	}
	if err := value.Launch(testNow); !errors.Is(err, common.ErrState) {
		t.Fatalf("paused project should not launch directly: %v", err)
	}
}
