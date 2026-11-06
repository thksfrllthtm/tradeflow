package tnvedagent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

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
