package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func geminiMockServer(t *testing.T, status int, result any) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("API key header was not sent")
		}
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		} else if config, ok := requestBody["generationConfig"].(map[string]any); !ok || !hasStructuredJSONConfig(config) {
			t.Errorf("structured JSON response was not requested")
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		payload, _ := json.Marshal(result)
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]string{"text": string(payload)}}}}}})
	}))
	count := &calls
	return server, count
}

func hasStructuredJSONConfig(config map[string]any) bool {
	format, ok := config["responseFormat"].(map[string]any)
	if !ok {
		return false
	}
	textConfig, ok := format["text"].(map[string]any)
	return ok && textConfig["mimeType"] == "APPLICATION_JSON" && textConfig["schema"] != nil
}

func TestGeminiGenerateValidatesStructuredResult(t *testing.T) {
	server, _ := geminiMockServer(t, http.StatusOK, AIClassificationResult{
		PrimaryCode: "0101210000", PositionName: "Demo product", Explanation: "Based on supplied fields", Alternatives: []AIAlternative{}, MissingCharacteristics: []string{}, Questions: []string{}, Status: statusProposed,
	})
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	result, err := client.Generate(context.Background(), 1, ProductAnalysisInput{Name: "steel pan"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.PrimaryCode != "0101210000" || result.Status != statusProposed {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGeminiGenerateRejectsInvalidCode(t *testing.T) {
	server, _ := geminiMockServer(t, http.StatusOK, AIClassificationResult{
		PrimaryCode: "123", PositionName: "x", Explanation: "x", Alternatives: []AIAlternative{}, MissingCharacteristics: []string{}, Questions: []string{}, Status: statusProposed,
	})
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	_, err := client.Generate(context.Background(), 2, ProductAnalysisInput{Name: "x"})
	if err != ErrGeminiBadResponse {
		t.Fatalf("expected ErrGeminiBadResponse, got %v", err)
	}
}

func TestGeminiGenerateRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]string{"text": "not-json"}}}}}})
	}))
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	_, err := client.Generate(context.Background(), 7, ProductAnalysisInput{Name: "x"})
	if !errors.Is(err, ErrGeminiBadResponse) {
		t.Fatalf("expected ErrGeminiBadResponse, got %v", err)
	}
}

func TestGeminiGenerateConcatenatesAnswerPartsAndAcceptsFence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := `{"primaryCode":"0101210000","positionName":"Demo product","explanation":"Based on supplied fields","alternatives":[],"missingCharacteristics":[],"questions":[],"status":"` + statusProposed + `"}`
		cut := len(payload) / 2
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{
			map[string]any{"text": "```json\n" + payload[:cut]},
			map[string]any{"text": payload[cut:] + "\n```"},
		}}}}})
	}))
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	result, err := client.Generate(context.Background(), 8, ProductAnalysisInput{Name: "x"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.PrimaryCode != "0101210000" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGeminiGenerateMissingKey(t *testing.T) {
	client := NewGeminiClient("", "gemini-3.8-flash")
	_, err := client.Generate(context.Background(), 1, ProductAnalysisInput{Name: "x"})
	if err != ErrGeminiNotConfigured {
		t.Fatalf("expected ErrGeminiNotConfigured, got %v", err)
	}
}

func TestGeminiGenerateDoesNotRetryRateLimit(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	if _, err := client.Generate(context.Background(), 3, ProductAnalysisInput{Name: strings.Repeat("x", 4)}); !errors.Is(err, ErrGeminiRateLimited) {
		t.Fatalf("expected rate-limit error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected exactly one upstream request, got %d", calls)
	}
}

func TestGeminiGenerateMapsProviderRejections(t *testing.T) {
	tests := []struct {
		name string
		code int
		want error
	}{
		{name: "bad request", code: http.StatusBadRequest, want: ErrGeminiRequestRejected},
		{name: "model missing", code: http.StatusNotFound, want: ErrGeminiModelUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, calls := geminiMockServer(t, tt.code, nil)
			defer server.Close()
			client := NewGeminiClient("test-key", "gemini-3.8-flash")
			client.endpoint = server.URL
			_, err := client.Generate(context.Background(), int64(tt.code), ProductAnalysisInput{Name: "x"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
			if atomic.LoadInt32(calls) != 1 {
				t.Fatalf("expected one upstream request, got %d", *calls)
			}
		})
	}
}

func TestGeminiGenerateUnauthorized(t *testing.T) {
	server, _ := geminiMockServer(t, http.StatusUnauthorized, nil)
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	_, err := client.Generate(context.Background(), 5, ProductAnalysisInput{Name: "x"})
	if !errors.Is(err, ErrGeminiUnauthorized) {
		t.Fatalf("expected ErrGeminiUnauthorized, got %v", err)
	}
}

type deadlineRoundTripper struct{}

func (deadlineRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.DeadlineExceeded
}

func TestGeminiGenerateTimeout(t *testing.T) {
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.httpClient = &http.Client{Transport: deadlineRoundTripper{}}
	_, err := client.Generate(context.Background(), 6, ProductAnalysisInput{Name: "x"})
	if !errors.Is(err, ErrGeminiTimeout) {
		t.Fatalf("expected ErrGeminiTimeout, got %v", err)
	}
}

func TestGeminiGenerateRejectsOversizedInput(t *testing.T) {
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	_, err := client.Generate(context.Background(), 4, ProductAnalysisInput{Name: strings.Repeat("x", maxAIInputBytes)})
	if err != ErrAIRequestTooLarge {
		t.Fatalf("expected ErrAIRequestTooLarge, got %v", err)
	}
}

func TestGeminiGenerateChatValidatesResponse(t *testing.T) {
	server, calls := geminiMockServer(t, http.StatusOK, AIChatResponse{
		Message:         "Уточните материал изделия.",
		Questions:       []string{"Из какого материала изготовлен товар?"},
		ProposedChanges: []AIProposedChange{},
		Candidates:      []AIChatCandidate{{Code: "7323930000", PositionName: "Изделия из стали", Explanation: "Предварительный вариант"}},
		NeedsRefresh:    false,
	})
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	result, err := client.GenerateChat(context.Background(), 101, ProductChatContext{Product: ProductAnalysisInput{Name: "кастрюля"}}, "Уточните код")
	if err != nil {
		t.Fatalf("GenerateChat() error = %v", err)
	}
	if result.Candidates[0].Code != "7323930000" || len(result.Questions) != 1 {
		t.Fatalf("unexpected chat result: %+v", result)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("expected one upstream request, got %d", *calls)
	}
}

func TestGeminiGenerateChatRejectsInvalidCandidate(t *testing.T) {
	server, _ := geminiMockServer(t, http.StatusOK, AIChatResponse{
		Message:         "Ответ",
		Questions:       []string{},
		ProposedChanges: []AIProposedChange{},
		Candidates:      []AIChatCandidate{{Code: "123", PositionName: "", Explanation: ""}},
		NeedsRefresh:    false,
	})
	defer server.Close()
	client := NewGeminiClient("test-key", "gemini-3.8-flash")
	client.endpoint = server.URL
	_, err := client.GenerateChat(context.Background(), 102, ProductChatContext{Product: ProductAnalysisInput{Name: "товар"}}, "Вопрос")
	if !errors.Is(err, ErrGeminiBadResponse) {
		t.Fatalf("expected ErrGeminiBadResponse, got %v", err)
	}
}
