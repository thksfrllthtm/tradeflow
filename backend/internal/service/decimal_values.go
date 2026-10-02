package service

// Decimal parsing, validation and formatting helpers used by the calculation services.

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

func ParsePositive(value, field string, allowEmpty bool) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" && allowEmpty {
		return decimal.Zero, nil
	}
	parsed, err := ParseDecimal(value)
	if err != nil || !parsed.IsPositive() {
		return decimal.Zero, &ValidationError{Field: field, Message: "Значение должно быть положительным числом"}
	}
	return parsed, nil
}

func ParseOptionalPositive(value, field string) (*decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := ParseDecimal(value)
	if err != nil || !parsed.IsPositive() {
		return nil, &ValidationError{Field: field, Message: "Значение должно быть положительным числом"}
	}
	return &parsed, nil
}

func CalcOptionalNonNegative(value, field string) (*decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := ParseDecimal(value)
	if err != nil || parsed.IsNegative() {
		return nil, &ValidationError{Field: field, Message: "Значение не может быть отрицательным"}
	}
	return &parsed, nil
}

func ResolveCustomsFee(directKZT, mrpValue, multiplierValue string) (*decimal.Decimal, error) {
	direct := strings.TrimSpace(directKZT)
	if direct != "" {
		return CalcOptionalNonNegative(direct, "customsFeeKzt")
	}
	mrp := strings.TrimSpace(mrpValue)
	multiplier := strings.TrimSpace(multiplierValue)
	if mrp == "" && multiplier == "" {
		return nil, nil
	}
	mrpDecimal, err := CalcOptionalNonNegative(mrp, "mrp")
	if err != nil {
		return nil, err
	}
	if mrpDecimal == nil {
		return nil, &ValidationError{Field: "mrp", Message: "Укажите МРП для расчёта таможенного сбора"}
	}
	mult, err := CalcOptionalNonNegative(multiplier, "feeMultiplier")
	if err != nil {
		return nil, err
	}
	if mult == nil {
		return nil, &ValidationError{Field: "feeMultiplier", Message: "Укажите множитель для расчёта таможенного сбора"}
	}
	total := mrpDecimal.Mul(*mult).Round(2)
	return &total, nil
}

func ParseNonNegativeDefault(value string, def decimal.Decimal, field string) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return def, nil
	}
	parsed, err := ParseDecimal(value)
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, &ValidationError{Field: field, Message: "Значение не может быть отрицательным"}
	}
	return parsed, nil
}

// parseDecimal reads a number the way it is written in Kazakhstan and Russia: a dot or a comma
// separates the fraction («4.125» and «4,125» are both 4.125), thousands are separated only by
// spaces («1 500,50»). The rule is the same for every number: typed on the site or from Excel.
// A number with both a dot and a comma («1,500.50») or several separators is rejected.
func ParseDecimal(value string) (decimal.Decimal, error) {
	value = strings.NewReplacer(" ", "", " ", "", " ", "").Replace(strings.TrimSpace(value))
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
	}
	value = strings.ReplaceAll(value, ",", ".")
	return decimal.NewFromString(value)
}

func ParseItemDecimal(value, label string, warnings *[]string) *decimal.Decimal {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := ParseDecimal(value)
	if err != nil {
		*warnings = append(*warnings, label+": исправьте число")
		return nil
	}
	return &parsed
}

func ValidWeightUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kg", "кг", "g", "г":
		return true
	default:
		return false
	}
}

func DecimalPtrString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func DecPtr(v decimal.Decimal) *decimal.Decimal { return &v }

func MustDecimal(value string) decimal.Decimal { v, _ := decimal.NewFromString(value); return v }

func StringValue(m map[string]any, key string) string { v, _ := m[key].(string); return v }

func DecimalValue(m map[string]any, key string) (decimal.Decimal, bool) {
	v := StringValue(m, key)
	if v == "" {
		if n, ok := m[key].(float64); ok {
			return decimal.NewFromFloat(n), true
		}
	}
	d, err := ParseDecimal(v)
	return d, err == nil
}

func DecimalValueFromAny(value any) (decimal.Decimal, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := ParseDecimal(typed)
		return parsed, err == nil
	case float64:
		return decimal.NewFromFloat(typed), true
	case float32:
		return decimal.NewFromFloat(float64(typed)), true
	case decimal.Decimal:
		return typed, true
	default:
		parsed, err := ParseDecimal(fmt.Sprint(value))
		return parsed, err == nil
	}
}

func DecimalValueOrZero(value *decimal.Decimal) decimal.Decimal {
	if value == nil {
		return decimal.Zero
	}
	return *value
}

// decimalText — число или пусто, если его нет.
func DecimalText(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}
