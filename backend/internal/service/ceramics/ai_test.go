package ceramics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

func TestParseCeramicsCandidatesRejectsUnknownTypes(t *testing.T) {
	allowed := []string{"Керамическая тарелка", "Керамическая тарелка салатница"}
	result, err := parseCeramicsCandidates("```json\n"+`{"candidates":[
		{"productType":"Керамическая миска","confidence":0.9,"reason":"x","sources":["image"]},
		{"productType":"керамическая  тарелка САЛАТНИЦА","confidence":"0.8","reason":"沙拉盘","sources":["chinese_name","image","bogus"]},
		{"productType":"Керамическая тарелка","confidence":55,"reason":"плоская"}
	]}`+"\n```", allowed, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].ProductType != "Керамическая тарелка салатница" || result.Candidates[1].Confidence != 0.55 {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if strings.Join(result.Candidates[0].Sources, ",") != "chinese_name" {
		t.Fatalf("image must be dropped when no photo was sent: %v", result.Candidates[0].Sources)
	}
	if _, err := parseCeramicsCandidates("not json", allowed, false); !errors.Is(err, ai.ErrAIBadResponse) {
		t.Fatalf("expected bad response, got %v", err)
	}
}

func TestParseCeramicsComparison(t *testing.T) {
	candidates := []string{"Керамическая тарелка", "Керамическая тарелка салатница"}
	result, err := parseCeramicsComparison(`{"decision":"MATCHED","productType":"Керамическая тарелка салатница","confidence":0.9,"reason":"沙拉盘"}`, candidates, true)
	if err != nil || !result.Matched || result.ProductType != "Керамическая тарелка салатница" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	for _, answer := range []string{
		`{"decision":"MATCHED","productType":"Керамическая миска","confidence":0.95}`,
		`{"decision":"MATCHED","productType":"Керамическая тарелка","confidence":0.4}`,
		`{"decision":"UNSURE","productType":"","confidence":0.5}`,
	} {
		result, err := parseCeramicsComparison(answer, candidates, true)
		if err != nil || result.Matched {
			t.Fatalf("%s must not match: %+v, %v", answer, result, err)
		}
	}
}

// fakeCeramicsAI answers stage A with the given candidates and stage B with the given decision.
func fakeCeramicsAI(t *testing.T, stageA, stageB string) (*CeramicsAIClient, *[]string) {
	t.Helper()
	prompts := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		system, _ := request.Messages[0].Content.(string)
		prompts = append(prompts, system)
		content := stageA
		if strings.Contains(system, "из нескольких похожих") {
			content = stageB
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(server.Close)
	return NewCeramicsAIClient(CeramicsAIConfig{APIKey: "test", Model: "test", BaseURL: server.URL}), &prompts
}

// Gemini 3: thinking cannot be disabled, so the request asks for minimal reasoning and leaves room for it.
func TestCeramicsAIGeminiRequest(t *testing.T) {
	var request chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "{}"}}}})
	}))
	t.Cleanup(server.Close)
	client := NewCeramicsAIClient(CeramicsAIConfig{APIKey: "test", Model: "gemini-3.8-flash", BaseURL: server.URL + "/generativelanguage.googleapis.com"})
	if _, err := client.complete(context.Background(), "system", map[string]any{}, ""); err != nil {
		t.Fatal(err)
	}
	if request.ReasoningEffort != "low" || request.MaxTokens != ceramicsAIMaxTokens+geminiThinkingHeadroom || request.Thinking != nil {
		t.Fatalf("request = %+v", request)
	}
}
