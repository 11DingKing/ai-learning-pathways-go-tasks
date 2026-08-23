package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/learning"
	"github.com/11DingKing/ai-learning-pathways-go/internal/service"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	tenantID, err := common.ParseID("tenant_id", input.TenantID)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.Login(r.Context(), tenantID, input.Email, input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": result.Token, "expires_at": result.Session.ExpiresAt, "user": result.User})
}
func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer "))
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	actor, _ := actorFrom(r.Context())
	if err := s.service.Logout(r.Context(), actor, bearer(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type outcomeBody struct {
	Code, Title, Description, Kind string
	Required                       bool
}
type curriculumBody struct {
	Code, Title, Summary, Stage                string
	Outcomes                                   []outcomeBody
	Prerequisites                              []string
	MinimumEthicsHours, MinimumPracticeMinutes int
}

func (body curriculumBody) input() (service.CurriculumInput, error) {
	stage, err := identity.ParseEducationStage(body.Stage)
	if err != nil {
		return service.CurriculumInput{}, err
	}
	outcomes := make([]curriculum.Outcome, 0, len(body.Outcomes))
	for _, item := range body.Outcomes {
		outcome, err := curriculum.NewOutcome(item.Code, item.Title, item.Description, curriculum.OutcomeKind(item.Kind), item.Required)
		if err != nil {
			return service.CurriculumInput{}, err
		}
		outcomes = append(outcomes, outcome)
	}
	ids := make([]common.ID, 0, len(body.Prerequisites))
	for _, raw := range body.Prerequisites {
		id, err := common.ParseID("prerequisite", raw)
		if err != nil {
			return service.CurriculumInput{}, err
		}
		ids = append(ids, id)
	}
	return service.CurriculumInput{Code: body.Code, Title: body.Title, Summary: body.Summary, Stage: stage, Outcomes: outcomes, Prerequisites: ids, MinimumEthicsHours: body.MinimumEthicsHours, MinimumPracticeMinutes: body.MinimumPracticeMinutes}, nil
}
func (s *Server) createCurriculum(w http.ResponseWriter, r *http.Request) {
	var body curriculumBody
	if !decodeJSON(w, r, &body) {
		return
	}
	input, err := body.input()
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.CreateCurriculum(r.Context(), actor, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func pathID(r *http.Request) (common.ID, error) { return common.ParseID("id", r.PathValue("id")) }
func (s *Server) submitCurriculum(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.SubmitCurriculum(r.Context(), actor, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) approveCurriculum(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct{ Comment string }
	if !decodeJSON(w, r, &body) {
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.ApproveCurriculum(r.Context(), actor, id, body.Comment)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) publishCurriculum(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.PublishCurriculum(r.Context(), actor, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createOffering(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurriculumID, Code, Title string
		StartsAt, EndsAt          time.Time
		Capacity, MinimumAge      int
		RequiresGuardian          bool
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	courseID, err := common.ParseID("curriculum_id", body.CurriculumID)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.CreateOffering(r.Context(), actor, service.OfferingInput{CurriculumID: courseID, Code: body.Code, Title: body.Title, StartsAt: body.StartsAt, EndsAt: body.EndsAt, Capacity: body.Capacity, MinimumAge: body.MinimumAge, RequiresGuardian: body.RequiresGuardian})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (s *Server) activateOffering(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.ActivateOffering(r.Context(), actor, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	var body struct{ OfferingID, IdempotencyKey, GuardianConsentID string }
	if !decodeJSON(w, r, &body) {
		return
	}
	offeringID, err := common.ParseID("offering_id", body.OfferingID)
	if err != nil {
		writeError(w, err)
		return
	}
	var consentID *common.ID
	if body.GuardianConsentID != "" {
		id, err := common.ParseID("guardian_consent_id", body.GuardianConsentID)
		if err != nil {
			writeError(w, err)
			return
		}
		consentID = &id
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.Enroll(r.Context(), actor, service.EnrollmentInput{OfferingID: offeringID, IdempotencyKey: body.IdempotencyKey, GuardianConsentID: consentID})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) submitEvidence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EnrollmentID, OutcomeCode, ArtifactURI, ArtifactSHA256, Reflection string
		RubricVersion, Attempt                                             int
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	enrollmentID, err := common.ParseID("enrollment_id", body.EnrollmentID)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := actorFrom(r.Context())
	value, err := s.service.SubmitEvidence(r.Context(), actor, service.EvidenceInput{EnrollmentID: enrollmentID, OutcomeCode: body.OutcomeCode, RubricVersion: body.RubricVersion, Attempt: body.Attempt, ArtifactURI: body.ArtifactURI, ArtifactSHA256: body.ArtifactSHA256, Reflection: body.Reflection})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (s *Server) reviewEvidence(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Outcome, Feedback, CompetencyCode string
		Score, CompetencyLevel            int
		ValidForHours                     int
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	actor, _ := actorFrom(r.Context())
	value, competencyValue, err := s.service.ReviewEvidence(r.Context(), actor, service.ReviewInput{EvidenceID: id, Outcome: learning.ReviewOutcome(body.Outcome), Score: body.Score, Feedback: body.Feedback, CompetencyCode: body.CompetencyCode, CompetencyLevel: learning.CompetencyLevel(body.CompetencyLevel), ValidFor: time.Duration(body.ValidForHours) * time.Hour})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evidence": value, "competency": competencyValue})
}
