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
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/utilfee"
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
	// Вопросов модели для фото (look_at_photo) за запуск.
	agentMaxPhotoQuestions = 3
)

// agentData — откуда инструменты берут данные; в тестах подменяется.
type agentData interface {
	SearchNomenclature(ctx context.Context, query string, limit int) ([]directory.NomenclatureCode, int64, error)
	LookupCodes(ctx context.Context, codes []string) ([]directory.NomenclatureCode, error)
	DetailsPage(ctx context.Context, code, url string) (string, bool, error)
	// TKSNotes — резерв: пояснения к группе и позиции с tks.ru, когда страницы TWS нет или она не открылась.
	TKSNotes(ctx context.Context, code string) (string, bool, error)
	SearchDecisions(ctx context.Context, query string, limit int) ([]repository.CodeDecision, error)
	SearchDirectories(ctx context.Context, query string, limit int) ([]repository.DirectoryHint, error)
	GoodsHelp(ctx context.Context, code string) (directory.WebPage, error)
	// GeneralActs — общая база «Документов для ввоза»: перечни СС/ДС и Решение КТС №299.
	GeneralActs(ctx context.Context) ([]directory.WebLink, error)
	WebPage(ctx context.Context, url string) (directory.WebPage, error)
	// ExtractedPage — страница текстом внешнего сервиса (extract вызывается, только если её нет в базе).
	ExtractedPage(ctx context.Context, url string, extract func(context.Context, string) (string, error)) (directory.WebPage, error)
	// Declarations — примеры декларирования (статистика графы 31): по словам или внутри позиции/кода.
	Declarations(ctx context.Context, query, within string) ([]directory.DeclarationSource, bool, error)
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

func (d repositoryAgentData) TKSNotes(ctx context.Context, code string) (string, bool, error) {
	return d.web.TKSNotes(ctx, code)
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

func (d repositoryAgentData) GeneralActs(ctx context.Context) ([]directory.WebLink, error) {
	return d.web.GeneralActs(ctx)
}

func (d repositoryAgentData) WebPage(ctx context.Context, url string) (directory.WebPage, error) {
	return d.web.Page(ctx, url)
}

func (d repositoryAgentData) Declarations(ctx context.Context, query, within string) ([]directory.DeclarationSource, bool, error) {
	return d.web.Declarations(ctx, query, within)
}

func (d repositoryAgentData) ExtractedPage(ctx context.Context, url string, extract func(context.Context, string) (string, error)) (directory.WebPage, error) {
	return d.web.Extracted(ctx, url, extract)
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
			"code":          stringProperty("Код, 10 цифр"),
			"verdict":       map[string]any{"type": "string", "enum": []string{"fits", "doubtful", "no"}},
			"quote":         stringProperty("Дословная цитата из пояснений к ТН ВЭД на странице open_details этого кода; не из наименования и дерева; пусто, если нет"),
			"decisionQuote": stringProperty(decisionQuoteHint),
			"reason":        stringProperty("Почему подходит или не подходит, 1–2 предложения"),
			"check":         stringProperty("Что уточнить; пусто, если нечего"),
		}, "code", "verdict", "reason")},
		"name":             stringProperty("Наименование для декларации по правилу — только русская часть, без перевода на китайский"),
		"notes":            stringProperty("Замечания сотруднику; пусто, если нет"),
		"confidence":       map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Уверенность в лучшем коде"},
		"confidenceReason": stringProperty("Почему такая уверенность, одна фраза: что подтверждает код (текст позиции, дерево, пояснения, база компании, декларации) и что остаётся под вопросом"),
		"sources": map[string]any{"type": "array", "description": "Страницы из интернета, на которые опирался (адреса из web_search или web_page); без интернета — пусто", "items": objectSchema(map[string]any{
			"title": stringProperty("Что это за страница: «предварительное решение ФТС», «разъяснение ЕЭК»"),
			"url":   stringProperty("Адрес страницы"),
		}, "url")},
	}, "description", "candidates", "name", "confidence"))
	if reference {
		finish = tool("finish", "Ответ сотруднику. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description": stringProperty("Ответ сотруднику. На первый вопрос — что это за товар, какой код и главное отличие от соседних, 2–4 предложения. На уточнение — только прямой ответ на него, без пересказа прежнего ответа"),
			"candidates": map[string]any{"type": "array", "description": "1–4 кода: подходящие первыми, затем 1–2 соседних неподходящих (verdict no) с объяснением", "items": objectSchema(map[string]any{
				"code":          stringProperty("Код, 10 цифр"),
				"verdict":       map[string]any{"type": "string", "enum": []string{"fits", "doubtful", "no"}},
				"quote":         stringProperty("Дословная цитата из пояснений к ТН ВЭД на странице open_details этого кода; не из наименования и дерева; пусто, если нет"),
				"decisionQuote": stringProperty(decisionQuoteHint),
				"reason":        stringProperty("Почему подходит или не подходит, 1–2 предложения"),
				"check":         stringProperty("Что уточнить; пусто, если нечего"),
			}, "code", "verdict", "reason")},
			"name":             stringProperty("Краткое наименование для декларации по правилу — только русская часть, без перевода на китайский"),
			"nameFull":         stringProperty("Подробное наименование для декларации: краткое плюс признаки, от которых зависит код (материал, устройство, назначение, комплектность), до 200 символов; только русская часть"),
			"notes":            stringProperty("Короткое дополнение, которого нет в description; пусто, если нечего добавить. Не повторяй description"),
			"confidence":       map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Уверенность в лучшем коде"},
			"confidenceReason": stringProperty("Почему такая уверенность, одна фраза: что подтверждает код (текст позиции, дерево, пояснения, база компании, декларации) и что остаётся под вопросом"),
			"sources": map[string]any{"type": "array", "description": "Страницы из интернета, на которые опирался (адреса из web_search или web_page); без интернета — пусто", "items": objectSchema(map[string]any{
				"title": stringProperty("Что это за страница: «предварительное решение ФТС», «разъяснение ЕЭК»"),
				"url":   stringProperty("Адрес страницы"),
			}, "url")},
		}, "description", "candidates", "name", "nameFull", "confidence"))
	}
	extra := []agentTool{}
	if documents {
		finish = tool("finish", "Итог по документам для ввоза. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description": stringProperty("Короткое резюме сотруднику, 2–4 предложения: какие документы и разрешения нужны для ввоза этого товара; если справка недоступна — скажи об этом"),
			"documents": map[string]any{"type": "array", "description": "По одной записи на меру из справки (и из прочитанных актов), плюс одна запись conditional — условный выпуск", "items": objectSchema(map[string]any{
				"kind":        map[string]any{"type": "string", "enum": []string{"cc", "dc", "sgr", "conditional", "other"}, "description": "Вид документа: cc — сертификат соответствия (СС), dc — декларация соответствия (ДС), sgr — свидетельство о государственной регистрации (СГР), conditional — условный выпуск, other — прочие меры (санэпиднадзор без СГР, лицензия, квота, ветеринарный контроль…)"},
				"measure":     stringProperty("Вид меры полностью, с большой буквы, без сокращений: для техрегламента — номер и название в кавычках («Сертификат соответствия ТР ТС 010/2011 «О безопасности машин и оборудования»», «Декларация соответствия ТР ТС 019/2011 «О безопасности средств индивидуальной защиты»»); для санэпиднадзора — раздел Решения КТС №299 и нужно ли СГР («Санитарно-эпидемиологический надзор (раздел I, без СГР)»); прочие — «Ветеринарный контроль», «Условный выпуск», «Лицензия», «Тарифная квота», «Экспортный контроль»"),
				"act":         stringProperty("Акт, например «Решение КТС №299 от 28.05.2010»"),
				"url":         stringProperty("Ссылка на акт или документ — только адрес с открытых страниц; пусто, если нет"),
				"requirement": stringProperty("Что конкретно нужно для этого товара, 1–2 предложения; для maybe — что уточнить"),
				"applies":     map[string]any{"type": "string", "enum": []string{"yes", "maybe", "no"}, "description": "Относится ли мера к этому товару по пояснению под актом. Зависит от назначения товара (контакт с пищей, для детей, бытовое) — всегда yes и nameHint, даже если в наименовании назначение другое; решает декларант"},
				"basis":       stringProperty("Дословная цитата (5–40 слов) из справки tnved.info, акта или страницы кода, на которой основан вывод applies: пояснение под мерой («Относится к товарам: …») или пункт перечня. Программа проверит цитату; без неё «нужно» не принимается"),
				"nameHint":    stringProperty("Если мера зависит от признака товара (бытовое или промышленное назначение, контакт с пищей, напряжение, детский товар): при каком реальном признаке она не применяется, какое наименование тогда указать в декларации и какими документами поставщика это подтвердить. Пусто, если от наименования не зависит"),
			}, "kind", "measure", "act", "requirement", "applies", "basis")},
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
		tool("open_details", "Открыть страницу кода (TWS.BY) целиком: дерево кода, дополнительная единица, обязательные сведения, меры нетарифного регулирования, пояснения к ТН ВЭД — к позиции, общие положения группы и раздела (что включается и что исключается). Если страницы TWS.BY нет — пояснения к группе и позиции с tks.ru (без дерева). Цитаты для итога бери отсюда.",
			objectSchema(map[string]any{"code": stringProperty("Код, 10 цифр")}, "code")),
		tool("search_decisions", "База решений компании: товары, которые раньше оформляли, с итоговым кодом и наименованием; проверенные администратором — первыми.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования (по-русски или по-китайски)")}, "query")),
		tool("search_directory", "Справочник товаров компании: наименования, синонимы, код ТН ВЭД и статус проверки.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования")}, "query")),
		tool("search_declarations", "Примеры декларирования: как такой товар уже декларировали — обезличенные описания из деклараций (графа 31), неофициальные, сразу с нескольких сайтов. Без within: ifcg.ru — товарные позиции с числом записей и описаниями, tks.ru — коды с числом записей и описаниями. within 4 цифры — коды внутри позиции (ifcg.ru); within 10 цифр — описания товаров под кодом (ifcg.ru). Обязательно перед итогом.",
			objectSchema(map[string]any{
				"query":  stringProperty("1–3 слова вида товара по-русски, например «ключ газовый», или 10-значный код"),
				"within": stringProperty("Пусто, товарная позиция (4 цифры) или код (10 цифр) — искать внутри"),
			}, "query")),
	}, append(askUserTools(reference), append([]agentTool{finish}, extra...)...)...)
}

// askUserTools — вопрос сотруднику есть только в подборе для строки сметы: в справочнике сотрудник ждёт
// ответ сразу, а что уточнить, агент пишет в check.
func askUserTools(reference bool) []agentTool {
	if reference {
		return nil
	}
	return []agentTool{{Type: "function", Function: agentToolFunction{Name: "ask_user", Description: "Задать сотруднику один короткий вопрос, если без ответа код не определить. После вызова жди ответа.",
		Parameters: objectSchema(map[string]any{"question": stringProperty("Вопрос по-русски, одно предложение")}, "question")}}}
}

// lookPhotoTool — вопрос модели для фото: есть, только когда модель рассуждений фото не видит.
func lookPhotoTool() agentTool {
	return agentTool{Type: "function", Function: agentToolFunction{
		Name:        "look_at_photo",
		Description: fmt.Sprintf("Спросить модель, которая видит фото товара: один конкретный вопрос о том, что видно на фото (материал, механизм, надписи и маркировка, комплектность, детали). Ответ — текстом. До %d вопросов за подбор.", agentMaxPhotoQuestions),
		Parameters:  objectSchema(map[string]any{"question": stringProperty("Вопрос по-русски, например «Есть ли у ножниц пружина между ручками?»")}, "question"),
	}}
}

// AgentCandidate — код из итога агента после проверки программой.
type AgentCandidate struct {
	Code    string `json:"code"`
	Verdict string `json:"verdict"`
	Quote   string `json:"quote"`
	// page — цитата найдена на открытой странице кода, help — в справке tnved.info или документе по
	// ссылке, name — в наименовании кода; "" — цитаты нет.
	QuoteSource string `json:"quoteSource"`
	// DecisionQuote — второе доказательство после пояснений: дословно из классификационного решения на
	// странице кода; DecisionSource — вид решения и страна («Предварительное · RU - Россия»).
	DecisionQuote  string                      `json:"decisionQuote,omitempty"`
	DecisionSource string                      `json:"decisionSource,omitempty"`
	Reason         string                      `json:"reason"`
	Check          string                      `json:"check"`
	PageOpened     bool                        `json:"pageOpened"`
	Nomenclature   *directory.NomenclatureCode `json:"nomenclature"`
	// Declarations — описания из деклараций под кодом (nil — агент примеры декларирования не искал).
	Declarations *AgentDeclarations `json:"declarations,omitempty"`
}

// AgentDeclarations — поиск ifcg.ru внутри кода по словам агента; Count — сколько записей под кодом
// нашлось в его поиске (0 — код в найденном не встретился).
type AgentDeclarations struct {
	URL   string `json:"url"`
	Query string `json:"query"`
	Count int    `json:"count,omitempty"`
}

type AgentResult struct {
	Description string           `json:"description"`
	Candidates  []AgentCandidate `json:"candidates"`
	Name        string           `json:"name"`
	// NameFull — подробный вариант наименования (только в справочнике; Name там — краткий).
	NameFull string `json:"nameFull,omitempty"`
	Notes    string `json:"notes"`
	// Confidence — уверенность агента в лучшем коде: high, medium, low ("" — не указал).
	Confidence string `json:"confidence,omitempty"`
	// ConfidenceReason — почему такая уверенность: что подтверждает код и что под вопросом, одна фраза.
	ConfidenceReason string `json:"confidenceReason,omitempty"`
	// Sources — страницы из интернета, на которые опирался агент; только адреса, которые он видел.
	Sources []AgentSource `json:"sources,omitempty"`
	// FileCode — код из файла поставщика, если лучший код агента другой: с пошлинами обоих (считает
	// программа, а не модель) — сотрудник видит, во что обходится расхождение.
	FileCode *AgentFileCode `json:"fileCode,omitempty"`
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
	// Kind — вид документа: cc — сертификат соответствия, dc — декларация соответствия, sgr — свидетельство
	// о государственной регистрации, conditional — условный выпуск, other — прочие меры.
	Kind string `json:"kind"`
	// UtilFee — у меры утильсбора (РОП): за что платёж по перечню № 689 — «за товар», «за упаковку»,
	// «за товар и упаковку»; "" — не утильсбор или кода нет в перечне.
	UtilFee     string `json:"utilFee,omitempty"`
	Measure     string `json:"measure"`
	Act         string `json:"act"`
	URL         string `json:"url"`
	Requirement string `json:"requirement"`
	Applies     string `json:"applies"`
	// Basis — дословная цитата из справки, акта или страницы кода, на которой основан вывод (проверена
	// программой); NameHint — при каком реальном признаке товара мера не применяется и какое наименование
	// тогда указать в декларации.
	Basis string `json:"basis,omitempty"`
	// BasisSource — где найдена цитата: «справка tnved.info», название акта, «страница кода … (TWS.BY)».
	BasisSource string `json:"basisSource,omitempty"`
	NameHint    string `json:"nameHint,omitempty"`
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

// codePath — путь кода для модели. В выгрузке TWS у половины кодов путь обрезан до двух уровней
// («аппараты точильные и шлифовальные › углошлифовальные» без «прочие › прочие» над ними) — такой
// путь помечается «… ›»: полный путь — в «Дереве кода» open_details.
func codePath(name string) string {
	if strings.Count(name, "🠺") < 2 {
		return "… › " + treeName(name)
	}
	return treeName(name)
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
		rows = append(rows, agentCodeRow{Code: item.Code, Name: codePath(item.Name), Duty: dutyText(item)})
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
	docs  map[string]directory.WebPage
	// documentsCode — запуск по кнопке «Документы для ввоза» для этого кода ("" — обычный подбор).
	documentsCode string
	// general — поиск кода по общей базе за этот запуск (nil — ещё не искали); generalAsked — пропущенные
	// перечни уже возвращались агенту.
	general      *generalSearch
	generalAsked bool
	// search — поиск в интернете (nil — не подключён); webCalls — обращений к Tavily за этот запуск,
	// pages — разные страницы, открытые в обычном подборе.
	search   webSearcher
	webCalls int
	pages    map[string]bool
	// lookPhoto — ответ модели для фото на вопрос (nil — инструмента нет); photoQuestions — вопросов за запуск.
	lookPhoto      func(ctx context.Context, question string) (string, error)
	photoQuestions int
	// requireDeclarations — итог не принимается с первого раза, пока агент не искал примеры декларирования
	// (declarationsChecked — искал в этом подборе).
	requireDeclarations bool
	declarationsChecked bool
	// declarationQuery — первый запрос search_declarations; declarationCounts — по кодам (10 цифр)
	// наибольшее число записей и запрос, которым оно найдено: ссылка «Декларации» у кодов итога.
	declarationQuery  string
	declarationCounts map[string]declarationCount
	// fileCode — код из файла поставщика ("" — не было): итог сравнивается с ним.
	fileCode string
}

// AgentFileCode — код из файла поставщика рядом с подобранным: пошлины ЕТТ обоих.
type AgentFileCode struct {
	Code      string `json:"code"`
	Found     bool   `json:"found"`
	Duty      string `json:"duty"`
	BestCode  string `json:"bestCode"`
	BestDuty  string `json:"bestDuty"`
	DutyDiffs bool   `json:"dutyDiffers"`
}

func newAgentToolbox(data agentData) *agentToolbox {
	return &agentToolbox{data: data, opened: map[string]string{}, web: map[string]string{}, links: map[string]bool{}, docs: map[string]directory.WebPage{}, pages: map[string]bool{}, declarationCounts: map[string]declarationCount{}}
}

type declarationCount struct {
	query string
	count int
}

// rememberDeclarations — запрос и число записей под каждым кодом из ответа search_declarations.
func (t *agentToolbox) rememberDeclarations(query string, sources []directory.DeclarationSource) {
	if t.declarationQuery == "" {
		t.declarationQuery = query
	}
	for _, source := range sources {
		for _, group := range source.Groups {
			if len(group.Code) == 10 && group.Count > t.declarationCounts[group.Code].count {
				t.declarationCounts[group.Code] = declarationCount{query: query, count: group.Count}
			}
		}
	}
}

// declarationsLink — ссылка на описания из деклараций под кодом: по запросу, которым код найден, иначе
// по первому запросу агента.
func (t *agentToolbox) declarationsLink(code string) *AgentDeclarations {
	found, ok := t.declarationCounts[code]
	if !ok {
		if t.declarationQuery == "" {
			return nil
		}
		found.query = t.declarationQuery
	}
	return &AgentDeclarations{URL: directory.DeclarationsURL(found.query, code), Query: found.query, Count: found.count}
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

// allowedURL — адрес для web_page, open_link и search_in_document: из результатов поиска, ссылка с
// открытой страницы или tnved.info, tws.by.
func (t *agentToolbox) allowedURL(raw string) (string, string) {
	address, ok := directory.CanonicalWebURL(raw)
	if !ok {
		return "", "Нужен полный адрес https://…"
	}
	// Результаты поиска запомнены как прислал Tavily — сверяем и исходный адрес.
	if !t.links[address] && !t.links[strings.TrimSpace(raw)] && !directory.TrustedWebHost(address) {
		return address, "Можно открывать только адреса из результатов поиска, ссылки с уже открытых страниц и адреса tnved.info, tws.by."
	}
	return address, ""
}

// webProblem — ответ агенту, если страница не открылась: файл и SPA — ссылку отдать сотруднику.
func webProblem(err error) (answer, detail string) {
	switch {
	case errors.Is(err, errWebPageLimit):
		return fmt.Sprintf("Лимит страниц на этот подбор (%d) исчерпан — решай по уже прочитанному; части прочитанных страниц открываются.", agentMaxWebPages), "лимит страниц"
	case errors.Is(err, errTavilyLimit):
		return "Сайт не отдаёт страницу серверу, а лимит обращений к Tavily на этот подбор исчерпан — опирайся на краткий текст результата поиска.", "лимит Tavily"
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

// documentText — полный текст документа: из этого запуска или через загрузчик (база, сайт). В обычном
// подборе — не больше agentMaxWebPages разных страниц, а то, что сервер не прочитал (скрипт, сайт не
// отдаёт страницу, PDF), — через Tavily Extract. В режиме «Документы» файл и SPA отдаются сотруднику.
func (t *agentToolbox) documentText(ctx context.Context, address string) (directory.WebPage, error) {
	if page, ok := t.docs[address]; ok {
		page.FromCache = true
		return page, nil
	}
	web := t.documentsCode == ""
	if web {
		if !t.pages[address] && len(t.pages) >= agentMaxWebPages {
			return directory.WebPage{URL: address}, errWebPageLimit
		}
		t.pages[address] = true
	}
	page, err := t.data.WebPage(ctx, address)
	// PDF, страница скриптом или сайт не отдал страницу — текст через Tavily (и в «Документах для ввоза»:
	// решения ЕЭК на docs.eaeunion.org — PDF), сохраняется в базе.
	if err != nil && t.search != nil &&
		(errors.Is(err, directory.ErrWebPageScript) || errors.Is(err, directory.ErrWebPageUnavailable) || errors.Is(err, directory.ErrWebPageFile)) {
		page, err = t.data.ExtractedPage(ctx, address, t.webPageText)
	}
	// Карточка решения Коллегии на docs.eaeunion.org — только описание, текст в PDF: читаем сводную
	// редакцию на adilet (адрес по номеру и году с карточки). Не открылась — остаётся карточка.
	if adilet := directory.EECCardAdiletURL(page); err == nil && adilet != "" {
		if full, fullErr := t.data.WebPage(ctx, adilet); fullErr == nil && strings.TrimSpace(full.Text) != "" {
			t.links[adilet] = true
			full.Links = append(full.Links, directory.WebLink{URL: adilet, Text: "Сводная редакция на adilet.zan.kz"})
			page = full
		}
	}
	if err == nil {
		t.docs[address] = page
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
		Query    string   `json:"query"`
		Codes    []string `json:"codes"`
		Prefix   string   `json:"prefix"`
		Code     string   `json:"code"`
		URL      string   `json:"url"`
		Part     int      `json:"part"`
		Question string   `json:"question"`
		Within   string   `json:"within"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error":"Аргументы не JSON"}`, agentToolEvent{Tool: name, Label: "Ошибка вызова инструмента", Failed: true}
	}
	switch name {
	case "web_search":
		return t.webSearch(ctx, args.Query)
	case "web_page":
		if t.search != nil {
			return t.webPage(ctx, args.URL, args.Part)
		}
	}
	// Справка и документы по ссылкам — только по кнопке «Документы для ввоза»; поиск по странице в
	// обычном подборе — когда подключён интернет.
	if t.documentsCode == "" && (name == "open_goods_help" || name == "open_link" || (name == "search_in_document" && t.search == nil)) {
		name = "unknown:" + name
	}
	if name == "look_at_photo" && t.lookPhoto == nil {
		name = "unknown:" + name
	}
	switch name {
	case "search_declarations":
		return t.searchDeclarations(ctx, args.Query, args.Within)
	case "look_at_photo":
		question := service.TruncateRunes(strings.TrimSpace(args.Question), 300)
		label := "Вопрос по фото: " + quoted(question)
		switch {
		case question == "":
			return `{"error":"Пустой вопрос"}`, agentToolEvent{Tool: name, Label: "Вопрос по фото", Detail: "пустой вопрос", Failed: true}
		case t.photoQuestions >= agentMaxPhotoQuestions:
			return toolJSON(map[string]string{"error": fmt.Sprintf("Лимит вопросов по фото (%d) исчерпан — решай по описанию фото и ответам.", agentMaxPhotoQuestions)}),
				agentToolEvent{Tool: name, Label: label, Detail: "лимит вопросов", Failed: true}
		}
		t.photoQuestions++
		answer, err := t.lookPhoto(ctx, question)
		if err != nil || answer == "" {
			return `{"error":"Модель для фото не ответила — решай по описанию фото."}`, agentToolEvent{Tool: name, Label: label, Detail: "нет ответа", Failed: true}
		}
		return toolJSON(map[string]string{"question": question, "answer": answer}), agentToolEvent{Tool: name, Label: label, Detail: service.TruncateRunes(answer, 300)}
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
			row := map[string]any{"code": code, "found": true, "name": codePath(item.Name), "duty": dutyText(item), "vat": item.VATPercent + "%", "hasDetails": item.DetailsURL != ""}
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
		answer := map[string]any{"total": total, "codes": codeRows(items),
			"note": "Коды — в порядке дерева: на каждом уровне конкретные ветки идут раньше «прочих». Путь с «… ›» неполный — уровень кода сверь по «Дереву кода» в open_details."}
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
			return t.tksNotesFallback(ctx, code, label, "у кода нет страницы TWS.BY")
		}
		text, cached, err := t.data.DetailsPage(ctx, code, items[0].DetailsURL)
		if err != nil || text == "" {
			return t.tksNotesFallback(ctx, code, label, "страница TWS.BY не открылась")
		}
		t.opened[code] = text
		detail := fmt.Sprintf("с TWS.BY, %d символов", utf8.RuneCountInString(text))
		if cached {
			detail = fmt.Sprintf("сохранённая копия, %d символов", utf8.RuneCountInString(text))
		}
		if decisionsBlock(text) != "" {
			detail += " · есть классификационные решения"
		}
		return toolJSON(map[string]any{"code": code, "text": t.withoutRepeatedNotes(code, decisionsMarked(text))}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: items[0].DetailsURL}
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
			return t.generalAnswer(ctx, code, label, "лимит tnved.info на сегодня исчерпан", page.URL)
		}
		if err != nil || page.Text == "" {
			return t.generalAnswer(ctx, code, label, "справка tnved.info не открылась", page.URL)
		}
		// Решения Коллегии ЕЭК — текстом с adilet, а не карточкой docs.eaeunion.org с PDF.
		page = directory.WithAdiletLinks(page)
		// Условный выпуск по Правилам РК: ссылка на текст Правил — вместе со справкой, чтобы агент мог
		// открыть их (open_link, search_in_document) и указать в итоге.
		page.Text += "\n\nУсловный выпуск (РК):\n" + conditionalReleaseTitle + " — " + conditionalReleaseURL
		page.Links = append(page.Links, directory.WebLink{URL: conditionalReleaseURL, Text: conditionalReleaseTitle})
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
		answer := map[string]any{"url": address, "query": query, "matches": matches, "fragments": fragments}
		if t.documentsCode != "" {
			t.rememberPage(address, strings.Join(fragments, "\n"), nil)
		} else {
			// Страница из интернета — не источник цитат («справка tnved.info»), только её ссылки.
			t.rememberPage(address, "", page.Links)
			answer["source"] = "web"
		}
		if matches == 0 {
			answer["hint"] = "Ничего не найдено. Попробуй другие слова: основу слова, синоним, код или группу ТН ВЭД, номер раздела."
		} else if matches > len(fragments) {
			answer["hint"] = fmt.Sprintf("Показаны %d самых подходящих из %d абзацев — уточни запрос, если нужного нет.", len(fragments), matches)
		}
		return toolJSON(answer), agentToolEvent{Tool: name, Label: label, Detail: fmt.Sprintf("фрагментов %d из %d", len(fragments), matches), URL: address}
	}
	return `{"error":"Нет такого инструмента"}`, agentToolEvent{Tool: name, Label: "Неизвестный инструмент " + name, Failed: true}
}

// Примеры декларирования для модели: групп и примеров в группе — по уровню поиска.
const (
	agentDeclarationGroups   = 12
	agentHeadingExamples     = 3
	agentCodeGroupExamples   = 6
	agentCodeExamplesInGroup = 25
)

// searchDeclarations — search_declarations: под какими позициями и кодами декларировали такой товар.
// Поиск отмечается как сделанный и при недоступном сайте: требовать невозможное итог не должен.
func (t *agentToolbox) searchDeclarations(ctx context.Context, query, within string) (string, agentToolEvent) {
	const name = "search_declarations"
	query = service.TruncateRunes(strings.TrimSpace(query), 100)
	within = nonDigits.ReplaceAllString(within, "")
	label := "Примеры декларирования: " + quoted(query)
	if within != "" {
		label += " в " + FormatTNVEDCode(within)
	}
	switch {
	case query == "":
		return `{"error":"Пустой запрос"}`, agentToolEvent{Tool: name, Label: "Примеры декларирования", Detail: "пустой запрос", Failed: true}
	case within != "" && len(within) != 4 && len(within) != 10:
		return `{"error":"within — 4 или 10 цифр"}`, agentToolEvent{Tool: name, Label: label, Detail: "неверный код", Failed: true}
	}
	address := directory.DeclarationsURL(query, within)
	sources, cached, err := t.data.Declarations(ctx, query, within)
	t.declarationsChecked = true
	if err != nil {
		return `{"error":"Сайты с примерами декларирования не ответили — опирайся на номенклатуру и базу решений и скажи об этом в notes."}`,
			agentToolEvent{Tool: name, Label: label, Detail: "сайты не ответили", URL: address, Failed: true}
	}
	t.rememberDeclarations(query, sources)
	examples := agentHeadingExamples
	switch len(within) {
	case 4:
		examples = agentCodeGroupExamples
	case 10:
		examples = agentCodeExamplesInGroup
	}
	found, details := 0, []string{}
	for index, source := range sources {
		if len(source.Groups) > agentDeclarationGroups {
			source.Groups = source.Groups[:agentDeclarationGroups]
		}
		counts := []string{}
		for position := range source.Groups {
			group := &source.Groups[position]
			if len(group.Examples) > examples {
				group.Examples = group.Examples[:examples]
			}
			if len(counts) < 2 {
				counts = append(counts, fmt.Sprintf("%s — %d", FormatTNVEDCode(group.Code), group.Count))
			}
		}
		found += len(source.Groups)
		switch {
		case source.Error != "":
			counts = []string{"не ответил"}
		case len(counts) == 0:
			counts = []string{"ничего"}
		}
		details = append(details, source.Site+": "+strings.Join(counts, ", "))
		sources[index] = source
	}
	answer := map[string]any{"query": query, "within": within, "sources": sources,
		"note": "Обезличенные описания из деклараций (графа 31) на разных сайтах, неофициальные: count — сколько записей с этими словами. Учитывай только записи о таком же товаре, случайные совпадения слов отбрось. Сравни сайты между собой: если указывают на разные коды — скажи об этом в notes."}
	switch {
	case found == 0:
		answer["hint"] = "Ничего не нашлось. Попробуй другие слова: основу слова, синоним, вид товара без характеристик (tks.ru ищет все слова сразу)."
	case within == "":
		answer["hint"] = "Посмотри коды внутри подходящей позиции на ifcg.ru: within — 4 цифры."
	case len(within) == 4:
		answer["hint"] = "Описания товаров под кодом: within — 10 цифр."
	}
	detail := strings.Join(details, "; ")
	if cached {
		detail += " · сохранённая копия"
	}
	return toolJSON(answer), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: address}
}

// Страница кода TWS.BY целиком: пояснения к ТН ВЭД (к позиции, общие положения группы и раздела) —
// с заголовка notesHeading до конца. У соседних кодов они одинаковые и длинные: модели — один раз.
const (
	notesHeading    = "Дополнительная информация"
	sameNotesPrefix = "Пояснения к ТН ВЭД — те же, что на странице кода "
)

// tksNotesPrefix — начало резервного текста open_details: он целиком пояснения, цитаты из него принимаются.
const tksNotesPrefix = "Резерв: "

// tksNotesFallback — open_details без страницы TWS.BY: пояснения к группе и позиции с tks.ru (без дерева,
// мер и решений). Текст запоминается в opened — цитаты из него проверяются как со страницы кода.
func (t *agentToolbox) tksNotesFallback(ctx context.Context, code, label, reason string) (string, agentToolEvent) {
	const name = "open_details"
	notes, cached, err := t.data.TKSNotes(ctx, code)
	if err != nil || notes == "" {
		return toolJSON(map[string]any{"code": code, "error": "Страница не открылась (" + reason + ", пояснения tks.ru недоступны); опирайся на наименование кода"}),
			agentToolEvent{Tool: name, Label: label, Detail: reason + ", пояснения tks.ru недоступны", Failed: true}
	}
	text := tksNotesPrefix + reason + ". Ниже пояснения к ТН ВЭД с tks.ru — без дерева кода и мер; дерево и ветки уровня смотри в get_codes и list_heading.\n\n" + notes
	t.opened[code] = text
	detail := fmt.Sprintf("%s · пояснения tks.ru, %d символов", reason, utf8.RuneCountInString(notes))
	if cached {
		detail += ", сохранённая копия"
	}
	return toolJSON(map[string]any{"code": code, "text": text}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: directory.TKSNotesURL(code[:4])}
}

// decisionsHeading — блок страницы кода TWS.BY с классификационными и предварительными решениями.
const decisionsHeading = "Классификационные решения"

// decisionQuoteHint — поле decisionQuote кандидата в finish.
const decisionQuoteHint = "Второе доказательство после quote: дословная цитата из блока «Классификационные решения» на странице open_details этого кода, только если товар решения такой же (вид, материал, устройство, назначение); пусто, если нет"

// decisionsNote — пометка перед блоком решений для модели: решения — не главный довод.
const decisionsNote = "[Классификационные решения ниже — дополнительное доказательство: сначала обоснуй код текстом позиции, деревом и пояснениями (quote), решение по такому же товару приводи в decisionQuote. Предварительное решение выдано конкретному заявителю на его товар.]\n"

// decisionsBounds — границы блока классификационных решений на странице кода (start < 0 — блока нет).
func decisionsBounds(page string) (start, end int) {
	start = strings.Index(page, decisionsHeading)
	if start < 0 {
		return -1, -1
	}
	end = len(page)
	for _, stop := range []string{"Реестр объектов интеллектуальной собственности", notesHeading} {
		if index := strings.Index(page[start+len(decisionsHeading):], stop); index >= 0 && start+len(decisionsHeading)+index < end {
			end = start + len(decisionsHeading) + index
		}
	}
	return start, end
}

// decisionsBlock — блок классификационных решений ("" — нет).
func decisionsBlock(page string) string {
	start, end := decisionsBounds(page)
	if start < 0 {
		return ""
	}
	return page[start:end]
}

// decisionsMarked — страница кода для модели: перед блоком решений — пометка decisionsNote.
func decisionsMarked(page string) string {
	start, _ := decisionsBounds(page)
	if start < 0 {
		return page
	}
	return page[:start] + decisionsNote + page[start:]
}

// containsQuote — есть ли цитата в тексте без учёта регистра, знаков и пробелов: на TWS.BY слова бывают
// разорваны («мет аллические»), и дословная цитата с целым словом иначе не находится.
func containsQuote(text, quote string) bool {
	squash := func(value string) string { return strings.ReplaceAll(decisions.NormalizeDecisionName(value), " ", "") }
	return strings.Contains(squash(text), squash(quote))
}

// decisionSource — вид и страна решения, в котором дословно есть цитата: «Предварительное · RU - Россия».
// Решения в блоке идут подряд, каждое заканчивается строкой «Страна : …». "" — цитаты в решениях нет.
func decisionSource(page, quote string) string {
	normalized := decisions.NormalizeDecisionName(quote)
	if utf8.RuneCountInString(normalized) < agentMinQuoteRunes {
		return ""
	}
	var entry []string
	for _, line := range strings.Split(decisionsBlock(page), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, decisionsHeading) {
			continue
		}
		entry = append(entry, line)
		country, isEnd := strings.CutPrefix(line, "Страна")
		if !isEnd {
			continue
		}
		if containsQuote(strings.Join(entry, "\n"), normalized) {
			parts := []string{"классификационное решение"}
			// Первая строка решения — вид («Предварительное»), если это короткое слово, а не код.
			if kind := entry[0]; utf8.RuneCountInString(kind) <= 40 && !strings.ContainsAny(kind, "0123456789") {
				parts[0] = strings.ToLower(kind) + " решение"
			}
			if country = strings.TrimSpace(strings.TrimLeft(country, " :")); country != "" {
				parts = append(parts, country)
			}
			return strings.Join(parts, " · ")
		}
		entry = nil
	}
	// Блок без строк «Страна» (решения ЕЭК и др.): цитата есть в блоке — решение без подробностей.
	if containsQuote(decisionsBlock(page), normalized) {
		return "классификационное решение"
	}
	return ""
}

// notesOf — пояснения к ТН ВЭД со страницы кода ("" — нет).
func notesOf(page string) string {
	if index := strings.Index(page, notesHeading); index >= 0 {
		return page[index:]
	}
	return ""
}

// withoutRepeatedNotes — страница для модели: пояснения, уже показанные на странице другого кода этого
// запуска, заменены ссылкой на неё (для проверки цитат в opened остаётся полный текст).
func (t *agentToolbox) withoutRepeatedNotes(code, page string) string {
	notes := notesOf(page)
	if utf8.RuneCountInString(notes) < 500 {
		return page
	}
	for other, text := range t.opened {
		if other != code && notesOf(text) == notes {
			return strings.TrimSuffix(page, notes) + sameNotesPrefix + other + " выше."
		}
	}
	return page
}

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
		if result != nil {
			problem = t.checkGeneral(ctx, result, problem)
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
		item.Kind = documentKind(document.Kind, item.Measure+" "+item.Act)
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
		return firstNonEmpty(act, "акт по ссылке")
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
