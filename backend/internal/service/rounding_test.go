package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestRoundingByDeclarationRules(t *testing.T) {
	d := decimal.RequireFromString
	cases := []struct{ got, want string }{
		{MoneyText(d("1.005")), "1.01"}, // математически: пятёрка — вверх
		{MoneyText(d("2.675")), "2.68"},
		{MoneyText(d("1234.5649")), "1234.56"},
		{WeightKGText(d("3073.8")), "3073.800"},
		{WeightKGText(d("12.3455")), "12.346"},
		{WeightKGText(d("0.0004567")), "0.000457"}, // меньше грамма — шесть знаков
		{WeightKGText(d("0.0009996")), "0.001"},    // после округления — ровно грамм
		{WeightKGText(d("0.001")), "0.001"},
		{WeightKGText(decimal.Zero), "0.000"},
		{RoundUnitPrice(d("0.16667")).String(), "0.1667"},
		{RoundUnitPrice(d("4.07")).StringFixed(4), "4.0700"},
		{RoundExtraUnits(d("12.345")).String(), "12.35"},
		{RoundExtraUnits(d("0.0012345")).String(), "0.001235"}, // меньше 0,01 — шесть знаков
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %s, want %s", c.got, c.want)
		}
	}
}
