package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	client := NewDeepSeekClient("test-key", DefaultDeepSeekModel)
	client.endpoint = server.URL
	return client
}

var testPriceRequest = PriceWeightsRequest{Currency: "USD", Groups: []PriceGroup{
	{Code: "7310299000", Description: "Бочки стальные", Quantity: 800, Unit: "шт", Places: 100, GrossKg: 1220, NetKg: 1170},
	{Code: "6116109500", Description: "Перчатки", Quantity: 1200, Unit: "пар", NetKg: 60},
}}

func TestDeepSeekSendsJSONRequestWithoutBudget(t *testing.T) {
	client := deepSeekServer(t, http.StatusOK, `{"prices":[{"code":"7310299000","unitPrice":7.5,"reason":"сталь"},{"code":"6116109500","unitPrice":0.4,"reason":"трикотаж"}]}`, func(body map[string]any, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer key")
		}
		if body["model"] != DefaultDeepSeekModel {
			t.Errorf("unexpected model %v", body["model"])
		}
		if format, _ := body["response_format"].(map[string]any); format["type"] != "json_object" {
			t.Errorf("JSON output mode must be requested, got %v", body["response_format"])
		}
		if thinking, _ := body["thinking"].(map[string]any); thinking["type"] != "disabled" {
			t.Errorf("thinking mode must be disabled so temperature applies, got %v", body["thinking"])
		}
		encoded, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encoded), "json") || strings.Contains(strings.ToLower(string(encoded)), "budget") || strings.Contains(string(encoded), "бюджет") {
			t.Errorf("prompt must mention json and never the budget: %s", encoded)
		}
	})
	prices, err := client.SuggestUnitPrices(context.Background(), 1, testPriceRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 2 || prices[0].UnitPrice != 7.5 || prices[1].Code != "6116109500" || prices[1].Reason != "трикотаж" {
		t.Fatalf("unexpected prices %+v", prices)
	}
}

func TestDeepSeekDropsUnknownCodesAndInvalidPrices(t *testing.T) {
	content := "```json\n" + `{"prices":[
		{"code":"0000000000","unitPrice":3},
		{"code":"7310299000","unitPrice":-2},
		{"code":"7310299000","unitPrice":"7,5","reason":"строкой"},
		{"code":"6116109500","unitPrice":0},
		{"code":"6116109500","unitPrice":"abc"}
	]}` + "\n```"
	prices, err := deepSeekServer(t, http.StatusOK, content, nil).SuggestUnitPrices(context.Background(), 1, testPriceRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 || prices[0].Code != "7310299000" || prices[0].UnitPrice != 7.5 {
		t.Fatalf("only the valid price of a requested code must remain, got %+v", prices)
	}
}

func TestDeepSeekRejectsGarbageAndErrors(t *testing.T) {
	for name, testCase := range map[string]struct {
		status  int
		content string
		want    error
	}{
		"not json":            {http.StatusOK, "Цены: бочки 7 долларов", ErrGeminiBadResponse},
		"empty content":       {http.StatusOK, "", ErrGeminiBadResponse},
		"no valid price":      {http.StatusOK, `{"prices":[{"code":"1","unitPrice":5},{"code":"7310299000","unitPrice":-1}]}`, ErrGeminiBadResponse},
		"wrong key":           {http.StatusUnauthorized, "", ErrGeminiUnauthorized},
		"insufficient credit": {http.StatusPaymentRequired, "", ErrGeminiUnauthorized},
		"rate limit":          {http.StatusTooManyRequests, "", ErrGeminiRateLimited},
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
	for name, request := range map[string]PriceWeightsRequest{
		"no groups":      {},
		"euro":           {Currency: "EUR", Groups: testPriceRequest.Groups},
		"zero quantity":  {Groups: []PriceGroup{{Code: "1", Quantity: 0}}},
		"duplicate code": {Groups: []PriceGroup{{Code: "1", Quantity: 1}, {Code: "1", Quantity: 2}}},
	} {
		var validation *ValidationError
		if _, err := client.SuggestUnitPrices(context.Background(), 1, request); !errors.As(err, &validation) {
			t.Fatalf("%s: expected a validation error, got %v", name, err)
		}
	}
	if called {
		t.Fatal("an invalid request must not reach DeepSeek")
	}
	if _, err := NewDeepSeekClient("", DefaultDeepSeekModel).SuggestUnitPrices(context.Background(), 1, testPriceRequest); !errors.Is(err, ErrGeminiNotConfigured) {
		t.Fatalf("missing key must report not configured, got %v", err)
	}
}
