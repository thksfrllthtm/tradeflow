package service

// Small value helpers of the import flow: decimals from cells, JSON encoding, filenames.

import (
	"encoding/json"
	"errors"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

func parseOptionalDecimal(value, label string, positive bool) (*decimal.Decimal, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, ""
	}
	parsed, err := parseDecimalStrict(trimmed)
	if err != nil {
		return nil, fmt.Sprintf("%s: %s", label, err.Error())
	}
	if positive && !parsed.IsPositive() {
		return nil, fmt.Sprintf("%s должно быть положительным", label)
	}
	return &parsed, ""
}

func parseDecimalStrict(value string) (decimal.Decimal, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\u00a0", " ")
	if strings.HasPrefix(value, "+") {
		value = strings.TrimPrefix(value, "+")
	}
	if strings.Count(value, " ") > 0 {
		parts := strings.Fields(value)
		if len(parts) < 2 {
			return decimal.Decimal{}, errors.New("некорректное число")
		}
		for _, part := range parts[1:] {
			integerGroup := part
			if separator := strings.IndexAny(integerGroup, ".,"); separator >= 0 {
				fraction := integerGroup[separator+1:]
				integerGroup = integerGroup[:separator]
				if !allDigits(fraction) {
					return decimal.Decimal{}, errors.New("неоднозначный разделитель тысяч")
				}
			}
			if len(integerGroup) != 3 || !allDigits(integerGroup) {
				return decimal.Decimal{}, errors.New("неоднозначный разделитель тысяч")
			}
		}
		value = strings.Join(parts, "")
	}
	comma, dot := strings.LastIndex(value, ","), strings.LastIndex(value, ".")
	if comma >= 0 && dot >= 0 {
		decimalSeparator := ','
		thousandSeparator := '.'
		if dot > comma {
			decimalSeparator, thousandSeparator = '.', ','
		}
		chunks := strings.Split(value, string(thousandSeparator))
		for index, chunk := range chunks {
			if index > 0 && len(chunk) != 3 && !strings.ContainsRune(chunk, decimalSeparator) {
				return decimal.Decimal{}, errors.New("неоднозначный формат числа")
			}
		}
		value = strings.ReplaceAll(value, string(thousandSeparator), "")
		value = strings.ReplaceAll(value, string(decimalSeparator), ".")
	} else if comma >= 0 || dot >= 0 {
		separator := ","
		if dot >= 0 {
			separator = "."
		}
		if strings.Count(value, separator) != 1 {
			return decimal.Decimal{}, errors.New("неоднозначный формат числа")
		}
		parts := strings.Split(value, separator)
		if len(parts) != 2 || !allDigits(strings.TrimPrefix(parts[0], "-")) || !allDigits(parts[1]) {
			return decimal.Decimal{}, errors.New("некорректное число")
		}
		// A three-digit fractional part is ambiguous for values such as 1,000,
		// but a leading-zero decimal like 0.041 is unambiguously fractional.
		if len(parts[1]) == 3 && strings.TrimPrefix(parts[0], "-") != "0" {
			return decimal.Decimal{}, errors.New("неоднозначный десятичный разделитель; исправьте значение вручную")
		}
		value = parts[0] + "." + parts[1]
	}
	if !regexp.MustCompile(`^-?\d+(\.\d+)?$`).MatchString(value) {
		return decimal.Decimal{}, errors.New("некорректное число")
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Decimal{}, errors.New("некорректное число")
	}
	return parsed, nil
}

func encodeJSON(value any) repository.JSON {
	bytes, _ := json.Marshal(value)
	return repository.JSON(bytes)
}

func decodeWarnings(value repository.JSON) []string {
	var warnings []string
	_ = json.Unmarshal(value, &warnings)
	return warnings
}

func safeFilename(value string) string {
	name := filepath.Base(value)
	name = strings.TrimSpace(name)
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
