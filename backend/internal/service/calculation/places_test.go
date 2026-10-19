package calculation

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func placeWarnings(in CalculationItemInput) (CalculationItem, []string) {
	out := CalculateItem(in, decimal.RequireFromString("450"), decimal.Zero, decimal.Zero)
	var found []string
	for _, warning := range append(slices.Clone(out.Errors), out.Warnings...) {
		if strings.Contains(warning, "в месте") || strings.Contains(warning, "мест") {
			found = append(found, warning)
		}
	}
	return out, found
}

// Смешанная упаковка из инвойса LDO-10: 70 пар в 9 местах (коробки по 7 и 8 пар) — не ошибка,
// пар в месте просто нет. Ровное деление по-прежнему даёт пар в месте.
func TestMixedPackingIsNotAnError(t *testing.T) {
	out, warnings := placeWarnings(CalculationItemInput{Pairs: "70", Places: "9"})
	if len(warnings) != 0 || out.PairsPerPlace != "" || out.Pairs != "70" || out.Places != "9" {
		t.Fatalf("70 пар / 9 мест: warnings %v, pairs %q places %q per place %q", warnings, out.Pairs, out.Places, out.PairsPerPlace)
	}
	out, warnings = placeWarnings(CalculationItemInput{Pairs: "72", Places: "9"})
	if len(warnings) != 0 || out.PairsPerPlace != "8" {
		t.Fatalf("72 пары / 9 мест: warnings %v, per place %q", warnings, out.PairsPerPlace)
	}
}

// Пар в месте, указанные явно, проверяются как раньше.
func TestExplicitPairsPerPlaceStillChecked(t *testing.T) {
	if _, warnings := placeWarnings(CalculationItemInput{Pairs: "70", Places: "9", PairsPerPlace: "7.5"}); len(warnings) == 0 {
		t.Fatal("дробные пар в месте из файла должны давать ошибку")
	}
	if _, warnings := placeWarnings(CalculationItemInput{Pairs: "70", Places: "9", PairsPerPlace: "8"}); len(warnings) == 0 {
		t.Fatal("70 ≠ 9 × 8 должно давать ошибку")
	}
}
