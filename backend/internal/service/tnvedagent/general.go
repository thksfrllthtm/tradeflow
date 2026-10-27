package tnvedagent

// «Документы для ввоза» по общей базе, без ИИ: дневной лимит подборов исчерпан, а ответа агента по коду
// в базе нет. Программа проверяет код по перечням: СС и ДС — перечни к ТР ТС на странице кода TWS.BY
// («Требуется документ …»), СГР — только раздел II Решения КТС №299 (sgr299.go), утильсбор — перечень
// приказа № 689 (utilfee). Подходит ли товар под позицию перечня, программа не решает: позиция есть —
// «возможно» с её названием, кода нет в перечне — «не нужно».

import (
	"fmt"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/utilfee"
)

// twsRegulation — запись «Требование соответствия Техническому Регламенту» со страницы кода TWS.BY.
type twsRegulation struct {
	code, act, document string
	items               []string
}

// twsRegulations — записи ТР ТС из блока «Меры нетарифного регулирования» (без утративших силу).
// Санитарные меры, лицензирование и белорусское «Сертифицирование» пропускаются.
func twsRegulations(page string) []twsRegulation {
	var found []twsRegulation
	var current *twsRegulation
	inBlock := false
	for _, line := range strings.Split(page, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "Меры нетарифного регулирования":
			inBlock = true
			continue
		case !inBlock:
			continue
		case strings.HasPrefix(line, "Меры нетарифного регулирования, утратившие"), strings.HasPrefix(line, "Классификационные решения"),
			strings.HasPrefix(line, "Дополнительная информация"), strings.HasPrefix(line, "Реестр объектов"):
			return found
		case strings.HasPrefix(line, "Требование соответствия Техническому Регламенту"):
			current = &twsRegulation{}
		case current == nil || line == "" || strings.HasPrefix(line, "- "):
		case strings.HasPrefix(line, "Требуется документ"):
			document := strings.TrimSpace(strings.TrimPrefix(line, "Требуется документ"))
			if strings.Contains(strings.ToLower(document), "соответств") && current.act != "" {
				current.document = document
				found = append(found, *current)
			}
			current = nil
		case twsOtherMeasure(line):
			current = nil
		case current.code == "":
			current.code = strings.ReplaceAll(line, " ", "")
		case current.act == "":
			current.act = line
		default:
			current.items = append(current.items, line)
		}
	}
	return found
}

// twsOtherMeasure — заголовок другой меры в блоке: запись ТР ТС закончилась без «Требуется документ».
func twsOtherMeasure(line string) bool {
	for _, prefix := range []string{"Санитарные меры", "Ветеринарно", "Лицензирование", "Сертифицирование", "Возможно", "Возможность", "Код в перечне", "Организация сбора", "Обязательность"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// generalDocuments — ответ по общей базе. twsPage пустая — страница кода не открылась (СС/ДС не проверены).
func generalDocuments(code, twsPage string) AgentResult {
	result := AgentResult{DocumentsCode: code, Candidates: []AgentCandidate{}, DroppedCodes: []string{}, Documents: []AgentDocument{}}
	var summary []string

	// СС и ДС.
	regulations := twsRegulations(twsPage)
	seen := map[string]bool{}
	for _, regulation := range regulations {
		key := regulation.act + "|" + regulation.document + "|" + strings.Join(regulation.items, "|")
		if seen[key] || len(result.Documents) == agentMaxDocuments-3 {
			continue
		}
		seen[key] = true
		kind := documentKindCertificate
		if strings.Contains(strings.ToLower(regulation.document), "деклараци") {
			kind = documentKindDeclaration
		}
		requirement := "Нужно, если товар входит в позицию перечня для кода " + FormatTNVEDCode(regulation.code)
		if len(regulation.items) > 0 {
			requirement += ": " + service.TruncateRunes(strings.Join(regulation.items, "; "), 500)
		}
		result.Documents = append(result.Documents, AgentDocument{Kind: kind, Measure: "Требуется документ: " + regulation.document,
			Act: service.TruncateRunes(regulation.act, 300), Requirement: requirement, Applies: "maybe", BasisSource: "страница кода (TWS.BY)"})
	}
	switch {
	case twsPage == "":
		summary = append(summary, "СС/ДС: страница кода TWS.BY не открылась — проверьте вручную")
		result.Documents = append(result.Documents, AgentDocument{Kind: documentKindCertificate, Measure: "СС и ДС не проверены: страница кода TWS.BY не открылась", Applies: "maybe"})
	case len(regulations) == 0:
		summary = append(summary, "СС/ДС: не нужны — кода нет в перечнях к ТР ТС")
	default:
		summary = append(summary, fmt.Sprintf("СС/ДС: возможно нужны — код есть в перечнях к ТР ТС (%d), нужны, если товар подходит под позицию", len(seen)))
		result.Documents = append(result.Documents, AgentDocument{Kind: documentKindConditional, Measure: "Условный выпуск без СС/ДС",
			Act: conditionalReleaseTitle, URL: conditionalReleaseURL, Applies: "maybe",
			Requirement: "Если товару нужен СС или ДС, а документа нет — выпуск условно, с обязательством представить документ в срок по Правилам."})
	}

	// СГР — только раздел II Решения №299.
	sgr, partialNames := 0, []string{}
	for _, position := range sgrPositions {
		if !strings.HasPrefix(code, position.code) {
			continue
		}
		sgr++
		if position.partial {
			partialNames = append(partialNames, service.TruncateRunes(position.name, 150))
		}
		requirement := "Нужно для продукции пунктов 6–11 раздела II (дезсредства, бытовая химия, опасные химические вещества, водоподготовка, гигиена, контакт с пищей) этой позиции."
		if position.partial {
			requirement = "Позиция «из»: нужно, только если товар — " + position.name
		}
		result.Documents = append(result.Documents, AgentDocument{Kind: documentKindRegistration, Measure: "СГР: позиция раздела II " + FormatTNVEDCode(position.code) + " — " + service.TruncateRunes(position.name, 200),
			Act: sgrActTitle, URL: sgrActURL, Requirement: requirement, Applies: "maybe"})
	}
	switch {
	case sgr == 0:
		summary = append(summary, "СГР: не нужно — кода нет в разделе II Решения КТС №299")
	case len(partialNames) == sgr:
		summary = append(summary, "СГР: только если товар — "+strings.Join(partialNames, "; ")+" (раздел II Решения КТС №299)")
	default:
		summary = append(summary, "СГР: возможно — код есть в разделе II Решения КТС №299")
	}

	// Утильсбор.
	matches, _ := utilfee.Lookup(code)
	for _, match := range matches {
		result.Documents = append(result.Documents, AgentDocument{Kind: documentKindOther,
			Measure: fmt.Sprintf("Утилизационный платёж: раздел %d «%s», пункт %s", match.Section, match.Title, match.Item),
			Act:     "Приказ МЭГПР РК № 689 от 09.11.2022 (перечень продукции)", Requirement: match.Note, Applies: "yes"})
	}
	if len(matches) == 0 {
		summary = append(summary, "утильсбор: кода нет в перечне")
	} else {
		summary = append(summary, "утильсбор: код в перечне — платёж нужен")
	}

	result.Description = strings.Join(summary, "; ") + "."
	result.Notes = "Дневной лимит подборов исчерпан, ответа агента по этому коду в базе нет — проверено программой по общей базе, без ИИ: " +
		"СС/ДС — перечни к ТР ТС на странице кода TWS.BY, СГР — раздел II Решения КТС №299, утильсбор — перечень приказа № 689. " +
		"Подходит ли товар под позицию перечня, сверьте по наименованию."
	return result
}
