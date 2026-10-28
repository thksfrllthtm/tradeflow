package tnvedagent

// Общая база «Документов для ввоза» (directory.GeneralActs): код ищется прямо в текстах перечней СС/ДС
// и раздела II Решения КТС №299. Нужна, когда справки tnved.info по коду нет (лимит) — тогда находки
// получает агент и решает по товару, — и когда исчерпан дневной лимит подборов, а ответа агента по коду
// в базе нет — тогда ответ собирает программа без ИИ (generalDocuments).
//
// Перечень в тексте — строки таблицы подряд: пункт («3. Оборудование гаражное…»), наименование продукции,
// строки кодов («из 8467», «8418 10 200 1», «из 5208, из 5209,», «из 61»), документ («сертификат
// соответствия», «декларация о соответствии / или сертификат соответствия») и примечание («кроме …»).
// Документ обычно после кодов, в перечне к ТР ТС 004 — под пунктом, перед кодами.

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/utilfee"
)

// actMatch — позиция перечня, в которой встретился код.
type actMatch struct {
	act      directory.WebLink
	kind     string // cc, dc, sgr
	heading  string // пункт перечня
	product  string // наименование продукции над кодами
	codes    string // строки кодов позиции
	document string // документ об оценке соответствия
	note     string // примечание после документа
	partial  bool   // позиция «из …»: только названная часть
}

var (
	actCodeToken = regexp.MustCompile(`^(?i:(из)\s+)?(\d{2}[\d\s]*?)(?:\s*[-–]\s*(\d{4}))?$`)
	actHeading   = regexp.MustCompile(`^([0-9]+|[IVXLC]+)\.\s`)
	actSubitem   = regexp.MustCompile(`^[0-9]+\)\s`)
)

type actCode struct {
	digits, rangeEnd string
	partial          bool
}

// actCodes — строка состоит только из кодов ТН ВЭД (через запятую или точку с запятой). Без «из» код —
// от 4 цифр: «10», «2018» и номера колонок кодами не считаются.
func actCodes(line string) ([]actCode, bool) {
	var codes []actCode
	for _, part := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		match := actCodeToken.FindStringSubmatch(part)
		if match == nil {
			return nil, false
		}
		code := actCode{digits: strings.Join(strings.Fields(match[2]), ""), rangeEnd: match[3], partial: match[1] != ""}
		if len(code.digits) > 10 || (!code.partial && len(code.digits) < 4) || (code.rangeEnd != "" && len(code.digits) != 4) {
			return nil, false
		}
		codes = append(codes, code)
	}
	return codes, len(codes) > 0
}

// covers — код товара входит в позицию перечня; partial — позиция «из».
func (c actCode) covers(code string) bool {
	if c.rangeEnd != "" {
		return len(code) >= 4 && code[:4] >= c.digits && code[:4] <= c.rangeEnd
	}
	return strings.HasPrefix(code, c.digits)
}

func isDocumentLine(line string) bool {
	lower := strings.ToLower(line)
	return len([]rune(line)) < 120 && (strings.Contains(lower, "сертификат соответствия") || strings.Contains(lower, "декларация о соответствии") || strings.Contains(lower, "декларация соответствия"))
}

// sgrSection — раздел II Решения №299 (государственная регистрация): остальные разделы — санэпиднадзор без СГР.
func sgrSection(lines []string) []string {
	start := -1
	for index, line := range lines {
		switch {
		case start < 0 && strings.HasPrefix(line, "Раздел II ") && !strings.HasPrefix(line, "Раздел III"):
			start = index
		case start >= 0 && strings.HasPrefix(line, "Раздел III"):
			return lines[start:index]
		}
	}
	if start < 0 {
		return nil
	}
	return lines[start:]
}

// findInAct — позиции перечня с кодом товара.
func findInAct(code string, act directory.WebLink, text string) []actMatch {
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
	}
	sgr := act.URL == directory.SGRActURL
	if sgr {
		lines = sgrSection(lines)
	}
	isCode := func(index int) bool { _, ok := actCodes(lines[index]); return ok }
	var found []actMatch
	for start := 0; start < len(lines); start++ {
		if !isCode(start) {
			continue
		}
		end := start
		for end+1 < len(lines) && isCode(end+1) {
			end++
		}
		hit, partial := false, false
		for index := start; index <= end; index++ {
			codes, _ := actCodes(lines[index])
			for _, item := range codes {
				if item.covers(code) {
					hit, partial = true, partial || item.partial
				}
			}
		}
		if !hit {
			start = end
			continue
		}
		match := actMatch{act: act, partial: partial, codes: service.TruncateRunes(strings.Join(lines[start:end+1], " "), 300)}
		// Наименование — строки над кодами до документа, других кодов или пункта.
		var product []string
		for index := start - 1; index >= 0 && len(product) < 12; index-- {
			line := lines[index]
			if line == "" || isCode(index) || isDocumentLine(line) || actHeading.MatchString(line) || actSubitem.MatchString(line) {
				break
			}
			product = append([]string{line}, product...)
		}
		match.product = strings.Join(product, " ")
		for index := start - 1; index >= 0; index-- {
			if actHeading.MatchString(lines[index]) {
				match.heading = lines[index]
				break
			}
		}
		if sgr {
			// В разделе II наименование — строка под кодом.
			match.kind, match.document = documentKindRegistration, "свидетельство о государственной регистрации"
			if end+1 < len(lines) {
				match.product = lines[end+1]
			}
		} else {
			match.document, match.note = actDocument(lines, start, end)
			match.kind = documentKindCertificate
			if strings.Contains(strings.ToLower(match.document), "деклараци") {
				match.kind = documentKindDeclaration
			}
		}
		found = append(found, match)
		start = end
	}
	return found
}

// actDocument — документ позиции: после кодов (до следующей позиции), иначе ближайший выше (ТР ТС 004).
func actDocument(lines []string, start, end int) (document, note string) {
	for index := end + 1; index < len(lines) && index <= end+15; index++ {
		line := lines[index]
		if isDocumentLine(line) {
			document = line
			if index+1 < len(lines) && strings.HasPrefix(strings.ToLower(lines[index+1]), "или ") {
				index++
				document += " " + lines[index]
			}
			if index+1 < len(lines) {
				lower := strings.ToLower(lines[index+1])
				if strings.HasPrefix(lower, "кроме") || strings.HasPrefix(lower, "за исключением") {
					note = service.TruncateRunes(lines[index+1], 300)
				}
			}
			return document, note
		}
		if _, ok := actCodes(line); ok || actHeading.MatchString(line) || actSubitem.MatchString(line) {
			break
		}
	}
	for index := start - 1; index >= 0; index-- {
		if isDocumentLine(lines[index]) {
			return lines[index], ""
		}
	}
	return "", ""
}

// generalMatches — код в перечнях общей базы. Тексты — из базы (сводные редакции adilet), карточка
// решения docs.eaeunion.org — через её PDF «Рус». unread — перечни, текст которых не прочитан. Акты
// запоминаются как открытые страницы: агент может искать в них и цитировать.
func (t *agentToolbox) generalMatches(ctx context.Context, code string) ([]actMatch, []string, int) {
	if t.general == nil || t.general.code != code {
		search := generalSearch{code: code}
		search.matches, search.unread, search.total = t.readGeneral(ctx, code)
		t.general = &search
	}
	return t.general.matches, t.general.unread, t.general.total
}

// generalSearch — результат поиска кода по общей базе (один раз за запуск: тексты актов большие).
type generalSearch struct {
	code    string
	matches []actMatch
	unread  []string
	total   int
}

func (t *agentToolbox) readGeneral(ctx context.Context, code string) (matches []actMatch, unread []string, total int) {
	acts, _ := t.data.GeneralActs(ctx)
	for _, act := range acts {
		page, err := t.actText(ctx, act.URL)
		if err != nil || strings.TrimSpace(page.Text) == "" {
			unread = append(unread, act.Text)
			continue
		}
		total++
		t.rememberPage(act.URL, page.Text, nil)
		matches = append(matches, findInAct(code, act, page.Text)...)
	}
	return matches, unread, total
}

// actText — текст акта; у карточки docs.eaeunion.org (сам текст в PDF) — текст PDF «Рус».
func (t *agentToolbox) actText(ctx context.Context, address string) (directory.WebPage, error) {
	page, err := t.documentText(ctx, address)
	if err != nil {
		return page, err
	}
	if parsed, _ := url.Parse(address); parsed != nil && parsed.Hostname() == "docs.eaeunion.org" {
		for _, link := range page.Links {
			if link.Text == "Рус" && strings.HasSuffix(strings.ToLower(link.URL), ".pdf") {
				t.rememberPage(address, "", page.Links)
				return t.documentText(ctx, link.URL)
			}
		}
	}
	return page, nil
}

var (
	actNumber     = regexp.MustCompile(`№\s*(\d+)`)
	actRegulation = regexp.MustCompile(`\d{3}/\d{4}`)
)

// covered — перечень учтён в ответе агента: та же ссылка, номер решения или номер техрегламента в акте
// или мере. Раздел II №299 учтён, только если в ответе есть СГР (санэпиднадзор по разделу I — не то).
func covered(match actMatch, documents []AgentDocument) bool {
	if match.kind == documentKindRegistration {
		for _, document := range documents {
			if document.Kind == documentKindRegistration {
				return true
			}
		}
		return false
	}
	var patterns []*regexp.Regexp
	if number := actNumber.FindStringSubmatch(match.act.Text); number != nil {
		patterns = append(patterns, regexp.MustCompile(`№\s*`+number[1]+`\b`))
	}
	if regulation := actRegulation.FindString(match.act.Text); regulation != "" {
		patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(regulation)))
	}
	for _, document := range documents {
		if document.URL == match.act.URL {
			return true
		}
		for _, pattern := range patterns {
			if pattern.MatchString(document.Act + " " + document.Measure) {
				return true
			}
		}
	}
	return false
}

// missingActs — перечни общей базы с кодом, которых нет в ответе: по первой позиции на перечень.
func missingActs(matches []actMatch, documents []AgentDocument) []actMatch {
	var missing []actMatch
	seen := map[string]bool{}
	for _, match := range matches {
		key := match.act.URL + "|" + match.kind
		if seen[key] || covered(match, documents) {
			continue
		}
		seen[key] = true
		missing = append(missing, match)
	}
	return missing
}

// checkGeneral — сверка ответа «Документы для ввоза» с общей базой (программой, без ИИ). Пропущенные
// перечни — в ответ строкой «⚠ …»; первый раз ещё и агенту на проверку (problem — finish снова, тогда
// ответ пересобирается и предупреждения остаются только у того, что агент так и не учёл).
func (t *agentToolbox) checkGeneral(ctx context.Context, result *AgentResult, problem string) string {
	matches, _, _ := t.generalMatches(ctx, t.documentsCode)
	missing := missingActs(matches, result.Documents)
	if len(missing) == 0 {
		return problem
	}
	var lines []string
	for index, match := range missing {
		if index < 5 {
			name := service.TruncateRunes(firstNonEmpty(match.product, match.heading), 150)
			result.Documents = append(result.Documents, AgentDocument{Kind: match.kind,
				Measure: "⚠ Код есть в перечне " + actShortTitle(match.act.Text) + ", позиция «" + name + "» — проверьте, подходит ли товар",
				Act:     match.act.Text, URL: match.act.URL, Applies: "maybe", Basis: match.codes, BasisSource: match.act.Text,
				Requirement: "Агент эту позицию не учёл. Документ по перечню: " + firstNonEmpty(match.document, "—") + "."})
		}
		if index < 8 {
			lines = append(lines, fmt.Sprintf("%d) %s — позиция «%s», коды: %s; документ: %s; url: %s", index+1, match.act.Text,
				service.TruncateRunes(firstNonEmpty(match.product, match.heading), 200), match.codes, firstNonEmpty(match.document, "—"), match.act.URL))
		}
	}
	if t.generalAsked {
		return problem
	}
	t.generalAsked = true
	return strings.TrimSpace(problem + " Сверка с общей базой: код есть в перечнях, которых нет в documents:\n" + strings.Join(lines, "\n") +
		"\nЭти перечни открыты (search_in_document по url). Реши по товару, относится ли позиция, и добавь по записи на каждый перечень: applies yes, maybe или no, basis — строка позиции, url — адрес перечня. Вызови finish снова.")
}

// generalAnswer — open_goods_help без справки: находки общей базы для агента.
func (t *agentToolbox) generalAnswer(ctx context.Context, code, label, reason, helpURL string) (string, agentToolEvent) {
	matches, unread, total := t.generalMatches(ctx, code)
	t.rememberPage(conditionalReleaseURL, "", []directory.WebLink{{URL: conditionalReleaseURL, Text: conditionalReleaseTitle}})
	positions := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		positions = append(positions, map[string]any{"act": match.act.Text, "url": match.act.URL, "kind": match.kind, "item": match.heading,
			"product": match.product, "codes": match.codes, "document": match.document, "note": match.note, "partial": match.partial})
	}
	answer := map[string]any{"code": code, "error": reason, "general": positions, "actsRead": total,
		"links": []directory.WebLink{{URL: conditionalReleaseURL, Text: conditionalReleaseTitle}},
		"hint": "Справки tnved.info нет — ниже общая база: позиции перечней СС/ДС и раздела II Решения КТС №299, где встречается код. " +
			"Реши по товару, относится ли каждая позиция; partial — позиция «из …»: только названная продукция. Кода нет в перечне — этот документ не нужен."}
	if len(unread) > 0 {
		answer["unread"] = unread
	}
	detail := fmt.Sprintf("%s — общая база: позиций %d в %d перечнях", reason, len(matches), total)
	return toolJSON(answer), agentToolEvent{Tool: "open_goods_help", Label: label, Detail: detail, URL: helpURL, Failed: true}
}

// generalDocuments — ответ по общей базе без ИИ: позиции перечней «возможно» (подходит ли товар,
// решает сотрудник по наименованию), кода нет в перечнях — документ не нужен; утильсбор — перечень № 689.
func generalDocuments(code string, matches []actMatch, unread []string, total int) AgentResult {
	result := AgentResult{DocumentsCode: code, Candidates: []AgentCandidate{}, DroppedCodes: []string{}, Documents: []AgentDocument{}}
	conformity, sgr := []string{}, []string{}
	for _, match := range matches {
		name := firstNonEmpty(match.product, match.heading)
		if match.kind == documentKindRegistration {
			sgr = append(sgr, service.TruncateRunes(name, 120))
		} else {
			conformity = append(conformity, actShortTitle(match.act.Text))
		}
		if len(result.Documents) >= agentMaxDocuments-3 {
			continue
		}
		requirement := "Нужно, если товар — " + service.TruncateRunes(name, 300)
		if match.partial {
			requirement += " (позиция «из»: только эта продукция)"
		}
		if match.note != "" {
			requirement += ". " + match.note
		}
		measure := "Позиция перечня: " + service.TruncateRunes(name, 200)
		if document := []rune(match.document); len(document) > 0 {
			measure = strings.ToUpper(string(document[:1])) + string(document[1:]) + " — " + service.TruncateRunes(name, 200)
		}
		result.Documents = append(result.Documents, AgentDocument{Kind: match.kind, Measure: measure, Act: match.act.Text, URL: match.act.URL,
			Requirement: requirement, Applies: "maybe", Basis: match.codes, BasisSource: match.act.Text})
	}
	var summary []string
	if len(conformity) == 0 {
		summary = append(summary, fmt.Sprintf("СС/ДС: не нужны — кода нет в перечнях общей базы (прочитано %d)", total))
	} else {
		summary = append(summary, "СС/ДС: нужны, если товар подходит под позицию — "+strings.Join(unique(conformity), ", "))
		result.Documents = append(result.Documents, AgentDocument{Kind: documentKindConditional, Measure: "Условный выпуск без СС/ДС",
			Act: conditionalReleaseTitle, URL: conditionalReleaseURL, Applies: "maybe",
			Requirement: "Если товару нужен СС или ДС, а документа нет — выпуск условно, с обязательством представить документ в срок по Правилам."})
	}
	if len(sgr) == 0 {
		summary = append(summary, "СГР: не нужно — кода нет в разделе II Решения КТС №299")
	} else {
		summary = append(summary, "СГР: только если товар — "+strings.Join(unique(sgr), "; "))
	}
	if fees, _ := utilfee.Lookup(code); len(fees) > 0 {
		summary = append(summary, "утильсбор: код в перечне — платёж нужен")
		for _, fee := range fees {
			result.Documents = append(result.Documents, AgentDocument{Kind: documentKindOther, UtilFee: utilFeeObject([]utilfee.Match{fee}),
				Measure: fmt.Sprintf("Утилизационный платёж: раздел %d «%s», пункт %s", fee.Section, fee.Title, fee.Item),
				Act:     "Приказ МЭГПР РК № 689 от 09.11.2022 (перечень продукции)", Requirement: fee.Note, Applies: "yes"})
		}
	} else {
		summary = append(summary, "утильсбор: кода нет в перечне")
	}
	result.Description = strings.Join(summary, "; ") + "."
	result.Notes = "Дневной лимит подборов исчерпан, ответа агента по этому коду в базе нет — проверено программой по общей базе, без ИИ: " +
		"перечни СС/ДС к техрегламентам, раздел II Решения КТС №299, перечень утильсбора № 689. Подходит ли товар под позицию, сверьте по наименованию."
	if len(unread) > 0 {
		result.Notes += " Не удалось прочитать: " + strings.Join(unread, "; ") + "."
	}
	return result
}

// isUtilFeeMeasure — мера про утильсбор: «утилизационный платёж», «утилизация отходов (РОП)»,
// «расширенные обязательства производителей».
func isUtilFeeMeasure(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "утилиз") || strings.Contains(text, "утильсбор") ||
		(strings.Contains(text, "расширенн") && strings.Contains(text, "обязательств"))
}

// utilFeeObject — за что утильсбор по позициям перечня № 689: раздел 18 — за упаковку, остальные
// (товар, транспорт) — за сам товар.
func utilFeeObject(fees []utilfee.Match) string {
	product, packaging := false, false
	for _, fee := range fees {
		if fee.Kind == utilfee.KindPackaging {
			packaging = true
		} else {
			product = true
		}
	}
	switch {
	case product && packaging:
		return "за товар и упаковку"
	case packaging:
		return "за упаковку"
	case product:
		return "за товар"
	}
	return ""
}

// actShortTitle —«Решение Коллегии ЕЭК №6 от 16.01.2018 (ТР ТС 010/2011)» → «№6 (ТР ТС 010/2011)».
func actShortTitle(title string) string {
	if index := strings.Index(title, "№"); index >= 0 {
		short := strings.TrimSpace(title[index:])
		if open, close := strings.Index(short, " от "), strings.Index(short, "("); open > 0 && close > open {
			return short[:open] + " " + short[close:]
		}
		return short
	}
	return title
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
