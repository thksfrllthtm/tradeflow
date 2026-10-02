// Сегодняшняя дата по местному времени в формате YYYY-MM-DD (так её хранит backend).
// toISOString() дал бы дату по UTC — в Казахстане до 5 утра это ещё вчера.
export function todayIso() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
}

// YYYY-MM-DD → «21.09.2026» для списка смет.
export function formatRuDate(value: string) {
  return new Intl.DateTimeFormat('ru-RU').format(new Date(`${value}T00:00:00`))
}
