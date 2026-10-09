package calculation

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func dutySnapshot(kind, percent, amount, currency, unit string) map[string]any {
	return map[string]any{"tariffText": "ставка", "duty": map[string]any{"type": kind, "percent": percent, "amount": amount, "currency": currency, "unit": unit}}
}

func dec(value string) *decimal.Decimal { parsed := decimal.RequireFromString(value); return &parsed }

// ТС 10 000 ₸, нетто 100 кг, 5 000 штук, курс EUR 500, USD 450.
func TestDutyAmountBySchemes(t *testing.T) {
	customs, net, quantity := decimal.RequireFromString("10000"), dec("100"), dec("5000")
	usd, euro := decimal.RequireFromString("450"), decimal.RequireFromString("500")
	for name, test := range map[string]struct {
		snapshot map[string]any
		want     string
	}{
		"процент": {dutySnapshot("percentage", "10", "", "", ""), "1000"},
		"не менее — берётся за кг":   {dutySnapshot("combined_minimum", "15", "0.15", "EUR", "кг"), "7500"},
		"не менее — берётся процент": {dutySnapshot("combined_minimum", "15", "0.01", "EUR", "кг"), "1500"},
		"плюс — сумма":               {dutySnapshot("combined_plus", "5", "0.2", "EUR", "кг"), "10500"},
		"за пару":                    {dutySnapshot("specific", "", "1", "EUR", "пару"), "2500000"},
		"за 1000 шт":                 {dutySnapshot("specific", "", "2", "EUR", "1000 шт"), "5000"},
		"за тонну в USD":             {dutySnapshot("specific", "", "10", "USD", "т"), "450"},
	} {
		var warnings []string
		got, ok := dutyAmount(test.snapshot, customs, net, quantity, usd, euro, &warnings)
		if !ok || !got.Equal(decimal.RequireFromString(test.want)) {
			t.Errorf("%s: %s, %v, %v", name, got, ok, warnings)
		}
	}
}

func TestDutyAmountReportsWhatIsMissing(t *testing.T) {
	customs, usd := decimal.RequireFromString("10000"), decimal.RequireFromString("450")
	check := func(snapshot map[string]any, net *decimal.Decimal, euro decimal.Decimal, want string, unsupported bool) {
		t.Helper()
		var warnings []string
		if _, ok := dutyAmount(snapshot, customs, net, nil, usd, euro, &warnings); ok || len(warnings) != 1 || !strings.Contains(warnings[0], want) {
			t.Fatalf("ожидалось «%s»: %v", want, warnings)
		}
		errorsList, _ := SplitCalculationIssues(warnings)
		if len(errorsList) != 1 || UnsupportedSchemeIssue(warnings[0]) != unsupported {
			t.Fatalf("«%s» должно быть ошибкой строки (итог неполный: %v)", warnings[0], unsupported)
		}
	}
	check(dutySnapshot("specific", "", "0.5", "EUR", "л"), dec("1"), decimal.RequireFromString("500"), "не поддерживается", true)
	check(dutySnapshot("combined_minimum", "15", "0.15", "EUR", "кг"), nil, decimal.RequireFromString("500"), "укажите вес нетто", false)
	check(dutySnapshot("specific", "", "0.5", "EUR", "кг"), dec("1"), decimal.Zero, "нет курса EUR", false)
	check(dutySnapshot("unknown", "", "", "", ""), dec("1"), decimal.Zero, "не поддерживается", true)
}
