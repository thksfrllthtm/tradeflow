package service

// Export of a gloves calculation result to an XLSX workbook.

import (
	"strconv"
	"strings"
	"unicode"
)

func formatExportNumber(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	if normalized == "" {
		return value
	}

	sign := ""
	if normalized[0] == '-' || normalized[0] == '+' {
		sign = normalized[:1]
		normalized = normalized[1:]
	}
	parts := strings.Split(normalized, ".")
	if len(parts) > 2 || parts[0] == "" {
		return value
	}
	for _, char := range parts[0] {
		if !unicode.IsDigit(char) {
			return value
		}
	}
	if len(parts) == 2 {
		for _, char := range parts[1] {
			if !unicode.IsDigit(char) {
				return value
			}
		}
	}

	integer := parts[0]
	for index := len(integer) - 3; index > 0; index -= 3 {
		integer = integer[:index] + " " + integer[index:]
	}
	if len(parts) == 2 {
		return sign + integer + "," + parts[1]
	}
	return sign + integer
}

// exportNumericCell returns a real numeric value for Excel instead of a
// pre-formatted string. Excel stores the numeric value independently from the
// display separator and applies the user's locale (comma in Russian Excel).
func exportNumericCell(value string) any {
	normalized := strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, ",", ".")
	if normalized == "" {
		return ""
	}
	number, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return value
	}
	return number
}

func calculationItemDisplayName(item CalculationItem) string {
	if item.DirectorySnapshot != nil {
		if name := directoryDisplayName(item.DirectorySnapshot); strings.TrimSpace(name) != "" {
			return name
		}
	}
	return strings.TrimSpace(item.StandardName)
}
