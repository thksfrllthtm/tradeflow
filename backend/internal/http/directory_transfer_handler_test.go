package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

func TestDirectoryTransferRequiresSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/product-directory/export"},
		{http.MethodPost, "/api/product-directory/import"},
	} {
		request := httptest.NewRequest(route.method, route.path, bytes.NewBufferString(""))
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected %d, got %d", route.method, route.path, http.StatusUnauthorized, response.Code)
		}
	}
}
