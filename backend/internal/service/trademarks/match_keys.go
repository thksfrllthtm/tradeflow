package trademarks

// Ключи сравнения наименований: «на вид», транслитерация, отличие в одну букву, части наименования знака.

import (
	"regexp"
	"strings"
	"unicode"
)

// similarSpelling — ключи отличаются одной буквой (см. правила в начале match.go). Любая одна буква — только
// у латиницы (latin): в русском одна буква — обычно другое слово или окончание («Детские» и «ДЕТСКОЕ»).
func similarSpelling(word, key []rune, latin bool) bool {
	if len(word) == len(key) {
		diff := -1
		for index := range word {
			if word[index] != key[index] {
				if diff >= 0 {
					return false
				}
				diff = index
			}
		}
		return diff >= 0 && ((latin && len(key) >= 7) || (len(key) >= 4 && confusable(word[diff], key[diff])))
	}
	if !latin || len(key) < 7 || len(word)-len(key) > 1 || len(key)-len(word) > 1 {
		return false
	}
	short, long := word, key
	if len(short) > len(long) {
		short, long = long, short
	}
	for index := range long {
		if string(long[:index])+string(long[index+1:]) == string(short) {
			return true
		}
	}
	return false
}

var confusablePairs = map[[2]rune]bool{}

func init() {
	for _, group := range []string{"OQD", "IJ", "UV", "VY", "EF", "CG", "MN", "BD", "SZ", "PR", "KX", "ИЙ", "ШЩ", "ЬЪБ"} {
		letters := []rune(group)
		for _, a := range letters {
			for _, b := range letters {
				if a != b {
					confusablePairs[[2]rune{a, b}] = true
				}
			}
		}
	}
}

func confusable(a, b rune) bool { return confusablePairs[[2]rune{a, b}] }

func deletions(key string) []string {
	runes := []rune(key)
	out := make([]string, 0, len(runes))
	for index := range runes {
		out = append(out, string(runes[:index])+string(runes[index+1:]))
	}
	return out
}

// nameVariants — наименование знака целиком и его части: «ARIEL ЭЙРИЛ» — ещё ARIEL и ЭЙРИЛ, «X / Y» и
// «X (Y)» — X и Y.
func nameVariants(name string) []string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune("/()[],;«»\"“”\n", r) })
	out := []string{}
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if letterCount(value) > 0 && !seen[value] && len(splitWords(value)) <= maxWindow {
			seen[value] = true
			out = append(out, value)
		}
	}
	add(name)
	for _, part := range parts {
		add(part)
		// «BALLANTINE’S FINEST B», «ESSE EXCHANGE M»: буква через пробел в конце — пометка варианта знака,
		// в наименовании товара её обычно нет.
		if match := trailingLetter.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			add(match[1])
		}
		latin, cyrillic := []string{}, []string{}
		for _, word := range splitWords(part) {
			if isCyrillic(word) {
				cyrillic = append(cyrillic, word)
			} else {
				latin = append(latin, word)
			}
		}
		// Части другого алфавита — отдельный знак, только если в каждой от 3 букв («ВД-40» не даёт «40»).
		if letterCount(strings.Join(latin, "")) >= 3 && letterCount(strings.Join(cyrillic, "")) >= 3 {
			add(strings.Join(latin, " "))
			add(strings.Join(cyrillic, " "))
		}
	}
	return out
}

var trailingLetter = regexp.MustCompile(`^(.*\S)\s+\pL\.?$`)

func isCyrillic(word string) bool {
	cyrillic, latin := 0, 0
	for _, r := range word {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	return cyrillic > latin
}

func splitWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '$' && r != '@' && r != '|'
	})
}

func upperAlnum(text string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

var lookalikes = map[rune]rune{
	'А': 'A', 'В': 'B', 'Е': 'E', 'Ё': 'E', 'К': 'K', 'М': 'M', 'Н': 'H', 'О': 'O', 'Р': 'P', 'С': 'C', 'Т': 'T',
	'У': 'Y', 'Х': 'X', 'І': 'I', 'Ѕ': 'S', 'Ј': 'J', '0': 'O', '1': 'I', '|': 'I', 'L': 'I', '5': 'S', '8': 'B', '$': 'S', '@': 'A',
}

// visualKey — как слово выглядит: двойники и путаемые символы сведены к одной латинской букве (без учёта
// регистра: l и I в строчном и заглавном виде — одна буква).
func visualKey(text string) string {
	text = strings.ReplaceAll(strings.ToLower(text), "rn", "m")
	var builder strings.Builder
	for _, r := range strings.ToUpper(text) {
		if mapped, ok := lookalikes[r]; ok {
			r = mapped
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return strings.ReplaceAll(builder.String(), "VV", "W")
}

var translit = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "E", 'Ж': "ZH", 'З': "Z", 'И': "I", 'Й': "Y",
	'К': "K", 'Л': "L", 'М': "M", 'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U", 'Ф': "F",
	'Х': "H", 'Ц': "TS", 'Ч': "CH", 'Ш': "SH", 'Щ': "SCH", 'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "YU", 'Я': "YA",
	'Ә': "A", 'Ғ': "G", 'Қ': "K", 'Ң': "N", 'Ө': "O", 'Ұ': "U", 'Ү': "U", 'Һ': "H", 'І': "I",
}

// translitKey — латиницей: кириллица транслитерацией, латиница как есть.
func translitKey(text string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(text) {
		if latin, ok := translit[r]; ok {
			builder.WriteString(latin)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func shortText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
