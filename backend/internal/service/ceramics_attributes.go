package service

// Ceramics module: attributes read from the input without AI — normalized mark, size
// mentioned in the Chinese (or client's Russian) name, size encoded in a known mark series,
// Chinese terms and matches of the Chinese name with names already in the base.

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// Size units of the ceramics base.
const (
	SizeUnitInch = "inch"
	SizeUnitCm   = "cm"
	SizeUnitMm   = "mm"
	SizeUnitMl   = "ml"
	SizeUnitL    = "l"
)

func validSizeUnit(unit string) bool {
	switch unit {
	case SizeUnitInch, SizeUnitCm, SizeUnitMm, SizeUnitMl, SizeUnitL:
		return true
	}
	return false
}

// toHalfWidth turns full-width letters, digits and punctuation (Ａ１－) into ASCII and the
// ideographic space into a space: Chinese spreadsheets often mix both forms.
func toHalfWidth(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '　':
			return ' '
		case r >= '！' && r <= '～':
			return r - 0xFEE0
		}
		return r
	}, value)
}

// NormalizeCeramicMark is the key for the exact mark search: half-width, upper case,
// without spaces, every dash variant as "-".
func NormalizeCeramicMark(mark string) string {
	mark = toHalfWidth(mark)
	var builder strings.Builder
	for _, r := range mark {
		switch {
		case unicode.IsSpace(r):
			continue
		case r == '‐' || r == '‑' || r == '‒' || r == '–' || r == '—' || r == '−':
			builder.WriteByte('-')
		default:
			builder.WriteRune(unicode.ToUpper(r))
		}
	}
	return builder.String()
}

// normalizeCeramicTypeKey compares type names without case and extra spaces.
func normalizeCeramicTypeKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(toHalfWidth(name)), " "))
}

func cleanCeramicTypeName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// CeramicSize is a size with its unit; Value is a canonical decimal string ("7", "7.5").
type CeramicSize struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

func newCeramicSize(value decimal.Decimal, unit string) CeramicSize {
	return CeramicSize{Value: value.Round(2).String(), Unit: unit}
}

func (s CeramicSize) key() string { return s.Value + " " + s.Unit }

// FormatCeramicSize shows a size the way operators write it: 7", 20 см, 500 мл.
func FormatCeramicSize(size CeramicSize) string {
	switch size.Unit {
	case SizeUnitInch:
		return size.Value + `"`
	case SizeUnitCm:
		return size.Value + " см"
	case SizeUnitMm:
		return size.Value + " мм"
	case SizeUnitMl:
		return size.Value + " мл"
	case SizeUnitL:
		return size.Value + " л"
	}
	return size.Value
}

// sizeMention is a size found in a text: "7寸" → 7 inch.
type sizeMention struct {
	Size CeramicSize
	Raw  string
}

// Unit words after a number, longest first. Latin and Cyrillic words must not continue
// with a letter ("7 inches" but not "7 ink").
var sizeUnitWords = []struct {
	word string
	unit string
}{
	{"英寸", SizeUnitInch}, {"寸", SizeUnitInch}, {"吋", SizeUnitInch},
	{"inches", SizeUnitInch}, {"inch", SizeUnitInch},
	{"дюймов", SizeUnitInch}, {"дюйма", SizeUnitInch}, {"дюйм", SizeUnitInch},
	{`"`, SizeUnitInch}, {"”", SizeUnitInch}, {"″", SizeUnitInch}, {"''", SizeUnitInch},
	{"厘米", SizeUnitCm}, {"公分", SizeUnitCm}, {"cm", SizeUnitCm}, {"см", SizeUnitCm},
	{"毫米", SizeUnitMm}, {"mm", SizeUnitMm}, {"мм", SizeUnitMm},
	{"毫升", SizeUnitMl}, {"ml", SizeUnitMl}, {"мл", SizeUnitMl},
	{"升", SizeUnitL}, {"литра", SizeUnitL}, {"литров", SizeUnitL}, {"литр", SizeUnitL}, {"l", SizeUnitL}, {"л", SizeUnitL},
}

var chineseDigits = map[rune]int{'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}

// parseChineseNumber reads 七, 十, 十二, 二十, 二十四 (up to 99).
func parseChineseNumber(runes []rune) (int, bool) {
	switch len(runes) {
	case 1:
		if runes[0] == '十' {
			return 10, true
		}
		value, ok := chineseDigits[runes[0]]
		return value, ok && value > 0
	case 2:
		if runes[0] == '十' {
			value, ok := chineseDigits[runes[1]]
			return 10 + value, ok
		}
		if runes[1] == '十' {
			value, ok := chineseDigits[runes[0]]
			return value * 10, ok && value > 1
		}
	case 3:
		if runes[1] == '十' {
			tens, ok1 := chineseDigits[runes[0]]
			ones, ok2 := chineseDigits[runes[2]]
			return tens*10 + ones, ok1 && ok2 && tens > 1
		}
	}
	return 0, false
}

func isChineseNumeral(r rune) bool {
	_, ok := chineseDigits[r]
	return ok || r == '十'
}

// matchSizeUnit returns the unit starting at text[pos:] and the length of its word.
func matchSizeUnit(runes []rune, pos int) (string, int) {
	for pos < len(runes) && (runes[pos] == ' ' || runes[pos] == '-') {
		pos++
	}
	lowerRest := strings.ToLower(string(runes[pos:]))
	for _, candidate := range sizeUnitWords {
		if !strings.HasPrefix(lowerRest, candidate.word) {
			continue
		}
		length := utf8.RuneCountInString(candidate.word)
		first, _ := utf8.DecodeRuneInString(candidate.word)
		if first < 0x2E80 && unicode.IsLetter(first) {
			if next := pos + length; next < len(runes) && unicode.IsLetter(runes[next]) {
				continue
			}
		}
		return candidate.unit, pos + length
	}
	return "", pos
}

// extractSizeMentions finds every "number + unit" in a text: 7寸, 七寸, 7.5 inch, 20cm,
// 9 дюймов. Dimensions such as 20*15cm are reported separately: one of their numbers is
// not the size of the product.
func extractSizeMentions(text string) (mentions []sizeMention, compound bool) {
	runes := []rune(toHalfWidth(text))
	seen := map[string]bool{}
	for index := 0; index < len(runes); {
		start := index
		var value decimal.Decimal
		switch {
		case runes[index] >= '0' && runes[index] <= '9':
			for index < len(runes) && runes[index] >= '0' && runes[index] <= '9' {
				index++
			}
			if index+1 < len(runes) && (runes[index] == '.' || runes[index] == ',') && runes[index+1] >= '0' && runes[index+1] <= '9' {
				index++
				for index < len(runes) && runes[index] >= '0' && runes[index] <= '9' {
					index++
				}
			}
			parsed, err := decimal.NewFromString(strings.ReplaceAll(string(runes[start:index]), ",", "."))
			if err != nil {
				continue
			}
			value = parsed
		case isChineseNumeral(runes[index]):
			for index < len(runes) && isChineseNumeral(runes[index]) {
				index++
			}
			parsed, ok := parseChineseNumber(runes[start:index])
			if !ok {
				continue
			}
			value = decimal.NewFromInt(int64(parsed))
		default:
			index++
			continue
		}
		// The number continues a word or code (A705, 705-7DJ) — not a size.
		if start > 0 && runes[start-1] < 0x2E80 && (unicode.IsLetter(runes[start-1]) || runes[start-1] == '-') {
			continue
		}
		if dimensionSeparator(runes, start-1, true) || dimensionSeparator(runes, index, false) {
			compound = true
			continue
		}
		unit, end := matchSizeUnit(runes, index)
		if unit == "" || !value.IsPositive() {
			continue
		}
		size := newCeramicSize(value, unit)
		index = end
		if !seen[size.key()] {
			seen[size.key()] = true
			mentions = append(mentions, sizeMention{Size: size, Raw: strings.TrimSpace(string(runes[start:end]))})
		}
	}
	return mentions, compound
}

// dimensionSeparator reports "20*15": a multiplication sign next to the number with a
// digit on its other side.
func dimensionSeparator(runes []rune, pos int, before bool) bool {
	for pos >= 0 && pos < len(runes) && runes[pos] == ' ' {
		if before {
			pos--
		} else {
			pos++
		}
	}
	if pos < 0 || pos >= len(runes) || !strings.ContainsRune("*xX×хХ", runes[pos]) {
		return false
	}
	next := pos + 1
	if before {
		next = pos - 1
	}
	for next >= 0 && next < len(runes) && runes[next] == ' ' {
		if before {
			next--
		} else {
			next++
		}
	}
	return next >= 0 && next < len(runes) && runes[next] >= '0' && runes[next] <= '9'
}

// chineseBaseName is the Chinese name without sizes, numbers, spaces and punctuation:
// 成人陶瓷沙拉盘7寸 and 成人陶瓷沙拉盘9寸 share 成人陶瓷沙拉盘, 成年人陶瓷盘7高脚水果盘 and
// 成年人陶瓷盘9高脚水果盘 share 成年人陶瓷盘高脚水果盘.
func chineseBaseName(name string) string {
	text := toHalfWidth(name)
	mentions, _ := extractSizeMentions(text)
	for _, mention := range mentions {
		text = strings.ReplaceAll(text, mention.Raw, " ")
	}
	var builder strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// Chinese tableware terms shown to the operator as evidence and to the model as hints.
// They explain the name; the product type still comes only from the base.
var ceramicsGlossary = []struct{ term, meaning string }{
	{"水果盘", "блюдо для фруктов (фруктовница)"}, {"沙拉盘", "салатная тарелка (салатница)"}, {"沙拉碗", "салатник, миска для салата"},
	{"汤盘", "глубокая (суповая) тарелка"}, {"深盘", "глубокая тарелка"}, {"平盘", "плоская тарелка"},
	{"西餐盘", "тарелка для европейских блюд"}, {"牛排盘", "тарелка для стейка"}, {"鱼盘", "блюдо для рыбы"},
	{"果盘", "блюдо для фруктов"}, {"点心盘", "тарелка для десерта"}, {"餐盘", "обеденная тарелка"},
	{"盘子", "тарелка"}, {"盘", "тарелка, блюдо"}, {"碟子", "блюдце, маленькая тарелка"}, {"碟", "блюдце, маленькая тарелка"},
	{"汤碗", "суповая миска"}, {"面碗", "миска для лапши"}, {"饭碗", "рисовая пиала"}, {"碗", "миска, пиала"},
	{"马克杯", "кружка"}, {"咖啡杯", "кофейная чашка"}, {"茶杯", "чайная чашка"}, {"杯碟", "чашка с блюдцем"}, {"杯", "чашка, стакан"},
	{"茶壶", "заварочный чайник"}, {"水壶", "кувшин, чайник для воды"}, {"壶", "чайник, кувшин"},
	{"花瓶", "ваза"}, {"烟灰缸", "пепельница"}, {"汤勺", "суповая ложка"}, {"勺", "ложка"},
	{"调味罐", "баночка для специй"}, {"罐", "банка"}, {"餐具", "столовая посуда"}, {"套装", "набор"},
	{"骨瓷", "костяной фарфор"}, {"陶瓷", "керамический"}, {"瓷", "фарфор, керамика"}, {"釉", "глазурь"},
	{"成年人", "для взрослых"}, {"成人", "для взрослых"}, {"高脚", "на высокой ножке"}, {"儿童", "детский"}, {"手绘", "ручная роспись"}, {"金边", "золотая кайма"},
}

// ChineseTerm is a recognised word of a Chinese name.
type ChineseTerm struct {
	Text    string `json:"text"`
	Meaning string `json:"meaning"`
}

// chineseTerms splits a name into known terms, longest first, without overlaps.
func chineseTerms(name string) []ChineseTerm {
	runes := []rune(name)
	terms := []ChineseTerm{}
	for index := 0; index < len(runes); {
		matched := false
		for _, entry := range ceramicsGlossary {
			term := []rune(entry.term)
			if index+len(term) <= len(runes) && string(runes[index:index+len(term)]) == entry.term {
				terms = append(terms, ChineseTerm{Text: entry.term, Meaning: entry.meaning})
				index += len(term)
				matched = true
				break
			}
		}
		if !matched {
			index++
		}
	}
	return terms
}

// markSeries is a group of known marks that differ only by the size inside them:
// 705-7DJ, 705-9DJ, 705-10DJ → 705-{SIZE}DJ.
type markSeries struct {
	Template string
	Unit     string
	Sizes    map[string]bool
	TypeKeys map[string]int
	Examples []string
}

// MarkSeriesSuggestion is a size read from a mark by a series of known marks.
type MarkSeriesSuggestion struct {
	Template string      `json:"template"`
	Size     CeramicSize `json:"size"`
	Examples []string    `json:"examples"`
	TypeKeys []string    `json:"-"`
}

type digitRun struct{ start, end int }

func markDigitRuns(mark string) []digitRun {
	runs := []digitRun{}
	for index := 0; index < len(mark); {
		if mark[index] < '0' || mark[index] > '9' {
			index++
			continue
		}
		start := index
		for index < len(mark) && mark[index] >= '0' && mark[index] <= '9' {
			index++
		}
		runs = append(runs, digitRun{start, index})
	}
	return runs
}

// buildMarkSeries finds, for every known mark with an integer size, the digit runs equal to
// that size. A template counts as a series only when marks of at least two different sizes
// share it: one mark 607-7 of 7" does not prove that its 7 is the size.
func buildMarkSeries(rows []ceramicCatalogEntry) map[string]*markSeries {
	all := map[string]*markSeries{}
	for _, row := range rows {
		if row.Size == nil || !isWholeNumber(row.Size.Value) {
			continue
		}
		for _, run := range markDigitRuns(row.MarkKey) {
			if strings.TrimLeft(row.MarkKey[run.start:run.end], "0") != row.Size.Value {
				continue
			}
			template := row.MarkKey[:run.start] + "{SIZE}" + row.MarkKey[run.end:]
			key := template + "|" + row.Size.Unit
			series := all[key]
			if series == nil {
				series = &markSeries{Template: template, Unit: row.Size.Unit, Sizes: map[string]bool{}, TypeKeys: map[string]int{}}
				all[key] = series
			}
			series.Sizes[row.Size.Value] = true
			series.TypeKeys[row.TypeKey]++
			if len(series.Examples) < 6 {
				series.Examples = append(series.Examples, row.Mark+" → "+FormatCeramicSize(*row.Size))
			}
		}
	}
	for key, series := range all {
		if len(series.Sizes) < 2 {
			delete(all, key)
		}
	}
	return all
}

func isWholeNumber(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

// inferSizeFromMarkPattern returns the sizes the mark gets from known series.
func inferSizeFromMarkPattern(markKey string, series map[string]*markSeries) []MarkSeriesSuggestion {
	suggestions := []MarkSeriesSuggestion{}
	for _, run := range markDigitRuns(markKey) {
		template := markKey[:run.start] + "{SIZE}" + markKey[run.end:]
		value := strings.TrimLeft(markKey[run.start:run.end], "0")
		if value == "" {
			continue
		}
		for _, unit := range []string{SizeUnitInch, SizeUnitCm, SizeUnitMm, SizeUnitMl, SizeUnitL} {
			found := series[template+"|"+unit]
			if found == nil {
				continue
			}
			typeKeys := make([]string, 0, len(found.TypeKeys))
			for key := range found.TypeKeys {
				typeKeys = append(typeKeys, key)
			}
			sort.Slice(typeKeys, func(a, b int) bool { return found.TypeKeys[typeKeys[a]] > found.TypeKeys[typeKeys[b]] })
			suggestions = append(suggestions, MarkSeriesSuggestion{Template: template, Size: CeramicSize{Value: value, Unit: unit}, Examples: found.Examples, TypeKeys: typeKeys})
		}
	}
	return suggestions
}

// similarCeramicTypes reports type names where one is a more specific form of the other:
// «Керамическая тарелка» and «Керамическая тарелка салатница».
func similarCeramicTypes(a, b string) bool {
	a, b = normalizeCeramicTypeKey(a), normalizeCeramicTypeKey(b)
	if a == b {
		return false
	}
	return strings.HasPrefix(b, a+" ") || strings.HasPrefix(a, b+" ")
}

// bareSizeNumbers are numbers of a Chinese name without a unit: in 成年人陶瓷盘7高脚水果盘 the 7
// is most likely the size. Numbers of codes (A7, 7DJ), dimensions and counts (12头, 4件) are not.
func bareSizeNumbers(text string) []string {
	runes := []rune(toHalfWidth(text))
	values := []string{}
	for index := 0; index < len(runes); {
		if runes[index] < '0' || runes[index] > '9' {
			index++
			continue
		}
		start := index
		for index < len(runes) && ((runes[index] >= '0' && runes[index] <= '9') || (runes[index] == '.' && index+1 < len(runes) && runes[index+1] >= '0' && runes[index+1] <= '9')) {
			index++
		}
		if start > 0 && runes[start-1] < 0x2E80 && (unicode.IsLetter(runes[start-1]) || runes[start-1] == '-' || runes[start-1] == '.') {
			continue
		}
		if index < len(runes) && runes[index] < 0x2E80 && unicode.IsLetter(runes[index]) {
			continue
		}
		if dimensionSeparator(runes, start-1, true) || dimensionSeparator(runes, index, false) {
			continue
		}
		if unit, _ := matchSizeUnit(runes, index); unit != "" {
			continue
		}
		if index < len(runes) && strings.ContainsRune("头件个只套片支双", runes[index]) {
			continue
		}
		value, err := decimal.NewFromString(string(runes[start:index]))
		if err != nil || !value.IsPositive() {
			continue
		}
		if text := value.String(); !containsString(values, text) {
			values = append(values, text)
		}
	}
	return values
}

// ceramicName is a name of the base split into its parts:
// «Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘» →
// type «Керамическая фруктовница для взрослых», size 7", Chinese 成年人陶瓷盘7高脚水果盘.
type ceramicName struct {
	Russian     string
	Chinese     string
	ProductType string
	Size        *CeramicSize
	// Template is the Russian part with the size replaced by {SIZE}: names of new marks of
	// this type are written the same way.
	Template string
}

func isHan(r rune) bool { return unicode.Is(unicode.Han, r) }

// splitCeramicName separates the Russian part from the Chinese one: at the last "/" before the
// first Chinese character, otherwise at that character.
func splitCeramicName(name string) (russian, chinese string) {
	name = strings.TrimSpace(name)
	firstHan := strings.IndexFunc(name, isHan)
	if firstHan < 0 {
		return name, ""
	}
	before := name[:firstHan]
	cut, skip := strings.LastIndex(before, "/"), 1
	if full := strings.LastIndex(before, "／"); full > cut {
		cut, skip = full, len("／")
	}
	if cut < 0 {
		cut, skip = firstHan, 0
	}
	return strings.TrimSpace(name[:cut]), strings.TrimSpace(name[cut+skip:])
}

func tidyCeramicText(text string) string {
	return strings.Trim(strings.Join(strings.Fields(text), " "), " ,;/-")
}

func parseCeramicName(name string) ceramicName {
	russian, chinese := splitCeramicName(name)
	result := ceramicName{Russian: russian, Chinese: chinese, ProductType: tidyCeramicText(russian)}
	mentions, _ := extractSizeMentions(russian)
	if len(mentions) == 1 {
		size := mentions[0].Size
		result.Size = &size
		result.ProductType = tidyCeramicText(strings.Replace(russian, mentions[0].Raw, " ", 1))
		result.Template = strings.Replace(russian, mentions[0].Raw, "{SIZE}", 1)
	} else if len(mentions) == 0 {
		if chineseMentions, _ := extractSizeMentions(chinese); len(chineseMentions) == 1 {
			size := chineseMentions[0].Size
			result.Size = &size
		}
	}
	if result.ProductType == "" {
		result.ProductType = tidyCeramicText(chinese)
	}
	return result
}

// ceramicRussianName writes the Russian part of a name for a type and size, the way the base
// writes names of this type.
func ceramicRussianName(template, productType string, size *CeramicSize) string {
	switch {
	case size == nil:
		return productType
	case template != "":
		return strings.Replace(template, "{SIZE}", formatCeramicSizeLong(*size), 1)
	default:
		return productType + " " + formatCeramicSizeLong(*size)
	}
}

// chineseFullKey compares Chinese names with their numbers, without spaces and punctuation.
func chineseFullKey(name string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(toHalfWidth(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
