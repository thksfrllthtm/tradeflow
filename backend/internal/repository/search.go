package repository

// Общие помощники поиска по словам: основа слова, экранирование LIKE и условие «каждое слово встречается».

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var digitsQuery = regexp.MustCompile(`^[\d\s.]+$`)

// SearchStem cuts the ending off a word so other forms match: «резиновые» → «резин»
// finds «резиной», «фарфоровая» → «фарфор» finds «фарфора». Short words stay whole.
func SearchStem(word string) string {
	runes := []rune(word)
	n := len(runes)
	switch {
	case n <= 4:
		return word
	case n <= 7:
		return string(runes[:max(4, n-2)])
	default:
		return string(runes[:n-4])
	}
}

// wordsWhere: every word's stem occurs in one of the columns.
func wordsWhere(query *gorm.DB, text string, columns ...string) *gorm.DB {
	expr, args := wordsExpr(text, columns...)
	return query.Where(expr, args...)
}

// wordsExpr — условие «каждое слово запроса встречается в одной из колонок» и его аргументы.
func wordsExpr(text string, columns ...string) (string, []any) {
	conditions := []string{}
	args := []any{}
	for _, word := range strings.Fields(text) {
		pattern := "%" + escapeLike(SearchStem(strings.ToLower(word))) + "%"
		parts := make([]string, 0, len(columns))
		for _, column := range columns {
			parts = append(parts, column+" ILIKE ?")
			args = append(args, pattern)
		}
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}
	if len(conditions) == 0 {
		return "TRUE", nil
	}
	return strings.Join(conditions, " AND "), args
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
}
