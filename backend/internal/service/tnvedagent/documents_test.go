package tnvedagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

const (
	testHelpURL = "https://tnved.info/Goodshelp/?Code=6116102000&date=05.10.2026&dir=1"
	testActURL  = "https://adilet.zan.kz/rus/docs/H10T0000299"
	testPDFURL  = "https://docs.eaeunion.org/upload/iblock/a/reshenie.pdf"
)

func (d *fakeAgentData) GoodsHelp(_ context.Context, code string) (directory.WebPage, error) {
	if d.helpLimit {
		return directory.WebPage{URL: testHelpURL}, directory.ErrGoodsHelpLimit
	}
	return directory.WebPage{
		URL:   testHelpURL,
		Title: "Справка по товару " + FormatTNVEDCode(code),
		Text: "Меры нетарифного регулирования (ограничения):\n1. Решение Комиссии Таможенного Союза №299 от 28.05.2010 — " + testActURL +
			"\n   Мера: Входит в перечень продукции, подлежащей государственному санитарно-эпидемиологическому надзору." +
			"\n   Относится к товарам: только изделия, контактирующие с пищевыми продуктами" +
			"\n2. Решение Коллегии ЕЭК №79 от 13.06.2012 — " + testPDFURL + "\n   Мера: Требуется декларация соответствия ТР ТС 019/2011.",
		Links: []directory.WebLink{{URL: testActURL, Text: "Решение КТС №299"}, {URL: testPDFURL, Text: "Решение Коллегии ЕЭК №79"}},
	}, nil
}

// testActText — «огромный» акт: тысячи строк перечня, нужная — одна.
func testActText() string {
	lines := []string{"О применении санитарных мер в таможенном союзе"}
	for index := 0; index < 3000; index++ {
		lines = append(lines, fmt.Sprintf("%d. Прочая продукция раздела %d (из следующих групп ТН ВЭД ЕАЭС: 01, 02, 03)", index, index))
	}
	lines[1500] = "Раздел 3. Материалы и изделия, контактирующие с пищевыми продуктами: перчатки трикотажные с покрытием (из групп 39, 40, 61)"
	return strings.Join(lines, "\n")
}

func (d *fakeAgentData) WebPage(_ context.Context, url string) (directory.WebPage, error) {
	d.webOpened = append(d.webOpened, url)
	switch url {
	case testEECCardURL:
		return directory.WebPage{URL: url, Text: "Информация о документе\nКороткий заголовок документа\nРешение Коллегии ЕЭК № 137\nДата принятия документа\n11.10.2021\npdf"}, nil
	case testEECAdiletURL:
		return directory.WebPage{URL: url, Title: "О внесении изменений в Решение Коллегии ЕЭК от 21 апреля 2015 г. № 30", Text: "Список F\nCH2FCF3\nГФУ-134а\nиз 2903 45 000 0"}, nil
	case testGeneralActURL:
		text, err := os.ReadFile("testdata/act_019.txt")
		return directory.WebPage{URL: url, Text: string(text)}, err
	case testActURL:
		return directory.WebPage{URL: url, Title: "О применении санитарных мер", Text: testActText(), Links: []directory.WebLink{{URL: "https://adilet.zan.kz/rus/docs/H10T0000317", Text: "Решение №317"}}}, nil
	case testPDFURL:
		return directory.WebPage{URL: url}, directory.ErrWebPageFile
	case testMakerURL:
		return directory.WebPage{URL: url, Title: "Ножницы садовые", Text: testMakerText(), Links: []directory.WebLink{{URL: testSpecURL, Text: "Характеристики"}}}, nil
	case testScriptURL:
		return directory.WebPage{URL: url}, directory.ErrWebPageScript
	}
	if strings.HasPrefix(url, "https://ok.example.com/") {
		return directory.WebPage{URL: url, Text: "Страница " + url}, nil
	}
	return directory.WebPage{}, directory.ErrWebPageUnavailable
}

// ExtractedPage — запасной путь через Tavily: в тестах без базы, extract вызывается всегда.
func (d *fakeAgentData) ExtractedPage(ctx context.Context, url string, extract func(context.Context, string) (string, error)) (directory.WebPage, error) {
	text, err := extract(ctx, url)
	if err != nil {
		return directory.WebPage{URL: url}, err
	}
	return directory.WebPage{URL: url, Text: text}, nil
}

func toolNames(tools []agentTool) string {
	names := []string{}
	for _, tool := range tools {
		names = append(names, tool.Function.Name)
	}
	return strings.Join(names, ",")
}

var documentsToolNames = []string{"open_goods_help", "open_link", "search_in_document"}

// Без кнопки — всё как раньше: веб-инструментов у модели нет, подсказка и лимит шагов прежние.
func TestNormalModeHasNoDocumentsTools(t *testing.T) {
	for _, reference := range []bool{false, true} {
		names := toolNames(agentTools(reference, false))
		for _, name := range documentsToolNames {
			if strings.Contains(names, name) {
				t.Errorf("reference=%v: %s in normal tools %s", reference, name, names)
			}
		}
	}
	for _, prompt := range []string{tnvedAgentPrompt(""), tnvedReferencePrompt("")} {
		if strings.Contains(prompt, "open_goods_help") || strings.Contains(prompt, "Документы для ввоза") {
			t.Error("normal prompt mentions documents mode")
		}
	}
	data := &fakeAgentData{}
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("w1", "open_link", map[string]string{"url": "https://www.tws.by/tws/repository/65"}),
		call("f", "finish", map[string]any{"description": "Перчатки", "name": "Перчатки", "candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Да."}}}),
	}}}}
	runner, _ := newTestRunner(model, data)
	if runner.stepLimit() != agentMaxSteps || agentMaxSteps != 12 {
		t.Fatalf("step limit %d", runner.stepLimit())
	}
	outcome := runner.execute(context.Background())
	if outcome.result == nil || outcome.result.DocumentsCode != "" || outcome.result.Documents != nil {
		t.Fatalf("outcome %+v", outcome.result)
	}
	for _, name := range documentsToolNames {
		if strings.Contains(toolNames(model.lastTools), name) {
			t.Errorf("model got %s in normal mode", name)
		}
	}
	// Даже если модель вызовет веб-инструмент, в обычном режиме он не выполняется.
	if len(data.webOpened) != 0 {
		t.Fatalf("web page opened in normal mode: %v", data.webOpened)
	}
}

func newDocumentsRunner(model *scriptedModel, data *fakeAgentData) (*agentRunner, *[]emitted) {
	runner, events := newTestRunner(model, data)
	runner.documents = true
	runner.toolbox.documentsCode = "6116102000"
	runner.system += "\n\n" + tnvedDocumentsPrompt
	return runner, events
}

func TestDocumentsModeToolsAndSteps(t *testing.T) {
	names := toolNames(agentTools(true, true))
	for _, name := range documentsToolNames {
		if !strings.Contains(names, name) {
			t.Errorf("%s missing in documents tools %s", name, names)
		}
	}
	for _, tool := range agentTools(false, true) {
		if tool.Function.Name == "finish" {
			data, _ := json.Marshal(tool.Function.Parameters)
			if !strings.Contains(string(data), `"documents"`) || !strings.Contains(string(data), `"applies"`) {
				t.Fatalf("documents finish schema: %s", data)
			}
		}
	}
	runner, _ := newDocumentsRunner(&scriptedModel{}, &fakeAgentData{})
	if runner.stepLimit() != agentDocumentsMaxSteps || agentDocumentsMaxSteps <= agentMaxSteps {
		t.Fatalf("documents step limit %d", runner.stepLimit())
	}
}

func toolAnswer(t *testing.T, runner *agentRunner, id string) map[string]any {
	t.Helper()
	for _, message := range runner.transcript {
		if message["role"] == "tool" && message["tool_call_id"] == id {
			var answer map[string]any
			if err := json.Unmarshal([]byte(message["content"].(string)), &answer); err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			return answer
		}
	}
	t.Fatalf("no answer for %s", id)
	return nil
}

func TestDocumentsRunReadsHelpAndActs(t *testing.T) {
	data := &fakeAgentData{}
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("h", "open_goods_help", map[string]string{"code": "6116102000"})}},
		{ToolCalls: []agentToolCall{
			call("l1", "open_link", map[string]string{"url": "http://adilet.zan.kz/rus/docs/H10T0000299#z5"}),
			call("s1", "search_in_document", map[string]string{"url": testActURL, "query": "перчатки контактирующие пищевыми"}),
			call("l2", "open_link", map[string]string{"url": "https://evil.example.com/steal"}),
			call("l3", "open_link", map[string]string{"url": testPDFURL}),
		}},
		{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
			"description": "Нужен санитарно-эпидемиологический надзор, если перчатки контактируют с пищей.",
			"documents": []map[string]string{
				{"measure": "Санитарно-эпидемиологический надзор", "act": "Решение КТС №299 от 28.05.2010", "url": testActURL, "requirement": "Только для изделий, контактирующих с пищей.", "applies": "maybe"},
				{"measure": "Декларация соответствия ТР ТС 019/2011", "act": "Решение Коллегии ЕЭК №79", "url": testPDFURL, "requirement": "Декларация о соответствии.", "applies": "yes", "basis": "Требуется декларация соответствия ТР ТС 019/2011"},
				{"measure": "Выдуманная мера", "act": "Акт", "url": "https://evil.example.com/act", "requirement": "—", "applies": "странно"},
				{"kind": "conditional", "measure": "Условный выпуск", "act": "Правила РК об условно выпущенных товарах (2018)", "url": conditionalReleaseURL, "requirement": "ДС представляется после выпуска в срок по Правилам.", "applies": "maybe"},
			},
		})}},
	}}
	runner, events := newDocumentsRunner(model, data)
	outcome := runner.execute(context.Background())
	if outcome.err != nil || outcome.result == nil {
		t.Fatalf("outcome %+v", outcome)
	}
	for _, name := range documentsToolNames {
		if !strings.Contains(toolNames(model.lastTools), name) {
			t.Errorf("model has no %s in documents mode", name)
		}
	}
	help := toolAnswer(t, runner, "h")
	if help["url"] != testHelpURL || !strings.Contains(help["text"].(string), "контактирующие с пищевыми продуктами") {
		t.Fatalf("help answer: %v", help)
	}
	// Большой документ: только начало и подсказка искать.
	link := toolAnswer(t, runner, "l1")
	if text := link["text"].(string); len([]rune(text)) > agentPageRunes || link["hint"] == nil || link["url"] != testActURL {
		t.Fatalf("open_link answer: url=%v hint=%v len=%d", link["url"], link["hint"], len([]rune(text)))
	}
	// Поиск — фрагменты с нужной строкой, а не весь документ.
	search := toolAnswer(t, runner, "s1")
	fragments, _ := search["fragments"].([]any)
	data2, _ := json.Marshal(search)
	if len(fragments) == 0 || len(fragments) > agentSearchFragments || len(data2) > 12000 || !strings.Contains(fragments[0].(string)+fragments[len(fragments)-1].(string), "перчатки трикотажные с покрытием") {
		t.Fatalf("search answer (%d bytes): %v", len(data2), search["fragments"])
	}
	// Ссылки не с открытых страниц не открываются, PDF — «дай ссылку».
	if evil := toolAnswer(t, runner, "l2"); evil["error"] == nil {
		t.Fatalf("evil link opened: %v", evil)
	}
	if pdf := toolAnswer(t, runner, "l3"); !strings.Contains(fmt.Sprint(pdf["error"]), "Дай эту ссылку сотруднику") {
		t.Fatalf("pdf answer: %v", pdf)
	}
	for _, opened := range data.webOpened {
		if strings.Contains(opened, "evil") {
			t.Fatalf("evil url fetched: %v", data.webOpened)
		}
	}
	result := outcome.result
	if result.DocumentsCode != "6116102000" || len(result.Documents) != 4 || result.LinksRemoved != 1 {
		t.Fatalf("result: %+v", result)
	}
	if result.Documents[0].URL != testActURL || result.Documents[1].URL != testPDFURL || result.Documents[2].URL != "" || result.Documents[2].Applies != "maybe" {
		t.Fatalf("documents: %+v", result.Documents)
	}
	// Вид документа: от модели или по словам меры; ссылка на Правила об условном выпуске — со справки.
	if kinds := result.Documents[0].Kind + "," + result.Documents[1].Kind + "," + result.Documents[2].Kind + "," + result.Documents[3].Kind; kinds != "other,dc,other,conditional" {
		t.Fatalf("kinds: %s", kinds)
	}
	if result.Documents[3].URL != conditionalReleaseURL || !strings.Contains(help["text"].(string), conditionalReleaseURL) {
		t.Fatalf("conditional release link: %+v", result.Documents[3])
	}
	// В шагах чата — адрес открытой страницы.
	urls := 0
	for _, event := range *events {
		if tool, ok := event.content.(agentToolEvent); ok && tool.URL != "" {
			urls++
		}
	}
	if urls < 4 {
		t.Fatalf("tool events without url: %+v", *events)
	}
}

// После ask_user второй запуск восстанавливает справку и ссылки: open_link не отказывает, ссылка в итоге остаётся.
func TestDocumentsRestoreAfterAskUser(t *testing.T) {
	data := &fakeAgentData{}
	first := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("h", "open_goods_help", map[string]string{"code": "6116102000"}),
		call("s", "search_in_document", map[string]string{"url": testActURL, "query": "перчатки трикотажные"}),
		call("q", "ask_user", map[string]string{"question": "Перчатки контактируют с пищей?"}),
	}}}}
	runner, _ := newDocumentsRunner(first, data)
	if outcome := runner.execute(context.Background()); outcome.status != "waiting" {
		t.Fatalf("first run: %+v", outcome)
	}
	raw, _ := json.Marshal(append(runner.transcript, map[string]any{"role": "user", "content": "да, для кухни"}))
	var restored []map[string]any
	_ = json.Unmarshal(raw, &restored)
	second := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("l", "open_link", map[string]string{"url": testActURL})}},
		{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
			"description": "Нужен санэпиднадзор.",
			"documents":   []map[string]string{{"measure": "Санэпиднадзор", "act": "Решение КТС №299", "url": testPDFURL, "requirement": "Нужно.", "applies": "yes", "basis": "Входит в перечень продукции, подлежащей государственному санитарно-эпидемиологическому надзору"}},
		})}},
	}}
	runner2, _ := newDocumentsRunner(second, data)
	runner2.transcript = restored
	runner2.restoreOpened(context.Background())
	outcome := runner2.execute(context.Background())
	if answer := toolAnswer(t, runner2, "l"); answer["error"] != nil {
		t.Fatalf("open_link refused after restore: %v", answer)
	}
	if outcome.result == nil || outcome.result.Documents[0].URL != testPDFURL || outcome.result.LinksRemoved != 0 {
		t.Fatalf("result after restore: %+v", outcome.result)
	}
	// Цитата из справки, открытой в первом запуске, засчитывается как «help».
	if source := runner2.toolbox.quoteSource("6116102000", "Входит в перечень продукции, подлежащей государственному санитарно-эпидемиологическому надзору"); source != "help" {
		t.Fatalf("quote source %q", source)
	}
}

func TestSearchDocumentReturnsFragments(t *testing.T) {
	text := testActText()
	fragments, matches := searchDocument(text, "перчатки трикотажные")
	if matches != 1 || len(fragments) != 1 || !strings.Contains(fragments[0], "перчатки трикотажные с покрытием") || len(fragments[0]) > len(text)/10 {
		t.Fatalf("matches %d fragments %v", matches, fragments)
	}
	if fragments, matches := searchDocument(text, "раздела"); matches < 3000 || len(fragments) != agentSearchFragments {
		t.Fatalf("common word: %d fragments, %d matches", len(fragments), matches)
	}
	long := strings.Repeat("слово ", 2000) + "искомая перчатка " + strings.Repeat("хвост ", 2000)
	if fragment := aroundMatch(long, []string{"перчат"}, 300); !strings.Contains(fragment, "искомая перчатка") || len([]rune(fragment)) > 302 {
		t.Fatalf("around match: %q", fragment)
	}
}

func TestDocumentsAnsweredFindsPreviousAnswer(t *testing.T) {
	result, _ := json.Marshal(AgentResult{DocumentsCode: "6116102000", Documents: []AgentDocument{{Measure: "x"}}})
	plain, _ := json.Marshal(AgentResult{Candidates: []AgentCandidate{{Code: "4015190000"}}})
	messages := []repository.TNVEDAgentMessage{
		{Kind: "result", Content: repository.JSON(plain)},
		{Kind: "documents", Content: repository.JSON(`{"code":"6116102000"}`)},
		{Kind: "result", Content: repository.JSON(result)},
	}
	if !documentsAnswered(messages, "6116102000") {
		t.Fatal("previous answer not found — the agent would run again")
	}
	if documentsAnswered(messages, "4015190000") || documentsAnswered(messages[:2], "6116102000") {
		t.Fatal("no documents answer for this code yet")
	}
}

// Неизвестный код — ошибка до запуска агента (400), сессия и база не трогаются.
func TestDocumentsRejectsUnknownCode(t *testing.T) {
	agent := &TNVEDAgentService{data: &fakeAgentData{}}
	for _, code := range []string{"9999999999", "6116", ""} {
		_, err := agent.Documents(context.Background(), repository.User{ID: 1}, 1, code)
		var validation *service.ValidationError
		if !errors.As(err, &validation) || validation.Field != "code" {
			t.Errorf("%q: expected validation error, got %v", code, err)
		}
		// «/документы <товар> <код>» без чата: код проверяется до создания чата (repository nil — паника бы).
		if code == "" {
			continue
		}
		_, err = agent.AskReference(context.Background(), repository.User{ID: 1}, ReferenceInput{Text: "Наждачный круг", DocumentsCode: code})
		if !errors.As(err, &validation) || validation.Field != "code" {
			t.Errorf("ask %q: expected validation error, got %v", code, err)
		}
	}
	if !strings.Contains(documentsRequest("6116102000", fakeCodes[0].Name, "Перчатки трикотажные с покрытием"), "6116 10 200 0") {
		t.Error("documents request without code")
	}
}

// Основание меры — только дословная цитата с открытых страниц: выдуманная убирается; «нужно» без
// основания возвращается агенту; подсказка по наименованию сохраняется.
func TestDocumentsBasisAndNameHint(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.documentsCode = "6116102000"
	toolbox.run(context.Background(), "open_goods_help", `{"code":"6116102000"}`)
	hint := "Если перчатки действительно не контактируют с пищей (паспорт поставщика), наименование: «Перчатки рабочие трикотажные» — санэпиднадзор не нужен."
	finish := func(basis string) (*AgentResult, string) {
		args, _ := json.Marshal(map[string]any{"description": "Санэпиднадзор возможен.", "documents": []map[string]string{
			{"measure": "Санэпиднадзор", "act": "Решение КТС №299", "requirement": "Только изделия для контакта с пищей.", "applies": "yes", "basis": basis, "nameHint": hint},
		}})
		result, problem, _ := toolbox.finish(context.Background(), string(args))
		return result, problem
	}
	result, problem := finish("«только изделия, контактирующие с пищевыми продуктами»")
	if problem != "" || result.Documents[0].Basis != "только изделия, контактирующие с пищевыми продуктами" || result.Documents[0].BasisSource != "справка tnved.info" || result.Documents[0].NameHint != hint || result.QuotesRemoved != 0 {
		t.Fatalf("основание из справки: %+v %s", result.Documents, problem)
	}
	result, problem = finish("мера применяется ко всем перчаткам без исключения")
	if result.Documents[0].Basis != "" || result.QuotesRemoved != 1 || !strings.Contains(problem, "нет основания") {
		t.Fatalf("выдуманное основание: %+v %s", result.Documents, problem)
	}
}

func TestDocumentKind(t *testing.T) {
	for _, tc := range []struct{ kind, text, want string }{
		{"sgr", "что угодно", "sgr"},
		{"CC ", "", "cc"},
		{"", "Входит в перечень продукции, в отношении которой подача ДТ должна сопровождаться представлением там. органу сертификата соответствия согласно ТР ТС 004/2011", "cc"},
		{"", "подача ДТ должна сопровождаться представлением там. органу декларации о соответствии", "dc"},
		{"", "Государственная регистрация специализированной пищевой продукции", "sgr"},
		{"", "Требуется СГР", "sgr"},
		{"", "Условный выпуск по Правилам РК", "conditional"},
		{"", "Входит в перечень продукции, подлежащей государственному санитарно-эпидемиологическому надзору", "other"},
		{"странно", "Ветеринарный контроль", "other"},
	} {
		if got := documentKind(tc.kind, tc.text); got != tc.want {
			t.Errorf("documentKind(%q, %q) = %q, want %q", tc.kind, tc.text, got, tc.want)
		}
	}
}
