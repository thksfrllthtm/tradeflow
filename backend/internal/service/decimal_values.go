package service

// Decimal parsing, validation and formatting helpers used by the calculation services.

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

func parsePositive(value, field string, allowEmpty bool) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" && allowEmpty {
		return decimal.Zero, nil
	}
	parsed, err := parseDecimal(value)
	if err != nil || !parsed.IsPositive() {
		return decimal.Zero, &ValidationError{Field: field, Message: "Значение должно быть положительным числом"}
	}
	return parsed, nil
}

func parseOptionalPositive(value, field string) (*decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := parseDecimal(value)
	if err != nil || !parsed.IsPositive() {
		return nil, &ValidationError{Field: field, Message: "Значение должно быть положительным числом"}
	}
	return &parsed, nil
}

func calcOptionalNonNegative(value, field string) (*decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := parseDecimal(value)
	if err != nil || parsed.IsNegative() {
		return nil, &ValidationError{Field: field, Message: "Значение не может быть отрицательным"}
	}
	return &parsed, nil
}

func resolveCustomsFee(directKZT, mrpValue, multiplierValue string) (*decimal.Decimal, error) {
	direct := strings.TrimSpace(directKZT)
	if direct != "" {
		return calcOptionalNonNegative(direct, "customsFeeKzt")
	}
	mrp := strings.TrimSpace(mrpValue)
	multiplier := strings.TrimSpace(multiplierValue)
	if mrp == "" && multiplier == "" {
		return nil, nil
	}
	mrpDecimal, err := calcOptionalNonNegative(mrp, "mrp")
	if err != nil {
		return nil, err
	}
	if mrpDecimal == nil {
		return nil, &ValidationError{Field: "mrp", Message: "Укажите МРП для расчёта таможенного сбора"}
	}
	mult, err := calcOptionalNonNegative(multiplier, "feeMultiplier")
	if err != nil {
		return nil, err
	}
	if mult == nil {
		return nil, &ValidationError{Field: "feeMultiplier", Message: "Укажите множитель для расчёта таможенного сбора"}
	}
	total := mrpDecimal.Mul(*mult).Round(2)
	return &total, nil
}

func parseNonNegativeDefault(value string, def decimal.Decimal, field string) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return def, nil
	}
	parsed, err := parseDecimal(value)
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, &ValidationError{Field: field, Message: "Значение не может быть отрицательным"}
	}
	return parsed, nil
}

func parseDecimal(value string) (decimal.Decimal, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), " ", "")
	if strings.Contains(value, ",") && strings.Contains(value, ".") {
		return decimal.Zero, fmt.Errorf("ambiguous decimal")
	}
	separator := "."
	if strings.Contains(value, ",") {
		separator = ","
	}
	if strings.Count(value, separator) > 1 {
		return decimal.Zero, fmt.Errorf("ambiguous decimal")
	}
	if strings.Contains(value, separator) {
		parts := strings.Split(value, separator)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return decimal.Zero, fmt.Errorf("invalid decimal")
		}
		integerPart := strings.TrimPrefix(parts[0], "-")
		if len(parts[1]) == 3 && integerPart != "0" {
			return decimal.Zero, fmt.Errorf("ambiguous decimal")
		}
	}
	value = strings.ReplaceAll(value, ",", ".")
	return decimal.NewFromString(value)
}

func parseItemDecimal(value, label string, warnings *[]string) *decimal.Decimal {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := parseDecimal(value)
	if err != nil {
		*warnings = append(*warnings, label+": исправьте число")
		return nil
	}
	return &parsed
}

func validWeightUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kg", "кг", "g", "г":
		return true
	default:
		return false
	}
}

func decimalPtrString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func decPtr(v decimal.Decimal) *decimal.Decimal { return &v }

func mustDecimal(value string) decimal.Decimal { v, _ := decimal.NewFromString(value); return v }

func stringValue(m map[string]any, key string) string { v, _ := m[key].(string); return v }

func decimalValue(m map[string]any, key string) (decimal.Decimal, bool) {
	v := stringValue(m, key)
	if v == "" {
		if n, ok := m[key].(float64); ok {
			return decimal.NewFromFloat(n), true
		}
	}
	d, err := parseDecimal(v)
	return d, err == nil
}

func decimalValueFromAny(value any) (decimal.Decimal, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := parseDecimal(typed)
		return parsed, err == nil
	case float64:
		return decimal.NewFromFloat(typed), true
	case float32:
		return decimal.NewFromFloat(float64(typed)), true
	case decimal.Decimal:
		return typed, true
	default:
		parsed, err := parseDecimal(fmt.Sprint(value))
		return parsed, err == nil
	}
}

func decimalValueOrZero(value *decimal.Decimal) decimal.Decimal {
	if value == nil {
		return decimal.Zero
	}
	return *value
}
