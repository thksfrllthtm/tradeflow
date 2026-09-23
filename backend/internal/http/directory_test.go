package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

func TestTNVEDDirectoryRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	request := httptest.NewRequest(http.MethodGet, "/api/tnved-directory?category=Перчатки", nil)
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}
