package service

// Инструменты агента подбора кода: все только читают данные (номенклатура, страницы TWS, база
// решений, справочники). Итог агента (finish) проверяется: коды — только из номенклатуры,
// цитаты — только дословные, из открытой страницы кода или из его наименования.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

const (
	agentSearchLimit    = 30
	agentHeadingLimit   = 80
	agentGetCodesLimit  = 10
	agentDecisionsLimit = 10
	agentDirectoryLimit = 8
	agentMaxCandidates  = 5
	agentMinQuoteRunes  = 12
	agentMinQuoteWords  = 3
)

// agentData — откуда инструменты берут данные; в тестах подменяется.
type agentData interface {
	SearchNomenclature(ctx context.Context, query string, limit int) ([]NomenclatureCode, int64, error)
	LookupCodes(ctx context.Context, codes []string) ([]NomenclatureCode, error)
	DetailsPage(ctx context.Context, code, url string) (string, bool, error)
	SearchDecisions(ctx context.Context, query string, limit int) ([]repository.CodeDecision, error)
	SearchDirectories(ctx context.Context, query string, limit int) ([]repository.DirectoryHint, error)
}

// repositoryAgentData — настоящие данные из базы и TWS.
type repositoryAgentData struct {
	repository   *repository.Repository
	nomenclature *TNVEDNomenclatureService
	details      *DetailsFetcher
}

func (d repositoryAgentData) SearchNomenclature(ctx context.Context, query string, limit int) ([]NomenclatureCode, int64, error) {
	return d.nomenclature.List(ctx, query, limit, 0)
}

func (d repositoryAgentData) LookupCodes(ctx context.Context, codes []string) ([]NomenclatureCode, error) {
	return d.nomenclature.Lookup(ctx, codes)
}

func (d repositoryAgentData) DetailsPage(ctx context.Context, code, url string) (string, bool, error) {
	return d.details.Page(ctx, code, url)
}

func (d repositoryAgentData) SearchDecisions(ctx context.Context, query string, limit int) ([]repository.CodeDecision, error) {
	return d.repository.SearchCodeDecisions(ctx, query, limit)
}

func (d repositoryAgentData) SearchDirectories(ctx context.Context, query string, limit int) ([]repository.DirectoryHint, error) {
	return d.repository.SearchDirectoriesForAgent(ctx, query, limit)
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func agentTools() []agentTool {
	tool := func(name, description string, parameters map[string]any) agentTool {
		return agentTool{Type: "function", Function: agentToolFunction{Name: name, Description: description, Parameters: parameters}}
	}
	return []agentTool{
		tool("search_nomenclature", "Поиск кодов в номенклатуре ТН ВЭД ЕАЭС по словам наименования (по-русски). Каждое слово ищется по основе, все слова должны встретиться в наименовании кода с его деревом. Лучше 1–3 слова. Возвращает до 30 кодов с наименованием и пошлиной ЕТТ.",
			objectSchema(map[string]any{"query": stringProperty("Слова для поиска, например «перчатки трикотажные»")}, "query")),
		tool("get_codes", "Проверить конкретные коды: есть ли в номенклатуре, наименование с деревом, пошлина ЕТТ, НДС.",
			objectSchema(map[string]any{"codes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "До 10 кодов по 10 цифр"}}, "codes")),
		tool("list_heading", "Все коды, начинающиеся с данных цифр (4–8 цифр): соседние коды товарной позиции или субпозиции, включая «прочие».",
			objectSchema(map[string]any{"prefix": stringProperty("Начало кода, 4–8 цифр, например «6116» или «611610»")}, "prefix")),
		tool("open_details", "Открыть страницу кода (TWS.BY): дерево кода, дополнительная единица, обязательные сведения, пояснения и примечания к ТН ВЭД, меры нетарифного регулирования. Цитаты для итога бери отсюда.",
			objectSchema(map[string]any{"code": stringProperty("Код, 10 цифр")}, "code")),
		tool("search_decisions", "База решений компании: товары, которые раньше оформляли, с итоговым кодом и наименованием; проверенные администратором — первыми.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования (по-русски или по-китайски)")}, "query")),
		tool("search_directory", "Справочники компании (справочник товаров и справочник ТН ВЭД): наименования, синонимы, коды и статус проверки.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования")}, "query")),
		tool("ask_user", "Задать сотруднику один короткий вопрос, если без ответа код не определить. После вызова жди ответа.",
			objectSchema(map[string]any{"question": stringProperty("Вопрос по-русски, одно предложение")}, "question")),
		tool("finish", "Итог подбора. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description": stringProperty("Что это за товар, 1–2 предложения; расхождения наименования с фото"),
			"candidates": map[string]any{"type": "array", "description": "1–4 кандидата, лучший первым", "items": objectSchema(map[string]any{
				"code":    stringProperty("Код, 10 цифр"),
				"verdict": map[string]any{"type": "string", "enum": []string{"fits", "doubtful", "no"}},
				"quote":   stringProperty("Дословная цитата из open_details этого кода или из его наименования; пусто, если нет"),
				"reason":  stringProperty("Почему подходит или не подходит, 1–2 предложения"),
				"check":   stringProperty("Что уточнить; пусто, если нечего"),
			}, "code", "verdict", "reason")},
			"name":  stringProperty("Наименование для декларации по правилу: «русское / твой перевод на китайский»"),
			"notes": stringProperty("Замечания сотруднику; пусто, если нет"),
		}, "description", "candidates", "name")),
	}
}

// AgentCandidate — код из итога агента после проверки программой.
type AgentCandidate struct {
	Code    string `json:"code"`
	Verdict string `json:"verdict"`
	Quote   string `json:"quote"`
	// page — цитата найдена на открытой странице кода, name — в наименовании кода; "" — цитаты нет.
	QuoteSource  string            `json:"quoteSource"`
	Reason       string            `json:"reason"`
	Check        string            `json:"check"`
	PageOpened   bool              `json:"pageOpened"`
	Nomenclature *NomenclatureCode `json:"nomenclature"`
}

type AgentResult struct {
	Description string           `json:"description"`
	Candidates  []AgentCandidate `json:"candidates"`
	Name        string           `json:"name"`
	Notes       string           `json:"notes"`
	// Коды, которых нет в номенклатуре, и сколько цитат не нашлось в источниках.
	DroppedCodes  []string `json:"droppedCodes"`
	QuotesRemoved int      `json:"quotesRemoved"`
}

// Best — первый подходящий код (или первый не «не подходит»).
func (r *AgentResult) Best() *AgentCandidate {
	if r == nil {
		return nil
	}
	for index := range r.Candidates {
		if r.Candidates[index].Verdict == "fits" {
			return &r.Candidates[index]
		}
	}
	for index := range r.Candidates {
		if r.Candidates[index].Verdict != "no" {
			return &r.Candidates[index]
		}
	}
	return nil
}

// agentToolEvent — шаг агента для чата: значок, подпись, подробность и ссылка.
type agentToolEvent struct {
	Tool   string `json:"tool"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	URL    string `json:"url,omitempty"`
	Failed bool   `json:"failed,omitempty"`
}

var nonDigits = regexp.MustCompile(`\D`)

// FormatTNVEDCode: «6116102000» → «6116 10 200 0».
func FormatTNVEDCode(code string) string {
	if len(code) != 10 {
		return code
	}
	return code[:4] + " " + code[4:6] + " " + code[6:9] + " " + code[9:]
}

func treeName(name string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(name, "🠺", "›")), " ")
}

func dutyText(code NomenclatureCode) string {
	if text := strings.TrimSpace(code.TariffText); text != "" {
		return text
	}
	return "не указана"
}

type agentCodeRow struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Duty string `json:"duty"`
}

func codeRows(items []NomenclatureCode) []agentCodeRow {
	rows := make([]agentCodeRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, agentCodeRow{Code: item.Code, Name: treeName(item.Name), Duty: dutyText(item)})
	}
	return rows
}

func toolJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `{"error":"internal"}`
	}
	return string(data)
}

func quoted(value string) string {
	return "«" + truncateRunes(strings.TrimSpace(value), 60) + "»"
}

// agentToolbox выполняет инструменты и помнит открытые страницы (для проверки цитат).
type agentToolbox struct {
	data   agentData
	opened map[string]string
}

func newAgentToolbox(data agentData) *agentToolbox {
	return &agentToolbox{data: data, opened: map[string]string{}}
}

// run выполняет инструмент (кроме ask_user и finish): ответ модели и шаг для чата.
func (t *agentToolbox) run(ctx context.Context, name, arguments string) (string, agentToolEvent) {
	var args struct {
		Query  string   `json:"query"`
		Codes  []string `json:"codes"`
		Prefix string   `json:"prefix"`
		Code   string   `json:"code"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error":"Аргументы не JSON"}`, agentToolEvent{Tool: name, Label: "Ошибка вызова инструмента", Failed: true}
	}
	switch name {
	case "search_nomenclature":
		query := truncateRunes(strings.TrimSpace(args.Query), 100)
		items, total, err := t.data.SearchNomenclature(ctx, query, agentSearchLimit)
		if err != nil {
			return `{"error":"Поиск недоступен"}`, agentToolEvent{Tool: name, Label: "Номенклатура: " + quoted(query), Detail: "ошибка поиска", Failed: true}
		}
		answer := map[string]any{"total": total, "codes": codeRows(items)}
		if total == 0 {
			answer["hint"] = "Ничего не найдено. Попробуй меньше слов или другие: основу слова, синоним, материал, назначение."
		} else if total > int64(len(items)) {
			answer["hint"] = fmt.Sprintf("Показаны первые %d из %d — уточни запрос или смотри list_heading.", len(items), total)
		}
		return toolJSON(answer), agentToolEvent{Tool: name, Label: "Номенклатура: " + quoted(query), Detail: fmt.Sprintf("найдено %d", total)}
	case "get_codes":
		codes := make([]string, 0, len(args.Codes))
		for _, code := range args.Codes {
			if normalized := nonDigits.ReplaceAllString(code, ""); len(normalized) == 10 && len(codes) < agentGetCodesLimit {
				codes = append(codes, normalized)
			}
		}
		items, err := t.data.LookupCodes(ctx, codes)
		if err != nil {
			return `{"error":"Номенклатура недоступна"}`, agentToolEvent{Tool: name, Label: "Проверка кодов", Detail: "ошибка", Failed: true}
		}
		found := map[string]NomenclatureCode{}
		for _, item := range items {
			found[item.Code] = item
		}
		rows := make([]map[string]any, 0, len(codes))
		for _, code := range codes {
			item, ok := found[code]
			if !ok {
				rows = append(rows, map[string]any{"code": code, "found": false})
				continue
			}
			rows = append(rows, map[string]any{"code": code, "found": true, "name": treeName(item.Name), "duty": dutyText(item), "vat": item.VATPercent + "%", "hasDetails": item.DetailsURL != ""})
		}
		formatted := make([]string, 0, len(codes))
		for _, code := range codes {
			formatted = append(formatted, FormatTNVEDCode(code))
		}
		return toolJSON(map[string]any{"codes": rows}), agentToolEvent{Tool: name, Label: "Проверка кодов: " + strings.Join(formatted, ", "), Detail: fmt.Sprintf("в номенклатуре %d из %d", len(found), len(codes))}
	case "list_heading":
		prefix := nonDigits.ReplaceAllString(args.Prefix, "")
		if len(prefix) < 4 || len(prefix) > 8 {
			return `{"error":"Нужно 4–8 цифр"}`, agentToolEvent{Tool: name, Label: "Коды позиции", Detail: "неверное начало кода", Failed: true}
		}
		items, total, err := t.data.SearchNomenclature(ctx, prefix, agentHeadingLimit)
		if err != nil {
			return `{"error":"Номенклатура недоступна"}`, agentToolEvent{Tool: name, Label: "Коды позиции " + prefix, Detail: "ошибка", Failed: true}
		}
		answer := map[string]any{"total": total, "codes": codeRows(items)}
		if total > int64(len(items)) {
			answer["hint"] = fmt.Sprintf("Показаны первые %d из %d — возьми начало кода длиннее.", len(items), total)
		}
		return toolJSON(answer), agentToolEvent{Tool: name, Label: "Коды позиции " + prefix, Detail: fmt.Sprintf("%d кодов", total)}
	case "open_details":
		code := nonDigits.ReplaceAllString(args.Code, "")
		label := "Страница " + FormatTNVEDCode(code)
		items, err := t.data.LookupCodes(ctx, []string{code})
		if err != nil || len(items) == 0 {
			return toolJSON(map[string]any{"code": code, "error": "Кода нет в номенклатуре"}), agentToolEvent{Tool: name, Label: label, Detail: "кода нет в номенклатуре", Failed: true}
		}
		if items[0].DetailsURL == "" {
			return toolJSON(map[string]any{"code": code, "error": "У кода нет страницы"}), agentToolEvent{Tool: name, Label: label, Detail: "у кода нет страницы", Failed: true}
		}
		text, cached, err := t.data.DetailsPage(ctx, code, items[0].DetailsURL)
		if err != nil || text == "" {
			return toolJSON(map[string]any{"code": code, "error": "Страница не открылась; опирайся на наименование кода"}), agentToolEvent{Tool: name, Label: label, Detail: "не открылась", URL: items[0].DetailsURL, Failed: true}
		}
		t.opened[code] = text
		detail := "с TWS.BY"
		if cached {
			detail = "сохранённая копия"
		}
		return toolJSON(map[string]any{"code": code, "text": text}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: items[0].DetailsURL}
	case "search_decisions":
		query := truncateRunes(strings.TrimSpace(args.Query), 100)
		rows, err := t.data.SearchDecisions(ctx, query, agentDecisionsLimit)
		if err != nil {
			return `{"error":"База недоступна"}`, agentToolEvent{Tool: name, Label: "База решений: " + quoted(query), Detail: "ошибка", Failed: true}
		}
		examples := make([]map[string]any, 0, len(rows))
		verified := 0
		for _, row := range rows {
			if row.Status == "verified" {
				verified++
			}
			examples = append(examples, map[string]any{
				"verified": row.Status == "verified", "invoiceName": row.InvoiceName, "supplierName": row.SupplierName,
				"finalName": row.FinalName, "code": row.Code, "reason": truncateRunes(row.Reason, 300),
			})
		}
		return toolJSON(map[string]any{"examples": examples}), agentToolEvent{Tool: name, Label: "База решений: " + quoted(query), Detail: fmt.Sprintf("примеров %d, проверено %d", len(rows), verified)}
	case "search_directory":
		query := truncateRunes(strings.TrimSpace(args.Query), 100)
		hints, err := t.data.SearchDirectories(ctx, query, agentDirectoryLimit)
		if err != nil {
			return `{"error":"Справочники недоступны"}`, agentToolEvent{Tool: name, Label: "Справочники: " + quoted(query), Detail: "ошибка", Failed: true}
		}
		entries := make([]map[string]any, 0, len(hints))
		for _, hint := range hints {
			entries = append(entries, map[string]any{"kind": hint.Kind, "name": hint.Name, "nameZh": hint.NameZH, "code": hint.Code, "verified": hint.VerificationStatus == "verified"})
		}
		return toolJSON(map[string]any{"entries": entries}), agentToolEvent{Tool: name, Label: "Справочники: " + quoted(query), Detail: fmt.Sprintf("записей %d", len(hints))}
	}
	return `{"error":"Нет такого инструмента"}`, agentToolEvent{Tool: name, Label: "Неизвестный инструмент " + name, Failed: true}
}

// finishArgs — аргументы finish как их прислала модель.
type finishArgs struct {
	Description string `json:"description"`
	Candidates  []struct {
		Code    string `json:"code"`
		Verdict string `json:"verdict"`
		Quote   string `json:"quote"`
		Reason  string `json:"reason"`
		Check   string `json:"check"`
	} `json:"candidates"`
	Name  string `json:"name"`
	Notes string `json:"notes"`
}

// finish проверяет итог агента. problem — что исправить (модели дают одну попытку).
func (t *agentToolbox) finish(ctx context.Context, arguments string) (*AgentResult, string, error) {
	var args finishArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil, "Аргументы finish — не JSON по схеме", nil
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
	byCode := map[string]NomenclatureCode{}
	for _, item := range items {
		byCode[item.Code] = item
	}
	result := &AgentResult{
		Description: truncateRunes(strings.TrimSpace(args.Description), 1000), Name: truncateRunes(strings.TrimSpace(args.Name), 300),
		Notes: truncateRunes(strings.TrimSpace(args.Notes), 1000), Candidates: []AgentCandidate{}, DroppedCodes: []string{},
	}
	used := map[string]bool{}
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
		quote := truncateRunes(strings.TrimSpace(strings.Trim(candidate.Quote, "«»\"' ")), 600)
		source := ""
		if quote != "" {
			source = t.quoteSource(code, item.Name, quote)
			if source == "" {
				quote = ""
				result.QuotesRemoved++
			}
		}
		_, opened := t.opened[code]
		copyItem := item
		result.Candidates = append(result.Candidates, AgentCandidate{
			Code: code, Verdict: verdict, Quote: quote, QuoteSource: source, PageOpened: opened, Nomenclature: &copyItem,
			Reason: truncateRunes(strings.TrimSpace(candidate.Reason), 600), Check: truncateRunes(strings.TrimSpace(candidate.Check), 400),
		})
		if len(result.Candidates) == agentMaxCandidates {
			break
		}
	}
	if len(result.Candidates) == 0 {
		return nil, "Ни одного кода из номенклатуры: проверь коды через get_codes или search_nomenclature и вызови finish ещё раз.", nil
	}
	return result, "", nil
}

// quoteSource ищет цитату дословно (без учёта регистра и знаков препинания) на открытой странице
// кода, затем в наименовании кода.
func (t *agentToolbox) quoteSource(code, name, quote string) string {
	normalized := normalizeDecisionName(quote)
	// Цифры кода («6116 10 800 0 прочие») ничего не подтверждают: нужны хотя бы три слова.
	words := 0
	for _, word := range strings.Fields(normalized) {
		if strings.IndexFunc(word, unicode.IsLetter) >= 0 {
			words++
		}
	}
	if len([]rune(normalized)) < agentMinQuoteRunes || words < agentMinQuoteWords {
		// Слишком короткая цитата («прочие») ничего не подтверждает, если это не наименование кода.
		if normalized != "" && strings.Contains(normalizeDecisionName(name), normalized) {
			return "name"
		}
		return ""
	}
	if page, ok := t.opened[code]; ok && strings.Contains(normalizeDecisionName(page), normalized) {
		return "page"
	}
	if strings.Contains(normalizeDecisionName(name), normalized) {
		return "name"
	}
	return ""
}
