package service

// Округление по правилам заполнения таможенной декларации (по номеру графы). Везде — по
// математическим правилам: 5 и больше — вверх, меньше 5 — вниз (decimal.Round округляет
// половину от нуля).
//   - деньги: гр.12, 22, 42, 45, 47 (основа в денежных единицах и сумма), B; в СД — гр.8, 21 —
//     до двух знаков после запятой;
//   - вес брутто и нетто (гр.35, 38; в СД — гр.18, 19) — до трёх знаков, а если масса меньше
//     одного грамма — до шести;
//   - дополнительные единицы (гр.41) — до двух знаков, а если меньше 0,01 — до шести; штуки и
//     пары — целые (в программе количество и так только целое).

import "github.com/shopspring/decimal"

var (
	oneGramKG     = decimal.New(1, -3)
	minExtraUnits = decimal.New(1, -2)
)

// RoundMoney — денежная сумма до двух знаков.
func RoundMoney(value decimal.Decimal) decimal.Decimal {
	return value.Round(2)
}

// MoneyText — денежная сумма строкой с двумя знаками («1234.50»).
func MoneyText(value decimal.Decimal) string {
	return RoundMoney(value).StringFixed(2)
}

// RoundWeightKG — вес в килограммах: до трёх знаков, меньше одного грамма — до шести.
func RoundWeightKG(value decimal.Decimal) decimal.Decimal {
	if !value.IsZero() && value.Abs().LessThan(oneGramKG) {
		return value.Round(6)
	}
	return value.Round(3)
}

// WeightKGText — вес строкой: три знака («3073.800»), меньше грамма — шесть («0.000450»).
func WeightKGText(value decimal.Decimal) string {
	rounded := RoundWeightKG(value)
	if !rounded.IsZero() && rounded.Abs().LessThan(oneGramKG) {
		return rounded.StringFixed(6)
	}
	return rounded.StringFixed(3)
}

// RoundExtraUnits — количество в дополнительных единицах: до двух знаков, меньше 0,01 — до шести.
func RoundExtraUnits(value decimal.Decimal) decimal.Decimal {
	if !value.IsZero() && value.Abs().LessThan(minExtraUnits) {
		return value.Round(6)
	}
	return value.Round(2)
}
