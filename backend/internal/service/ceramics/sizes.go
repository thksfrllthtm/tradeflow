package ceramics

// Ceramics sizes read without AI: a number with a unit in a name (7寸, 20 см, 9 дюймов) or a bare number.

import (
	"slices"
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
		if text := value.String(); !slices.Contains(values, text) {
			values = append(values, text)
		}
	}
	return values
}
