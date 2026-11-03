package tnvedagent

// Устойчивость к слабой модели: принудительный итог при исчерпании шагов, поиск в акте без общих слов,
// запись по перечню, когда под-агент не ответил.

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// На исчерпании шагов модель зовёт не finish, а другой инструмент: он не выполняется, модель получает
// отказ и ещё одну попытку; итог принимается. Две попытки без finish — итога нет, но и цикла нет.
func TestForcedFinishRejectsOtherTools(t *testing.T) {
	data := &fakeAgentData{}
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("l", "list_heading", map[string]string{"prefix": "6116"})}},
		{ToolCalls: []agentToolCall{call("o", "open_details", map[string]string{"code": "6116102000"})}},
		{ToolCalls: []agentToolCall{finishWith(nil)}},
	}}
	runner, _ := newTestRunner(model, data)
	runner.maxSteps = 1
	outcome := runner.execute(context.Background())
	if outcome.result == nil || len(data.pagesOpened) != 0 || model.calls != 3 {
		t.Fatalf("итог %+v, открыто страниц %d, вызовов %d", outcome.result, len(data.pagesOpened), model.calls)
	}
	raw, _ := json.Marshal(runner.transcript)
	if !strings.Contains(string(raw), "инструменты больше недоступны") || model.forced[1] != "finish" || model.forced[2] != "finish" {
		t.Fatalf("отказ инструменту и принуждение: %v %s", model.forced, raw)
	}
	stubborn := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("l", "list_heading", map[string]string{"prefix": "6116"})}},
		{ToolCalls: []agentToolCall{call("o1", "open_details", map[string]string{"code": "6116102000"})}},
		{ToolCalls: []agentToolCall{call("o2", "open_details", map[string]string{"code": "6116102000"})}},
		{ToolCalls: []agentToolCall{finishWith(nil)}},
	}}
	runner, _ = newTestRunner(stubborn, data)
	runner.maxSteps = 1
	if outcome := runner.execute(context.Background()); outcome.result != nil || stubborn.calls != 3 || len(data.pagesOpened) != 0 {
		t.Fatalf("упрямая модель: %+v, вызовов %d", outcome, stubborn.calls)
	}
}

// Общее слово запроса («из» почти в каждой строке перечня) не перевешивает редкое: находится строка с товаром.
func TestSearchDocumentIgnoresCommonWords(t *testing.T) {
	lines := []string{}
	for index := 0; index < 50; index++ {
		lines = append(lines, "из прочих товаров позиция "+strconv.Itoa(index))
	}
	lines = append(lines, "из 3920 скатерти и клеёнки из пластмасс")
	text := strings.Join(lines, "\n")
	fragments, matches := searchDocument(text, "из скатерти")
	if matches != 1 || len(fragments) != 1 || !strings.Contains(fragments[0], "скатерти") {
		t.Fatalf("общее слово перевесило: %d совпадений, %v", matches, fragments)
	}
	if _, matches := searchDocument(text, "из"); matches != 51 {
		t.Fatalf("одно слово ищется как есть: %d", matches)
	}
	if _, matches := searchDocument(text, "из прочих"); matches != 51 {
		t.Fatalf("все слова общие — считаются как есть: %d", matches)
	}
}

// Позиция перечня, найденная программой в самом акте, попадает в ответ, даже если под-агент о ней промолчал;
// если под-агент акт упомянул — повтора нет.
func TestMergeDocumentsAlwaysListsProgramPositions(t *testing.T) {
	act := directory.WebLink{URL: "https://adilet.zan.kz/rus/docs/V1800017400", Text: "Приказ МИИР РК №435 от 09.06.2023"}
	position := actMatch{act: act, kind: "cc", heading: "Приложение 4", product: "плёнка полимерная для строительства", codes: "из 3920", document: "сертификат соответствия"}
	task := documentsTask{label: "№435", title: act.Text, url: act.URL}
	silent := &AgentResult{Documents: []AgentDocument{{Kind: "other", Measure: "Прочее", Act: "Другой акт", Applies: "no"}}}
	result := mergeDocuments("3920439000", []documentsTaskResult{{task: task, result: silent, positions: []actMatch{position}}}, nil, "", nil, nil, 0)
	var added []string
	for _, document := range result.Documents {
		if document.URL == act.URL {
			added = append(added, document.Measure)
		}
	}
	if len(added) != 1 || !strings.Contains(added[0], "плёнка полимерная для строительства") {
		t.Fatalf("позиция без ответа под-агента: %+v", result.Documents)
	}
	spoken := &AgentResult{Documents: []AgentDocument{{Kind: "cc", Measure: "Сертификат соответствия ТР РК", Act: act.Text, URL: act.URL, Applies: "no"}}}
	result = mergeDocuments("3920439000", []documentsTaskResult{{task: task, result: spoken, positions: []actMatch{position}}}, nil, "", nil, nil, 0)
	count := 0
	for _, document := range result.Documents {
		if document.URL == act.URL {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("под-агент акт оценил — повтора быть не должно: %+v", result.Documents)
	}
}

// Сходство ответов по документам — по набору мер «нужно»: акт (адрес) и вид документа; «возможно» не в счёт.
func TestDocumentsAgree(t *testing.T) {
	sgr := AgentDocument{Kind: "sgr", Measure: "СГР", URL: testActURL, Applies: "yes"}
	sanitary := AgentDocument{Kind: "other", Measure: "Санэпиднадзор", URL: testActURL, Applies: "yes"}
	cc := AgentDocument{Kind: "cc", Measure: "Сертификат ТР РК", URL: "https://adilet.zan.kz/rus/docs/V1800017400", Applies: "yes"}
	maybe := AgentDocument{Kind: "cc", Measure: "Сертификат ТР РК", URL: "https://adilet.zan.kz/rus/docs/V1800017400", Applies: "maybe"}
	conditional := AgentDocument{Kind: "conditional", Measure: "Условный выпуск", Applies: "no"}
	for name, c := range map[string]struct {
		main, shadow []AgentDocument
		want         bool
	}{
		"те же меры, другой порядок":   {[]AgentDocument{sgr, sanitary, conditional}, []AgentDocument{sanitary, sgr}, true},
		"тень нашла лишний сертификат": {[]AgentDocument{sgr, sanitary}, []AgentDocument{sgr, sanitary, cc}, false},
		"«возможно» не разногласие":    {[]AgentDocument{sgr, maybe}, []AgentDocument{sgr}, true},
		"нужно против нет":             {[]AgentDocument{sgr}, []AgentDocument{{Kind: "sgr", Measure: "СГР", URL: testActURL, Applies: "no"}}, false},
		"без адреса — по названию":     {[]AgentDocument{{Kind: "other", Measure: "Утильсбор", Applies: "yes"}}, []AgentDocument{{Kind: "other", Measure: "утильсбор ", Applies: "yes"}}, true},
	} {
		if got := documentsAgree(c.main, c.shadow); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
	contents := []json.RawMessage{json.RawMessage(`{"candidates":[]}`), json.RawMessage(`{"documentsCode":"3920439000","documents":[{"kind":"sgr","applies":"yes"}]}`), json.RawMessage(`{"documentsCode":"6116102000","documents":[]}`)}
	if found := lastDocumentsResult(contents, "3920439000"); found == nil || len(found.Documents) != 1 || lastDocumentsResult(contents, "0000000000") != nil {
		t.Fatalf("последний ответ по коду: %+v", found)
	}
}

// Под-агент не ответил, но программа нашла в перечне позицию с кодом: запись по ней, а не только «⚠».
func TestMergeDocumentsFailedTaskUsesListPosition(t *testing.T) {
	act := directory.WebLink{URL: testActURL, Text: "Решение КТС №299 от 28.05.2010"}
	position := actMatch{act: act, kind: "sgr", heading: "Раздел II", product: "скатерти из полимерных материалов", codes: "из 3920", document: "свидетельство о государственной регистрации", partial: true}
	task := documentsTask{label: "№299", title: act.Text, url: act.URL, body: "СГР"}
	result := mergeDocuments("3920439000", []documentsTaskResult{{task: task, err: errors.New("boom"), positions: []actMatch{position}}}, nil, "", nil, nil, 0)
	document := result.Documents[0]
	if !strings.HasPrefix(document.Measure, "⚠ №299 — агент не ответил") || !strings.Contains(document.Measure, "скатерти из полимерных") || document.Kind != "sgr" ||
		document.Applies != "maybe" || document.Basis != "из 3920" || !strings.Contains(document.Requirement, "свидетельство о государственной регистрации") {
		t.Fatalf("запись по перечню: %+v", document)
	}
	plain := mergeDocuments("3920439000", []documentsTaskResult{{task: task, err: errors.New("boom")}}, nil, "", nil, nil, 0)
	if !strings.HasPrefix(plain.Documents[0].Measure, "⚠ №299 — агент не ответил, проверьте по ссылке") {
		t.Fatalf("без позиций — прежняя запись: %+v", plain.Documents[0])
	}
}
