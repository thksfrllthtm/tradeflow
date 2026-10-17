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
		for _, tool := range agentTools(reference, false) {
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
