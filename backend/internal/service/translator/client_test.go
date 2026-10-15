package translator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

func TestClientRetriesRateLimitAndStopsOnAuthError(t *testing.T) {
	statuses := []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusOK}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		json.NewDecoder(r.Body).Decode(&request)
		if r.Header.Get("Authorization") != "Bearer key" || request["model"] != "m" {
			t.Errorf("request %v", request)
		}
		status := statuses[min(calls, len(statuses)-1)]
		calls++
		w.WriteHeader(status)
		w.Write([]byte(`{"choices":[{"message":{"content":"  Договор <g1>поставки</g1>\n"}}]}`))
	}))
	defer server.Close()
	client := NewClient("key", server.URL, "m")
	client.retryDelay = 0
	text, err := client.Translate(context.Background(), "system", "user")
	if err != nil || text != "Договор <g1>поставки</g1>" || calls != 3 {
		t.Fatalf("text %q, err %v, calls %d", text, err, calls)
	}

	statuses, calls = []int{http.StatusUnauthorized}, 0
	if _, err := client.Translate(context.Background(), "system", "user"); !errors.Is(err, ai.ErrAIUnauthorized) || calls != 1 {
		t.Fatalf("err %v, calls %d", err, calls)
	}
}
