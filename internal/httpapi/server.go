package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
	"github.com/11DingKing/ai-learning-pathways-go/internal/service"
)

type Server struct {
	service *service.Service
	store   repository.Store
	logger  *slog.Logger
	timeout time.Duration
	mux     *http.ServeMux
}

func New(serviceLayer *service.Service, store repository.Store, logger *slog.Logger, timeout time.Duration) (*Server, error) {
	if serviceLayer == nil || store == nil || logger == nil || timeout <= 0 {
		return nil, common.FieldError{Field: "http_server", Message: "dependencies and timeout are required"}
	}
	server := &Server{service: serviceLayer, store: store, logger: logger, timeout: timeout, mux: http.NewServeMux()}
	server.routes()
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.recoverPanic(s.requestID(s.accessLog(s.timeoutRequests(s.mux))))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.HandleFunc("POST /v1/sessions", s.login)
	s.mux.Handle("DELETE /v1/session", s.authenticate(http.HandlerFunc(s.logout)))
	s.mux.Handle("POST /v1/curricula", s.authenticate(http.HandlerFunc(s.createCurriculum)))
	s.mux.Handle("POST /v1/curricula/{id}/submit", s.authenticate(http.HandlerFunc(s.submitCurriculum)))
	s.mux.Handle("POST /v1/curricula/{id}/approve", s.authenticate(http.HandlerFunc(s.approveCurriculum)))
	s.mux.Handle("POST /v1/curricula/{id}/publish", s.authenticate(http.HandlerFunc(s.publishCurriculum)))
	s.mux.Handle("POST /v1/offerings", s.authenticate(http.HandlerFunc(s.createOffering)))
	s.mux.Handle("POST /v1/offerings/{id}/activate", s.authenticate(http.HandlerFunc(s.activateOffering)))
	s.mux.Handle("POST /v1/enrollments", s.authenticate(http.HandlerFunc(s.enroll)))
	s.mux.Handle("POST /v1/evidence", s.authenticate(http.HandlerFunc(s.submitEvidence)))
	s.mux.Handle("POST /v1/evidence/{id}/reviews", s.authenticate(http.HandlerFunc(s.reviewEvidence)))
}

type contextKey string

const actorKey contextKey = "authenticated-actor"

func actorFrom(ctx context.Context) (service.Actor, bool) {
	actor, ok := ctx.Value(actorKey).(service.Actor)
	return actor, ok
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, common.ErrUnauthenticated)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		actor, err := s.service.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, err)
			return
		}
		actor.RequestID = requestIDFrom(r.Context())
		ctx := context.WithValue(r.Context(), actorKey, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, common.FieldError{Field: "body", Message: "must be valid JSON: " + err.Error()})
		return false
	}
	if decoder.More() {
		writeError(w, common.FieldError{Field: "body", Message: "must contain one JSON value"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, context.Canceled):
		status, code = 499, "request_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		status, code = http.StatusGatewayTimeout, "deadline_exceeded"
	case errors.Is(err, common.ErrUnauthenticated), errors.Is(err, common.ErrExpired):
		status, code = http.StatusUnauthorized, "unauthenticated"
	case errors.Is(err, common.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, common.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, common.ErrInvalid), errors.Is(err, common.ErrState):
		status, code = http.StatusUnprocessableEntity, "invalid_request"
	case errors.Is(err, common.ErrCapacity):
		status, code = http.StatusConflict, "capacity_exhausted"
	case errors.Is(err, common.ErrConflict), errors.Is(err, common.ErrLeaseLost):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, common.ErrDependency):
		status, code = http.StatusServiceUnavailable, "dependency_unavailable"
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	var envelope errorEnvelope
	envelope.Error.Code = code
	envelope.Error.Message = message
	writeJSON(w, status, envelope)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
