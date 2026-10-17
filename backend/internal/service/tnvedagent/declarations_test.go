package tnvedagent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// Declarations — примеры декларирования. По словам: ifcg.ru — позиции 6116 и 4015 (и ещё 20), tks.ru —
// код с описанием (на «перчатки резиновые» не отвечает). Внутри позиции — коды, внутри кода — 30 описаний
// (только ifcg.ru).
func (d *fakeAgentData) Declarations(_ context.Context, query, within string) ([]directory.DeclarationSource, bool, error) {
	d.declarations = append(d.declarations, query+"|"+within)
	if d.declarationsDown {
		return nil, false, directory.ErrDeclarationsUnavailable
	}
	example := func(code, text string) directory.DeclarationExample {
		return directory.DeclarationExample{Code: code, Text: text}
	}
	ifcg := directory.DeclarationSource{Site: "ifcg.ru", URL: directory.DeclarationsURL(query, within)}
	switch len(within) {
	case 0:
		ifcg.Groups = []directory.DeclarationGroup{
			{Code: "6116", Name: "Перчатки трикотажные", Count: 140, Examples: []directory.DeclarationExample{example("6116102000", "ПЕРЧАТКИ ТРИКОТАЖНЫЕ С ПОКРЫТИЕМ ЛАДОНИ"), example("6116102000", "ПЕРЧАТКИ РАБОЧИЕ"), example("6116108000", "ПЕРЧАТКИ"), example("6116102000", "ЛИШНИЙ")}},
			{Code: "4015", Name: "Одежда из резины", Count: 12, Examples: []directory.DeclarationExample{example("4015190000", "ПЕРЧАТКИ РЕЗИНОВЫЕ")}},
		}
		for index := 0; index < 20; index++ {
			ifcg.Groups = append(ifcg.Groups, directory.DeclarationGroup{Code: fmt.Sprintf("%04d", 9000+index), Count: 1})
		}
		tks := directory.DeclarationSource{Site: "tks.ru", URL: "https://www.tks.ru/db/tnved/goods/", Groups: []directory.DeclarationGroup{
			{Code: "6116102000", Count: 77, Examples: []directory.DeclarationExample{example("6116102000", "ПЕРЧАТКИ ТРИКОТАЖНЫЕ С НИТРИЛОВЫМ ПОКРЫТИЕМ")}},
		}}
		if query == "перчатки резиновые" {
			tks.Groups, tks.Error = []directory.DeclarationGroup{}, "сайт не ответил"
		}
		return []directory.DeclarationSource{ifcg, tks}, false, nil
	case 4:
		ifcg.Groups = []directory.DeclarationGroup{{Code: "6116102000", Name: "перчатки с покрытием", Count: 120, Examples: []directory.DeclarationExample{example("6116102000", "ПЕРЧАТКИ ТРИКОТАЖНЫЕ С ПОКРЫТИЕМ ЛАДОНИ")}}}
		return []directory.DeclarationSource{ifcg}, false, nil
	}
	examples := []directory.DeclarationExample{}
	for index := 0; index < 30; index++ {
		examples = append(examples, example(within, fmt.Sprintf("ПЕРЧАТКИ %d", index)))
	}
	ifcg.Groups = []directory.DeclarationGroup{{Code: within, Count: 30, Examples: examples}}
	return []directory.DeclarationSource{ifcg}, true, nil
}

// search_declarations: ответы всех сайтов (до 12 групп, по 3 примера), не ответивший сайт помечен;
// внутри кода — до 25 описаний; ссылка на поиск — в шаге чата.
func TestSearchDeclarationsTool(t *testing.T) {
	data := &fakeAgentData{}
	toolbox := newAgentToolbox(data)
	content, event := toolbox.run(context.Background(), "search_declarations", `{"query":"перчатки трикотажные"}`)
	sources := decode(t, content)["sources"].([]any)
	ifcg, tks := sources[0].(map[string]any), sources[1].(map[string]any)
	groups := ifcg["groups"].([]any)
	first := groups[0].(map[string]any)
	if len(sources) != 2 || len(groups) != agentDeclarationGroups || first["code"] != "6116" || first["count"] != float64(140) || len(first["examples"].([]any)) != agentHeadingExamples {
		t.Fatalf("ifcg.ru: %d групп, первая %v", len(groups), first)
	}
	if tks["site"] != "tks.ru" || len(tks["groups"].([]any)) != 1 {
		t.Fatalf("tks.ru: %v", tks)
	}
	if event.Detail != "ifcg.ru: 6116 — 140, 4015 — 12; tks.ru: 6116 10 200 0 — 77" || !strings.HasPrefix(event.URL, "https://www.ifcg.ru/kb/tnved/search/") || !toolbox.declarationsChecked {
		t.Fatalf("шаг: %+v", event)
	}
	if content, event := toolbox.run(context.Background(), "search_declarations", `{"query":"перчатки резиновые"}`); !strings.Contains(content, `"error":"сайт не ответил"`) || !strings.HasSuffix(event.Detail, "tks.ru: не ответил") || event.Failed {
		t.Fatalf("один сайт не ответил — остальные отдаются: %s %+v", content, event)
	}
	content, event = toolbox.run(context.Background(), "search_declarations", `{"query":"перчатки","within":"6116 10 200 0"}`)
	inCode := decode(t, content)["sources"].([]any)[0].(map[string]any)["groups"].([]any)[0].(map[string]any)["examples"].([]any)
	if len(inCode) != agentCodeExamplesInGroup || !strings.Contains(event.Detail, "сохранённая копия") || !strings.Contains(event.Label, "6116 10 200 0") {
		t.Fatalf("описания под кодом: %d, %+v", len(inCode), event)
	}
	if content, event := toolbox.run(context.Background(), "search_declarations", `{"query":"перчатки","within":"611"}`); !strings.Contains(content, "4 или 10") || !event.Failed {
		t.Fatalf("неверный within: %s", content)
	}
	if data.declarations[2] != "перчатки|6116102000" || len(data.declarations) != 3 {
		t.Fatalf("запросы: %v", data.declarations)
	}
}

// Итог без поиска по декларациям возвращается агенту один раз; после поиска — принимается. Сайт не
// отвечает — поиск всё равно засчитан. После ответа сотрудника поиск не требуется заново.
func TestDeclarationsRequiredBeforeFinish(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{finishWith(nil)}},
		{ToolCalls: []agentToolCall{call("d", "search_declarations", map[string]string{"query": "перчатки трикотажные"})}},
		{ToolCalls: []agentToolCall{finishWith(nil)}},
	}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.toolbox.requireDeclarations = true
	outcome := runner.execute(context.Background())
	if outcome.result == nil || model.calls != 3 || !strings.Contains(toolAnswer(t, runner, "f")["error"].(string), "search_declarations") {
		t.Fatalf("итог после поиска по декларациям: %+v, вызовов %d", outcome.result, model.calls)
	}

	stubborn := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}}, {ToolCalls: []agentToolCall{finishWith(nil)}}}}
	runner, _ = newTestRunner(stubborn, &fakeAgentData{})
	runner.toolbox.requireDeclarations = true
	if outcome := runner.execute(context.Background()); outcome.result == nil || stubborn.calls != 2 {
		t.Fatalf("второй итог без поиска принимается — без бесконечного круга: %+v", outcome)
	}

	down := &fakeAgentData{declarationsDown: true}
	toolbox := newAgentToolbox(down)
	if content, event := toolbox.run(context.Background(), "search_declarations", `{"query":"перчатки"}`); !strings.Contains(content, "не ответил") || !event.Failed || !toolbox.declarationsChecked {
		t.Fatalf("сайт не отвечает: %s", content)
	}

	asking := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("d", "search_declarations", map[string]string{"query": "перчатки"}),
		call("q", "ask_user", map[string]string{"question": "Покрытие из резины?"}),
	}}}}
	runner, _ = newTestRunner(asking, &fakeAgentData{})
	runner.toolbox.requireDeclarations = true
	runner.execute(context.Background())
	next, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	next.transcript = runner.transcript
	next.restoreOpened(context.Background())
	if !next.toolbox.declarationsChecked {
		t.Fatal("поиск по декларациям из первого запуска засчитан после ответа сотрудника")
	}
}

// У кодов итога — ссылка на описания из деклараций под кодом: с числом записей и запросом, которым код
// найден; код, которого в поиске не было, — по первому запросу без числа; без поиска ссылки нет. После
// ответа сотрудника — из прежнего запуска.
func TestCandidateDeclarationsLink(t *testing.T) {
	both := func(runner *agentRunner) []AgentCandidate {
		outcome := runner.execute(context.Background())
		if outcome.result == nil {
			t.Fatal("нет итога")
		}
		return outcome.result.Candidates
	}
	finishBoth := call("f", "finish", map[string]any{"description": "Перчатки", "name": "Перчатки", "candidates": []map[string]string{
		{"code": "6116102000", "verdict": "fits", "reason": "Да."}, {"code": "4015190000", "verdict": "no", "reason": "Нет."}}})

	runner, _ := newTestRunner(&scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("d1", "search_declarations", map[string]string{"query": "перчатки рабочие"})}},
		{ToolCalls: []agentToolCall{call("d2", "search_declarations", map[string]string{"query": "перчатки трикотажные", "within": "6116"})}},
		{ToolCalls: []agentToolCall{finishBoth}},
	}}, &fakeAgentData{})
	candidates := both(runner)
	fits, no := candidates[0].Declarations, candidates[1].Declarations
	if fits == nil || fits.Count != 120 || fits.Query != "перчатки трикотажные" || fits.URL != directory.DeclarationsURL("перчатки трикотажные", "6116102000") {
		t.Fatalf("код из поиска: %+v", fits)
	}
	if no == nil || no.Count != 0 || no.URL != directory.DeclarationsURL("перчатки рабочие", "4015190000") {
		t.Fatalf("код вне поиска: %+v", no)
	}

	runner, _ = newTestRunner(&scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishBoth}}}}, &fakeAgentData{})
	if candidates := both(runner); candidates[0].Declarations != nil {
		t.Fatalf("без поиска: %+v", candidates[0].Declarations)
	}

	asking, _ := newTestRunner(&scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("d", "search_declarations", map[string]string{"query": "перчатки трикотажные"}),
		call("q", "ask_user", map[string]string{"question": "Покрытие из резины?"}),
	}}}}, &fakeAgentData{})
	asking.execute(context.Background())
	next, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	next.transcript = asking.transcript
	next.restoreOpened(context.Background())
	if link := next.toolbox.declarationsLink("6116102000"); link == nil || link.Count != 77 {
		t.Fatalf("после ответа сотрудника: %+v", link)
	}
}

// На принудительном итоге (лимит шагов) итог без поиска по декларациям не теряется.
func TestForcedFinishIgnoresDeclarationsRequirement(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("l", "list_heading", map[string]string{"prefix": "6116"})}},
		{ToolCalls: []agentToolCall{finishWith(nil)}},
	}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.toolbox.requireDeclarations = true
	runner.maxSteps = 1
	if outcome := runner.execute(context.Background()); outcome.result == nil {
		t.Fatalf("принудительный итог: %+v", outcome)
	}
}
