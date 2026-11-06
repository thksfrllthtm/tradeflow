package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// deepSeekServer answers every request with the given assistant content.
func deepSeekServer(t *testing.T, status int, content string, inspect func(body map[string]any, r *http.Request)) *DeepSeekClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		if inspect != nil {
			inspect(body, r)
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	t.Cleanup(server.Close)
	client := NewDeepSeekClient("test-key", "deepseek-flash")
	client.endpoint = server.URL
	return client
}

var testPriceRequest = PriceWeightsRequest{Currency: "USD", Groups: []PriceGroup{
	{Code: "8201100000", Unit: "шт", Products: []PriceProduct{
		{ID: "p1", Name: "Лопата штыковая", Quantity: 2400, Places: 200, GrossKg: 1680, NetKg: 1580},
		{ID: "p2", Name: "Лопата совковая", Quantity: 6000, Places: 500, GrossKg: 4200, NetKg: 3950},
	}},
	{Code: "6116109500", Unit: "пар", Products: []PriceProduct{{ID: "p3", Name: "Перчатки", Quantity: 1200, NetKg: 60}}},
}}

func TestDeepSeekSendsProductsWithoutBudget(t *testing.T) {
	client := deepSeekServer(t, http.StatusOK, `{"prices":[{"id":"p1","unitPrice":2.1,"reason":"штыковая"},{"id":"p2","unitPrice":1.9,"reason":"совковая"},{"id":"p3","unitPrice":0.4,"reason":"трикотаж"}]}`, func(body map[string]any, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer key")
		}
		if body["model"] != "deepseek-flash" {
			t.Errorf("unexpected model %v", body["model"])
		}
		if format, _ := body["response_format"].(map[string]any); format["type"] != "json_object" {
			t.Errorf("JSON output mode must be requested, got %v", body["response_format"])
		}
		// The slow thinking mode is off; a low temperature keeps prices stable.
		if thinking, _ := body["thinking"].(map[string]any); thinking["type"] != "disabled" || body["temperature"] != 0.2 {
			t.Errorf("thinking must be disabled with temperature 0.2, got %v and %v", body["thinking"], body["temperature"])
		}
		encoded, _ := json.Marshal(body["messages"])
		prompt := string(encoded)
		if !strings.Contains(prompt, "json") || strings.Contains(strings.ToLower(prompt), "budget") || strings.Contains(prompt, "бюджет") {
			t.Errorf("prompt must mention json and never the budget: %s", prompt)
		}
		// Products are grouped by code with weights per unit: 1580 kg / 2400 pcs.
		if !strings.Contains(prompt, `\"netKgPerUnit\":0.6583`) || !strings.Contains(prompt, `\"unitsPerPlace\":12`) || !strings.Contains(prompt, "Лопата совковая") {
			t.Errorf("prompt must list products with per-unit weights: %s", prompt)
		}
	})
	prices, err := client.SuggestUnitPrices(context.Background(), 1, testPriceRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 3 || prices[0].ID != "p1" || prices[0].Code != "8201100000" || prices[1].UnitPrice != 1.9 || prices[2].Code != "6116109500" || prices[2].Reason != "трикотаж" {
		t.Fatalf("unexpected prices %+v", prices)
	}
}

func TestDeepSeekDropsUnknownProductsAndInvalidPrices(t *testing.T) {
	content := "```json\n" + `{"prices":[
		{"id":"p9","unitPrice":3},
		{"id":"p1","unitPrice":-2},
		{"id":"p1","unitPrice":"2,5","reason":"строкой"},
		{"id":"p1","unitPrice":4},
		{"id":"p2","unitPrice":0},
		{"id":"p3","unitPrice":"abc"}
	]}` + "\n```"
	prices, err := deepSeekServer(t, http.StatusOK, content, nil).SuggestUnitPrices(context.Background(), 1, testPriceRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 || prices[0].ID != "p1" || prices[0].UnitPrice != 2.5 {
		t.Fatalf("only the first valid price of a requested product must remain, got %+v", prices)
	}
}

func TestDeepSeekRejectsGarbageAndErrors(t *testing.T) {
	for name, testCase := range map[string]struct {
		status  int
		content string
		want    error
	}{
		"not json":            {http.StatusOK, "Цены: лопаты 2 доллара", ErrAIBadResponse},
		"empty content":       {http.StatusOK, "", ErrAIBadResponse},
		"no valid price":      {http.StatusOK, `{"prices":[{"id":"x","unitPrice":5},{"id":"p1","unitPrice":-1}]}`, ErrAIBadResponse},
		"wrong key":           {http.StatusUnauthorized, "", ErrAIUnauthorized},
		"insufficient credit": {http.StatusPaymentRequired, "", ErrAIUnauthorized},
		"rate limit":          {http.StatusTooManyRequests, "", ErrAIRateLimited},
		"server down":         {http.StatusServiceUnavailable, "", ErrAIUnavailable},
	} {
		_, err := deepSeekServer(t, testCase.status, testCase.content, nil).SuggestUnitPrices(context.Background(), 1, testPriceRequest)
		if !errors.Is(err, testCase.want) {
			t.Fatalf("%s: expected %v, got %v", name, testCase.want, err)
		}
	}
}

func TestDeepSeekValidatesRequestBeforeCalling(t *testing.T) {
	called := false
	client := deepSeekServer(t, http.StatusOK, `{"prices":[]}`, func(map[string]any, *http.Request) { called = true })
	product := func(id string, quantity float64) []PriceProduct {
		return []PriceProduct{{ID: id, Name: "Товар", Quantity: quantity}}
	}
	for name, request := range map[string]PriceWeightsRequest{
		"no groups":      {},
		"euro":           {Currency: "EUR", Groups: testPriceRequest.Groups},
		"no products":    {Groups: []PriceGroup{{Code: "1"}}},
		"zero quantity":  {Groups: []PriceGroup{{Code: "1", Products: product("p1", 0)}}},
		"duplicate code": {Groups: []PriceGroup{{Code: "1", Products: product("p1", 1)}, {Code: "1", Products: product("p2", 2)}}},
		"duplicate id":   {Groups: []PriceGroup{{Code: "1", Products: product("p1", 1)}, {Code: "2", Products: product("p1", 2)}}},
		"empty id":       {Groups: []PriceGroup{{Code: "1", Products: product(" ", 1)}}},
	} {
		var validation *service.ValidationError
		if _, err := client.SuggestUnitPrices(context.Background(), 1, request); !errors.As(err, &validation) {
			t.Fatalf("%s: expected a validation error, got %v", name, err)
		}
	}
	if called {
		t.Fatal("an invalid request must not reach DeepSeek")
	}
	if _, err := NewDeepSeekClient("", "deepseek-flash").SuggestUnitPrices(context.Background(), 1, testPriceRequest); !errors.Is(err, ErrAINotConfigured) {
		t.Fatalf("missing key must report not configured, got %v", err)
	}
}
