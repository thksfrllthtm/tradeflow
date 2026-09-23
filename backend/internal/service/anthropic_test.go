package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAnthropicGenerateValidatesResultAndHeaders(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("Anthropic API key header was not sent")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("Anthropic version header was not sent")
		}
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		if requestBody["model"] != "claude-haiku-4-5-20251001" {
			t.Errorf("unexpected model: %v", requestBody["model"])
		}
		payload := `{"primaryCode":"0101210000","positionName":"Степлер","explanation":"Недостаточно характеристик для окончательного выбора","alternatives":[],"missingCharacteristics":["материал корпуса"],"questions":["Укажите материал корпуса"],"status":"нужно уточнение"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]string{"type": "text", "text": payload}}})
	}))
	defer server.Close()
	client := NewAnthropicClient("test-key", "claude-haiku-4-5-20251001")
	client.endpoint = server.URL
	result, err := client.Generate(context.Background(), 1, ProductAnalysisInput{Name: "степлер"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.PrimaryCode != "0101210000" || result.Status != statusQuestions {
		t.Fatalf("unexpected result: %+v", result)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected one upstream request, got %d", calls)
	}
}

func TestAnthropicGenerateMapsErrors(t *testing.T) {
	tests := []struct {
		name string
		code int
		want error
	}{
		{name: "bad request", code: http.StatusBadRequest, want: ErrGeminiRequestRejected},
		{name: "unauthorized", code: http.StatusUnauthorized, want: ErrGeminiUnauthorized},
		{name: "rate limit", code: http.StatusTooManyRequests, want: ErrGeminiRateLimited},
		{name: "server unavailable", code: http.StatusServiceUnavailable, want: ErrAIUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tt.code) }))
			defer server.Close()
			client := NewAnthropicClient("test-key", "claude-haiku-4-5-20251001")
			client.endpoint = server.URL
			_, err := client.Generate(context.Background(), int64(tt.code), ProductAnalysisInput{Name: strings.Repeat("x", 2)})
			if err != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}
