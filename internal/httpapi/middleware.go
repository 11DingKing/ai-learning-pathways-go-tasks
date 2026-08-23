package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
		r.ResponseWriter.WriteHeader(status)
	}
}
func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	count, err := r.ResponseWriter.Write(body)
	r.bytes += count
	return count, err
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if len(value) > 100 {
			value = ""
		}
		if value == "" {
			id, err := common.NewID("req")
			if err != nil {
				value = fmt.Sprintf("req-%d", time.Now().UnixNano())
			} else {
				value = id.String()
			}
		}
		w.Header().Set("X-Request-ID", value)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey("request-id"), value)))
	})
}

func requestIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(contextKey("request-id")).(string)
	return value
}

func (s *Server) timeoutRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		s.logger.Info("http request", "request_id", requestIDFrom(r.Context()), "method", r.Method, "path", r.URL.Path, "status", status, "bytes", recorder.bytes, "duration_ms", time.Since(started).Milliseconds())
	})
}

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic recovered", "request_id", requestIDFrom(r.Context()), "panic", recovered, "stack", string(debug.Stack()))
				writeError(w, fmt.Errorf("panic: %v", recovered))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
