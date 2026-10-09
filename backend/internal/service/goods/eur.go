package goods

// Курс EUR для сметы: введён — берётся он; нет — НБРК на дату сметы, но только когда евро нужен
// (ставка пошлины в EUR или стоимость строки в евро).

import (
	"context"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/rates"
	"github.com/shopspring/decimal"
)

// fetchEURRate — курс EUR НБРК на дату; в тестах подменяется.
var fetchEURRate = func(ctx context.Context, date string) (decimal.Decimal, error) {
	rate, err := rates.FetchNBKRate(ctx, date, "EUR")
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(rate.Rate)
}

func needsEUR(items []GoodsItemInput, snapshots []map[string]any) bool {
	for index, item := range items {
		if item.Excluded {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.CostCurrency), "EUR") {
			return true
		}
		duty, _ := snapshots[index]["duty"].(map[string]any)
		if strings.EqualFold(service.StringValue(duty, "currency"), "EUR") {
			return true
		}
	}
	return false
}

// resolveEUR — курс EUR сметы; НБРК не ответил — nil, строки с евро получат понятное предупреждение.
func resolveEUR(ctx context.Context, entered *decimal.Decimal, date string, items []GoodsItemInput, snapshots []map[string]any) *decimal.Decimal {
	if entered != nil || !needsEUR(items, snapshots) {
		return entered
	}
	rate, err := fetchEURRate(ctx, date)
	if err != nil || !rate.IsPositive() {
		return nil
	}
	return &rate
}
