package tnvedagent

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// searchDocument — абзацы с наибольшим числом слов запроса (по основе слова) с соседними абзацами,
// в порядке документа; matches — сколько всего абзацев нашлось.
func searchDocument(text, query string) (fragments []string, matches int) {
	stems := queryStems(query)
	lines := strings.Split(text, "\n")
	chosen, matches := documentHits(lines, stems, agentSearchFragments)
	return fragmentsAround(lines, chosen, stems), matches
}

// queryStems — основы слов запроса (от двух знаков).
func queryStems(query string) []string {
	stems := []string{}
	for _, word := range strings.Fields(strings.ToLower(query)) {
		word = strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if utf8.RuneCountInString(word) >= 2 {
			stems = append(stems, repository.SearchStem(word))
		}
	}
	return stems
}

// documentHits — до limit строк с наибольшим числом основ, не из окрестности друг друга, в порядке
// документа; matches — сколько всего строк содержит хоть одну основу.
func documentHits(lines []string, stems []string, limit int) (chosen []int, matches int) {
	hits := scoreLines(lines, stems)
	return pickHits(hits, limit), len(hits)
}

// documentHit — строка документа и сколько основ запроса в ней.
type documentHit struct{ line, score int }

// scoreLines — строки хотя бы с одной основой: сильнейшие первыми, при равенстве — по порядку документа.
// Общие слова запроса (есть больше чем в пятой части строк: «из», «для», «продукция» в перечне) при
// нескольких основах не считаются — иначе они перевешивают редкие (код, вид товара) и выдача по
// большому акту становится случайной.
func scoreLines(lines []string, stems []string) []documentHit {
	lowered := make([]string, len(lines))
	for index, line := range lines {
		lowered[index] = strings.ToLower(line)
	}
	stems = rareStems(lowered, stems)
	hits := []documentHit{}
	for index, lower := range lowered {
		score := 0
		for _, stem := range stems {
			if strings.Contains(lower, stem) {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, documentHit{index, score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	return hits
}

// rareStems — основы, которые встречаются не больше чем в пятой части строк; одна основа или все общие —
// как есть.
func rareStems(lowered []string, stems []string) []string {
	if len(stems) < 2 {
		return stems
	}
	rare := []string{}
	for _, stem := range stems {
		count := 0
		for _, line := range lowered {
			if strings.Contains(line, stem) {
				count++
			}
		}
		if count*5 <= len(lowered) {
			rare = append(rare, stem)
		}
	}
	if len(rare) == 0 {
		return stems
	}
	return rare
}

// pickHits — до limit строк не из окрестности друг друга, по возрастанию.
func pickHits(hits []documentHit, limit int) []int {
	chosen := []int{}
	for _, candidate := range hits {
		if !nearChosen(chosen, candidate.line) {
			chosen = append(chosen, candidate.line)
		}
		if len(chosen) == limit {
			break
		}
	}
	sort.Ints(chosen)
	return chosen
}

// nearChosen — строка попадает в окрестность уже выбранной (та же, что показывает fragmentAt).
func nearChosen(chosen []int, line int) bool {
	for _, picked := range chosen {
		if line >= picked-1 && line <= picked+3 {
			return true
		}
	}
	return false
}

// fragmentsAround — найденные строки (по возрастанию) с одной соседней выше и двумя ниже; найденная
// обрезана вокруг первой основы. Пересекающиеся окрестности — один фрагмент, а не повтор строк. Над
// фрагментом — подпись раздела документа, в котором он стоит (sectionPrefix).
func fragmentsAround(lines []string, chosen []int, stems []string) []string {
	headings := sectionHeadings(lines)
	picked := map[int]bool{}
	for _, line := range chosen {
		picked[line] = true
	}
	fragments := []string{}
	for index := 0; index < len(chosen); {
		start, end := max(0, chosen[index]-1), min(len(lines), chosen[index]+3)
		for index++; index < len(chosen) && chosen[index]-1 < end; index++ {
			end = min(len(lines), chosen[index]+3)
		}
		parts := []string{}
		for at := start; at < end; at++ {
			if picked[at] {
				parts = append(parts, aroundMatch(lines[at], stems, agentFragmentRunes/2))
			} else {
				parts = append(parts, service.TruncateRunes(lines[at], agentFragmentRunes/6))
			}
		}
		fragment := strings.Join(parts, "\n")
		if label := sectionAt(lines, headings, start); label != "" {
			fragment = sectionPrefix + label + "]\n" + fragment
		}
		fragments = append(fragments, fragment)
	}
	return fragments
}

// Разделы большого акта: у похожих списков кодов в разных частях документа разный смысл (в Решении №299
// раздел II Единого перечня — госрегистрация, раздел 16 Единых требований — гигиенические требования к
// материалам для контакта с пищей). Фрагмент поиска подписывается заголовком раздела, в котором стоит, —
// агент не путает части документа, а программа сама ставит раздел у цитаты-основания.
const (
	sectionPrefix    = "[Раздел документа: "
	sectionLabelRune = 140
)

var sectionHeading = regexp.MustCompile(`^(Раздел|РАЗДЕЛ|Глава|ГЛАВА|Приложение|ПРИЛОЖЕНИЕ)\s+(\d+|[IVXLC]+)\b`)

// sectionHeadings — номера строк-заголовков разделов, глав и приложений по порядку документа.
func sectionHeadings(lines []string) []int {
	headings := []int{}
	for index, line := range lines {
		if sectionHeading.MatchString(strings.TrimSpace(line)) {
			headings = append(headings, index)
		}
	}
	return headings
}

// sectionAt — заголовок ближайшего раздела не ниже строки line ("" — заголовка выше нет). Короткий
// заголовок («Раздел II ПЕРЕЧЕНЬ») продолжается следующей строкой («продукции (товаров), подлежащей
// государственной регистрации») — она добавляется.
func sectionAt(lines []string, headings []int, line int) string {
	at := sort.SearchInts(headings, line+1) - 1
	if at < 0 {
		return ""
	}
	heading := headings[at]
	label := strings.Join(strings.Fields(lines[heading]), " ")
	if utf8.RuneCountInString(label) < 40 && heading+1 < len(lines) && heading+1 <= line {
		label += " " + strings.Join(strings.Fields(lines[heading+1]), " ")
	}
	return service.TruncateRunes(label, sectionLabelRune)
}

// sectionOfQuote — раздел акта, в котором агент видел цитату: подпись фрагмента поиска, где она стоит
// ("" — цитата не во фрагменте с подписью).
func sectionOfQuote(text, quote string) string {
	label := ""
	var chunk strings.Builder
	found := func() bool { return label != "" && containsQuote(chunk.String(), quote) }
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, sectionPrefix); ok {
			if found() {
				return label
			}
			label = strings.TrimSuffix(rest, "]")
			chunk.Reset()
			continue
		}
		chunk.WriteString(line)
		chunk.WriteString("\n")
	}
	if found() {
		return label
	}
	return ""
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
