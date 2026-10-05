package tnvedagent

import (
	"encoding/json"
	"strings"
	"testing"
)

// Справочник: итог без наименования для декларации, подсказка без правила наименования.
func TestReferenceModeHasNoDeclarationName(t *testing.T) {
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
	if !strings.Contains(finishSchema(false), `"name"`) || strings.Contains(finishSchema(true), `"name"`) {
		t.Fatal("наименование — только в подборе для строки сметы")
	}
	prompt := tnvedReferencePrompt()
	if strings.Contains(prompt, "Правило наименования") || strings.Contains(prompt, "{naming_rule}") || !strings.Contains(prompt, "быстрый справочник") {
		t.Fatal("справочнику не нужно правило наименования")
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
	for _, want := range []string{"«Какие документы нужны?»", "не пересказывай", "повтори прежние коды"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("follow-up prompt has no %q", want)
		}
	}
	if !strings.Contains(tnvedReferencePrompt(), "Уточняющие вопросы после ответа") {
		t.Error("reference prompt has no follow-up rule")
	}
}
