package tnvedagent

import (
	"context"
	"strings"
	"testing"
)

type fakeSearcher struct {
	queries []string
	domains [][]string
}

func (f *fakeSearcher) Search(_ context.Context, query string, domains []string) ([]webResult, error) {
	f.queries = append(f.queries, query)
	f.domains = append(f.domains, domains)
	return []webResult{{Title: "Формочки для пластилина", URL: "https://detail.1688.com/offer/1.html", Content: "пластиковые формочки"}}, nil
}

func (f *fakeSearcher) Extract(context.Context, string) (string, error) {
	return "текст страницы", nil
}

func finishWith(sources []map[string]string) agentToolCall {
	return call("f", "finish", map[string]any{"description": "Перчатки", "name": "Перчатки", "confidence": "high", "sources": sources,
		"candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Да."}}})
}

// Интернет: поиск товара и решений таможни, источник принимается, только если агент видел страницу;
// первый итог уходит на самопроверку, принимается второй.
func TestAgentWebSearchSourcesAndReview(t *testing.T) {
	search := &fakeSearcher{}
	sources := []map[string]string{{"title": "карточка 1688", "url": "https://detail.1688.com/offer/1.html"}, {"title": "выдумка", "url": "https://example.com/fake"}}
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("w1", "web_search", map[string]string{"query": "动物模具", "focus": "product"}), call("w2", "web_search", map[string]string{"query": "классификация формочки", "focus": "customs"})}},
		{ToolCalls: []agentToolCall{finishWith(sources)}},
		{ToolCalls: []agentToolCall{finishWith(sources)}},
	}}
	runner, events := newTestRunner(model, &fakeAgentData{})
	runner.toolbox.search = search
	runner.review = true
	outcome := runner.execute(context.Background())
	if outcome.result == nil || model.calls != 3 {
		t.Fatalf("итог после самопроверки: %+v, вызовов модели %d", outcome.result, model.calls)
	}
	if outcome.result.Confidence != "high" || len(outcome.result.Sources) != 1 || outcome.result.Sources[0].URL != "https://detail.1688.com/offer/1.html" {
		t.Fatalf("уверенность и источники: %+v", outcome.result)
	}
	if search.domains[0] != nil || len(search.domains[1]) == 0 {
		t.Fatalf("product — везде, customs — сайты решений: %v", search.domains)
	}
	names := toolNames(model.lastTools)
	if !strings.Contains(names, "web_search") || !strings.Contains(names, "web_page") {
		t.Fatalf("с ключом у агента есть интернет: %s", names)
	}
	reviewed := false
	for _, event := range *events {
		if tool, ok := event.content.(agentToolEvent); ok && tool.Tool == "review" {
			reviewed = true
		}
	}
	if !reviewed {
		t.Fatal("самопроверка видна в ходе подбора")
	}
}

// Без ключа интернета у агента нет; лимит обращений на подбор.
func TestAgentWebLimitAndDisabled(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	if content, _ := toolbox.run(context.Background(), "web_search", `{"query":"x","focus":"product"}`); !strings.Contains(content, "не подключён") {
		t.Fatalf("без ключа: %s", content)
	}
	toolbox.search = &fakeSearcher{}
	for index := 0; index < agentMaxWebCalls; index++ {
		toolbox.run(context.Background(), "web_page", `{"url":"https://detail.1688.com/offer/1.html"}`)
	}
	if content, event := toolbox.run(context.Background(), "web_search", `{"query":"x","focus":"product"}`); !strings.Contains(content, "лимит") || !event.Failed {
		t.Fatalf("лимит: %s", content)
	}
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	if outcome := runner.execute(context.Background()); outcome.result == nil || strings.Contains(toolNames(model.lastTools), "web_search") {
		t.Fatal("без ключа и самопроверки — итог с первого раза, без интернета")
	}
}
