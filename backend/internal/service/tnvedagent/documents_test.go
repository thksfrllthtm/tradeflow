package tnvedagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	d.mu.Lock()
	d.webOpened = append(d.webOpened, url)
	d.mu.Unlock()
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
	case conditionalReleaseURL:
		return directory.WebPage{URL: url, Title: "Правила", Text: "Глава 2\n4. " + conditionalReleaseQuote + ", в отношении которых соблюдение запретов и ограничений подтверждается после выпуска товаров (условный выпуск)."}, nil
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
		names := toolNames(agentTools(reference))
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

// Под-агент: итог по мерам акта (без conditional — его считает программа), ссылка и поиск; без ask_user
// и справки (её открывает программа).
func TestDocumentsTools(t *testing.T) {
	names := toolNames(documentsTools())
	if names != "finish,open_link,search_in_document" {
		t.Fatalf("documents tools %s", names)
	}
	data, _ := json.Marshal(documentsTools()[0].Function.Parameters)
	if !strings.Contains(string(data), `"documents"`) || !strings.Contains(string(data), `"applies"`) || strings.Contains(string(data), `"conditional"`) {
		t.Fatalf("documents finish schema: %s", data)
	}
	if strings.Contains(tnvedDocumentsActPrompt, "open_goods_help") || !strings.Contains(tnvedDocumentsActPrompt, "условный выпуск и утильсбор считает программа") {
		t.Fatal("act prompt must not ask for the help page or conditional release")
	}
}

// actModel — сценарий для под-агентов: ответы по акту, названному в сообщении (подстрока); вызовы идут
// параллельно, поэтому под мьютексом. errors — акт, на котором модель ломается.
type actModel struct {
	mu      sync.Mutex
	scripts map[string][]agentReply
	errors  map[string]error
	calls   map[string]int
	seen    map[string][]agentMessage
	tools   []agentTool
}

func (m *actModel) key(messages []agentMessage) string {
	for _, message := range messages {
		content, _ := message.Content.(string)
		if message.Role != "user" {
			continue
		}
		for key := range m.scripts {
			if strings.Contains(content, key) {
				return key
			}
		}
		for key := range m.errors {
			if strings.Contains(content, key) {
				return key
			}
		}
	}
	return ""
}

func (m *actModel) Complete(_ context.Context, messages []agentMessage, tools []agentTool, _ string) (agentReply, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == nil {
		m.calls, m.seen = map[string]int{}, map[string][]agentMessage{}
	}
	key := m.key(messages)
	m.seen[key], m.tools = messages, tools
	if err := m.errors[key]; err != nil {
		return agentReply{}, err
	}
	replies := m.scripts[key]
	if key == "" || m.calls[key] >= len(replies) {
		return agentReply{}, errors.New("script is over for " + key)
	}
	reply := replies[m.calls[key]]
	m.calls[key]++
	return reply, nil
}

func (m *actModel) Configured() bool { return true }
func (m *actModel) Name() string     { return "acts" }

// toolAnswer — ответ инструмента из переписки раннера (по id вызова).
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

// toolContent — ответ инструмента из переписки под-агента (по id вызова).
func toolContent(t *testing.T, messages []agentMessage, id string) map[string]any {
	t.Helper()
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == id {
			var answer map[string]any
			if err := json.Unmarshal([]byte(message.Content.(string)), &answer); err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			return answer
		}
	}
	t.Fatalf("no answer for %s", id)
	return nil
}

func newDocumentsRun(model agentModel, data *fakeAgentData) (*documentsRun, *[]emitted, *[]agentUsage) {
	events, usages := []emitted{}, []agentUsage{}
	run := &documentsRun{
		model: model, data: data, code: "6116102000", product: "Перчатки рабочие трикотажные покрытые резиной",
		photo:  "Перчатки трикотажные с резиновым покрытием ладони.",
		emit:   func(kind string, content any) { events = append(events, emitted{kind, content}) },
		onStep: func(usage agentUsage) { usages = append(usages, usage) },
	}
	return run, &events, &usages
}

// Справка с двумя актами → два под-агента параллельно: один ищет в №299, другой открывает PDF («дай ссылку»);
// итог собран в порядке справки, conditional — от программы по cc/dc, чужой url убран, шаги в чате с актом.
func TestDocumentsRunParallel(t *testing.T) {
	data := &fakeAgentData{generalActs: true}
	model := &actModel{scripts: map[string][]agentReply{
		"№299": {
			{ToolCalls: []agentToolCall{
				call("s1", "search_in_document", map[string]string{"url": "http://adilet.zan.kz/rus/docs/H10T0000299#z5", "query": "перчатки контактирующие пищевыми"}),
				call("l2", "open_link", map[string]string{"url": "https://evil.example.com/steal"}),
			}},
			{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
				"description": "Санэпиднадзор, если перчатки контактируют с пищей.",
				"documents": []map[string]string{
					{"measure": "Санитарно-эпидемиологический надзор", "act": "Решение КТС №299 от 28.05.2010", "url": testActURL, "requirement": "Только для изделий, контактирующих с пищей.", "applies": "maybe"},
					{"kind": "conditional", "measure": "Условный выпуск", "act": "Правила РК", "url": conditionalReleaseURL, "requirement": "—", "applies": "yes"},
					{"measure": "Выдуманная мера", "act": "Акт", "url": "https://evil.example.com/act", "requirement": "—", "applies": "странно"},
				},
				"notes": "Проверьте контакт с пищей.",
			})}},
		},
		"№79": {
			{ToolCalls: []agentToolCall{call("l3", "open_link", map[string]string{"url": testPDFURL})}},
			{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
				"description": "Нужна декларация соответствия.",
				"documents": []map[string]string{
					{"kind": "dc", "measure": "Декларация соответствия ТР ТС 019/2011", "act": "Решение Коллегии ЕЭК №79", "url": testPDFURL, "requirement": "Декларация о соответствии.", "applies": "yes", "basis": "Требуется декларация соответствия ТР ТС 019/2011"},
				},
			})}},
		},
	}}
	run, events, usages := newDocumentsRun(model, data)
	outcome := run.execute(context.Background())
	if outcome.err != nil || outcome.result == nil || outcome.partial {
		t.Fatalf("outcome %+v", outcome)
	}
	if names := toolNames(model.tools); names != "finish,open_link,search_in_document" {
		t.Fatalf("sub-agent tools: %s", names)
	}
	// Под-агент видит товар, описание фото, свой акт и позиции перечня, найденные программой.
	first, _ := model.seen["№79"][1].Content.(string)
	if !strings.Contains(first, "Перчатки рабочие") || !strings.Contains(first, "резиновым покрытием") || !strings.Contains(first, "Решение Коллегии ЕЭК №79") ||
		strings.Contains(first, "№299") || !strings.Contains(first, `"codes":"из 61, из 62`) {
		t.Fatalf("task message: %s", first)
	}
	if system, _ := model.seen["№79"][0].Content.(string); model.seen["№79"][0].Role != "system" || !strings.Contains(system, "ОДИН акт") {
		t.Fatalf("system prompt: %q", system)
	}
	// Поиск — фрагменты с нужной строкой; чужая ссылка не открывается; PDF — «дай ссылку».
	search := toolContent(t, model.seen["№299"], "s1")
	fragments, _ := search["fragments"].([]any)
	if len(fragments) == 0 || !strings.Contains(fragments[0].(string), "перчатки трикотажные с покрытием") {
		t.Fatalf("search answer: %v", search)
	}
	if evil := toolContent(t, model.seen["№299"], "l2"); evil["error"] == nil {
		t.Fatalf("evil link opened: %v", evil)
	}
	if pdf := toolContent(t, model.seen["№79"], "l3"); !strings.Contains(fmt.Sprint(pdf["error"]), "Дай эту ссылку сотруднику") {
		t.Fatalf("pdf answer: %v", pdf)
	}
	for _, opened := range data.webOpened {
		if strings.Contains(opened, "evil") {
			t.Fatalf("evil url fetched: %v", data.webOpened)
		}
	}
	result := outcome.result
	kinds := []string{}
	for _, document := range result.Documents {
		kinds = append(kinds, document.Kind)
	}
	if result.DocumentsCode != "6116102000" || strings.Join(kinds, ",") != "other,other,dc,conditional" || result.LinksRemoved != 1 || result.Notes != "[№299] Проверьте контакт с пищей." {
		t.Fatalf("result: %+v", result)
	}
	if result.Documents[0].URL != testActURL || result.Documents[1].URL != "" || result.Documents[1].Applies != "maybe" || result.Documents[2].URL != testPDFURL || result.Documents[2].Basis == "" {
		t.Fatalf("documents: %+v", result.Documents)
	}
	conditional := result.Documents[3]
	if conditional.Applies != "yes" || conditional.URL != conditionalReleaseURL || conditional.Basis != conditionalReleaseQuote || !strings.Contains(conditional.Requirement, "60 календарных дней") {
		t.Fatalf("conditional: %+v", conditional)
	}
	// В описании только «нужно» и «возможно»: «не относится» — в свёрнутом списке карточки.
	if !strings.Contains(result.Description, "Нужно: Декларация соответствия ТР ТС 019/2011") || !strings.Contains(result.Description, "Возможно: Санитарно-эпидемиологический надзор") ||
		!strings.Contains(result.Description, "Условный выпуск: нужен") || strings.Contains(result.Description, "Не нужно") {
		t.Fatalf("description: %s", result.Description)
	}
	// В чате: справка, общая база, шаги под-агентов с подписью акта и один собранный итог; частичных итогов нет.
	labels := []string{}
	last := (*events)[len(*events)-1]
	if last.kind != "result" || last.content.(*AgentResult) != result {
		t.Fatalf("last event: %+v", last)
	}
	for _, event := range (*events)[:len(*events)-1] {
		if event.kind != "tool" {
			t.Fatalf("unexpected event %s: %+v", event.kind, event.content)
		}
		labels = append(labels, event.content.(agentToolEvent).Label)
	}
	joined := strings.Join(labels, "|")
	if !strings.HasPrefix(joined, "Справка по товару 6116 10 200 0|Перечни общей базы|") || !strings.Contains(joined, "[№299] Поиск в документе") || !strings.Contains(joined, "[ТР ТС 019/2011] Ссылка") {
		t.Fatalf("events: %s", joined)
	}
	if len(*usages) != 4 {
		t.Fatalf("steps: %d", len(*usages))
	}
}

// Под-агент не ответил (ошибка модели или итога нет) — его акт помечен «⚠», итог есть, но в базу по коду не
// сохраняется; все упали — ошибка; остановка — ошибка без итога.
func TestDocumentsActFails(t *testing.T) {
	finish79 := []agentReply{
		{ToolCalls: []agentToolCall{call("l", "open_link", map[string]string{"url": testPDFURL})}},
		{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{"description": "ДС.",
			"documents": []map[string]string{{"kind": "dc", "measure": "Декларация соответствия ТР ТС 019/2011", "act": "Решение Коллегии ЕЭК №79", "requirement": "Нужна.", "applies": "maybe"}}})}},
	}
	for name, model := range map[string]*actModel{
		"ошибка модели": {scripts: map[string][]agentReply{"№79": finish79}, errors: map[string]error{"№299": errors.New("boom")}},
		"без итога":     {scripts: map[string][]agentReply{"№79": finish79, "№299": {{Content: "думаю"}, {Content: "всё ещё думаю"}}}},
	} {
		run, _, _ := newDocumentsRun(model, &fakeAgentData{})
		outcome := run.execute(context.Background())
		if outcome.err != nil || outcome.result == nil || !outcome.partial {
			t.Fatalf("%s: %+v", name, outcome)
		}
		warning := outcome.result.Documents[0]
		if !strings.HasPrefix(warning.Measure, "⚠ №299 — агент не ответил") || warning.Applies != "maybe" || warning.URL != testActURL || len(outcome.result.Documents) != 3 {
			t.Fatalf("%s: %+v", name, outcome.result.Documents)
		}
	}
	run, _, _ := newDocumentsRun(&actModel{errors: map[string]error{"№299": errors.New("boom"), "№79": errors.New("boom")}}, &fakeAgentData{})
	if outcome := run.execute(context.Background()); outcome.err == nil || outcome.result != nil {
		t.Fatalf("all failed: %+v", outcome)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run, _, _ = newDocumentsRun(&actModel{}, &fakeAgentData{})
	if outcome := run.execute(ctx); !errors.Is(outcome.err, context.Canceled) || outcome.result != nil {
		t.Fatalf("cancelled: %+v", outcome)
	}
}

// Разбор справки: нумерация заново в каждой группе, название акта в две строки, блок «Условный выпуск»
// не акт; позиция общей базы относится к акту по ссылке, номеру решения и техрегламента, №299 — по номеру.
func TestSplitHelpActs(t *testing.T) {
	text := "Справка по товару 6116 10 200 0 (tnved.info, ввоз в ЕАЭС)\nНаименование кода: перчатки\n\n" +
		"Меры нетарифного регулирования (ограничения):\n1. Решение Комиссии Таможенного союза от 28 мая 2010 г. № 299\nО применении санитарных мер — https://tnved.info/acts/299\n   Мера: Санэпиднадзор\n   Относится к товарам: контакт с пищей\n" +
		"2. Решение Коллегии ЕЭК от 13.06.2012 № 79 — https://adilet.zan.kz/rus/docs/H12EK000079\n   Мера: подача ДТ сопровождается декларацией о соответствии ТР ТС 019/2011\n   Изменения: Решение №12 — https://adilet.zan.kz/rus/docs/H19EK000012\n\n" +
		"Льготы:\n1. Решение Совета ЕЭК № 2 — https://adilet.zan.kz/rus/docs/H14EV000002\n   Мера: Преференции\n\n" +
		"Условный выпуск (РК):\n" + conditionalReleaseTitle + " — " + conditionalReleaseURL
	acts := splitHelpActs(text)
	if len(acts) != 3 {
		t.Fatalf("acts: %+v", acts)
	}
	if acts[0].Title != "Решение Комиссии Таможенного союза от 28 мая 2010 г. № 299 О применении санитарных мер" || acts[0].URL != "https://tnved.info/acts/299" ||
		acts[0].Group != helpRestrictions || !strings.Contains(acts[0].Body, "Относится к товарам: контакт с пищей") {
		t.Fatalf("act 299: %+v", acts[0])
	}
	if acts[1].URL != "https://adilet.zan.kz/rus/docs/H12EK000079" || !strings.Contains(acts[1].Body, "Изменения") || acts[2].Group != "Льготы" || acts[2].Title != "Решение Совета ЕЭК № 2" {
		t.Fatalf("acts: %+v", acts[1:])
	}
	sgr := actMatch{act: directory.WebLink{URL: directory.SGRActURL, Text: "Решение КТС №299 (раздел II — СГР)"}}
	act79 := actMatch{act: directory.WebLink{URL: "https://adilet.zan.kz/rus/docs/H12EK000079", Text: "Решение Коллегии ЕЭК №79 от 13.06.2012 (ТР ТС 019/2011)"}}
	act2 := actMatch{act: directory.WebLink{URL: "https://adilet.zan.kz/rus/docs/H14EK000002", Text: "Решение Коллегии ЕЭК №2 от 16.01.2014 (ТР ТС 020/2011)"}}
	if !actMatchesHelp(sgr, acts[0]) || actMatchesHelp(sgr, acts[1]) || !actMatchesHelp(act79, acts[1]) || actMatchesHelp(act2, acts[2]) {
		t.Fatal("matching general base positions to help acts")
	}
	// Льготы и прочие сведения справки — не документы на ввоз: заданий по ним нет.
	tasks := documentsTasks(acts, []actMatch{sgr, act79, act2})
	if len(tasks) != 3 || tasks[0].label != "№299" || len(tasks[0].positions) != 1 || tasks[1].label != "ТР ТС 019/2011" ||
		!tasks[2].general || tasks[2].label != "ТР ТС 020/2011" {
		t.Fatalf("tasks: %+v", tasks)
	}
	if many := documentsTasks(nil, manyMatches(15)); len(many) != documentsMaxActs || many[documentsMaxActs-1].label != "Остальные акты" || len(many[documentsMaxActs-1].positions) != 6 {
		t.Fatalf("too many tasks: %d %+v", len(many), many[len(many)-1])
	}
}

func manyMatches(count int) []actMatch {
	matches := []actMatch{}
	for index := 0; index < count; index++ {
		matches = append(matches, actMatch{act: directory.WebLink{URL: fmt.Sprintf("https://adilet.zan.kz/rus/docs/H%02d", index), Text: fmt.Sprintf("Решение №%d", index)}})
	}
	return matches
}

// Условный выпуск: нужен при СС/ДС «нужно», возможен при «возможно», иначе не применяется; основание — п. 4
// Правил, только если страница Правил прочитана. Сборка режет записи до 20, conditional остаётся.
func TestConditionalDocument(t *testing.T) {
	rules := "4. " + conditionalReleaseQuote + " …"
	yes := conditionalDocument([]AgentDocument{{Kind: "cc", Measure: "Сертификат соответствия ТР ТС 010/2011", Applies: "yes"}, {Kind: "dc", Applies: "maybe"}}, rules)
	if yes.Applies != "yes" || yes.Basis != conditionalReleaseQuote || yes.BasisSource != conditionalReleaseTitle || !strings.Contains(yes.Requirement, "Сертификат соответствия ТР ТС 010/2011") {
		t.Fatalf("yes: %+v", yes)
	}
	maybe := conditionalDocument([]AgentDocument{{Kind: "dc", Applies: "maybe"}, {Kind: "sgr", Applies: "yes"}}, "")
	if maybe.Applies != "maybe" || maybe.Basis != "" || maybe.Kind != documentKindConditional || maybe.URL != conditionalReleaseURL {
		t.Fatalf("maybe: %+v", maybe)
	}
	if no := conditionalDocument([]AgentDocument{{Kind: "other", Applies: "yes"}, {Kind: "cc", Applies: "no"}}, rules); no.Applies != "no" || no.Basis != "" {
		t.Fatalf("no: %+v", no)
	}
	documents := []AgentDocument{}
	for index := 0; index < 25; index++ {
		documents = append(documents, AgentDocument{Kind: "other", Measure: fmt.Sprintf("Мера %d", index), Applies: "no"})
	}
	merged := mergeDocuments("6116102000", []documentsTaskResult{{task: documentsTask{label: "x"}, result: &AgentResult{Documents: documents}}}, nil, "", directory.ErrGoodsHelpLimit, []string{"Решение №77"}, 13)
	if len(merged.Documents) != agentMaxDocuments+1 || merged.Documents[agentMaxDocuments].Kind != documentKindConditional ||
		!strings.HasPrefix(merged.Description, "Справка tnved.info сегодня недоступна (лимит запросов) — ответ по общей базе (прочитано перечней: 13)") ||
		!strings.Contains(merged.Description, "Обязательных документов и мер для этого товара не найдено (проверено мер: 20, к товару не относятся)") ||
		!strings.Contains(merged.Notes, "Не удалось прочитать перечни: Решение №77") {
		t.Fatalf("merged: %d %s %s", len(merged.Documents), merged.Description, merged.Notes)
	}
}

// Акт со ссылкой под-агент обязан прочитать сам: итог без open_link или search_in_document возвращается ему один
// раз; у акта без ссылки требования нет.
func TestDocumentsActMustBeRead(t *testing.T) {
	finish := call("f", "finish", map[string]any{"description": "ДС.", "documents": []map[string]string{
		{"kind": "dc", "measure": "Декларация соответствия ТР ТС 019/2011", "act": "Решение Коллегии ЕЭК №79", "requirement": "Нужна.", "applies": "maybe"}}})
	model := &actModel{scripts: map[string][]agentReply{
		"№79":  {{ToolCalls: []agentToolCall{finish}}, {ToolCalls: []agentToolCall{finish}}},
		"№299": {{ToolCalls: []agentToolCall{call("s", "search_in_document", map[string]string{"url": testActURL, "query": "перчатки"})}}, {ToolCalls: []agentToolCall{finish}}},
	}}
	run, _, usages := newDocumentsRun(model, &fakeAgentData{})
	outcome := run.execute(context.Background())
	if outcome.err != nil || outcome.result == nil || outcome.partial || len(*usages) != 4 {
		t.Fatalf("outcome %+v, steps %d", outcome, len(*usages))
	}
	if problem := toolContent(t, model.seen["№79"], "f"); !strings.Contains(fmt.Sprint(problem["error"]), "проверь сам акт") {
		t.Fatalf("finish without reading the act accepted: %v", problem)
	}
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.documentsCode = "6116102000"
	if _, problem, _ := toolbox.finish(context.Background(), `{"description":"x","documents":[{"kind":"other","measure":"Санэпиднадзор","act":"№299","requirement":"-","applies":"no"}]}`); problem != "" {
		t.Fatalf("act without a link must not require reading: %q", problem)
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
	// Описание фото из переписки — под-агентам без заголовка.
	transcript := []map[string]any{{"role": "user", "content": photoNotePrefix + " (составила модель):\nПерчатки с покрытием."}}
	if photoNoteText(transcript) != "Перчатки с покрытием." || photoNoteText(nil) != "" {
		t.Fatalf("photo note: %q", photoNoteText(transcript))
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
