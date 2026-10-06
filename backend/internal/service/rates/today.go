package rates

// Курсы НБРК на сегодня для страниц входа и регистрации. Запрос открыт без входа, поэтому дата
// берётся на сервере, а ответ запоминается: открытая страница не нагружает сайт НБРК.

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	todayTTL   = 30 * time.Minute
	todayRetry = 2 * time.Minute
)

// kzZone — время Казахстана (единый пояс UTC+5): «сегодня» для курса.
var kzZone = time.FixedZone("UTC+5", 5*60*60)

// TodayCurrencies — курсы на странице входа: доллар (расчёт) и евро (справочно).
var TodayCurrencies = []string{"USD", "EUR"}

type todayCache struct {
	mu        sync.Mutex
	day       string
	rates     []NBKRate
	fetchedAt time.Time
	failedAt  time.Time
	fetch     func(ctx context.Context, date, currency string) (NBKRate, error)
	now       func() time.Time
}

var today = &todayCache{fetch: FetchNBKRate, now: time.Now}

// TodayRates — курсы USD и EUR на сегодня. Ответ НБРК помнится 30 минут; после неудачи новый
// запрос к НБРК — не чаще раза в 2 минуты, а пока есть прежние курсы, отдаются они.
func TodayRates(ctx context.Context) ([]NBKRate, error) { return today.get(ctx) }

func (c *todayCache) get(ctx context.Context) ([]NBKRate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	day := now.In(kzZone).Format("2006-01-02")
	fresh := c.day == day && len(c.rates) == len(TodayCurrencies) && now.Sub(c.fetchedAt) < todayTTL
	if fresh || (!c.failedAt.IsZero() && now.Sub(c.failedAt) < todayRetry) {
		return c.cached()
	}
	// Ответ нужен всем, кто ждёт, — уход одного посетителя со страницы не обрывает запрос к НБРК.
	ctx = context.WithoutCancel(ctx)
	rates := make([]NBKRate, 0, len(TodayCurrencies))
	for _, currency := range TodayCurrencies {
		if rate, err := c.fetch(ctx, day, currency); err == nil {
			rates = append(rates, rate)
		}
	}
	if len(rates) == 0 {
		c.failedAt = now
		return c.cached()
	}
	c.day, c.rates, c.fetchedAt, c.failedAt = day, rates, now, time.Time{}
	return c.cached()
}

func (c *todayCache) cached() ([]NBKRate, error) {
	if len(c.rates) == 0 {
		return nil, errors.New("курс НБРК сейчас недоступен")
	}
	return append([]NBKRate(nil), c.rates...), nil
}
