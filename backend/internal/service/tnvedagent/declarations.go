package tnvedagent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

type declarationCount struct {
	query string
	count int
}

// decisionText — предварительное решение ЕАЭС: текст и подпись («предварительное решение · КАЗАХСТАН (НСИ ЕАЭС)»).
type decisionText struct {
	text, label string
}

// rememberDeclarations — запрос и число записей под каждым кодом из ответа search_declarations; решения
// сборника ЕАЭС — целиком, для проверки decisionQuote.
func (t *agentToolbox) rememberDeclarations(query string, sources []directory.DeclarationSource) {
	if t.declarationQuery == "" {
		t.declarationQuery = query
	}
	for _, source := range sources {
		for _, group := range source.Groups {
			if len(group.Code) == 10 && group.Count > t.declarationCounts[group.Code].count {
				t.declarationCounts[group.Code] = declarationCount{query: query, count: group.Count}
			}
			if len(group.Code) == 10 && group.Count > t.declarationSites[group.Code][source.Site].count {
				if t.declarationSites[group.Code] == nil {
					t.declarationSites[group.Code] = map[string]declarationCount{}
				}
				t.declarationSites[group.Code][source.Site] = declarationCount{query: query, count: group.Count}
			}
			if source.Site != directory.NSIDecisionsSite {
				continue
			}
			for _, example := range group.Examples {
				t.decisionTexts = append(t.decisionTexts, decisionText{text: example.Text, label: example.Source})
			}
		}
	}
}

// nsiDecisionSource — подпись предварительного решения ЕАЭС (из search_declarations), в котором дословно есть
// цитата; "" — нет такого.
func (t *agentToolbox) nsiDecisionSource(quote string) string {
	normalized := decisions.NormalizeDecisionName(quote)
	if utf8.RuneCountInString(normalized) < agentMinQuoteRunes {
		return ""
	}
	for _, decision := range t.decisionTexts {
		if containsQuote(decision.text, normalized) {
			return decision.label
		}
	}
	return ""
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
	link := &AgentDeclarations{URL: directory.DeclarationsURL(found.query, code), Query: found.query, Count: found.count}
	// По сайту: запрос, которым код найден там, иначе первый запрос агента.
	for _, site := range []string{"ifcg.ru", "tks.ru", directory.NSIDecisionsSite} {
		query, count := t.declarationQuery, 0
		if hit, ok := t.declarationSites[code][site]; ok {
			query, count = hit.query, hit.count
		}
		address, ok := "", true
		switch site {
		case "ifcg.ru":
			address = directory.DeclarationsURL(query, code)
		case "tks.ru":
			address, ok = directory.TKSDeclarationsURL(query)
		default:
			address = directory.NSIDecisionsURL(query)
		}
		if ok {
			link.Links = append(link.Links, AgentDeclarationLink{Site: site, URL: address, Count: count})
		}
	}
	return link
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
		"note": "ifcg.ru и tks.ru — обезличенные описания из деклараций (графа 31), неофициальные: count — сколько записей с этими словами. nsi.eaeunion.org — официальный сборник предварительных решений таможенных органов ЕАЭС: это решения, а не практика; решение по такому же товару — доказательство, его описание можно цитировать в decisionQuote дословно. Учитывай только записи о таком же товаре, случайные совпадения слов отбрось. Сравни источники между собой: если указывают на разные коды — скажи об этом в notes."}
	switch {
	case found == 0:
		answer["hint"] = "Ничего не нашлось. Попробуй другие слова: основу слова, синоним, вид товара без характеристик (tks.ru ищет все слова сразу)."
	case within == "":
		answer["hint"] = "Число записей под кодами видно в ответе (tks.ru, nsi.eaeunion.org), под кодами кандидатов программа покажет его сама. within — только если код внутри позиции так и не ясен."
	case len(within) == 4:
		answer["hint"] = "Описания товаров под кодом: within — 10 цифр."
	}
	detail := strings.Join(details, "; ")
	if cached {
		detail += " · сохранённая копия"
	}
	return toolJSON(answer), agentToolEvent{Tool: name, Label: label, Detail: detail, URL: address}
}
