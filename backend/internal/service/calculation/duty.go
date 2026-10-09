package calculation

// Ввозная пошлина строки по ставке номенклатуры: процентная, специфическая («0,5 EUR за 1 кг»),
// комбинированная «N%, но не менее …» (большее из двух) и «N% плюс …» (сумма двух).

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

// Начало сообщения о ставке, которую расчёт не умеет: итог «Растаможка к оплате» не выводится.
const unsupportedDutyPrefix = "Пошлина: ставка"

// dutyUnits — сколько единиц ставки в строке: «кг» и «т» — по весу нетто, «пару» и «шт» — по
// количеству, «1000 шт» — количество / 1000. ok=false — единица неизвестна; nil — нет нужного
// значения в строке (labels — чего не хватает).
func dutyUnits(unit string, net, quantity *decimal.Decimal) (*decimal.Decimal, string, bool) {
	unit = strings.ToLower(strings.TrimSpace(unit))
	per := decimal.NewFromInt(1)
	if head, rest, found := strings.Cut(unit, " "); found {
		if value, err := strconv.Atoi(head); err == nil && value > 0 {
			per, unit = decimal.NewFromInt(int64(value)), strings.TrimSpace(rest)
		}
	}
	divide := func(value *decimal.Decimal, scale decimal.Decimal) *decimal.Decimal {
		if value == nil {
			return nil
		}
		result := value.Div(scale).Div(per)
		return &result
	}
	switch unit {
	case "кг":
		return divide(net, decimal.NewFromInt(1)), "вес нетто", true
	case "т":
		return divide(net, decimal.NewFromInt(1000)), "вес нетто", true
	case "пара", "пару", "пар", "шт", "штук", "штуку", "единицу":
		return divide(quantity, decimal.NewFromInt(1)), "количество", true
	}
	return nil, "", false
}

// dutyAmount — пошлина строки в тенге (без округления). customs — ТС в тенге.
func dutyAmount(snapshot map[string]any, customs decimal.Decimal, net, quantity *decimal.Decimal, usd, euro decimal.Decimal, warnings *[]string) (decimal.Decimal, bool) {
	duty, _ := snapshot["duty"].(map[string]any)
	kind := service.StringValue(duty, "type")
	addOnce := func(message string) {
		if !slices.Contains(*warnings, message) {
			*warnings = append(*warnings, message)
		}
	}
	percentPart := func() (decimal.Decimal, bool) {
		percent, ok := service.DecimalValue(duty, "percent")
		if !ok || percent.IsNegative() {
			addOnce("Ставка пошлины требует проверки")
			return decimal.Zero, false
		}
		return customs.Mul(percent).Div(decimal.NewFromInt(100)), true
	}
	specificPart := func() (decimal.Decimal, bool) {
		amount, ok := service.DecimalValue(duty, "amount")
		if !ok || amount.IsNegative() {
			addOnce("Ставка пошлины требует проверки")
			return decimal.Zero, false
		}
		unit := service.StringValue(duty, "unit")
		units, need, known := dutyUnits(unit, net, quantity)
		if !known {
			addOnce(fmt.Sprintf("%s «%s» за %s не поддерживается расчётом — посчитайте пошлину вручную", unsupportedDutyPrefix, service.StringValue(snapshot, "tariffText"), unit))
			return decimal.Zero, false
		}
		if units == nil {
			addOnce(fmt.Sprintf("Пошлина за %s: укажите %s", unit, need))
			return decimal.Zero, false
		}
		var rate decimal.Decimal
		switch currency := strings.ToUpper(service.StringValue(duty, "currency")); currency {
		case "EUR":
			if euro.IsZero() {
				addOnce("Пошлина в EUR: нет курса EUR — НБРК не ответил, укажите курс EUR в параметрах")
				return decimal.Zero, false
			}
			rate = euro
		case "USD":
			rate = usd
		case "KZT", "":
			rate = decimal.NewFromInt(1)
		default:
			addOnce(fmt.Sprintf("%s в %s не поддерживается расчётом", unsupportedDutyPrefix, currency))
			return decimal.Zero, false
		}
		return amount.Mul(*units).Mul(rate), true
	}
	switch kind {
	case "percentage":
		return percentPart()
	case "specific":
		return specificPart()
	case "combined_minimum", "combined_plus":
		byValue, ok := percentPart()
		if !ok {
			return decimal.Zero, false
		}
		byUnits, ok := specificPart()
		if !ok {
			return decimal.Zero, false
		}
		if kind == "combined_plus" {
			return byValue.Add(byUnits), true
		}
		return decimal.Max(byValue, byUnits), true
	}
	addOnce(fmt.Sprintf("%s «%s» не поддерживается расчётом — посчитайте пошлину вручную", unsupportedDutyPrefix, service.StringValue(snapshot, "tariffText")))
	return decimal.Zero, false
}
