package rates

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testToday(now *time.Time, fail *bool, calls *int) *todayCache {
	return &todayCache{
		now: func() time.Time { return *now },
		fetch: func(_ context.Context, date, currency string) (NBKRate, error) {
			*calls++
			if *fail {
				return NBKRate{}, errors.New("НБРК недоступен")
			}
			return NBKRate{Currency: currency, Rate: "454.98", Date: date, Source: "НБРК"}, nil
		},
	}
}

// Курсы на сегодня запоминаются: повторные открытия страницы входа не ходят в НБРК.
func TestTodayRatesAreCached(t *testing.T) {
	now := time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) // в Казахстане уже 7 октября, 01:00
	fail, calls := false, 0
	cache := testToday(&now, &fail, &calls)
	rates, err := cache.get(context.Background())
	if err != nil || len(rates) != 2 || rates[0].Date != "2026-10-07" || calls != 2 {
		t.Fatalf("rates %+v, err %v, calls %d", rates, err, calls)
	}
	now = now.Add(10 * time.Minute)
	if _, err := cache.get(context.Background()); err != nil || calls != 2 {
		t.Fatalf("повтор через 10 минут не должен ходить в НБРК: calls %d, err %v", calls, err)
	}
	now = now.Add(31 * time.Minute)
	if _, err := cache.get(context.Background()); err != nil || calls != 4 {
		t.Fatalf("через 30 минут курс обновляется: calls %d", calls)
	}
}

// НБРК недоступен: прежние курсы остаются, новая попытка — не раньше чем через 2 минуты.
func TestTodayRatesKeepLastRatesWhenNBKFails(t *testing.T) {
	now := time.Date(2026, 10, 6, 6, 0, 0, 0, time.UTC)
	fail, calls := true, 0
	cache := testToday(&now, &fail, &calls)
	if _, err := cache.get(context.Background()); err == nil || calls != 2 {
		t.Fatalf("без курсов — ошибка: calls %d", calls)
	}
	now = now.Add(time.Minute)
	if _, err := cache.get(context.Background()); err == nil || calls != 2 {
		t.Fatalf("повтор через минуту не должен ходить в НБРК: calls %d", calls)
	}
	fail = false
	now = now.Add(2 * time.Minute)
	if rates, err := cache.get(context.Background()); err != nil || len(rates) != 2 {
		t.Fatalf("после паузы — новая попытка: %v", err)
	}
	fail = true
	now = now.Add(time.Hour)
	if rates, err := cache.get(context.Background()); err != nil || len(rates) != 2 {
		t.Fatalf("при сбое отдаются прежние курсы: %+v %v", rates, err)
	}
}
