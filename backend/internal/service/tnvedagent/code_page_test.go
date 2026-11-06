package tnvedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// longPagesData — страницы кодов целиком: у обоих кодов одинаковые длинные пояснения к ТН ВЭД.
type longPagesData struct{ fakeAgentData }

const testGroupNotes = "Общие положения группы: перчатки трикотажные с покрытием из резины включаются в группу 61. "

func (d *longPagesData) DetailsPage(_ context.Context, code, _ string) (string, bool, error) {
	return "Дерево кода:\n" + code + " перчатки\n" + notesHeading + "\n" + strings.Repeat(testGroupNotes, 20), false, nil
}

// Страница кода целиком; одинаковые пояснения второй страницы — модели ссылкой на первую, а цитаты из
// них проверяются по полному тексту, в том числе после ответа сотрудника.
func TestOpenDetailsSendsRepeatedNotesOnce(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("a", "open_details", map[string]string{"code": "6116102000"}),
		call("b", "open_details", map[string]string{"code": "4015190000"}),
		ask("q", "Покрытие из резины?"),
	}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.toolbox = newAgentToolbox(&longPagesData{})
	if outcome := runner.execute(context.Background()); outcome.status != "waiting" {
		t.Fatalf("первый запуск: %+v", outcome)
	}
	first, second := toolAnswer(t, runner, "a")["text"].(string), toolAnswer(t, runner, "b")["text"].(string)
	if strings.Count(first, testGroupNotes) != 20 || strings.Contains(second, testGroupNotes) || !strings.Contains(second, sameNotesPrefix+"6116102000") {
		t.Fatalf("пояснения второй страницы — ссылкой: %q", service.TruncateRunes(second, 200))
	}
	raw, _ := json.Marshal(runner.transcript)
	var restored []map[string]any
	_ = json.Unmarshal(raw, &restored)
	next, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	next.toolbox = newAgentToolbox(&longPagesData{})
	next.transcript = restored
	next.restoreOpened(context.Background())
	if source := next.toolbox.quoteSource("4015190000", "перчатки трикотажные с покрытием из резины включаются в группу 61"); source != "page" {
		t.Fatalf("цитата из пояснений второй страницы после ответа сотрудника: %q", source)
	}
}

// groupPagesData — два кода одной группы из разных позиций: пояснения к позиции разные, общие положения
// группы и раздела одинаковые; на страницах есть меры нетарифного регулирования.
type groupPagesData struct{ fakeAgentData }

var (
	testSectionNotes = "РАЗДЕЛ XVI Машины, оборудование и механизмы\n" + strings.Repeat("В данный раздел не включаются приводные ремни из пластмасс группы 39. ", 8) + "\n"
	testGroup85Notes = "85 Электрические машины и оборудование, их части\n" + strings.Repeat("Электронные модули для аппаратуры позиции 8537 включаются в позицию 8538. ", 8) + "\n"
)

func (d *groupPagesData) LookupCodes(_ context.Context, codes []string) ([]directory.NomenclatureCode, error) {
	found := []directory.NomenclatureCode{}
	for _, code := range codes {
		found = append(found, directory.NomenclatureCode{Code: code, Name: "модули", DetailsURL: "https://www.tws.by/tws/tnved/code/" + code})
	}
	return found, nil
}

func (d *groupPagesData) DetailsPage(_ context.Context, code, _ string) (string, bool, error) {
	return "Дерево кода:\n" + code + " модули\n" + measuresHeading + "\nТребование соответствия Техническому Регламенту ТР ТС 004/2011\n" +
		notesHeading + "\n" + code[:4] + " Пояснения к позиции " + code[:4] + "\nТолько эта позиция.\n" + testGroup85Notes + testSectionNotes, false, nil
}

func TestOpenDetailsDropsMeasuresAndRepeatedGroupNotes(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("a", "open_details", map[string]string{"code": "8538909200"}),
		call("b", "open_details", map[string]string{"code": "8537109100"}),
		ask("q", "Модуль пассивный?"),
	}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.toolbox = newAgentToolbox(&groupPagesData{})
	if outcome := runner.execute(context.Background()); outcome.status != "waiting" {
		t.Fatalf("первый запуск: %+v", outcome)
	}
	first, second := toolAnswer(t, runner, "a")["text"].(string), toolAnswer(t, runner, "b")["text"].(string)
	if strings.Contains(first, "ТР ТС 004/2011") || !strings.Contains(first, measuresNote) || !strings.Contains(first, testGroup85Notes) {
		t.Fatalf("меры убраны, пояснения на месте: %q", first)
	}
	if strings.Contains(second, testGroup85Notes) || strings.Contains(second, testSectionNotes) || !strings.Contains(second, "Пояснения к позиции 8537") ||
		strings.Count(second, sameNotesPrefix+"8538909200") != 2 {
		t.Fatalf("общие положения второй страницы — ссылкой, пояснения к позиции целиком: %q", second)
	}
	raw, _ := json.Marshal(runner.transcript)
	var restored []map[string]any
	_ = json.Unmarshal(raw, &restored)
	next, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	next.toolbox = newAgentToolbox(&groupPagesData{})
	next.transcript = restored
	next.restoreOpened(context.Background())
	if source := next.toolbox.quoteSource("8537109100", "Электронные модули для аппаратуры позиции 8537 включаются в позицию 8538"); source != "page" {
		t.Fatalf("цитата из общих положений второй страницы после ответа сотрудника: %q", source)
	}
}
