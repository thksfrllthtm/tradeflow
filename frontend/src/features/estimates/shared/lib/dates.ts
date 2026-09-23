// Сегодняшняя дата в формате YYYY-MM-DD (так её хранит backend).
export const todayIso = () => new Date().toISOString().slice(0, 10)

// YYYY-MM-DD → «21.09.2026» для списка смет.
export function formatRuDate(value: string) {
  return new Intl.DateTimeFormat('ru-RU').format(new Date(`${value}T00:00:00`))
}
