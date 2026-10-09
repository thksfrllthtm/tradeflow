package tnvedagent

// Инструменты агента подбора кода: все только читают данные (номенклатура, страницы TWS, база
// решений, справочники; в режиме «Документы для ввоза» — справка tnved.info и акты по ссылкам).
// Итог агента (finish) проверяется: коды — только из номенклатуры, цитаты — только дословные, из
// открытой страницы кода, справки или документа либо из наименования кода, ссылки на документы —
// только с открытых страниц.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
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
	// Режим «документы»: начало страницы, ссылок со страницы, фрагментов поиска по документу.
	agentPageRunes       = 6000
	agentPageLinks       = 60
	agentSearchFragments = 5
	agentFragmentRunes   = 1500
	agentMaxDocuments    = 20
)

// agentData — откуда инструменты берут данные; в тестах подменяется.
type agentData interface {
	SearchNomenclature(ctx context.Context, query string, limit int) ([]directory.NomenclatureCode, int64, error)
	LookupCodes(ctx context.Context, codes []string) ([]directory.NomenclatureCode, error)
	DetailsPage(ctx context.Context, code, url string) (string, bool, error)
	SearchDecisions(ctx context.Context, query string, limit int) ([]repository.CodeDecision, error)
	SearchDirectories(ctx context.Context, query string, limit int) ([]repository.DirectoryHint, error)
	GoodsHelp(ctx context.Context, code string) (directory.WebPage, error)
	WebPage(ctx context.Context, url string) (directory.WebPage, error)
}

// repositoryAgentData — настоящие данные из базы и TWS.
type repositoryAgentData struct {
	repository   *repository.Repository
	nomenclature *directory.TNVEDNomenclatureService
	details      *directory.DetailsFetcher
	web          *directory.WebPageFetcher
}

func (d repositoryAgentData) SearchNomenclature(ctx context.Context, query string, limit int) ([]directory.NomenclatureCode, int64, error) {
	return d.nomenclature.List(ctx, query, limit, 0)
}

func (d repositoryAgentData) LookupCodes(ctx context.Context, codes []string) ([]directory.NomenclatureCode, error) {
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

func (d repositoryAgentData) GoodsHelp(ctx context.Context, code string) (directory.WebPage, error) {
	return d.web.GoodsHelp(ctx, code)
}

func (d repositoryAgentData) WebPage(ctx context.Context, url string) (directory.WebPage, error) {
	return d.web.Page(ctx, url)
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// agentTools — инструменты агента; в справочнике (reference) у итога нет наименования для декларации.
// documents — режим «Документы для ввоза»: ещё справка tnved.info и документы по ссылкам, итог — меры.
func agentTools(reference, documents bool) []agentTool {
	tool := func(name, description string, parameters map[string]any) agentTool {
		return agentTool{Type: "function", Function: agentToolFunction{Name: name, Description: description, Parameters: parameters}}
	}
	finish := tool("finish", "Итог подбора. Вызывается один раз в конце.", objectSchema(map[string]any{
		"description": stringProperty("Что это за товар, 1–2 предложения; расхождения наименования с фото"),
		"candidates": map[string]any{"type": "array", "description": "1–4 кандидата, лучший первым", "items": objectSchema(map[string]any{
			"code":    stringProperty("Код, 10 цифр"),
			"verdict": map[string]any{"type": "string", "enum": []string{"fits", "doubtful", "no"}},
			"quote":   stringProperty("Дословная цитата из open_details этого кода или из его наименования; пусто, если нет"),
			"reason":  stringProperty("Почему подходит или не подходит, 1–2 предложения"),
			"check":   stringProperty("Что уточнить; пусто, если нечего"),
		}, "code", "verdict", "reason")},
		"name":       stringProperty("Наименование для декларации по правилу — только русская часть, без перевода на китайский"),
		"notes":      stringProperty("Замечания сотруднику; пусто, если нет"),
		"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Уверенность в лучшем коде"},
		"sources": map[string]any{"type": "array", "description": "Страницы из интернета, на которые опирался (адреса из web_search или web_page); без интернета — пусто", "items": objectSchema(map[string]any{
			"title": stringProperty("Что это за страница: «карточка товара на 1688», «предварительное решение ФТС»"),
			"url":   stringProperty("Адрес страницы"),
		}, "url")},
	}, "description", "candidates", "name", "confidence"))
	if reference {
		finish = tool("finish", "Ответ сотруднику. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description": stringProperty("Ответ сотруднику. На первый вопрос — что это за товар, какой код и главное отличие от соседних, 2–4 предложения. На уточнение — только прямой ответ на него, без пересказа прежнего ответа"),
			"candidates": map[string]any{"type": "array", "description": "1–4 кода: подходящие первыми, затем 1–2 соседних неподходящих (verdict no) с объяснением", "items": objectSchema(map[string]any{
				"code":    stringProperty("Код, 10 цифр"),
				"verdict": map[string]any{"type": "string", "enum": []string{"fits", "doubtful", "no"}},
				"quote":   stringProperty("Дословная цитата из open_details этого кода или из его наименования; пусто, если нет"),
				"reason":  stringProperty("Почему подходит или не подходит, 1–2 предложения"),
				"check":   stringProperty("Что уточнить; пусто, если нечего"),
			}, "code", "verdict", "reason")},
			"notes":      stringProperty("Короткое дополнение, которого нет в description; пусто, если нечего добавить. Не повторяй description"),
			"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Уверенность в лучшем коде"},
			"sources": map[string]any{"type": "array", "description": "Страницы из интернета, на которые опирался (адреса из web_search или web_page); без интернета — пусто", "items": objectSchema(map[string]any{
				"title": stringProperty("Что это за страница: «карточка товара на 1688», «предварительное решение ФТС»"),
				"url":   stringProperty("Адрес страницы"),
			}, "url")},
		}, "description", "candidates", "confidence"))
	}
	extra := []agentTool{}
	if documents {
		finish = tool("finish", "Итог по документам для ввоза. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description": stringProperty("Короткое резюме сотруднику, 2–4 предложения: какие документы и разрешения нужны для ввоза этого товара; если справка недоступна — скажи об этом"),
			"documents": map[string]any{"type": "array", "description": "По одной записи на меру из справки (и из прочитанных актов)", "items": objectSchema(map[string]any{
				"measure":     stringProperty("Вид меры: санитарно-эпидемиологический надзор, ветеринарный контроль, декларация соответствия ТР ТС, лицензия, тарифная квота, экспортный контроль и т. п."),
				"act":         stringProperty("Акт, например «Решение КТС №299 от 28.05.2010»"),
				"url":         stringProperty("Ссылка на акт или документ — только адрес с открытых страниц; пусто, если нет"),
				"requirement": stringProperty("Что конкретно нужно для этого товара, 1–2 предложения; для maybe — что уточнить"),
				"applies":     map[string]any{"type": "string", "enum": []string{"yes", "maybe", "no"}, "description": "Относится ли мера к этому товару по пояснению под актом"},
			}, "measure", "act", "requirement", "applies")},
			"notes": stringProperty("Короткое дополнение; пусто, если нечего добавить"),
		}, "description", "documents"))
		extra = []agentTool{
			tool("open_goods_help", "Справка по товару tnved.info для кода (ввоз в ЕАЭС): акты со ссылками, меры нетарифного регулирования и пояснения, к каким товарам относится мера.",
				objectSchema(map[string]any{"code": stringProperty("Код, 10 цифр")}, "code")),
			tool("open_link", "Открыть ссылку с уже открытой страницы (справки или документа) или адрес tnved.info, tws.by: начало текста страницы и ссылки с неё. Большой документ целиком не отдаётся — ищи в нём через search_in_document.",
				objectSchema(map[string]any{"url": stringProperty("Полный адрес https://…")}, "url")),
			tool("search_in_document", "Найти в документе по ссылке фрагменты со словами запроса (с соседними абзацами). Для больших актов (перечни, единые требования) вместо чтения целиком.",
				objectSchema(map[string]any{
					"url":   stringProperty("Адрес документа — ссылка с открытой страницы"),
					"query": stringProperty("2–5 слов: вид товара, материал, код или группа ТН ВЭД, номер раздела"),
				}, "url", "query")),
		}
	}
	return append([]agentTool{
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
		tool("search_directory", "Справочник товаров компании: наименования, синонимы, код ТН ВЭД и статус проверки.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования")}, "query")),
		tool("ask_user", "Задать сотруднику один короткий вопрос, если без ответа код не определить. После вызова жди ответа.",
			objectSchema(map[string]any{"question": stringProperty("Вопрос по-русски, одно предложение")}, "question")),
		finish,
	}, extra...)
}

// AgentCandidate — код из итога агента после проверки программой.
type AgentCandidate struct {
	Code    string `json:"code"`
	Verdict string `json:"verdict"`
	Quote   string `json:"quote"`
	// page — цитата найдена на открытой странице кода, help — в справке tnved.info или документе по
	// ссылке, name — в наименовании кода; "" — цитаты нет.
	QuoteSource  string                      `json:"quoteSource"`
	Reason       string                      `json:"reason"`
	Check        string                      `json:"check"`
	PageOpened   bool                        `json:"pageOpened"`
	Nomenclature *directory.NomenclatureCode `json:"nomenclature"`
}

type AgentResult struct {
	Description string           `json:"description"`
	Candidates  []AgentCandidate `json:"candidates"`
	Name        string           `json:"name"`
	Notes       string           `json:"notes"`
	// Confidence — уверенность агента в лучшем коде: high, medium, low ("" — не указал).
	Confidence string `json:"confidence,omitempty"`
	// Sources — страницы из интернета, на которые опирался агент; только адреса, которые он видел.
	Sources []AgentSource `json:"sources,omitempty"`
	// Коды, которых нет в номенклатуре, и сколько цитат не нашлось в источниках.
	DroppedCodes  []string `json:"droppedCodes"`
	QuotesRemoved int      `json:"quotesRemoved"`
	// Ответ по кнопке «Документы для ввоза»: для какого кода, меры и сколько чужих ссылок убрано.
	DocumentsCode string          `json:"documentsCode,omitempty"`
	Documents     []AgentDocument `json:"documents,omitempty"`
	LinksRemoved  int             `json:"linksRemoved,omitempty"`
}

type AgentSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// AgentDocument — мера или документ для ввоза. Applies: yes — относится к товару, maybe — зависит от
// признака товара, no — по пояснению под актом мера для других товаров.
type AgentDocument struct {
	Measure     string `json:"measure"`
	Act         string `json:"act"`
	URL         string `json:"url"`
	Requirement string `json:"requirement"`
	Applies     string `json:"applies"`
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

func dutyText(code directory.NomenclatureCode) string {
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

func codeRows(items []directory.NomenclatureCode) []agentCodeRow {
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
	return "«" + service.TruncateRunes(strings.TrimSpace(value), 60) + "»"
}

// agentToolbox выполняет инструменты и помнит открытые страницы (для проверки цитат).
type agentToolbox struct {
	data   agentData
	opened map[string]string
	// web — текст справки и документов, который видел агент (для цитат); links — адреса с открытых
	// страниц (их можно открыть и указать в итоге); docs — полный текст документов этого запуска.
	web   map[string]string
	links map[string]bool
	docs  map[string]string
	// documentsCode — запуск по кнопке «Документы для ввоза» для этого кода ("" — обычный подбор).
	documentsCode string
	// search — поиск в интернете (nil — не подключён); webCalls — обращений за этот запуск.
	search   webSearcher
	webCalls int
}

func newAgentToolbox(data agentData) *agentToolbox {
	return &agentToolbox{data: data, opened: map[string]string{}, web: map[string]string{}, links: map[string]bool{}, docs: map[string]string{}}
}

// rememberPage — агент видел текст страницы и ссылки с неё.
func (t *agentToolbox) rememberPage(address, text string, links []directory.WebLink) {
	t.links[address] = true
	if text != "" {
		t.web[address] = strings.TrimSpace(t.web[address] + "\n" + text)
	}
	for _, link := range links {
		t.links[link.URL] = true
	}
}

// allowedURL — адрес для open_link и search_in_document: ссылка с открытой страницы или tnved.info, tws.by.
func (t *agentToolbox) allowedURL(raw string) (string, string) {
	address, ok := directory.CanonicalWebURL(raw)
	if !ok {
		return "", "Нужен полный адрес https://…"
	}
	if !t.links[address] && !directory.TrustedWebHost(address) {
		return address, "Можно открывать только ссылки с уже открытых страниц (справки или документа) и адреса tnved.info, tws.by."
	}
	return address, ""
}

// webProblem — ответ агенту, если страница не открылась: файл и SPA — ссылку отдать сотруднику.
func webProblem(err error) (answer, detail string) {
	switch {
	case errors.Is(err, directory.ErrWebPageFile):
		return "По ссылке файл (PDF или Word) — программа его не читает. Дай эту ссылку сотруднику.", "файл — откройте по ссылке"
	case errors.Is(err, directory.ErrWebPageScript):
		return "Страница открывается только в браузере — программа её не прочитает. Дай эту ссылку сотруднику.", "читается только в браузере"
	}
	return "Страница не открылась. Опирайся на уже открытые страницы.", "не открылась"
}

// shortURL — адрес для подписи шага: сайт и начало пути.
func shortURL(address string) string {
	return service.TruncateRunes(strings.TrimPrefix(strings.TrimPrefix(address, "https://"), "www."), 70)
}

// documentText — полный текст документа: из этого запуска или через загрузчик (база, сайт).
func (t *agentToolbox) documentText(ctx context.Context, address string) (directory.WebPage, error) {
	if text, ok := t.docs[address]; ok {
		return directory.WebPage{URL: address, Text: text, FromCache: true}, nil
	}
	page, err := t.data.WebPage(ctx, address)
	if err == nil {
		t.docs[address] = page.Text
	}
	return page, err
}

// searchDocument — абзацы с наибольшим числом слов запроса (по основе слова) с соседними абзацами,
// в порядке документа; matches — сколько всего абзацев нашлось.
func searchDocument(text, query string) (fragments []string, matches int) {
	stems := []string{}
	for _, word := range strings.Fields(strings.ToLower(query)) {
		word = strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if utf8.RuneCountInString(word) >= 2 {
			stems = append(stems, repository.SearchStem(word))
		}
	}
	lines := strings.Split(text, "\n")
	type hit struct{ line, score int }
	hits := []hit{}
	for index, line := range lines {
		lower := strings.ToLower(line)
		score := 0
		for _, stem := range stems {
			if strings.Contains(lower, stem) {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, hit{index, score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	chosen := []int{}
	for _, candidate := range hits {
		near := false
		for _, line := range chosen {
			if candidate.line >= line-1 && candidate.line <= line+3 {
				near = true
				break
			}
		}
		if !near {
			chosen = append(chosen, candidate.line)
		}
		if len(chosen) == agentSearchFragments {
			break
		}
	}
	sort.Ints(chosen)
	for _, line := range chosen {
		parts := []string{}
		for index := max(0, line-1); index < min(len(lines), line+3); index++ {
			if index == line {
				parts = append(parts, aroundMatch(lines[index], stems, agentFragmentRunes/2))
			} else {
				parts = append(parts, service.TruncateRunes(lines[index], agentFragmentRunes/6))
			}
		}
		fragments = append(fragments, strings.Join(parts, "\n"))
	}
	return fragments, len(hits)
}

// aroundMatch — длинный абзац (строка таблицы перечня) обрезается вокруг первого слова запроса.
func aroundMatch(line string, stems []string, limit int) string {
	runes := []rune(line)
	if len(runes) <= limit {
		return line
	}
	lower := []rune(strings.ToLower(line))
	first := len(lower)
	for _, stem := range stems {
		if index := strings.Index(string(lower), stem); index >= 0 {
			if position := utf8.RuneCountInString(string(lower)[:index]); position < first {
				first = position
			}
		}
	}
	start := max(0, min(first-limit/4, len(runes)-limit))
	text := string(runes[start : start+limit])
	if start > 0 {
		text = "…" + text
	}
	if start+limit < len(runes) {
		text += "…"
	}
	return text
}

// run выполняет инструмент (кроме ask_user и finish): ответ модели и шаг для чата.
func (t *agentToolbox) run(ctx context.Context, name, arguments string) (string, agentToolEvent) {
	var args struct {
		Query  string   `json:"query"`
		Codes  []string `json:"codes"`
		Prefix string   `json:"prefix"`
		Code   string   `json:"code"`
		URL    string   `json:"url"`
		Focus  string   `json:"focus"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error":"Аргументы не JSON"}`, agentToolEvent{Tool: name, Label: "Ошибка вызова инструмента", Failed: true}
	}
	if name == "web_search" || name == "web_page" {
		return t.runWeb(ctx, name, args.Query, args.Focus, args.URL)
	}
	// Веб-инструменты — только по кнопке «Документы для ввоза»: в обычном подборе их у модели нет.
	if t.documentsCode == "" && (name == "open_goods_help" || name == "open_link" || name == "search_in_document") {
		name = "unknown:" + name
	}
	switch name {
	case "search_nomenclature":
		query := service.TruncateRunes(strings.TrimSpace(args.Query), 100)
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
		found := map[string]directory.NomenclatureCode{}
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
			row := map[string]any{"code": code, "found": true, "name": treeName(item.Name), "duty": dutyText(item), "vat": item.VATPercent + "%", "hasDetails": item.DetailsURL != ""}
			if notes := antiDumpingNotes(item); len(notes) > 0 {
				row["antiDumping"] = notes
			}
			rows = append(rows, row)
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
		query := service.TruncateRunes(strings.TrimSpace(args.Query), 100)
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
				"finalName": row.FinalName, "code": row.Code, "reason": service.TruncateRunes(row.Reason, 300),
			})
		}
		return toolJSON(map[string]any{"examples": examples}), agentToolEvent{Tool: name, Label: "База решений: " + quoted(query), Detail: fmt.Sprintf("примеров %d, проверено %d", len(rows), verified)}
	case "search_directory":
		query := service.TruncateRunes(strings.TrimSpace(args.Query), 100)
		hints, err := t.data.SearchDirectories(ctx, query, agentDirectoryLimit)
		if err != nil {
			return `{"error":"Справочник товаров недоступен"}`, agentToolEvent{Tool: name, Label: "Справочник товаров: " + quoted(query), Detail: "ошибка", Failed: true}
		}
		entries := make([]map[string]any, 0, len(hints))
		for _, hint := range hints {
			entries = append(entries, map[string]any{"kind": hint.Kind, "name": hint.Name, "nameZh": hint.NameZH, "code": hint.Code, "verified": hint.VerificationStatus == "operator_verified"})
		}
		return toolJSON(map[string]any{"entries": entries}), agentToolEvent{Tool: name, Label: "Справочник товаров: " + quoted(query), Detail: fmt.Sprintf("записей %d", len(hints))}
	case "open_goods_help":
		code := nonDigits.ReplaceAllString(args.Code, "")
		label := "Справка по товару " + FormatTNVEDCode(code)
		if items, err := t.data.LookupCodes(ctx, []string{code}); err != nil || len(items) == 0 {
			return toolJSON(map[string]any{"code": code, "error": "Кода нет в номенклатуре"}), agentToolEvent{Tool: name, Label: label, Detail: "кода нет в номенклатуре", Failed: true}
		}
		page, err := t.data.GoodsHelp(ctx, code)
		if errors.Is(err, directory.ErrGoodsHelpLimit) {
			return toolJSON(map[string]any{"code": code, "error": "Лимит запросов к справке tnved.info на сегодня исчерпан. Возьми меры нетарифного регулирования со страницы кода (open_details, TWS.BY), белорусские нормы отбрось и скажи сотруднику, что справка tnved.info сегодня недоступна."}),
				agentToolEvent{Tool: name, Label: label, Detail: "лимит tnved.info на сегодня исчерпан — справку можно открыть в браузере", URL: page.URL, Failed: true}
		}
		if err != nil || page.Text == "" {
			return toolJSON(map[string]any{"code": code, "error": "Справка не открылась. Возьми меры со страницы кода (open_details, TWS.BY), белорусские нормы отбрось."}),
				agentToolEvent{Tool: name, Label: label, Detail: "не открылась", URL: page.URL, Failed: true}
		}
		t.rememberPage(page.URL, page.Text, page.Links)
		detail := "tnved.info"
		if page.FromCache {
			detail = "сохранённая копия"
		}
		return toolJSON(map[string]any{"code": code, "url": page.URL, "text": page.Text, "links": page.Links}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: page.URL}
	case "open_link":
		address, problem := t.allowedURL(args.URL)
		label := "Ссылка " + shortURL(firstNonEmpty(address, args.URL))
		if problem != "" {
			return toolJSON(map[string]any{"url": args.URL, "error": problem}), agentToolEvent{Tool: name, Label: label, Detail: "не с открытой страницы", Failed: true}
		}
		page, err := t.documentText(ctx, address)
		if err != nil {
			answer, detail := webProblem(err)
			return toolJSON(map[string]any{"url": address, "error": answer}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: address, Failed: !errors.Is(err, directory.ErrWebPageFile)}
		}
		if page.Title != "" {
			label = "Документ: " + quoted(page.Title)
		}
		text := service.TruncateRunes(page.Text, agentPageRunes)
		links := page.Links
		if len(links) > agentPageLinks {
			links = links[:agentPageLinks]
		}
		t.rememberPage(address, text, links)
		answer := map[string]any{"url": address, "title": page.Title, "text": text, "links": links}
		total := utf8.RuneCountInString(page.Text)
		detail := fmt.Sprintf("%d символов", total)
		if total > agentPageRunes {
			answer["hint"] = fmt.Sprintf("Показано начало: %d из %d символов. Нужное место найди через search_in_document.", agentPageRunes, total)
			detail += ", показано начало"
		}
		if page.FromCache {
			detail += " · сохранённая копия"
		}
		return toolJSON(answer), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: address}
	case "search_in_document":
		address, problem := t.allowedURL(args.URL)
		query := service.TruncateRunes(strings.TrimSpace(args.Query), 100)
		label := "Поиск в документе: " + quoted(query)
		if problem != "" {
			return toolJSON(map[string]any{"url": args.URL, "error": problem}), agentToolEvent{Tool: name, Label: label, Detail: "не с открытой страницы", Failed: true}
		}
		page, err := t.documentText(ctx, address)
		if err != nil {
			answer, detail := webProblem(err)
			return toolJSON(map[string]any{"url": address, "error": answer}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: address, Failed: !errors.Is(err, directory.ErrWebPageFile)}
		}
		fragments, matches := searchDocument(page.Text, query)
		t.rememberPage(address, strings.Join(fragments, "\n"), nil)
		answer := map[string]any{"url": address, "query": query, "matches": matches, "fragments": fragments}
		if matches == 0 {
			answer["hint"] = "Ничего не найдено. Попробуй другие слова: основу слова, синоним, код или группу ТН ВЭД, номер раздела."
		} else if matches > len(fragments) {
			answer["hint"] = fmt.Sprintf("Показаны %d самых подходящих из %d абзацев — уточни запрос, если нужного нет.", len(fragments), matches)
		}
		return toolJSON(answer), agentToolEvent{Tool: name, Label: label, Detail: fmt.Sprintf("фрагментов %d из %d", len(fragments), matches), URL: address}
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
	Name       string `json:"name"`
	Notes      string `json:"notes"`
	Confidence string `json:"confidence"`
	Sources    []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"sources"`
	Documents []struct {
		Measure     string `json:"measure"`
		Act         string `json:"act"`
		URL         string `json:"url"`
		Requirement string `json:"requirement"`
		Applies     string `json:"applies"`
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
		Description: service.TruncateRunes(strings.TrimSpace(args.Description), 1000), Name: service.TruncateRunes(russianNamePart(args.Name), maxRussianNameRunes),
		Notes: service.TruncateRunes(strings.TrimSpace(args.Notes), 1000), Candidates: []AgentCandidate{}, DroppedCodes: []string{},
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
		quote := service.TruncateRunes(strings.TrimSpace(strings.Trim(candidate.Quote, "«»\"' ")), 600)
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
			Reason: service.TruncateRunes(strings.TrimSpace(candidate.Reason), 600), Check: service.TruncateRunes(strings.TrimSpace(candidate.Check), 400),
		})
		if len(result.Candidates) == agentMaxCandidates {
			break
		}
	}
	if len(result.Candidates) == 0 {
		return nil, "Ни одного кода из номенклатуры: проверь коды через get_codes или search_nomenclature и вызови finish ещё раз.", nil
	}
	if args.Confidence == "high" || args.Confidence == "medium" || args.Confidence == "low" {
		result.Confidence = args.Confidence
	}
	// Источник — только страница, которую агент действительно видел в этом подборе.
	for _, source := range args.Sources {
		address := strings.TrimSpace(source.URL)
		if t.links[address] && len(result.Sources) < 5 {
			result.Sources = append(result.Sources, AgentSource{Title: service.TruncateRunes(strings.TrimSpace(source.Title), 200), URL: address})
		}
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
	for _, document := range args.Documents {
		item := AgentDocument{
			Measure: service.TruncateRunes(strings.TrimSpace(document.Measure), 200), Act: service.TruncateRunes(strings.TrimSpace(document.Act), 300),
			Requirement: service.TruncateRunes(strings.TrimSpace(document.Requirement), 600), Applies: document.Applies,
		}
		if item.Measure == "" && item.Act == "" {
			continue
		}
		if item.Applies != "yes" && item.Applies != "maybe" && item.Applies != "no" {
			item.Applies = "maybe"
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
	return result, ""
}

// quoteSource ищет цитату дословно (без учёта регистра и знаков препинания) на открытой странице
// кода, затем в наименовании кода.
func (t *agentToolbox) quoteSource(code, name, quote string) string {
	normalized := decisions.NormalizeDecisionName(quote)
	// Цифры кода («6116 10 800 0 прочие») ничего не подтверждают: нужны хотя бы три слова.
	words := 0
	for _, word := range strings.Fields(normalized) {
		if strings.IndexFunc(word, unicode.IsLetter) >= 0 {
			words++
		}
	}
	if len([]rune(normalized)) < agentMinQuoteRunes || words < agentMinQuoteWords {
		// Слишком короткая цитата («прочие») ничего не подтверждает, если это не наименование кода.
		if normalized != "" && strings.Contains(decisions.NormalizeDecisionName(name), normalized) {
			return "name"
		}
		return ""
	}
	if page, ok := t.opened[code]; ok && strings.Contains(decisions.NormalizeDecisionName(page), normalized) {
		return "page"
	}
	for _, text := range t.web {
		if strings.Contains(decisions.NormalizeDecisionName(text), normalized) {
			return "help"
		}
	}
	if strings.Contains(decisions.NormalizeDecisionName(name), normalized) {
		return "name"
	}
	return ""
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
