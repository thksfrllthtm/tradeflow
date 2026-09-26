package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

func TestCeramicsRoutesRequireSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/ceramics/status"},
		{http.MethodGet, "/api/ceramics/types"},
		{http.MethodGet, "/api/ceramics/products"},
		{http.MethodPost, "/api/ceramics/products"},
		{http.MethodPost, "/api/ceramics/products/import"},
		{http.MethodPost, "/api/ceramics/lookup"},
		{http.MethodPost, "/api/ceramics/analyze"},
		{http.MethodPost, "/api/ceramics/price"},
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
