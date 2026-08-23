package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/httpapi"
	"github.com/11DingKing/ai-learning-pathways-go/internal/service"
	"github.com/11DingKing/ai-learning-pathways-go/internal/storage/sqlite"
)

type httpFixture struct {
	db      *sqlite.DB
	handler http.Handler
	tenant  common.ID
}

func newHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	tenant := common.ID("tenant_http")
	hash, _ := service.HashPassword("http-admin-password")
	admin, _ := identity.NewUser(common.ID("admin_http"), tenant, "admin@http.edu", "HTTP Admin", hash, identity.RoleProgramAdmin, identity.StageUniversity, now)
	if err := db.BootstrapIdentity(context.Background(), tenant, "HTTP Tenant", "UTC", admin); err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(db, common.FixedClock{Value: now}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	api, err := httpapi.New(svc, db, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return httpFixture{db: db, handler: api.Handler(), tenant: tenant}
}

func (f httpFixture) close() { _ = f.db.Close() }

func TestHealthReadinessAndRequestID(t *testing.T) {
	f := newHTTPFixture(t)
	defer f.close()
	for _, path := range []string{"/healthz", "/readyz"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		if response.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s omitted request id", path)
		}
	}
}

func TestLoginAndProtectedEndpoint(t *testing.T) {
	f := newHTTPFixture(t)
	defer f.close()
	body, _ := json.Marshal(map[string]string{"tenant_id": f.tenant.String(), "email": "admin@http.edu", "password": "http-admin-password"})
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("login returned %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Token == "" {
		t.Fatal("login omitted token")
	}
	protected := httptest.NewRequest(http.MethodPost, "/v1/curricula", bytes.NewBufferString("{}"))
	protected.Header.Set("Authorization", "Bearer "+result.Token)
	protected.Header.Set("Content-Type", "application/json")
	protectedResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(protectedResponse, protected)
	if protectedResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid protected body returned %d: %s", protectedResponse.Code, protectedResponse.Body.String())
	}
}

func TestAuthenticationAndErrorMapping(t *testing.T) {
	f := newHTTPFixture(t)
	defer f.close()
	request := httptest.NewRequest(http.MethodPost, "/v1/curricula", bytes.NewBufferString("{}"))
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth returned %d", response.Code)
	}
	bad := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString("not-json"))
	bad.Header.Set("Content-Type", "application/json")
	badResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad json returned %d", badResponse.Code)
	}
	var envelope map[string]map[string]string
	if err := json.Unmarshal(badResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["error"]["code"] != "invalid_request" {
		t.Fatalf("unexpected error envelope: %v", envelope)
	}
}
