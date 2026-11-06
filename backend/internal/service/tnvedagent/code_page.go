package tnvedagent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

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
const decisionQuoteHint = "Второе доказательство после quote: дословная цитата из блока «Классификационные решения» на странице open_details этого кода или из описания предварительного решения nsi.eaeunion.org в ответе search_declarations, только если товар решения такой же (вид, материал, устройство, назначение); пусто, если нет"

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

// notesRef — часть пояснений, показанная модели ссылкой на страницу другого кода. Старый вид (до
// 2026-11-04) — без номера части: тогда ссылкой заменены все пояснения страницы.
type notesRef struct {
	code string
	part int
}

// notesBlockStart — начало части пояснений: пояснения к разделу, группе или позиции. Общие положения
// группы и раздела у кодов одной группы одинаковые и составляют половину страницы — модели один раз.
var notesBlockStart = regexp.MustCompile(`^(РАЗДЕЛ [IVXLC]+ |\d{2} \D|\d{4} \D)`)

// notesBlocks — пояснения по частям; части вместе дают исходный текст.
func notesBlocks(notes string) []string {
	var blocks []string
	for _, line := range strings.SplitAfter(notes, "\n") {
		if line == "" {
			continue
		}
		if len(blocks) == 0 || notesBlockStart.MatchString(line) {
			blocks = append(blocks, line)
			continue
		}
		blocks[len(blocks)-1] += line
	}
	return blocks
}

// notesMarker — ссылка вместо части пояснений, которую модель уже видела на странице другого кода.
func notesMarker(ref notesRef) string {
	return fmt.Sprintf("[%s%s выше, часть %d.]\n", sameNotesPrefix, ref.code, ref.part)
}

var notesMarkerPattern = regexp.MustCompile(`\[?` + regexp.QuoteMeta(sameNotesPrefix) + `(\d{10}) выше(?:, часть (\d+))?\.\]?\n?`)

// rememberNotes — части пояснений страницы, которые модель видела целиком (первая страница с частью).
func (t *agentToolbox) rememberNotes(code, page string) {
	for index, block := range notesBlocks(notesOf(page)) {
		if _, ok := t.notesShown[block]; !ok {
			t.notesShown[block] = notesRef{code: code, part: index + 1}
		}
	}
}

// expandNotes — текст страницы со ссылками на пояснения других страниц, развёрнутый обратно: для проверки
// цитат после ответа сотрудника.
func (t *agentToolbox) expandNotes(text string) string {
	return notesMarkerPattern.ReplaceAllStringFunc(text, func(marker string) string {
		match := notesMarkerPattern.FindStringSubmatch(marker)
		notes := notesOf(t.opened[match[1]])
		if match[2] == "" {
			return notes
		}
		blocks := notesBlocks(notes)
		if part, _ := strconv.Atoi(match[2]); part >= 1 && part <= len(blocks) {
			return blocks[part-1]
		}
		return marker
	})
}

// measuresHeading — меры нетарифного регулирования на странице кода (с утратившими силу): для выбора кода
// не нужны, их проверяют «Документы для ввоза» по справке, а модели они стоят пятую часть страницы.
const (
	measuresHeading = "Меры нетарифного регулирования"
	measuresNote    = "[Меры нетарифного регулирования убраны: для выбора кода они не нужны, их проверяют «Документы для ввоза».]\n"
)

// withoutMeasures — страница без мер: от их заголовка до решений, реестра ОИС или пояснений.
func withoutMeasures(page string) string {
	start := strings.Index(page, measuresHeading)
	if start < 0 {
		return page
	}
	end := len(page)
	for _, stop := range []string{decisionsHeading, "Реестр объектов интеллектуальной собственности", notesHeading} {
		if index := strings.Index(page[start:], stop); index >= 0 && start+index < end {
			end = start + index
		}
	}
	return page[:start] + measuresNote + page[end:]
}

// pageForModel — страница кода для модели: без мер, с пометкой перед решениями, а части пояснений, уже
// показанные на странице другого кода этого подбора, — ссылкой на неё (в opened остаётся полный текст).
func (t *agentToolbox) pageForModel(code, page string) string {
	shaped := decisionsMarked(withoutMeasures(page))
	notes := notesOf(shaped)
	var text strings.Builder
	text.WriteString(strings.TrimSuffix(shaped, notes))
	for _, block := range notesBlocks(notes) {
		if ref, ok := t.notesShown[block]; ok && ref.code != code && utf8.RuneCountInString(block) >= 300 {
			text.WriteString(notesMarker(ref))
			continue
		}
		text.WriteString(block)
	}
	t.rememberNotes(code, page)
	return text.String()
}
