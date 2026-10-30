package tnvedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Справочник: в итоге два варианта наименования (краткое и подробное) по правилу компании.
func TestReferenceModeHasTwoDeclarationNames(t *testing.T) {
	finishSchema := func(reference bool) string {
		for _, tool := range agentTools(reference) {
			if tool.Function.Name == "finish" {
				data, _ := json.Marshal(tool.Function.Parameters)
				return string(data)
			}
		}
		t.Fatal("finish tool missing")
		return ""
	}
	// Изменено: справочник тоже предлагает наименование — два варианта, краткое и подробное; в подборе для
	// строки сметы — одно.
	if !strings.Contains(finishSchema(false), `"name"`) || strings.Contains(finishSchema(false), `"nameFull"`) || !strings.Contains(finishSchema(true), `"nameFull"`) {
		t.Fatal("наименование: в строке сметы — одно, в справочнике — краткое и подробное")
	}
	prompt := tnvedReferencePrompt("Правило компании: образцы.")
	if !strings.Contains(prompt, "Правило наименования:\nПравило компании: образцы.") || strings.Contains(prompt, "{naming_rule}") || !strings.Contains(prompt, "nameFull — подробное") || !strings.Contains(prompt, "быстрый справочник") {
		t.Fatal("справочнику — правило наименования и два варианта")
	}
	// Объяснение — по шаблону «Код X, потому что …; если бы … — код Y», соседи — с условием «Этот код, если …».
	if !strings.Contains(prompt, "«Если бы [противоположный признак] — был бы код Y.»") || !strings.Contains(prompt, "«Этот код, если …»") {
		t.Fatal("справочнику — шаблон объяснения выбора")
	}
	toolbox := newAgentToolbox(&fakeAgentData{})
	result, problem, _ := toolbox.finish(context.Background(), `{"description":"Перчатки","name":"Перчатки трикотажные","nameFull":"Перчатки трикотажные покрытые резиной / 手套","confidence":"high","candidates":[{"code":"6116102000","verdict":"fits","reason":"Да."}]}`)
	if problem != "" || result.Name != "Перчатки трикотажные" || result.NameFull != "Перчатки трикотажные покрытые резиной" {
		t.Fatalf("наименования в итоге: %+v %s", result, problem)
	}
	message := initialReferenceMessage("лопата штыковая", 1, nil)
	text := message["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "лопата штыковая") || !strings.Contains(text, "Фото товара прилагается") || len(message["content"].([]map[string]any)) != 2 {
		t.Fatalf("первое сообщение: %v", message)
	}
	if referenceTitle("  ") != "Фото товара" || referenceTitle("Перчатки   трикотажные") != "Перчатки трикотажные" {
		t.Fatal("заголовок вопроса")
	}
	if resultTitle(result) != "Перчатки трикотажные" || resultTitle(&AgentResult{Name: "  "}) != "" {
		t.Fatal("заголовок по ответу — краткое наименование агента")
	}
}

// Уточнение в справочнике: вопрос сотрудника передаётся модели, прежний ответ не пересказывается.
func TestReferenceFollowUpAnswersTheQuestion(t *testing.T) {
	prompt := referenceFollowUp("Какие документы нужны?")
	for _, want := range []string{"«Какие документы нужны?»", "не пересказывай", "повтори прежние коды", "name и nameFull — прежние"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("follow-up prompt has no %q", want)
		}
	}
	if !strings.Contains(tnvedReferencePrompt(""), "Уточняющие вопросы после ответа") {
		t.Error("reference prompt has no follow-up rule")
	}
}

// Справочник отвечает сразу: инструмента ask_user у него нет, а попытка спросить возвращается модели
// ошибкой, и подбор продолжается до finish. В подборе для строки сметы вопрос есть.
func TestReferenceModeHasNoAskUser(t *testing.T) {
	if names := toolNames(agentTools(true)); strings.Contains(names, "ask_user") {
		t.Fatalf("в справочнике нет ask_user: %s", names)
	}
	if names := toolNames(agentTools(false)); !strings.Contains(names, "ask_user") {
		t.Fatalf("в подборе по строке ask_user есть: %s", names)
	}
	prompt := tnvedReferencePrompt("")
	if strings.Contains(prompt, "задай один короткий вопрос через ask_user") || !strings.Contains(prompt, "Сотрудника не спрашивай") {
		t.Fatal("подсказка справочника: без вопросов сотруднику")
	}
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("q", "ask_user", map[string]string{"question": "Трикотажные или нетканые?"})}},
		{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{"description": "Салфетки", "name": "Салфетки", "nameFull": "Салфетки из микрофибры", "confidence": "medium",
			"confidenceReason": " Дерево однозначно, но материал полотна не подтверждён ",
			"candidates":       []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Да.", "check": "уточнить материал"}}})}},
	}}
	runner, events := newTestRunner(model, &fakeAgentData{})
	runner.tools = agentTools(true)
	runner.toolbox.requireDeclarations = false
	outcome := runner.execute(context.Background())
	if outcome.status != "done" || outcome.result == nil || outcome.result.Candidates[0].Check != "уточнить материал" {
		t.Fatalf("справочник без вопроса доходит до итога: %+v", outcome)
	}
	if outcome.result.Confidence != "medium" || outcome.result.ConfidenceReason != "Дерево однозначно, но материал полотна не подтверждён" {
		t.Fatalf("уверенность с объяснением: %q %q", outcome.result.Confidence, outcome.result.ConfidenceReason)
	}
	for _, event := range *events {
		if event.kind == "question" {
			t.Fatal("вопрос сотруднику в справочнике не показывается")
		}
	}
	found := false
	for _, message := range runner.transcript {
		if content, ok := message["content"].(string); ok && strings.Contains(content, "вопросов сотруднику нет") {
			found = true
		}
	}
	if !found {
		t.Fatal("модели объяснено, что вопросов в справочнике нет")
	}
}
