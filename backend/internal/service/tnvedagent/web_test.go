package tnvedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const (
	testMakerURL  = "https://maker.example.com/pruner"
	testSpecURL   = "https://maker.example.com/pruner/spec"
	testScriptURL = "https://spa.example.com/decision/1"
	testAltaURL   = "https://www.alta.ru/tamdoc/decision/1/"
)

// testMakerText — страница производителя на три части по webPageRunes.
func testMakerText() string {
	return strings.Repeat("а", webPageRunes) + strings.Repeat("б", webPageRunes) + "конец страницы"
}

type fakeSearcher struct {
	queries   []string
	domains   [][]string
	extracted []string
	// emptyMain — на tks.ru ничего не находится.
	emptyMain bool
}

func (f *fakeSearcher) Search(_ context.Context, query string, domains []string) ([]webResult, error) {
	f.queries = append(f.queries, query)
	f.domains = append(f.domains, domains)
	if f.emptyMain && strings.Join(domains, ",") == "tks.ru" {
		return nil, nil
	}
	return []webResult{
		{Title: "Решение по формочкам для пластилина", URL: testAltaURL, Content: "пластиковые формочки"},
		{Title: "Ножницы садовые", URL: testMakerURL, Content: "секатор с пружиной"},
		{Title: "Решение", URL: testScriptURL, Content: "предварительное решение"},
	}, nil
}

func (f *fakeSearcher) Extract(_ context.Context, url string) (string, error) {
	f.extracted = append(f.extracted, url)
	return "текст через Tavily: " + url, nil
}

func finishWith(sources []map[string]string) agentToolCall {
	return call("f", "finish", map[string]any{"description": "Перчатки", "name": "Перчатки", "confidence": "high", "sources": sources,
		"candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Да."}}})
}

func webToolbox() (*agentToolbox, *fakeAgentData, *fakeSearcher) {
	data, search := &fakeAgentData{}, &fakeSearcher{}
	toolbox := newAgentToolbox(data)
	toolbox.search = search
	return toolbox, data, search
}

func decode(t *testing.T, content string) map[string]any {
	t.Helper()
	var answer map[string]any
	if err := json.Unmarshal([]byte(content), &answer); err != nil {
		t.Fatalf("not JSON: %s", content)
	}
	return answer
}

// Интернет: поиск решений таможни, источник принимается, только если агент видел страницу; итог
// принимается с первого finish. Изменено: поиска «что за товар» (focus product) нет, web_search ищет
// только по сайтам решений.
func TestAgentWebSearchSources(t *testing.T) {
	search := &fakeSearcher{}
	sources := []map[string]string{{"title": "решение alta.ru", "url": testAltaURL}, {"title": "выдумка", "url": "https://example.com/fake"}}
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("w1", "web_search", map[string]string{"query": "классификация формочки"})}},
		{ToolCalls: []agentToolCall{finishWith(sources)}},
	}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.toolbox.search = search
	outcome := runner.execute(context.Background())
	if outcome.result == nil || model.calls != 2 {
		t.Fatalf("итог с первого finish: %+v, вызовов модели %d", outcome.result, model.calls)
	}
	if outcome.result.Confidence != "high" || len(outcome.result.Sources) != 1 || outcome.result.Sources[0].URL != testAltaURL {
		t.Fatalf("уверенность и источники: %+v", outcome.result)
	}
	if len(search.queries) != 1 || search.queries[0] != "классификация формочки" {
		t.Fatalf("запрос к поиску: %v", search.queries)
	}
	names := toolNames(model.lastTools)
	for _, name := range []string{"web_search", "web_page", "search_in_document"} {
		if !strings.Contains(names, name) {
			t.Fatalf("с ключом у агента есть %s: %s", name, names)
		}
	}
}

// tks.ru — главный сайт: поиск сначала только по нему; другие сайты — вторым обращением, только если на
// tks.ru пусто, и не сверх лимита.
func TestWebSearchTKSFirst(t *testing.T) {
	toolbox, _, search := webToolbox()
	_, event := toolbox.run(context.Background(), "web_search", `{"query":"формочки"}`)
	if len(search.domains) != 1 || strings.Join(search.domains[0], ",") != "tks.ru" || toolbox.webCalls != 1 || !strings.HasPrefix(event.Label, "tks.ru") {
		t.Fatalf("нашлось на tks.ru — других сайтов нет: %v, обращений %d, %q", search.domains, toolbox.webCalls, event.Label)
	}

	toolbox, _, search = webToolbox()
	search.emptyMain = true
	content, event := toolbox.run(context.Background(), "web_search", `{"query":"формочки"}`)
	if len(search.domains) != 2 || strings.Contains(strings.Join(search.domains[1], ","), "tks.ru") || toolbox.webCalls != 2 || !strings.Contains(content, testAltaURL) || !strings.Contains(event.Label, "другие сайты") {
		t.Fatalf("на tks.ru пусто — другие сайты: %v, обращений %d, %q", search.domains, toolbox.webCalls, event.Label)
	}

	toolbox, _, search = webToolbox()
	search.emptyMain = true
	toolbox.webCalls = agentMaxWebCalls - 1
	toolbox.run(context.Background(), "web_search", `{"query":"формочки"}`)
	if len(search.domains) != 1 || toolbox.webCalls != agentMaxWebCalls {
		t.Fatalf("на последнем обращении — только tks.ru: %v, обращений %d", search.domains, toolbox.webCalls)
	}
}

// Без ключа интернета у агента нет; лимит обращений к поиску на подбор. Изменено: лимит считает
// обращения к Tavily (поиск и запасное чтение), а не все открытые страницы.
func TestAgentWebLimitAndDisabled(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	if content, _ := toolbox.run(context.Background(), "web_search", `{"query":"x"}`); !strings.Contains(content, "не подключён") {
		t.Fatalf("без ключа: %s", content)
	}
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"https://tnved.info/x"}`); !strings.Contains(content, "Нет такого инструмента") {
		t.Fatalf("без ключа web_page нет: %s", content)
	}
	if content, _ := toolbox.run(context.Background(), "search_in_document", `{"url":"https://tnved.info/x","query":"x"}`); !strings.Contains(content, "Нет такого инструмента") {
		t.Fatalf("без ключа search_in_document в подборе нет: %s", content)
	}
	toolbox.search = &fakeSearcher{}
	for index := 0; index < agentMaxWebCalls; index++ {
		toolbox.run(context.Background(), "web_search", `{"query":"x"}`)
	}
	if content, event := toolbox.run(context.Background(), "web_search", `{"query":"x"}`); !strings.Contains(content, "лимит") || !event.Failed {
		t.Fatalf("лимит: %s", content)
	}
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	if outcome := runner.execute(context.Background()); outcome.result == nil || strings.Contains(toolNames(model.lastTools), "web_search") {
		t.Fatal("без ключа — итог с первого раза, без интернета")
	}
}

// web_page: страница целиком по частям, ссылки — в первой части; повторное чтение частей не ходит на
// сайт и не тратит лимит; части за пределами — ошибка.
func TestWebPageReadsInParts(t *testing.T) {
	toolbox, data, _ := webToolbox()
	toolbox.run(context.Background(), "web_search", `{"query":"секатор"}`)
	first := decode(t, mustContent(toolbox.run(context.Background(), "web_page", `{"url":"`+testMakerURL+`"}`)))
	if first["part"] != float64(1) || first["parts"] != float64(3) || first["links"] == nil || len([]rune(first["text"].(string))) != webPageRunes {
		t.Fatalf("первая часть: part %v parts %v links %v", first["part"], first["parts"], first["links"])
	}
	second := decode(t, mustContent(toolbox.run(context.Background(), "web_page", `{"url":"`+testMakerURL+`","part":2}`)))
	if second["links"] != nil || !strings.HasPrefix(second["text"].(string), "б") {
		t.Fatalf("вторая часть: %v", second)
	}
	third, event := toolbox.run(context.Background(), "web_page", `{"url":"`+testMakerURL+`","part":3}`)
	if !strings.Contains(third, "конец страницы") || strings.Contains(third, "hint") || !strings.Contains(event.Detail, "часть 3 из 3") {
		t.Fatalf("последняя часть: %s %+v", third, event)
	}
	if content, event := toolbox.run(context.Background(), "web_page", `{"url":"`+testMakerURL+`","part":4}`); !strings.Contains(content, "всего 3") || !event.Failed {
		t.Fatalf("нет такой части: %s", content)
	}
	if len(data.webOpened) != 1 || len(toolbox.pages) != 1 || toolbox.webCalls != 1 {
		t.Fatalf("части — из памяти запуска: сайт %v, страниц %d, обращений к Tavily %d", data.webOpened, len(toolbox.pages), toolbox.webCalls)
	}
}

// Открыть можно только адреса из поиска, ссылки с прочитанных страниц и tnved.info, tws.by.
func TestWebPageOnlyAllowedAddresses(t *testing.T) {
	toolbox, data, _ := webToolbox()
	if content, event := toolbox.run(context.Background(), "web_page", `{"url":"https://random.example.org/x"}`); !strings.Contains(content, "Можно открывать только") || !event.Failed {
		t.Fatalf("произвольный адрес: %s", content)
	}
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"`+testSpecURL+`"}`); !strings.Contains(content, "Можно открывать только") {
		t.Fatalf("ссылка с непрочитанной страницы: %s", content)
	}
	toolbox.run(context.Background(), "web_search", `{"query":"секатор"}`)
	toolbox.run(context.Background(), "web_page", `{"url":"`+testMakerURL+`"}`)
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"`+testSpecURL+`"}`); strings.Contains(content, "error") {
		t.Fatalf("ссылка с прочитанной страницы: %s", content)
	}
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"https://ok.example.com/x"}`); !strings.Contains(content, "Можно открывать только") {
		t.Fatalf("адрес не из поиска: %s", content)
	}
	if len(data.webOpened) != 2 {
		t.Fatalf("на сайт — только разрешённые адреса: %v", data.webOpened)
	}
}

// Сайт не отдаёт страницу или она строится скриптом, PDF — через Tavily (обращение считается);
// лимит Tavily; лимит разных страниц, а части уже открытых страниц — после лимита тоже.
func TestWebPageTavilyFallbackAndLimits(t *testing.T) {
	toolbox, _, search := webToolbox()
	toolbox.run(context.Background(), "web_search", `{"query":"решение"}`)
	content, event := toolbox.run(context.Background(), "web_page", `{"url":"`+testScriptURL+`"}`)
	if !strings.Contains(content, "текст через Tavily") || !strings.Contains(event.Detail, "через Tavily") || toolbox.webCalls != 2 {
		t.Fatalf("SPA через Tavily: %s %+v обращений %d", content, event, toolbox.webCalls)
	}
	toolbox.links[testPDFURL] = true
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"`+testPDFURL+`"}`); !strings.Contains(content, "текст через Tavily") {
		t.Fatalf("PDF через Tavily: %s", content)
	}
	toolbox.run(context.Background(), "web_page", `{"url":"`+testScriptURL+`","part":1}`)
	if len(search.extracted) != 2 {
		t.Fatalf("повторное чтение не обращается к Tavily: %v", search.extracted)
	}
	toolbox.webCalls = agentMaxWebCalls
	toolbox.links["https://spa.example.com/other"] = true
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"https://spa.example.com/other"}`); !strings.Contains(content, "лимит обращений к Tavily") {
		t.Fatalf("лимит Tavily: %s", content)
	}

	toolbox, _, _ = webToolbox()
	for index := 0; index <= agentMaxWebPages; index++ {
		toolbox.links["https://ok.example.com/"+string(rune('a'+index))] = true
	}
	for index := 0; index < agentMaxWebPages; index++ {
		if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"https://ok.example.com/`+string(rune('a'+index))+`"}`); strings.Contains(content, "error") {
			t.Fatalf("страница %d: %s", index, content)
		}
	}
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"https://ok.example.com/`+string(rune('a'+agentMaxWebPages))+`"}`); !strings.Contains(content, "Лимит страниц") {
		t.Fatalf("лимит страниц: %s", content)
	}
	if content, _ := toolbox.run(context.Background(), "web_page", `{"url":"https://ok.example.com/a"}`); strings.Contains(content, "error") {
		t.Fatalf("уже открытая страница после лимита: %s", content)
	}
}

// В режиме «Документы» PDF решения ЕЭК читается через Tavily (текст сохраняется в базе).
func TestDocumentsModeReadsPDF(t *testing.T) {
	toolbox, _, search := webToolbox()
	toolbox.documentsCode = "6116102000"
	toolbox.links[testPDFURL] = true
	if content, _ := toolbox.run(context.Background(), "open_link", `{"url":"`+testPDFURL+`"}`); !strings.Contains(content, "текст через Tavily") || len(search.extracted) != 1 {
		t.Fatalf("документы: %s, Tavily %v", content, search.extracted)
	}
}

// search_in_document в обычном подборе (с интернетом): нужное место длинной страницы; текст — не
// источник цитат «справки».
func TestSearchInDocumentInNormalMode(t *testing.T) {
	toolbox, _, _ := webToolbox()
	toolbox.links[testActURL] = true
	content, _ := toolbox.run(context.Background(), "search_in_document", `{"url":"`+testActURL+`","query":"перчатки трикотажные"}`)
	if !strings.Contains(content, "перчатки трикотажные с покрытием") || len(toolbox.web) != 0 || !toolbox.links["https://adilet.zan.kz/rus/docs/H10T0000317"] {
		t.Fatalf("поиск по странице: %s, web %v", content, toolbox.web)
	}
}

// После ask_user второй запуск восстанавливает ссылки со страниц web_page.
func TestRestoreOpenedWebPageLinks(t *testing.T) {
	data := &fakeAgentData{}
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("s", "web_search", map[string]string{"query": "секатор"}),
		call("p", "web_page", map[string]string{"url": testMakerURL}),
		ask("q", "Лезвия из стали?"),
	}}}}
	runner, _ := newTestRunner(model, data)
	runner.toolbox.search = &fakeSearcher{}
	if outcome := runner.execute(context.Background()); outcome.status != "waiting" {
		t.Fatalf("первый запуск: %+v", outcome)
	}
	raw, _ := json.Marshal(runner.transcript)
	var restored []map[string]any
	_ = json.Unmarshal(raw, &restored)
	second, _ := newTestRunner(&scriptedModel{}, data)
	second.toolbox.search = &fakeSearcher{}
	second.transcript = restored
	second.restoreOpened(context.Background())
	if !second.toolbox.links[testSpecURL] || !second.toolbox.links[testMakerURL] {
		t.Fatalf("ссылки со страницы после ответа сотрудника: %v", second.toolbox.links)
	}
}

func mustContent(content string, _ agentToolEvent) string { return content }
