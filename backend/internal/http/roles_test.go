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

// routerAs runs requireAdmin for a signed-in user with the given role.
func routerAs(role string) *gin.Engine {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	router := gin.New()
	signedIn := func(c *gin.Context) { c.Set(userContextKey, repository.User{ID: 7, Role: role}) }
	router.POST("/admin-only", signedIn, server.requireAdmin, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/translator-only", signedIn, server.requireTranslator, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/not-translator", signedIn, server.requireNotTranslator, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/scope", signedIn, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"scope": estimateScope(c)}) })
	return router
}

func TestRequireAdminAllowsOnlyAdministrators(t *testing.T) {
	for role, want := range map[string]int{
		repository.RoleAdmin:      http.StatusNoContent,
		repository.RoleInvoicer:   http.StatusForbidden,
		repository.RoleTranslator: http.StatusForbidden,
		"":                        http.StatusForbidden,
	} {
		response := httptest.NewRecorder()
		routerAs(role).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/admin-only", nil))
		if response.Code != want {
			t.Fatalf("role %q: expected %d, got %d", role, want, response.Code)
		}
	}
}

func TestRequireTranslatorAllowsTranslatorsAndAdministrators(t *testing.T) {
	for role, want := range map[string]int{
		repository.RoleAdmin:      http.StatusNoContent,
		repository.RoleTranslator: http.StatusNoContent,
		repository.RoleInvoicer:   http.StatusForbidden,
		"":                        http.StatusForbidden,
	} {
		response := httptest.NewRecorder()
		routerAs(role).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/translator-only", nil))
		if response.Code != want {
			t.Fatalf("role %q: expected %d, got %d", role, want, response.Code)
		}
	}
}

func TestRequireNotTranslatorBlocksOnlyTranslators(t *testing.T) {
	for role, want := range map[string]int{
		repository.RoleAdmin:      http.StatusNoContent,
		repository.RoleInvoicer:   http.StatusNoContent,
		repository.RoleTranslator: http.StatusForbidden,
	} {
		response := httptest.NewRecorder()
		routerAs(role).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/not-translator", nil))
		if response.Code != want {
			t.Fatalf("role %q: expected %d, got %d", role, want, response.Code)
		}
	}
}

func TestEstimateScopeLetsOnlyAdministratorsSeeEveryone(t *testing.T) {
	for role, want := range map[string]string{
		repository.RoleAdmin:    `{"scope":-1}`,
		repository.RoleInvoicer: `{"scope":7}`,
	} {
		response := httptest.NewRecorder()
		routerAs(role).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/scope", nil))
		if response.Body.String() != want {
			t.Fatalf("role %q: expected %s, got %s", role, want, response.Body.String())
		}
	}
}

func TestEmployeesAndDirectoryChangesRequireSession(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/employees"},
		{http.MethodPost, "/api/employees"},
		{http.MethodPost, "/api/product-directory"},
		{http.MethodPatch, "/api/product-directory/1"},
		{http.MethodGet, "/api/tnved-nomenclature"},
		{http.MethodPost, "/api/tnved-nomenclature/lookup"},
		{http.MethodPost, "/api/tnved-nomenclature/import"},
		{http.MethodPost, "/api/util-fee"},
		{http.MethodPost, "/api/export-check"},
		{http.MethodPost, "/api/tnved-kz-rates/excel"},
		{http.MethodPost, "/api/tnved-kz-rates/import-pdf"},
		{http.MethodPost, "/api/tnved-kz-rates/import"},
		{http.MethodGet, "/api/code-decisions"},
		{http.MethodPost, "/api/code-decisions"},
		{http.MethodPost, "/api/code-decisions/import"},
		{http.MethodPost, "/api/code-decisions/1/verify"},
		{http.MethodGet, "/api/tnved-agent/status"},
		{http.MethodPost, "/api/tnved-agent/sessions"},
		{http.MethodGet, "/api/tnved-agent/sessions/1"},
		{http.MethodPost, "/api/tnved-agent/sessions/1/apply"},
		{http.MethodPost, "/api/tnved-agent/sessions/1/documents"},
		{http.MethodGet, "/api/translator/documents"},
		{http.MethodPost, "/api/translator/documents"},
		{http.MethodDelete, "/api/translator/documents/1"},
		{http.MethodPost, "/api/translator/documents/1/translate"},
		{http.MethodGet, "/api/translator/documents/1/export"},
		{http.MethodPatch, "/api/translator/segments/1"},
		{http.MethodPost, "/api/translator/segments/1/retranslate"},
	} {
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401, got %d", route.method, route.path, response.Code)
		}
	}
}
