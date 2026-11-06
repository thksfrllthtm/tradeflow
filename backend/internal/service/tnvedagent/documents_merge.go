package tnvedagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// documentsAgree — сходятся ли ответы двух моделей по документам: один и тот же набор мер «нужно» по акту
// (адрес, без него — название меры) и виду документа. «Возможно» разногласием не считается: обе признают,
// что решает декларант.
func documentsAgree(main, shadow []AgentDocument) bool {
	return maps.Equal(requiredDocuments(main), requiredDocuments(shadow))
}

func requiredDocuments(documents []AgentDocument) map[string]bool {
	keys := map[string]bool{}
	for _, document := range documents {
		if document.Applies != "yes" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(firstNonEmpty(document.URL, document.Measure)))
		keys[document.Kind+"|"+key] = true
	}
	return keys
}

// documentBest — лучшая оценка меры по акту и виду документа (у одного акта бывает несколько позиций
// перечня, каждая своей строкой): «нужно» старше «возможно», «возможно» старше «не относится».
type documentBest struct {
	index   int
	applies string
}

func bestDocuments(documents []AgentDocument) map[string]documentBest {
	rank := map[string]int{"no": 0, "maybe": 1, "yes": 2}
	best := map[string]documentBest{}
	for index, document := range documents {
		if document.Kind == documentKindConditional {
			continue
		}
		key := documentKey(document)
		if current, ok := best[key]; !ok || rank[document.Applies] > rank[current.applies] {
			best[key] = documentBest{index, document.Applies}
		}
	}
	return best
}

// documentKey — акт (адрес, без него — название меры) и вид документа.
func documentKey(document AgentDocument) string {
	return document.Kind + "|" + strings.ToLower(strings.TrimSpace(firstNonEmpty(document.URL, document.Measure)))
}

// mergeSecondOpinion — второе мнение по документам: мера, по которой модели разошлись («нужно» против «не
// относится» по лучшей оценке акта), становится «возможно» с доводом второй модели; мера «нужно», которую
// нашла только вторая, добавляется как «возможно». Возвращает число изменённых мер.
func mergeSecondOpinion(result *AgentResult, others []AgentDocument, name string) int {
	mainBest, otherBest := bestDocuments(result.Documents), bestDocuments(others)
	disputed := func(main, other string) bool {
		return (main == "yes" && other == "no") || (main == "no" && other == "yes")
	}
	changed := 0
	seen := map[string]bool{}
	for _, document := range others {
		key := documentKey(document)
		if document.Kind == documentKindConditional || seen[key] {
			continue
		}
		seen[key] = true
		other := others[otherBest[key].index]
		main, found := mainBest[key]
		switch {
		case !found && other.Applies == "yes":
			added := other
			added.Applies = "maybe"
			added.Requirement = "Нашла только вторая модель (" + name + "), по ней мера нужна: " + service.TruncateRunes(other.Requirement, 400)
			result.Documents = append(result.Documents, added)
			changed++
		case found && disputed(main.applies, other.Applies):
			document := &result.Documents[main.index]
			verdict := map[string]string{"yes": "нужно", "no": "не относится"}[other.Applies]
			document.Applies = "maybe"
			document.Requirement = strings.TrimSpace(document.Requirement) + "\nВторая модель (" + name + "): " + verdict + " — " + service.TruncateRunes(other.Requirement, 400)
			changed++
		}
	}
	return changed
}

// applySecondOpinion — второе мнение в итоге: спорные меры «возможно», условный выпуск и описание заново по
// новым оценкам, заметка сотруднику. Возвращает число изменённых мер (0 — итог не тронут).
func applySecondOpinion(result *AgentResult, others []AgentDocument, name, rulesText string, helpErr error, total int) int {
	changed := mergeSecondOpinion(result, others, name)
	if changed == 0 {
		return 0
	}
	kept := result.Documents[:0]
	for _, document := range result.Documents {
		if document.Kind != documentKindConditional {
			kept = append(kept, document)
		}
	}
	result.Documents = append(kept, conditionalDocument(kept, rulesText))
	result.Description = documentsDescription(result.Documents, helpErr, total)
	word := "мер"
	if changed == 1 {
		word = "меры"
	}
	note := fmt.Sprintf("Вторая модель (%s) не согласна с оценкой %d %s: они отмечены «возможно» с её доводом — проверьте по актам.", name, changed, word)
	result.Notes = strings.TrimSpace(strings.TrimSpace(result.Notes) + "\n" + note)
	return changed
}

// lastDocumentsResult — последний ответ «Документы для ввоза» для кода среди содержимого сообщений ленты.
func lastDocumentsResult(contents []json.RawMessage, code string) *AgentResult {
	var found *AgentResult
	for _, content := range contents {
		var result AgentResult
		if json.Unmarshal(content, &result) == nil && result.DocumentsCode == code {
			found = &result
		}
	}
	return found
}

// failedActDocument — под-агент по акту не ответил, но программа нашла в перечне позицию с кодом: запись по
// ней (документ перечня и строка позиции), чтобы сотрудник видел, что именно проверить, а не только «⚠».
func failedActDocument(task documentsTask, match actMatch) AgentDocument {
	name := service.TruncateRunes(firstNonEmpty(match.product, match.heading), 150)
	return AgentDocument{
		Kind:    firstNonEmpty(match.kind, documentKind("", task.title+" "+task.body)),
		Measure: "⚠ " + task.label + " — агент не ответил; код есть в перечне, позиция «" + name + "» — проверьте по ссылке",
		Act:     task.title, URL: task.url, Applies: "maybe", Basis: match.codes, BasisSource: match.act.Text,
		Requirement: "Меры акта агентом не проверены. Документ по перечню: " + firstNonEmpty(match.document, "—") + ". " + service.TruncateRunes(task.body, 300),
	}
}

// mergeDocuments — итог из ответов под-агентов: записи в порядке актов справки, ⚠ по упавшим заданиям и
// перечням общей базы, которых нет в ответах, условный выпуск от программы; описание по шаблону. Замечания —
// по строке на акт с его подписью, только от актов с мерами «нужно» или «возможно».
func mergeDocuments(code string, results []documentsTaskResult, matches []actMatch, rulesText string, helpErr error, unread []string, total int) *AgentResult {
	result := &AgentResult{DocumentsCode: code, Candidates: []AgentCandidate{}, DroppedCodes: []string{}, Documents: []AgentDocument{}}
	var notes []string
	for index := range results {
		item := &results[index]
		if item.failed() {
			if len(item.positions) == 0 {
				result.Documents = append(result.Documents, AgentDocument{
					Kind: documentKind("", item.task.title+" "+item.task.body), Measure: "⚠ " + item.task.label + " — агент не ответил, проверьте по ссылке",
					Act: item.task.title, URL: item.task.url, Applies: "maybe",
					Requirement: "Меры этого акта не проверены. " + service.TruncateRunes(item.task.body, 300),
				})
				continue
			}
			for _, match := range item.positions[:min(2, len(item.positions))] {
				result.Documents = append(result.Documents, failedActDocument(item.task, match))
			}
			continue
		}
		result.Documents = append(result.Documents, item.result.Documents...)
		relevant := false
		for _, document := range item.result.Documents {
			relevant = relevant || document.Applies != "no"
		}
		if note := strings.TrimSpace(item.result.Notes); note != "" && relevant {
			notes = append(notes, "["+item.task.label+"] "+note)
		}
		result.QuotesRemoved += item.result.QuotesRemoved
		result.LinksRemoved += item.result.LinksRemoved
	}
	// Позиции перечней с кодом, которые нашла программа (общая база, справка, сами акты), в ответе всегда:
	// под-агент их не упомянул — строка «⚠» по позиции. Так набор актов не зависит от модели и прогона.
	positions := append([]actMatch{}, matches...)
	for index := range results {
		positions = append(positions, results[index].positions...)
	}
	for _, match := range missingActs(positions, result.Documents) {
		result.Documents = append(result.Documents, missingActDocument(match))
	}
	if len(result.Documents) > agentMaxDocuments {
		result.Documents = result.Documents[:agentMaxDocuments]
	}
	result.Documents = append(result.Documents, conditionalDocument(result.Documents, rulesText))
	result.Description = documentsDescription(result.Documents, helpErr, total)
	if len(unread) > 0 {
		notes = append(notes, "Не удалось прочитать перечни: "+strings.Join(unread, "; "))
	}
	// По строке на замечание: карточка показывает их списком.
	result.Notes = service.TruncateRunes(strings.Join(notes, "\n"), 1500)
	return result
}

// conditionalDocument — условный выпуск по Правилам РК считает программа: нужен, если нужен СС или ДС;
// возможен, если они под вопросом; основание — п. 4 Правил, если страница Правил прочитана.
func conditionalDocument(documents []AgentDocument, rulesText string) AgentDocument {
	applies, needed := "no", []string{}
	for _, document := range documents {
		if document.Kind != documentKindCertificate && document.Kind != documentKindDeclaration {
			continue
		}
		switch document.Applies {
		case "yes":
			applies = "yes"
			needed = append(needed, service.TruncateRunes(document.Measure, 80))
		case "maybe":
			if applies != "yes" {
				applies = "maybe"
			}
		}
	}
	item := AgentDocument{Kind: documentKindConditional, Measure: "Условный выпуск без СС/ДС", Act: conditionalReleaseTitle, URL: conditionalReleaseURL, Applies: applies}
	switch applies {
	case "yes":
		item.Requirement = "Если на дату регистрации ДТ нет документа (" + strings.Join(needed, "; ") + "), товар выпускается условно: документ о подтверждении соответствия представляется в территориальный орган государственных доходов в течение 60 календарных дней с даты выпуска (п. 4 Правил)."
	case "maybe":
		item.Requirement = "Если товару нужен СС или ДС (меры «возможно»), а документа на дату регистрации ДТ нет — выпуск условно с представлением документа в течение 60 календарных дней с даты выпуска (п. 4 Правил)."
	default:
		item.Requirement = "Не применяется: сертификат или декларация соответствия по техрегламентам этому товару не нужны."
	}
	if applies != "no" && containsQuote(rulesText, conditionalReleaseQuote) {
		item.Basis, item.BasisSource = conditionalReleaseQuote, conditionalReleaseTitle
	}
	return item
}

// documentsDescription — резюме: только «Нужно: …» и «Возможно: …» с названиями мер и условный выпуск;
// меры «не относятся» — в свёрнутом списке карточки, в описании их нет. Справка недоступна — об этом в начале.
func documentsDescription(documents []AgentDocument, helpErr error, total int) string {
	groups := map[string][]string{}
	conditional := ""
	for _, document := range documents {
		if document.Kind == documentKindConditional {
			conditional = map[string]string{"yes": "нужен", "maybe": "возможен", "no": "не применяется"}[document.Applies]
			continue
		}
		groups[document.Applies] = append(groups[document.Applies], service.TruncateRunes(document.Measure, 120))
	}
	var parts []string
	if helpErr != nil {
		reason := "справка tnved.info сегодня недоступна"
		if errors.Is(helpErr, directory.ErrGoodsHelpLimit) {
			reason += " (лимит запросов)"
		}
		parts = append(parts, fmt.Sprintf("%s — ответ по общей базе (прочитано перечней: %d)", strings.ToUpper(reason[:2])+reason[2:], total))
	}
	for _, group := range []struct{ applies, title string }{{"yes", "Нужно"}, {"maybe", "Возможно"}} {
		if len(groups[group.applies]) > 0 {
			parts = append(parts, group.title+": "+strings.Join(unique(groups[group.applies]), "; "))
		}
	}
	if len(groups["yes"])+len(groups["maybe"]) == 0 {
		none := "Обязательных документов и мер для этого товара не найдено"
		if len(groups["no"]) > 0 {
			none += fmt.Sprintf(" (проверено мер: %d, к товару не относятся)", len(groups["no"]))
		}
		parts = append(parts, none)
	}
	if conditional != "" {
		parts = append(parts, "Условный выпуск: "+conditional)
	}
	return service.TruncateRunes(strings.Join(parts, ". ")+".", 1000)
}
