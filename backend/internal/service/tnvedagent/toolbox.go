package tnvedagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
)

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
	// general — поиск кода по общей базе за этот запуск (nil — ещё не искали).
	general *generalSearch
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
	// requireActRead — под-агент «Документов для ввоза» должен заглянуть в сам акт (open_link или
	// search_in_document), иначе первый итог возвращается ему; actRead — заглядывал.
	requireActRead bool
	actRead        bool
	// declarationQuery — первый запрос search_declarations; declarationCounts — по кодам (10 цифр)
	// наибольшее число записей и запрос, которым оно найдено: ссылка «Декларации» у кодов итога.
	declarationQuery  string
	declarationCounts map[string]declarationCount
	// declarationSites — то же по сайтам (код → сайт → число записей и запрос): ссылка на каждый сайт у кода итога.
	declarationSites map[string]map[string]declarationCount
	// decisionTexts — предварительные решения ЕАЭС из ответов search_declarations: источник decisionQuote
	// наравне с блоком решений на странице кода.
	decisionTexts []decisionText
	// notesShown — части пояснений к ТН ВЭД, которые модель уже видела целиком: на какой странице и какая часть.
	notesShown map[string]notesRef
	// fileCode — код из файла поставщика ("" — не было): итог сравнивается с ним.
	fileCode string
	// trademarks — поиск знаков ТРОИС (nil — инструмента нет).
	trademarks func(ctx context.Context, texts []string, code string) ([]trademarks.Hit, error)
}

func newAgentToolbox(data agentData) *agentToolbox {
	return &agentToolbox{data: data, opened: map[string]string{}, web: map[string]string{}, links: map[string]bool{}, docs: map[string]directory.WebPage{}, pages: map[string]bool{},
		declarationCounts: map[string]declarationCount{}, declarationSites: map[string]map[string]declarationCount{}, notesShown: map[string]notesRef{}}
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
	if (name == "look_at_photo" && t.lookPhoto == nil) || (name == "check_trademarks" && t.trademarks == nil) {
		name = "unknown:" + name
	}
	if name == "open_link" || name == "search_in_document" {
		t.actRead = true
	}
	switch name {
	case "check_trademarks":
		query := service.TruncateRunes(strings.TrimSpace(args.Query), 100)
		label := "ТРОИС: " + quoted(query)
		hits, err := t.trademarks(ctx, []string{query}, t.fileCode)
		if err != nil {
			return `{"error":"Реестр недоступен"}`, agentToolEvent{Tool: name, Label: label, Detail: "ошибка", Failed: true}
		}
		detail := "в реестре нет"
		if len(hits) > 0 {
			detail = trademarkSummary(hits)
		}
		return toolJSON(map[string]any{"hits": hits, "hint": "brand — знак как в реестре; lookalike — похожее написание (возможная имитация). Найденное отметь в notes."}), agentToolEvent{Tool: name, Label: label, Detail: detail}
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
		return toolJSON(map[string]any{"code": code, "text": t.pageForModel(code, text)}), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: items[0].DetailsURL}
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
		page, event, err := t.goodsHelp(ctx, code)
		switch {
		case errors.Is(err, errCodeUnknown):
			return toolJSON(map[string]any{"code": code, "error": "Кода нет в номенклатуре"}), event
		case err != nil:
			return t.generalAnswer(ctx, code, event.Label, event.Detail, page.URL)
		}
		return toolJSON(map[string]any{"code": code, "url": page.URL, "text": page.Text, "links": page.Links}), event
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

var errCodeUnknown = errors.New("code is not in nomenclature")

// goodsHelp — справка tnved.info по коду: решения Коллегии ЕЭК — ссылками на сводные тексты adilet (а не
// карточкой docs.eaeunion.org с PDF), плюс ссылка на Правила РК об условном выпуске; страница запоминается для
// цитат. Ошибка — справки нет: errCodeUnknown, directory.ErrGoodsHelpLimit (лимит) или сайт не ответил.
func (t *agentToolbox) goodsHelp(ctx context.Context, code string) (directory.WebPage, agentToolEvent, error) {
	const name = "open_goods_help"
	label := "Справка по товару " + FormatTNVEDCode(code)
	if items, err := t.data.LookupCodes(ctx, []string{code}); err != nil || len(items) == 0 {
		return directory.WebPage{}, agentToolEvent{Tool: name, Label: label, Detail: "кода нет в номенклатуре", Failed: true}, errCodeUnknown
	}
	page, err := t.data.GoodsHelp(ctx, code)
	if err == nil && page.Text == "" {
		err = directory.ErrWebPageUnavailable
	}
	if err != nil {
		detail := "справка tnved.info не открылась"
		if errors.Is(err, directory.ErrGoodsHelpLimit) {
			detail = "лимит tnved.info на сегодня исчерпан"
		}
		return page, agentToolEvent{Tool: name, Label: label, Detail: detail, URL: page.URL, Failed: true}, err
	}
	page = directory.WithAdiletLinks(page)
	page.Text += "\n\nУсловный выпуск (РК):\n" + conditionalReleaseTitle + " — " + conditionalReleaseURL
	page.Links = append(page.Links, directory.WebLink{URL: conditionalReleaseURL, Text: conditionalReleaseTitle})
	t.rememberPage(page.URL, page.Text, page.Links)
	detail := "tnved.info"
	if page.FromCache {
		detail = "сохранённая копия"
	}
	return page, agentToolEvent{Tool: name, Label: label, Detail: detail, URL: page.URL}, nil
}
