package tnvedagent

// Итог агента (finish) проверяется: коды — только из номенклатуры, цитаты — только дословные, из
// открытой страницы кода, справки или документа либо из наименования кода, ссылки на документы —
// только с открытых страниц.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/utilfee"
)

// finishArgs — аргументы finish как их прислала модель.
type finishArgs struct {
	Description string `json:"description"`
	Candidates  []struct {
		Code          string `json:"code"`
		Verdict       string `json:"verdict"`
		Quote         string `json:"quote"`
		DecisionQuote string `json:"decisionQuote"`
		Reason        string `json:"reason"`
		Check         string `json:"check"`
	} `json:"candidates"`
	Name             string `json:"name"`
	NameFull         string `json:"nameFull"`
	Notes            string `json:"notes"`
	Confidence       string `json:"confidence"`
	ConfidenceReason string `json:"confidenceReason"`
	Sources          []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"sources"`
	Documents []struct {
		Kind        string `json:"kind"`
		Measure     string `json:"measure"`
		Act         string `json:"act"`
		URL         string `json:"url"`
		Requirement string `json:"requirement"`
		Applies     string `json:"applies"`
		Basis       string `json:"basis"`
		NameHint    string `json:"nameHint"`
	} `json:"documents"`
}

// finish проверяет итог агента. problem — что исправить (модели дают одну попытку).
func (t *agentToolbox) finish(ctx context.Context, arguments string) (*AgentResult, string, error) {
	var args finishArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil, "Аргументы finish — не JSON по схеме", nil
	}
	if t.documentsCode != "" {
		result, problem := t.finishDocuments(args)
		if result != nil && problem == "" && t.requireActRead && !t.actRead {
			problem = "Итог ещё не принят: сначала проверь сам акт по ссылке — search_in_document по коду («" + t.documentsCode[:4] + "», «из " + t.documentsCode[:4] +
				"»), по виду товара и по словам «примечание», «не распространяется», «за исключением»; сверь позицию, заголовок её раздела и примечания с товаром. Затем вызови finish снова."
		}
		return result, problem, nil
	}
	codes := []string{}
	seen := map[string]bool{}
	for _, candidate := range args.Candidates {
		code := nonDigits.ReplaceAllString(candidate.Code, "")
		if len(code) == 10 && !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	items, err := t.data.LookupCodes(ctx, codes)
	if err != nil {
		return nil, "", err
	}
	byCode := map[string]directory.NomenclatureCode{}
	for _, item := range items {
		byCode[item.Code] = item
	}
	result := &AgentResult{
		Description: service.TruncateRunes(strings.TrimSpace(args.Description), 1000), Name: declarationName(args.Name),
		NameFull: declarationName(args.NameFull),
		Notes:    service.TruncateRunes(strings.TrimSpace(args.Notes), 1000), Candidates: []AgentCandidate{}, DroppedCodes: []string{},
	}
	used := map[string]bool{}
	// withoutNotes — коды, где решение дано без цитаты пояснений: решение убирается, модели — замечание.
	withoutNotes := []string{}
	for _, candidate := range args.Candidates {
		code := nonDigits.ReplaceAllString(candidate.Code, "")
		if used[code] {
			continue
		}
		item, ok := byCode[code]
		if !ok {
			if code != "" {
				result.DroppedCodes = append(result.DroppedCodes, code)
			}
			continue
		}
		used[code] = true
		verdict := candidate.Verdict
		if verdict != "fits" && verdict != "doubtful" && verdict != "no" {
			verdict = "doubtful"
		}
		quote := service.TruncateRunes(strings.TrimSpace(strings.Trim(candidate.Quote, "«»\"' ")), 600)
		source := ""
		if quote != "" {
			source = t.quoteSource(code, quote)
			if source == "" {
				quote = ""
				result.QuotesRemoved++
			}
		}
		// Решение — второе доказательство: только вместе с цитатой пояснений и дословно из блока решений
		// страницы этого кода, иначе убирается.
		decisionQuote := service.TruncateRunes(strings.TrimSpace(strings.Trim(candidate.DecisionQuote, "«»\"' ")), 600)
		decisionLabel := ""
		if decisionQuote != "" {
			if page := t.opened[code]; !strings.HasPrefix(page, tksNotesPrefix) {
				decisionLabel = decisionSource(page, decisionQuote)
			}
			if decisionLabel == "" {
				decisionLabel = t.nsiDecisionSource(decisionQuote)
			}
			if decisionLabel != "" && quote == "" {
				withoutNotes = append(withoutNotes, FormatTNVEDCode(code))
				decisionLabel = ""
			}
			if decisionLabel == "" {
				decisionQuote = ""
				result.QuotesRemoved++
			}
		}
		_, opened := t.opened[code]
		copyItem := item
		result.Candidates = append(result.Candidates, AgentCandidate{
			Code: code, Verdict: verdict, Quote: quote, QuoteSource: source, DecisionQuote: decisionQuote, DecisionSource: decisionLabel,
			PageOpened: opened, Nomenclature: &copyItem, Declarations: t.declarationsLink(code),
			Reason: service.TruncateRunes(strings.TrimSpace(candidate.Reason), 600), Check: service.TruncateRunes(strings.TrimSpace(candidate.Check), 400),
		})
		if len(result.Candidates) == agentMaxCandidates {
			break
		}
	}
	if len(result.Candidates) == 0 {
		return nil, "Ни одного кода из номенклатуры: проверь коды через get_codes или search_nomenclature и вызови finish ещё раз.", nil
	}
	if best := result.Best(); best != nil {
		result.Name, result.NameFull = fixGenericWord(result.Name, best.Code), fixGenericWord(result.NameFull, best.Code)
	}
	if args.Confidence == "high" || args.Confidence == "medium" || args.Confidence == "low" {
		result.Confidence = args.Confidence
		result.ConfidenceReason = service.TruncateRunes(strings.TrimSpace(args.ConfidenceReason), 300)
	}
	// Источник — только страница, которую агент действительно видел в этом подборе.
	for _, source := range args.Sources {
		address := strings.TrimSpace(source.URL)
		if t.links[address] && len(result.Sources) < 5 {
			result.Sources = append(result.Sources, AgentSource{Title: service.TruncateRunes(strings.TrimSpace(source.Title), 200), URL: address})
		}
	}
	if best := result.Best(); best != nil && t.fileCode != "" && t.fileCode != best.Code {
		result.FileCode = &AgentFileCode{Code: t.fileCode, BestCode: best.Code, BestDuty: dutyText(*best.Nomenclature), Duty: "кода нет в номенклатуре"}
		if found, err := t.data.LookupCodes(ctx, []string{t.fileCode}); err == nil && len(found) > 0 {
			result.FileCode.Found, result.FileCode.Duty = true, dutyText(found[0])
		}
		result.FileCode.DutyDiffs = result.FileCode.Duty != result.FileCode.BestDuty
	}
	if t.requireDeclarations && !t.declarationsChecked {
		return result, "Итог ещё не принят: сначала проверь, как такой товар декларировали, — search_declarations по виду товара, затем внутри подходящей позиции (within). Сверь с выбранным кодом, напиши в reason, что показали декларации, и вызови finish снова.", nil
	}
	if len(withoutNotes) > 0 {
		return result, "Классификационное решение без цитаты из пояснений у " + strings.Join(withoutNotes, ", ") + ": решение — второе доказательство, сначала quote — дословно скопируй из пояснений к ТН ВЭД на странице open_details этого кода слова, где назван товар (не пересказ). В пояснениях товара нет — убери decisionQuote. Вызови finish снова.", nil
	}
	return result, "", nil
}

// finishDocuments — итог по кнопке «Документы для ввоза»: меры с актами; ссылка остаётся, только если
// такой адрес был на открытых страницах. Итог отдаётся и с problem: после повтора показывается как есть.
func (t *agentToolbox) finishDocuments(args finishArgs) (*AgentResult, string) {
	result := &AgentResult{
		Description: service.TruncateRunes(strings.TrimSpace(args.Description), 1000), Notes: service.TruncateRunes(strings.TrimSpace(args.Notes), 1000),
		Candidates: []AgentCandidate{}, DroppedCodes: []string{}, DocumentsCode: t.documentsCode, Documents: []AgentDocument{},
	}
	// unfounded — мер «нужно» без проверенного основания.
	unfounded := 0
	for _, document := range args.Documents {
		item := AgentDocument{
			Measure: service.TruncateRunes(strings.TrimSpace(document.Measure), 200), Act: service.TruncateRunes(strings.TrimSpace(document.Act), 300),
			Requirement: service.TruncateRunes(strings.TrimSpace(document.Requirement), 600), Applies: document.Applies,
			NameHint: service.TruncateRunes(strings.TrimSpace(document.NameHint), 600),
		}
		if item.Measure == "" && item.Act == "" {
			continue
		}
		// Условный выпуск считает программа при сборке (conditionalDocument): запись модели не нужна.
		if item.Kind = documentKind(document.Kind, item.Measure+" "+item.Act); item.Kind == documentKindConditional {
			continue
		}
		if isUtilFeeMeasure(item.Measure + " " + item.Act) {
			fees, _ := utilfee.Lookup(t.documentsCode)
			item.UtilFee = utilFeeObject(fees)
		}
		if item.Applies != "yes" && item.Applies != "maybe" && item.Applies != "no" {
			item.Applies = "maybe"
		}
		// Основание — только дословная цитата из того, что агент видел; выдуманная убирается.
		if basis := service.TruncateRunes(strings.TrimSpace(strings.Trim(document.Basis, "«»\"' ")), 600); basis != "" {
			actURL, _ := directory.CanonicalWebURL(document.URL)
			if origin := t.quoteOrigin(basis, actURL, item.Act); origin != "" {
				item.Basis, item.BasisSource = basis, origin
			} else {
				result.QuotesRemoved++
			}
		}
		if item.Applies == "yes" && item.Basis == "" {
			unfounded++
		}
		if strings.TrimSpace(document.URL) != "" {
			if address, ok := directory.CanonicalWebURL(document.URL); ok && t.links[address] {
				item.URL = address
			} else {
				result.LinksRemoved++
			}
		}
		result.Documents = append(result.Documents, item)
		if len(result.Documents) == agentMaxDocuments {
			break
		}
	}
	if result.Description == "" && len(result.Documents) == 0 {
		return result, "Итог пустой: в description — резюме, в documents — меры из справки с решением, относятся ли они к товару."
	}
	if unfounded > 0 {
		return result, fmt.Sprintf("У %d мер «нужно» нет основания: в basis — дословная цитата пояснения под мерой из справки, пункта акта или страницы кода (цитаты, которой нет в тексте, программа убирает). Нет цитаты — поставь maybe. Вызови finish снова.", unfounded)
	}
	return result, ""
}

// Виды документов для ввоза. Правила РК об условном выпуске (совместный приказ МФ РК и МИР РК, 2018 г.):
// товары, которым нужен СС или ДС по ТР, выпускаются условно без документа с обязательством представить
// его в срок по Правилам. Текст — на alta.ru, добавляется к справке в режиме «Документы».
const (
	documentKindCertificate  = "cc"
	documentKindDeclaration  = "dc"
	documentKindRegistration = "sgr"
	documentKindConditional  = "conditional"
	documentKindOther        = "other"

	conditionalReleaseURL   = "https://www.alta.ru/tamdoc/18kz0287/"
	conditionalReleaseTitle = "Правила подтверждения соответствия требованиям ТР в отношении условно выпущенных товаров (совместный приказ МФ РК и МИР РК, 2018)"
)

// documentKind — вид документа из ответа модели; если модель его не поставила или поставила неизвестный,
// по словам меры: «сертификат соответствия» — cc, «декларация о соответствии» — dc, «государственная
// регистрация» — sgr, «условный выпуск» — conditional, иначе other.
func documentKind(kind, text string) string {
	switch kind = strings.ToLower(strings.TrimSpace(kind)); kind {
	case documentKindCertificate, documentKindDeclaration, documentKindRegistration, documentKindConditional, documentKindOther:
		return kind
	}
	text = decisions.NormalizeDecisionName(text)
	switch {
	case strings.Contains(text, "условн") && strings.Contains(text, "выпуск"):
		return documentKindConditional
	case strings.Contains(text, "сгр") || (strings.Contains(text, "государственн") && strings.Contains(text, "регистрац")):
		return documentKindRegistration
	case strings.Contains(text, "сертификат") && strings.Contains(text, "соответств"):
		return documentKindCertificate
	case strings.Contains(text, "деклараци") && strings.Contains(text, "соответств"):
		return documentKindDeclaration
	}
	return documentKindOther
}

// quoteOrigin — где агент видел цитату дословно (без учёта регистра и знаков): в акте этой меры, справке
// tnved.info, другом документе или на странице кода. "" — нигде (цитата выдумана).
func (t *agentToolbox) quoteOrigin(quote, actURL, act string) string {
	normalized := decisions.NormalizeDecisionName(quote)
	if utf8.RuneCountInString(normalized) < agentMinQuoteRunes {
		return ""
	}
	has := func(text string) bool { return containsQuote(text, normalized) }
	if actURL != "" && has(t.web[actURL]) {
		source := firstNonEmpty(act, "акт по ссылке")
		if section := sectionOfQuote(t.web[actURL], normalized); section != "" {
			source += " · " + section
		}
		return source
	}
	addresses := make([]string, 0, len(t.web))
	for address := range t.web {
		addresses = append(addresses, address)
	}
	// Справка tnved.info — первой: её пояснения под мерами и есть основание.
	sort.SliceStable(addresses, func(a, b int) bool {
		return strings.Contains(addresses[a], "tnved.info") && !strings.Contains(addresses[b], "tnved.info")
	})
	for _, address := range addresses {
		if has(t.web[address]) {
			if strings.Contains(address, "tnved.info") {
				return "справка tnved.info"
			}
			return "документ " + shortURL(address)
		}
	}
	codes := make([]string, 0, len(t.opened))
	for code := range t.opened {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		page := t.opened[code]
		switch {
		case !has(page):
		case strings.HasPrefix(page, tksNotesPrefix):
			return "пояснения к коду " + FormatTNVEDCode(code) + " (tks.ru)"
		case has(decisionsBlock(page)):
			return "классификационное решение, код " + FormatTNVEDCode(code) + " (TWS.BY)"
		default:
			return "страница кода " + FormatTNVEDCode(code) + " (TWS.BY)"
		}
	}
	return ""
}

// quoteSource — где цитата есть дословно (без учёта регистра и знаков): в пояснениях к коду TWS.BY
// («page») или резервных с tks.ru («tks»),
// в справке tnved.info («help») или в другом открытом документе («document»).
// Наименование кода, дерево и меры — не доказательство.
func (t *agentToolbox) quoteSource(code, quote string) string {
	normalized := decisions.NormalizeDecisionName(quote)
	// Цифры кода («6116 10 800 0 прочие») ничего не подтверждают: нужны хотя бы три слова.
	words := 0
	for _, word := range strings.Fields(normalized) {
		if strings.IndexFunc(word, unicode.IsLetter) >= 0 {
			words++
		}
	}
	if len([]rune(normalized)) < agentMinQuoteRunes || words < agentMinQuoteWords {
		return ""
	}
	if page, ok := t.opened[code]; ok {
		if strings.HasPrefix(page, tksNotesPrefix) {
			if containsQuote(page, normalized) {
				return "tks"
			}
		} else if containsQuote(notesOf(page), normalized) {
			return "page"
		}
	}
	source := ""
	for address, text := range t.web {
		if containsQuote(text, normalized) {
			if strings.Contains(address, "tnved.info") {
				return "help"
			}
			source = "document"
		}
	}
	return source
}

// antiDumpingNotes — антидемпинговые меры ЕАЭС для кода коротко: агент упоминает их в замечаниях.
func antiDumpingNotes(item directory.NomenclatureCode) []string {
	notes := []string{}
	for _, ref := range item.AntiDumping {
		rates := []string{}
		for _, country := range ref.Countries {
			rates = append(rates, country+" "+ref.Rates[country]+"%")
		}
		note := fmt.Sprintf("%s «%s»: %s", ref.Number, ref.ShortName, strings.Join(rates, ", "))
		if !ref.Exact {
			note += " (мера на часть товаров позиции — сверь описание)"
		}
		if ref.Suspended {
			note += " (временно не применяется)"
		}
		notes = append(notes, note)
	}
	return notes
}
