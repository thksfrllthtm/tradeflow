package tnvedagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

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
