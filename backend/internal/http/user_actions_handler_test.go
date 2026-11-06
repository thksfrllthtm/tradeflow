package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
)

// Каждый маршрут-изменение либо подписан для журнала, либо явно не пишется; в списках нет удалённых маршрутов.
func TestEveryChangeRouteIsClassified(t *testing.T) {
	server := NewServer(nil, config.Config{SessionCookieName: "test_session", SessionTTL: time.Hour})
	routes := map[string]bool{}
	for _, route := range server.Router().Routes() {
		key := route.Method + " " + route.Path
		routes[key] = true
		if route.Method == http.MethodGet {
			continue
		}
		_, labeled := actionLabels[key]
		if labeled == notRecorded[key] {
			t.Errorf("%s: нужен ровно в одном списке — actionLabels или notRecorded", key)
		}
	}
	for key := range actionLabels {
		if !routes[key] {
			t.Errorf("actionLabels: маршрута %s нет", key)
		}
	}
	for key := range notRecorded {
		if !routes[key] {
			t.Errorf("notRecorded: маршрута %s нет", key)
		}
	}
}
