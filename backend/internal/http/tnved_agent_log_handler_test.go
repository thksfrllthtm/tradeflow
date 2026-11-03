package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestParseLogFilter(t *testing.T) {
	now := time.Date(2026, 11, 3, 20, 30, 0, 0, time.UTC) // 4 ноября 01:30 по Казахстану
	filter, err := parseLogFilter(url.Values{}, now)
	if err != nil || filter.From.Format("2006-01-02") != "2026-10-06" || filter.To.Format("2006-01-02") != "2026-11-05" || filter.Limit != 50 || filter.Offset != 0 {
		t.Fatalf("по умолчанию: %+v %v", filter, err)
	}
	filter, err = parseLogFilter(url.Values{"from": {"2026-10-01"}, "to": {"2026-10-31"}, "userId": {"3"}, "mode": {"row"}, "only": {"disagree"}, "page": {"3"}, "limit": {"20"}}, now)
	if err != nil || filter.From.Format("2006-01-02") != "2026-10-01" || filter.To.Format("2006-01-02") != "2026-11-01" || filter.UserID != 3 ||
		filter.Mode != "row" || filter.Only != "disagree" || filter.Limit != 20 || filter.Offset != 40 {
		t.Fatalf("с параметрами: %+v %v", filter, err)
	}
	for name, query := range map[string]url.Values{
		"дата":        {"from": {"01.10.2026"}},
		"период":      {"from": {"2026-10-31"}, "to": {"2026-10-01"}},
		"сотрудник":   {"userId": {"x"}},
		"режим":       {"mode": {"documents"}},
		"выборка":     {"only": {"all"}},
		"страница":    {"page": {"0"}},
		"много строк": {"limit": {"500"}},
	} {
		if _, err := parseLogFilter(query, now); err == nil {
			t.Fatalf("%s: ошибки нет", name)
		}
	}
}

// Журнал — только администратору: остальным 403 ещё до обработчика.
func TestAgentLogRequiresAdmin(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	for _, role := range []string{repository.RoleInvoicer, repository.RoleTranslator} {
		router := gin.New()
		signedIn := func(c *gin.Context) { c.Set(userContextKey, repository.User{ID: 7, Role: role}) }
		router.GET("/log", signedIn, server.requireAdmin, server.agentLog)
		router.GET("/log/:id", signedIn, server.requireAdmin, server.agentLogEntry)
		for _, path := range []string{"/log", "/log/5"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusForbidden {
				t.Fatalf("%s %s: %d", role, path, response.Code)
			}
		}
	}
	for _, path := range []string{"/api/tnved-agent/log", "/api/tnved-agent/log/5"} {
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("без входа %s: %d", path, response.Code)
		}
	}
}
