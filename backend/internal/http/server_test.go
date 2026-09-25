package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

func TestGlovesRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodPost, "/api/gloves/calculate", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestEstimateRoutesRequireSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/gloves-copy/estimates"},
		{http.MethodPost, "/api/estimates/price-weights"},
		{http.MethodPost, "/api/estimates/price-history"},
	} {
		request := httptest.NewRequest(route.method, route.path, bytes.NewBufferString(`{}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		server.Router().ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected status %d, got %d", route.method, route.path, http.StatusUnauthorized, response.Code)
		}
	}
}

func TestRegisterValidatesBeforeDatabaseAccess(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"name":"","email":"bad","password":"short"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.Router().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestAuthAttemptRateLimit(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for attempt := 0; attempt < 10; attempt++ {
		if !server.allowAuthAttempt("127.0.0.1", "employee@example.com") {
			t.Fatalf("attempt %d was unexpectedly limited", attempt+1)
		}
	}
	if server.allowAuthAttempt("127.0.0.1", "employee@example.com") {
		t.Fatal("11th attempt should be limited")
	}
}
