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

type todayCache struct {
	mu        sync.Mutex
	day       string
	rates     []NBKRate
	fetchedAt time.Time
	failedAt  time.Time
	fetch     func(ctx context.Context) ([]NBKRate, error)
	now       func() time.Time
}

var today = &todayCache{fetch: FetchNBKLatestRates, now: time.Now}

// TodayRates — все валюты текущей ленты НБРК (шапка показывает USD и EUR, конвертер — все).
// Ответ НБРК помнится 30 минут; после неудачи новый
// запрос к НБРК — не чаще раза в 2 минуты, а пока есть прежние курсы, отдаются они.
func TodayRates(ctx context.Context) ([]NBKRate, error) { return today.get(ctx) }

func (c *todayCache) get(ctx context.Context) ([]NBKRate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	day := now.In(kzZone).Format("2006-01-02")
	fresh := c.day == day && len(c.rates) > 0 && now.Sub(c.fetchedAt) < todayTTL
	if fresh || (!c.failedAt.IsZero() && now.Sub(c.failedAt) < todayRetry) {
		return c.cached()
	}
	// Ответ нужен всем, кто ждёт, — уход одного посетителя со страницы не обрывает запрос к НБРК.
	ctx = context.WithoutCancel(ctx)
	rates, err := c.fetch(ctx)
	if err != nil || len(rates) == 0 {
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
