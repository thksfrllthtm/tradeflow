package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

func TestEstimateListRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodGet, "/api/estimates", nil)
	response := httptest.NewRecorder()

	server.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestProductImageRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodGet, "/api/products/1/images/1", nil)
	response := httptest.NewRecorder()

	server.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestProductAIRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodPost, "/api/products/1/ai-analyze", nil)
	response := httptest.NewRecorder()

	server.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

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

func TestProductAIDialogueRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodGet, "/api/products/1/ai-dialog", nil)
	response := httptest.NewRecorder()

	server.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
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
