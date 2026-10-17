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
	test1688URL   = "https://detail.1688.com/offer/1.html"
)

// testMakerText — страница производителя на три части по webPageRunes.
func testMakerText() string {
	return strings.Repeat("а", webPageRunes) + strings.Repeat("б", webPageRunes) + "конец страницы"
}

type fakeSearcher struct {
	queries   []string
	domains   [][]string
	excluded  [][]string
	extracted []string
}

func (f *fakeSearcher) Search(_ context.Context, query string, include, exclude []string) ([]webResult, error) {
	f.queries = append(f.queries, query)
	f.domains = append(f.domains, include)
	f.excluded = append(f.excluded, exclude)
	return []webResult{
		{Title: "Формочки для пластилина", URL: test1688URL, Content: "пластиковые формочки"},
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

// Интернет: поиск товара и решений таможни, источник принимается, только если агент видел страницу;
// первый итог уходит на самопроверку, принимается второй. Изменено: в поиске товара маркетплейсы
// исключены (exclude_domains), а результат с 1688 остаётся допустимым источником — по краткому тексту.
func TestAgentWebSearchSourcesAndReview(t *testing.T) {
	search := &fakeSearcher{}
	sources := []map[string]string{{"title": "карточка 1688", "url": test1688URL}, {"title": "выдумка", "url": "https://example.com/fake"}}
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
	if outcome.result.Confidence != "high" || len(outcome.result.Sources) != 1 || outcome.result.Sources[0].URL != test1688URL {
		t.Fatalf("уверенность и источники: %+v", outcome.result)
	}
	if search.domains[0] != nil || len(search.domains[1]) == 0 {
		t.Fatalf("product — везде, customs — сайты решений: %v", search.domains)
	}
	if !strings.Contains(strings.Join(search.excluded[0], ","), "1688.com") || search.excluded[1] != nil {
		t.Fatalf("product — без маркетплейсов, customs — без исключений: %v", search.excluded)
	}
	names := toolNames(model.lastTools)
	for _, name := range []string{"web_search", "web_page", "search_in_document"} {
		if !strings.Contains(names, name) {
			t.Fatalf("с ключом у агента есть %s: %s", name, names)
		}
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

// Без ключа интернета у агента нет; лимит обращений к поиску на подбор. Изменено: лимит считает
// обращения к Tavily (поиск и запасное чтение), а не все открытые страницы.
func TestAgentWebLimitAndDisabled(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	if content, _ := toolbox.run(context.Background(), "web_search", `{"query":"x","focus":"product"}`); !strings.Contains(content, "не подключён") {
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
		toolbox.run(context.Background(), "web_search", `{"query":"x","focus":"product"}`)
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

// web_page: страница целиком по частям, ссылки — в первой части; повторное чтение частей не ходит на
// сайт и не тратит лимит; части за пределами — ошибка.
func TestWebPageReadsInParts(t *testing.T) {
	toolbox, data, _ := webToolbox()
	toolbox.run(context.Background(), "web_search", `{"query":"секатор","focus":"product"}`)
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
	toolbox.run(context.Background(), "web_search", `{"query":"секатор","focus":"product"}`)
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
	toolbox.run(context.Background(), "web_search", `{"query":"решение","focus":"customs"}`)
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

// В режиме «Документы» файл отдаётся сотруднику, Tavily не тратится.
func TestDocumentsModeDoesNotUseTavily(t *testing.T) {
	toolbox, _, search := webToolbox()
	toolbox.documentsCode = "6116102000"
	toolbox.links[testPDFURL] = true
	if content, _ := toolbox.run(context.Background(), "open_link", `{"url":"`+testPDFURL+`"}`); !strings.Contains(content, "Дай эту ссылку сотруднику") || len(search.extracted) != 0 {
		t.Fatalf("документы: %s, Tavily %v", content, search.extracted)
	}
}

// Маркетплейсы не открываются (с поддоменами), в результатах поиска — пометка.
func TestMarketplacesAreNotRead(t *testing.T) {
	toolbox, data, _ := webToolbox()
	content, _ := toolbox.run(context.Background(), "web_search", `{"query":"секатор","focus":"product"}`)
	if !strings.Contains(content, marketplaceMessage) {
		t.Fatalf("пометка у результата маркетплейса: %s", content)
	}
	for _, name := range []string{"web_page", "search_in_document"} {
		if content, event := toolbox.run(context.Background(), name, `{"url":"`+test1688URL+`","query":"формочки"}`); !strings.Contains(content, marketplaceMessage) || !event.Failed {
			t.Fatalf("%s 1688: %s", name, content)
		}
	}
	if len(data.webOpened) != 0 {
		t.Fatalf("маркетплейс не скачивается: %v", data.webOpened)
	}
	for address, want := range map[string]bool{
		"https://item.taobao.com/item.htm?id=1": true, "https://m.aliexpress.ru/item/1": true, "https://www.alibaba.com/x": true,
		"https://tmall.com/": true, "https://not1688.com/x": false, "https://alibaba.com.evil.org/x": false, "https://tks.ru/x": false,
	} {
		if marketplaceURL(address) != want {
			t.Errorf("%s: want %v", address, want)
		}
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
		call("s", "web_search", map[string]string{"query": "секатор", "focus": "product"}),
		call("p", "web_page", map[string]string{"url": testMakerURL}),
		call("q", "ask_user", map[string]string{"question": "Лезвия из стали?"}),
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
