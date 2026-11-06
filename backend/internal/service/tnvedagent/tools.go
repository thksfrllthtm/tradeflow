package tnvedagent

// Инструменты агента подбора кода: все только читают данные (номенклатура, страницы TWS, база
// решений, справочники; в режиме «Документы для ввоза» — справка tnved.info и акты по ссылкам).
// Выполняет их agentToolbox (toolbox.go), итог проверяет finish (finish.go).

import (
	"context"
	"fmt"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
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

func tool(name, description string, parameters map[string]any) agentTool {
	return agentTool{Type: "function", Function: agentToolFunction{Name: name, Description: description, Parameters: parameters}}
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// agentTools — инструменты агента; в справочнике (reference) у итога нет наименования для декларации.
// Инструменты «Документов для ввоза» — у под-агентов (documentsTools).
func agentTools(reference bool) []agentTool {
	// Общие поля итога в подборе и в справочнике.
	candidate := objectSchema(map[string]any{
		"code":          stringProperty("Код, 10 цифр"),
		"verdict":       map[string]any{"type": "string", "enum": []string{"fits", "doubtful", "no"}},
		"quote":         stringProperty("Дословная цитата из пояснений к ТН ВЭД на странице open_details этого кода; не из наименования и дерева; пусто, если нет"),
		"decisionQuote": stringProperty(decisionQuoteHint),
		"reason":        stringProperty("Почему подходит или не подходит, 1–2 предложения"),
		"check":         stringProperty("Что уточнить; пусто, если нечего"),
	}, "code", "verdict", "reason")
	confidence := map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Уверенность в лучшем коде"}
	confidenceReason := stringProperty("Почему такая уверенность, одна фраза: что подтверждает код (текст позиции, дерево, пояснения, база компании, декларации) и что остаётся под вопросом")
	sources := map[string]any{"type": "array", "description": "Страницы из интернета, на которые опирался (адреса из web_search или web_page); без интернета — пусто", "items": objectSchema(map[string]any{
		"title": stringProperty("Что это за страница: «предварительное решение ФТС», «разъяснение ЕЭК»"),
		"url":   stringProperty("Адрес страницы"),
	}, "url")}
	finish := tool("finish", "Итог подбора. Вызывается один раз в конце.", objectSchema(map[string]any{
		"description":      stringProperty("Что это за товар, 1–2 предложения; расхождения наименования с фото"),
		"candidates":       map[string]any{"type": "array", "description": "1–4 кандидата, лучший первым", "items": candidate},
		"name":             stringProperty("Наименование для декларации по правилу — только русская часть, без перевода на китайский"),
		"notes":            stringProperty("Замечания сотруднику; пусто, если нет"),
		"confidence":       confidence,
		"confidenceReason": confidenceReason,
		"sources":          sources,
	}, "description", "candidates", "name", "confidence"))
	if reference {
		finish = tool("finish", "Ответ сотруднику. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description":      stringProperty("Ответ сотруднику. На первый вопрос — что это за товар, какой код и главное отличие от соседних, 2–4 предложения. На уточнение — только прямой ответ на него, без пересказа прежнего ответа"),
			"candidates":       map[string]any{"type": "array", "description": "1–4 кода: подходящие первыми, затем 1–2 соседних неподходящих (verdict no) с объяснением", "items": candidate},
			"name":             stringProperty("Краткое наименование для декларации по правилу — только русская часть, без перевода на китайский"),
			"nameFull":         stringProperty("Подробное наименование для декларации: краткое плюс признаки, от которых зависит код (материал, устройство, назначение, комплектность), до 200 символов; только русская часть"),
			"notes":            stringProperty("Короткое дополнение, которого нет в description; пусто, если нечего добавить. Не повторяй description"),
			"confidence":       confidence,
			"confidenceReason": confidenceReason,
			"sources":          sources,
		}, "description", "candidates", "name", "nameFull", "confidence"))
	}
	extra := []agentTool{tool("check_trademarks", "Таможенный реестр объектов интеллектуальной собственности (ТРОИС): есть ли бренд в реестре, включая похожие написания (SONV вместо SONY). Наименования строки и строку «Бренд» описания фото программа проверяет сама — вызывай для бренда, который увидел на фото сам.",
		objectSchema(map[string]any{"query": stringProperty("Бренд буква в букву, как написан на товаре")}, "query"))}
	return append([]agentTool{
		tool("search_nomenclature", "Поиск кодов в номенклатуре ТН ВЭД ЕАЭС по словам наименования (по-русски). Каждое слово ищется по основе, все слова должны встретиться в наименовании кода с его деревом. Лучше 1–3 слова. Возвращает до 30 кодов с наименованием и пошлиной ЕТТ.",
			objectSchema(map[string]any{"query": stringProperty("Слова для поиска, например «перчатки трикотажные»")}, "query")),
		tool("get_codes", "Проверить конкретные коды: есть ли в номенклатуре, наименование с деревом, пошлина ЕТТ, НДС.",
			objectSchema(map[string]any{"codes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "До 10 кодов по 10 цифр"}}, "codes")),
		tool("list_heading", "Все коды, начинающиеся с данных цифр (4–8 цифр): соседние коды товарной позиции или субпозиции, включая «прочие».",
			objectSchema(map[string]any{"prefix": stringProperty("Начало кода, 4–8 цифр, например «6116» или «611610»")}, "prefix")),
		tool("open_details", "Открыть страницу кода (TWS.BY): дерево кода, дополнительная единица, обязательные сведения, классификационные решения, пояснения к ТН ВЭД — к позиции, общие положения группы и раздела (что включается и что исключается). Если страницы TWS.BY нет — пояснения к группе и позиции с tks.ru (без дерева). Цитаты для итога бери отсюда.",
			objectSchema(map[string]any{"code": stringProperty("Код, 10 цифр")}, "code")),
		tool("search_decisions", "База решений компании: товары, которые раньше оформляли, с итоговым кодом и наименованием; проверенные администратором — первыми.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования (по-русски или по-китайски)")}, "query")),
		tool("search_directory", "Справочник товаров компании: наименования, синонимы, код ТН ВЭД и статус проверки.",
			objectSchema(map[string]any{"query": stringProperty("Слова наименования")}, "query")),
		tool("search_declarations", "Примеры декларирования и предварительные решения: как такой товар уже декларировали — обезличенные описания из деклараций (графа 31), неофициальные, сразу с нескольких сайтов, плюс официальный сборник предварительных решений таможенных органов ЕАЭС (nsi.eaeunion.org: код, описание товара, страна, обоснование). Без within: ifcg.ru — товарные позиции с числом записей и описаниями, tks.ru — коды с числом записей и описаниями. within 4 цифры — коды внутри позиции (ifcg.ru); within 10 цифр — описания товаров под кодом (ifcg.ru). Обязательно перед итогом.",
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
	return []agentTool{tool("ask_user", "Задать сотруднику один короткий вопрос, только если от ответа зависит код: у каждого варианта ответа свой код. Если все ответы ведут к одному коду, программа вопрос не пропустит. Не спрашивай технические характеристики по артикулу или модели производителя: их ищи сам. После вызова жди ответа.",
		objectSchema(map[string]any{
			"question": stringProperty("Вопрос по-русски, одно предложение: о том, что сотрудник может знать по товару, документам или у поставщика"),
			"options": map[string]any{"type": "array", "description": "2–4 варианта ответа, у каждого — код, к которому он ведёт; коды разные", "items": objectSchema(map[string]any{
				"answer": stringProperty("Вариант ответа, коротко"),
				"code":   stringProperty("10-значный код из номенклатуры при этом ответе"),
			}, "answer", "code")},
		}, "question", "options"))}
}

// lookPhotoTool — вопрос модели для фото: есть, только когда модель рассуждений фото не видит.
func lookPhotoTool() agentTool {
	return tool("look_at_photo",
		fmt.Sprintf("Спросить модель, которая видит фото товара: один конкретный вопрос о том, что видно на фото (материал, механизм, надписи и маркировка, комплектность, детали). Ответ — текстом. До %d вопросов за подбор.", agentMaxPhotoQuestions),
		objectSchema(map[string]any{"question": stringProperty("Вопрос по-русски, например «Есть ли у ножниц пружина между ручками?»")}, "question"))
}
