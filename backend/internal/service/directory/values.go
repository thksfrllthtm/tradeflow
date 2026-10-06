package directory

// Общие значения справочника товаров: синонимы (JSON-массив строк) и неотрицательные числа.

import (
	"encoding/json"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

func normalizeSynonyms(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseOptionalNonNegative(value, field string) (*decimal.Decimal, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) > 12) {
		return nil, &service.ValidationError{Field: field, Message: "Укажите не более 12 знаков после запятой"}
	}
	d, err := decimal.NewFromString(value)
	if err != nil || d.IsNegative() {
		return nil, &service.ValidationError{Field: field, Message: "Значение должно быть неотрицательным числом"}
	}
	return &d, nil
}

func mustJSON(value []string) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte("[]")
	}
	return data
}

// decodeJSONStrings is deliberately tolerant of legacy null values while the
// schema itself always writes arrays.
func decodeJSONStrings(raw repository.JSON) []string {
	var values []string
	if len(raw) == 0 || string(raw) == "null" {
		return values
	}
	// The operator service stores arrays; use a small JSON decoder without
	// exposing database internals in the HTTP layer.
	_ = json.Unmarshal(raw, &values)
	return values
}
