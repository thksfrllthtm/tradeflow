package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

// База документов — только администратору: сотрудникам 403 до обработчика, без входа — 401 на настоящих адресах.
func TestDocumentsBaseRequiresAdmin(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for _, role := range []string{repository.RoleInvoicer, repository.RoleTranslator} {
		router := gin.New()
		signedIn := func(c *gin.Context) { c.Set(userContextKey, repository.User{ID: 7, Role: role}) }
		router.GET("/documents-base", signedIn, server.requireAdmin, server.documentsBase)
		router.GET("/documents-base/codes/:code", signedIn, server.requireAdmin, server.documentsBaseCode)
		router.GET("/documents-base/sources", signedIn, server.requireAdmin, server.documentsSources)
		for _, path := range []string{"/documents-base", "/documents-base/codes/6302510001", "/documents-base/sources"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusForbidden {
				t.Fatalf("%s %s: %d", role, path, response.Code)
			}
		}
	}
	for _, path := range []string{"/api/tnved-agent/documents-base", "/api/tnved-agent/documents-base/codes/6302510001", "/api/tnved-agent/documents-base/sources"} {
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("без входа %s: %d", path, response.Code)
		}
	}
}
