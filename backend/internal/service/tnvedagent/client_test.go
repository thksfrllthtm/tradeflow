package tnvedagent

// Клиент модели — против локального httptest-сервера в формате OpenAI, настоящих вызовов нет.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/shopspring/decimal"
)

// fakeProvider — OpenAI-совместимый сервер: запоминает запросы и отвечает reply.
type fakeProvider struct {
	mu       sync.Mutex
	requests []map[string]any
	bodies   []string
	reply    map[string]any
}

func newFakeProvider(t *testing.T, reply map[string]any) (*fakeProvider, *httptest.Server) {
	provider := &fakeProvider{reply: reply}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		provider.mu.Lock()
		provider.requests = append(provider.requests, request)
		provider.bodies = append(provider.bodies, string(body))
		provider.mu.Unlock()
		_ = json.NewEncoder(w).Encode(provider.reply)
	}))
	t.Cleanup(server.Close)
	return provider, server
}

func textReply(content string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": content}}},
		"usage":   map[string]any{"prompt_tokens": 1000, "completion_tokens": 100, "prompt_cache_hit_tokens": 200},
	}
}

// testClient — клиент к локальному серверу. «/deepseek.com» в пути — чтобы клиент слал параметр thinking,
// как к настоящему DeepSeek.
func testClient(server *httptest.Server, model, effort string, vision bool) *AgentModelClient {
	return NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/deepseek.com", Model: model, Vision: vision, Effort: effort,
		PriceInput: decimal.RequireFromString("0.30"), PriceCached: decimal.RequireFromString("0.006"), PriceOutput: decimal.RequireFromString("1.20")})
}

// Режим размышлений: включается уровнем размышлений, без temperature и tool_choice; reasoning_content
// из ответа сохраняется и уходит модели в следующих запросах.
func TestClientThinkingModeAndReasoningContent(t *testing.T) {
	provider, server := newFakeProvider(t, map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": "", "reasoning_content": "Сначала проверю позицию 8201.",
			"tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "list_heading", "arguments": `{"prefix":"8201"}`}}}}}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
	})
	thinking := testClient(server, "deepseek-v4-pro", "high", false)
	reply, err := thinking.Complete(context.Background(), []agentMessage{
		{Role: "user", Content: "товар"},
		{Role: "assistant", Content: "", ReasoningContent: "прежние размышления", ToolCalls: []agentToolCall{call("c0", "get_codes", map[string]any{"codes": []string{}})}},
		{Role: "tool", ToolCallID: "c0", Content: "{}"},
	}, agentTools(false), "finish")
	if err != nil || reply.Reasoning != "Сначала проверю позицию 8201." || len(reply.ToolCalls) != 1 {
		t.Fatalf("ответ: %+v %v", reply, err)
	}
	request := provider.requests[0]
	if mode, _ := request["thinking"].(map[string]any); mode["type"] != "enabled" || request["reasoning_effort"] != "high" {
		t.Fatalf("размышления включены: %v", request["thinking"])
	}
	if _, ok := request["tool_choice"]; ok {
		t.Fatal("tool_choice в режиме размышлений не шлём")
	}
	if _, ok := request["temperature"]; ok {
		t.Fatal("temperature в режиме размышлений не шлём")
	}
	if !strings.Contains(provider.bodies[0], `"reasoning_content":"прежние размышления"`) {
		t.Fatal("размышления прежних шагов возвращаются модели")
	}

	plain := testClient(server, "deepseek-flash", "", true)
	if _, err := plain.Complete(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, agentTools(false), "finish"); err != nil {
		t.Fatal(err)
	}
	if mode, _ := provider.requests[1]["thinking"].(map[string]any); mode["type"] != "disabled" || provider.requests[1]["tool_choice"] == nil {
		t.Fatalf("без уровня размышлений — как раньше: %v", provider.requests[1])
	}
	if _, err := thinking.CompleteText(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, 100); err != nil {
		t.Fatal(err)
	}
	if mode, _ := provider.requests[2]["thinking"].(map[string]any); mode["type"] != "disabled" || provider.requests[2]["tools"] != nil {
		t.Fatalf("текстовый ответ — без размышлений и инструментов: %v", provider.requests[2])
	}
}

// Gemini 3 (OpenAI-совместимый адрес generativelanguage.googleapis.com): размышления не выключаются —
// вне цикла агента reasoning_effort low, max_tokens с запасом на размышления; параметр thinking DeepSeek не шлём.
func TestClientGeminiRequests(t *testing.T) {
	provider, server := newFakeProvider(t, textReply("кельма"))
	gemini := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/generativelanguage.googleapis.com", Model: "gemini-3.8-flash", Vision: true})
	if _, err := gemini.CompleteText(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, 100); err != nil {
		t.Fatal(err)
	}
	request := provider.requests[0]
	if request["reasoning_effort"] != "low" || request["max_tokens"] != float64(100+geminiThinkingHeadroom) || request["thinking"] != nil {
		t.Fatalf("запрос к Gemini: %v", request)
	}
	if _, err := gemini.CompleteJSON(context.Background(), []agentMessage{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if request := provider.requests[1]; request["reasoning_effort"] != "low" || request["max_tokens"] != float64(1024+geminiThinkingHeadroom) {
		t.Fatalf("JSON-запрос к Gemini: %v", request)
	}

	thinking := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/generativelanguage.googleapis.com", Model: "gemini-3.8-flash", Effort: "high"})
	if _, err := thinking.Complete(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, agentTools(false), "finish"); err != nil {
		t.Fatal(err)
	}
	if request := provider.requests[2]; request["reasoning_effort"] != "high" || request["max_tokens"] != float64(32768+geminiThinkingHeadroom) {
		t.Fatalf("цикл агента на Gemini сохраняет свой уровень размышлений: %v", request)
	}
}

// Эмбеддинги: запрос на /embeddings того же адреса с размерностью колонки; без модели — не настроено.
func TestClientEmbed(t *testing.T) {
	vector := make([]float64, embeddingDimensions)
	vector[0], vector[1] = 0.25, -0.5
	provider, server := newFakeProvider(t, map[string]any{"data": []map[string]any{{"embedding": vector}}})
	client := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/generativelanguage.googleapis.com", Model: "gemini-3.8-flash", EmbedModel: "gemini-embedding-001"})
	got, err := client.Embed(context.Background(), "кельма, мастерок")
	if err != nil || len(got) != embeddingDimensions || got[0] != 0.25 || got[1] != -0.5 {
		t.Fatalf("embedding: %v %v", len(got), err)
	}
	request := provider.requests[0]
	if request["model"] != "gemini-embedding-001" || request["input"] != "кельма, мастерок" || request["dimensions"] != float64(embeddingDimensions) {
		t.Fatalf("request: %v", request)
	}
	plain := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL, Model: "gemini-3.8-flash"})
	if plain.Embeds() {
		t.Fatal("no embed model — embeddings off")
	}
	if _, err := plain.Embed(context.Background(), "x"); !errors.Is(err, ai.ErrAINotConfigured) {
		t.Fatalf("err = %v", err)
	}
	short, shortServer := newFakeProvider(t, map[string]any{"data": []map[string]any{{"embedding": []float64{1, 2}}}})
	_ = short
	bad := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: shortServer.URL, Model: "m", EmbedModel: "e"})
	if _, err := bad.Embed(context.Background(), "x"); !errors.Is(err, ai.ErrAIBadResponse) {
		t.Fatalf("wrong dimensions must be a bad response: %v", err)
	}
}

// Сбой провайдера (503) и ограничение запросов DeepSeek (429) повторяются, и подбор не обрывается; ключ (401)
// — нет. У Gemini 429 — дневной лимит: не повторяется.
func TestClientRetriesProviderOutage(t *testing.T) {
	if retryable(ai.ErrAIRateLimited, "https://generativelanguage.googleapis.com/v1beta/openai") || !retryable(ai.ErrAIRateLimited, "https://api.deepseek.com") ||
		retryable(ai.ErrAITimeout, "https://api.deepseek.com") || !retryable(ai.ErrAIUnavailable, "https://generativelanguage.googleapis.com/v1beta/openai") {
		t.Fatal("retryable: 429 повторяется у DeepSeek, но не у Gemini; тайм-аут — никогда")
	}
	for _, test := range []struct {
		status   int
		calls    int
		expected error
	}{{http.StatusServiceUnavailable, 2, nil}, {http.StatusTooManyRequests, 2, nil}, {http.StatusUnauthorized, 1, ai.ErrAIUnauthorized}} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(test.status)
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		}))
		client := testClient(server, "model", "", false)
		client.retryDelay = 0
		reply, err := client.CompleteText(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, 10)
		server.Close()
		if calls != test.calls || !errors.Is(err, test.expected) || (test.expected == nil && reply.Content != "ok") {
			t.Errorf("status %d: calls %d, err %v, reply %q", test.status, calls, err, reply.Content)
		}
	}
}
