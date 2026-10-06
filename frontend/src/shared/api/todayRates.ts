import { useEffect, useState } from 'react'

// Курс НБРК на сегодня (USD и EUR) — для шапки и страниц входа и регистрации. Запрос открыт без
// входа (сервер запоминает ответ). На странице курс запрашивается один раз: шапка каждой
// страницы берёт уже полученный ответ, неудачный запрос повторяется при следующем показе.
export type TodayRate = { currency: string; rate: string; date: string; source: string }

let pending: Promise<TodayRate[]> | null = null

function loadTodayRates(): Promise<TodayRate[]> {
  pending ??= fetch('/api/rates/today', { credentials: 'include' })
    .then((response) => response.ok ? response.json() as Promise<{ rates?: TodayRate[] }> : { rates: [] })
    .then((body) => body.rates ?? [])
    .catch(() => [])
    .then((rates) => { if (rates.length === 0) pending = null; return rates })
  return pending
}

// null — курс ещё загружается; [] — курса нет.
export function useTodayRates() {
  const [rates, setRates] = useState<TodayRate[] | null>(null)
  useEffect(() => {
    let active = true
    void loadTodayRates().then((value) => { if (active) setRates(value) })
    return () => { active = false }
  }, [])
  return rates
}
