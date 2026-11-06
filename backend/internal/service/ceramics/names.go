package ceramics

// Ceramics names and marks read without AI: normalized mark and type, Chinese terms, the base
// name split into Russian part, size and Chinese part, keys for matching Chinese names.

import (
	"strings"
	"unicode"
)

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

// similarCeramicTypes reports type names where one is a more specific form of the other:
// «Керамическая тарелка» and «Керамическая тарелка салатница».
func similarCeramicTypes(a, b string) bool {
	a, b = normalizeCeramicTypeKey(a), normalizeCeramicTypeKey(b)
	if a == b {
		return false
	}
	return strings.HasPrefix(b, a+" ") || strings.HasPrefix(a, b+" ")
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
