package goods

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestResolveEURFetchesOnlyWhenNeeded(t *testing.T) {
	calls := 0
	original := fetchEURRate
	defer func() { fetchEURRate = original }()
	fetchEURRate = func(context.Context, string) (decimal.Decimal, error) { calls++; return decimal.RequireFromString("500.35"), nil }
	usdRow := []GoodsItemInput{{CostCurrency: "USD"}}
	eurDuty := []map[string]any{{"duty": map[string]any{"type": "specific", "currency": "EUR"}}}
	if rate := resolveEUR(context.Background(), nil, "2026-10-09", usdRow, []map[string]any{nil}); rate != nil || calls != 0 {
		t.Fatalf("евро не нужен — НБРК не спрашиваем: %v, %d", rate, calls)
	}
	if rate := resolveEUR(context.Background(), nil, "2026-10-09", usdRow, eurDuty); rate == nil || rate.String() != "500.35" || calls != 1 {
		t.Fatalf("пошлина в EUR — курс НБРК: %v", rate)
	}
	entered := decimal.RequireFromString("510")
	if rate := resolveEUR(context.Background(), &entered, "2026-10-09", usdRow, eurDuty); rate.String() != "510" || calls != 1 {
		t.Fatal("введённый курс важнее НБРК")
	}
	fetchEURRate = func(context.Context, string) (decimal.Decimal, error) { return decimal.Zero, errors.New("НБРК недоступен") }
	if rate := resolveEUR(context.Background(), nil, "2026-10-09", []GoodsItemInput{{CostCurrency: "eur"}}, []map[string]any{nil}); rate != nil {
		t.Fatal("НБРК не ответил — курса нет")
	}
}
