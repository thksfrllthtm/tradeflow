package tnvedagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// fakeAgentData — номенклатура из двух кодов, одна страница TWS, одна запись базы; для режима
// «документы» — справка tnved.info и документы по ссылкам (helpLimit — лимит tnved.info исчерпан).
type fakeAgentData struct {
	pagesOpened []string
	webOpened   []string
	helpLimit   bool
	// declarations — запросы примеров декларирования; declarationsDown — сайт не отвечает.
	declarations     []string
	declarationsDown bool
}

var fakeCodes = []directory.NomenclatureCode{
	{Code: "6116102000", Name: "Перчатки, рукавицы и митенки трикотажные: 🠺 пропитанные, с покрытием или дублированные пластмассой или резиной: 🠺 перчатки, пропитанные или покрытые резиной", TariffText: "10%", VATPercent: "16", DetailsURL: "https://www.tws.by/tws/tnved/code/1"},
	{Code: "4015190000", Name: "Одежда и принадлежности к одежде из вулканизованной резины: 🠺 перчатки, рукавицы и митенки: 🠺 прочие", TariffText: "10%", VATPercent: "16", DetailsURL: "https://www.tws.by/tws/tnved/code/2"},
}

func (d *fakeAgentData) SearchNomenclature(_ context.Context, query string, _ int) ([]directory.NomenclatureCode, int64, error) {
	var found []directory.NomenclatureCode
	for _, code := range fakeCodes {
		if strings.HasPrefix(code.Code, query) || strings.Contains(strings.ToLower(code.Name), strings.ToLower(SearchStemQuery(query))) {
			found = append(found, code)
		}
	}
	return found, int64(len(found)), nil
}

// SearchStemQuery — первое слово запроса по основе (как ищет настоящая номенклатура).
func SearchStemQuery(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	return repository.SearchStem(fields[0])
}

func (d *fakeAgentData) LookupCodes(_ context.Context, codes []string) ([]directory.NomenclatureCode, error) {
	var found []directory.NomenclatureCode
	for _, code := range fakeCodes {
		for _, wanted := range codes {
			if code.Code == wanted {
				found = append(found, code)
			}
		}
	}
	return found, nil
}

func (d *fakeAgentData) DetailsPage(_ context.Context, code, _ string) (string, bool, error) {
	d.pagesOpened = append(d.pagesOpened, code)
	if code == "6116102000" {
		return "Дерево кода:\n6116 10 200 0 перчатки, пропитанные или покрытые резиной\nПояснения к ТНВЭД: В данную субпозицию включаются трикотажные перчатки с покрытием ладони из резины или полиуретана.", false, nil
	}
	return "", false, directory.ErrDetailsUnavailable
}

func (d *fakeAgentData) SearchDecisions(context.Context, string, int) ([]repository.CodeDecision, error) {
	return []repository.CodeDecision{{Status: "verified", InvoiceName: "Перчатки х/б с обливом", FinalName: "Перчатки трикотажные", Code: "6116102000"}}, nil
}

func (d *fakeAgentData) SearchDirectories(context.Context, string, int) ([]repository.DirectoryHint, error) {
	return nil, nil
}

// scriptedModel отвечает заранее заданными шагами и запоминает, что ему прислали.
type scriptedModel struct {
	replies   []agentReply
	calls     int
	lastSeen  []agentMessage
	lastTools []agentTool
	forced    []string
}

func (m *scriptedModel) Complete(_ context.Context, messages []agentMessage, tools []agentTool, force string) (agentReply, error) {
	m.lastSeen = messages
	m.lastTools = tools
	m.forced = append(m.forced, force)
	if m.calls >= len(m.replies) {
		return agentReply{}, errors.New("script is over")
	}
	reply := m.replies[m.calls]
	m.calls++
	return reply, nil
}

func (m *scriptedModel) Configured() bool { return true }
func (m *scriptedModel) Name() string     { return "scripted" }

func call(id, name string, args any) agentToolCall {
	data, _ := json.Marshal(args)
	var tc agentToolCall
	tc.ID, tc.Type, tc.Function.Name, tc.Function.Arguments = id, "function", name, string(data)
	return tc
}

type emitted struct {
	kind    string
	content any
}

func newTestRunner(model *scriptedModel, data *fakeAgentData) (*agentRunner, *[]emitted) {
	events := []emitted{}
	initial := initialAgentMessage(AgentItemInput{Name: "Перчатки рабочие трикотажные покрытые резиной", SupplierName: "聚氨酯橡胶针织工作手套", Quantity: "1000", NetKg: "70"}, 1, nil)
	raw, _ := json.Marshal([]map[string]any{initial})
	var transcript []map[string]any
	_ = json.Unmarshal(raw, &transcript)
	runner := &agentRunner{
		model: model, toolbox: newAgentToolbox(data), system: tnvedAgentPrompt(""), transcript: transcript,
		photos: []string{"data:image/jpeg;base64,AAAA"}, vision: true,
		emit:   func(kind string, content any) { events = append(events, emitted{kind, content}) },
		onStep: func([]map[string]any, agentUsage) {},
	}
	return runner, &events
}

func kinds(events []emitted) string {
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.kind)
	}
	return strings.Join(names, ",")
}

func TestAgentRunnerChecksCodesAndQuotes(t *testing.T) {
	data := &fakeAgentData{}
	model := &scriptedModel{replies: []agentReply{
		{Content: "По фото — трикотажные перчатки с покрытием.", ToolCalls: []agentToolCall{
			call("c1", "search_nomenclature", map[string]string{"query": "перчатки трикотажные"}),
			call("c2", "search_decisions", map[string]string{"query": "перчатки"}),
		}},
		{ToolCalls: []agentToolCall{call("c3", "open_details", map[string]string{"code": "6116 10 200 0"}), call("c4", "open_details", map[string]string{"code": "4015190000"})}},
		{ToolCalls: []agentToolCall{call("c5", "finish", map[string]any{
			"description": "Трикотажные перчатки с полиуретановым покрытием ладони.",
			"name":        "Перчатки трикотажные с полиуретановым покрытием ладони",
			"candidates": []map[string]string{
				{"code": "6116102000", "verdict": "fits", "quote": "включаются трикотажные перчатки с покрытием ладони", "reason": "Текстиль с покрытием."},
				{"code": "4015190000", "verdict": "no", "quote": "из резины придающей основные свойства", "reason": "Не резиновое изделие."},
				{"code": "9999999999", "verdict": "doubtful", "reason": "Выдуманный код."},
			},
		})}},
	}}
	runner, events := newTestRunner(model, data)
	outcome := runner.execute(context.Background())
	if outcome.err != nil || outcome.status != "done" || outcome.result == nil {
		t.Fatalf("outcome: %+v", outcome)
	}
	result := outcome.result
	if len(result.Candidates) != 2 || result.Candidates[0].Code != "6116102000" || result.Candidates[0].QuoteSource != "page" || !result.Candidates[0].PageOpened {
		t.Fatalf("first candidate: %+v", result.Candidates)
	}
	if result.Candidates[1].Quote != "" || result.QuotesRemoved != 1 {
		t.Fatalf("invented quote must be removed: %+v", result)
	}
	if len(result.DroppedCodes) != 1 || result.DroppedCodes[0] != "9999999999" {
		t.Fatalf("code outside nomenclature must be dropped: %+v", result.DroppedCodes)
	}
	if result.Best().Code != "6116102000" || result.Candidates[0].Nomenclature == nil || result.Candidates[0].Nomenclature.TariffText != "10%" {
		t.Fatalf("best/nomenclature: %+v", result.Candidates[0])
	}
	if got := kinds(*events); got != "text,tool,tool,tool,tool,result" {
		t.Fatalf("events: %s", got)
	}
	// Фото уходит модели картинкой, а не ссылкой image_ref.
	parts, _ := model.lastSeen[1].Content.([]map[string]any)
	if len(parts) != 2 || parts[1]["type"] != "image_url" {
		t.Fatalf("first message parts: %#v", model.lastSeen[1].Content)
	}
	// На каждый вызов инструмента есть ответ tool.
	answered := map[string]bool{}
	for _, message := range runner.transcript {
		if message["role"] == "tool" {
			answered[message["tool_call_id"].(string)] = true
		}
	}
	for _, id := range []string{"c1", "c2", "c3", "c4", "c5"} {
		if !answered[id] {
			t.Fatalf("tool call %s without answer", id)
		}
	}
}

func TestAgentRunnerAsksUserAndContinuesWithOpenedPages(t *testing.T) {
	data := &fakeAgentData{}
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("c1", "open_details", map[string]string{"code": "6116102000"}), call("c2", "ask_user", map[string]string{"question": "Покрытие только на ладони?"})}},
	}}
	runner, events := newTestRunner(model, data)
	outcome := runner.execute(context.Background())
	if outcome.status != "waiting" || kinds(*events) != "tool,question" {
		t.Fatalf("outcome %+v events %s", outcome, kinds(*events))
	}
	// Второй запуск (после ответа) не открывает страницу заново, но цитата с неё проходит проверку.
	transcript := runner.transcript
	raw, _ := json.Marshal(append(transcript, map[string]any{"role": "user", "content": "только ладонь"}))
	var restored []map[string]any
	_ = json.Unmarshal(raw, &restored)
	second := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{call("c3", "finish", map[string]any{
		"description": "Перчатки", "name": "Перчатки трикотажные",
		"candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "quote": "трикотажные перчатки с покрытием ладони", "reason": "Да."}},
	})}}}}
	runner2, _ := newTestRunner(second, data)
	runner2.transcript = restored
	runner2.restoreOpened(context.Background())
	outcome = runner2.execute(context.Background())
	if outcome.result == nil || outcome.result.Candidates[0].QuoteSource != "page" {
		t.Fatalf("quote from a page opened in the first run: %+v", outcome.result)
	}
	if len(data.pagesOpened) != 1 {
		t.Fatalf("page opened again: %v", data.pagesOpened)
	}
}

func TestAgentRunnerNudgesAndForcesFinish(t *testing.T) {
	data := &fakeAgentData{}
	replies := []agentReply{{Content: "Думаю…"}}
	for index := 0; index < 3; index++ {
		replies = append(replies, agentReply{ToolCalls: []agentToolCall{call("s"+string(rune('a'+index)), "list_heading", map[string]string{"prefix": "6116"})}})
	}
	replies = append(replies, agentReply{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
		"description": "Перчатки", "name": "Перчатки", "candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Да."}},
	})}})
	model := &scriptedModel{replies: replies}
	runner, _ := newTestRunner(model, data)
	runner.maxSteps = 4
	outcome := runner.execute(context.Background())
	if outcome.result == nil {
		t.Fatalf("outcome: %+v", outcome)
	}
	if model.forced[len(model.forced)-1] != "finish" {
		t.Fatalf("last call must force finish: %v", model.forced)
	}
}

func TestAgentRunnerRetriesFinishWithoutValidCodes(t *testing.T) {
	data := &fakeAgentData{}
	bad := call("f1", "finish", map[string]any{"description": "x", "name": "x", "candidates": []map[string]string{{"code": "1111111111", "verdict": "fits", "reason": "?"}}})
	good := call("f2", "finish", map[string]any{"description": "x", "name": "x", "candidates": []map[string]string{{"code": "4015190000", "verdict": "doubtful", "reason": "?"}}})
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{bad}}, {ToolCalls: []agentToolCall{good}}}}
	runner, _ := newTestRunner(model, data)
	outcome := runner.execute(context.Background())
	if outcome.result == nil || outcome.result.Candidates[0].Code != "4015190000" || model.calls != 2 {
		t.Fatalf("outcome %+v calls %d", outcome, model.calls)
	}
}

func TestQuoteSourceNeedsVerbatimText(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.opened["6116102000"] = "В данную субпозицию включаются трикотажные перчатки с покрытием ладони."
	name := fakeCodes[0].Name
	cases := map[string]string{
		"«трикотажные перчатки, с покрытием ладони»": "page", // знаки и регистр не важны
		"перчатки, пропитанные или покрытые резиной": "name",
		"перчатки из нитрила для медицины":           "",
		"прочие":                 "", // коротко и нет в наименовании
		"6116 10 200 0 перчатки": "", // номер кода и одно слово ничего не подтверждают
	}
	for quote, want := range cases {
		if got := toolbox.quoteSource("6116102000", name, strings.Trim(quote, "«»")); got != want {
			t.Errorf("%q: got %q, want %q", quote, got, want)
		}
	}
}

func TestSortByVerdictPutsSameProductFirst(t *testing.T) {
	matches := []decisions.SimilarMatch{
		{Decision: decisions.CodeDecisionView{ID: 1}, Score: 90},
		{Decision: decisions.CodeDecisionView{ID: 2}, Score: 60, Verdict: "same"},
		{Decision: decisions.CodeDecisionView{ID: 3}, Score: 80, Verdict: "similar"},
		{Decision: decisions.CodeDecisionView{ID: 4}, Score: 70, Verdict: "same"},
	}
	sortByVerdict(matches)
	order := []int64{}
	for _, match := range matches {
		order = append(order, match.Decision.ID)
	}
	if order[0] != 4 || order[1] != 2 || order[2] != 3 || order[3] != 1 {
		t.Fatalf("order %v", order)
	}
}
