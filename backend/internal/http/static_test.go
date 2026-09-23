package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

func frontendServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=root></div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app-123.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour, StaticDir: dir})
}

func get(server *Server, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func TestFrontendPagesServeIndex(t *testing.T) {
	server := frontendServer(t)
	for _, path := range []string{"/", "/estimates/goods/12", "/directory/tnved"} {
		response := get(server, path)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "id=root") {
			t.Fatalf("%s: expected index.html, got %d %q", path, response.Code, response.Body.String())
		}
	}
}

func TestFrontendAssetsAndMissingFiles(t *testing.T) {
	server := frontendServer(t)
	asset := get(server, "/assets/app-123.js")
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: got %d, cache %q", asset.Code, asset.Header().Get("Cache-Control"))
	}
	if missing := get(server, "/assets/old-999.js"); missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset must be 404, got %d", missing.Code)
	}
	// Путь с ".." не должен отдать ничего, кроме index.html (или отказа 400).
	if escape := get(server, "/../../etc/passwd"); escape.Code != http.StatusBadRequest && !strings.Contains(escape.Body.String(), "id=root") {
		t.Fatalf("path outside build dir leaked: %d %q", escape.Code, escape.Body.String())
	}
}

func TestUnknownAPIPathStaysJSON404(t *testing.T) {
	response := get(frontendServer(t), "/api/unknown")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "not_found") {
		t.Fatalf("expected JSON 404, got %d %q", response.Code, response.Body.String())
	}
}

func TestRegistrationCanBeDisabled(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour, RegistrationDisabled: true})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"name":"A","email":"a@example.com","password":"long-enough-password"}`))
	request.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "registration_disabled") {
		t.Fatalf("expected 403 registration_disabled, got %d %q", response.Code, response.Body.String())
	}
}
