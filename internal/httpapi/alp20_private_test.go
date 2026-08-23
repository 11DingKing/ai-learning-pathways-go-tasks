package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnknownBearerTokenKeepsAuthenticationStatus(t *testing.T) {
	f := newHTTPFixture(t)
	defer f.close()
	request := httptest.NewRequest(http.MethodPost, "/v1/curricula", bytes.NewBufferString(`{}`))
	request.Header.Set("Authorization", "Bearer abcdefghijklmnopqrstuvwxyz123456")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown token returned %d: %s", response.Code, response.Body.String())
	}
}
