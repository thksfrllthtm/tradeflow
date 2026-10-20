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

// Время «10:01».
export function formatRuTime(value: Date = new Date()) {
  return value.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

// «Сохранено в 14:30»; сохранено не сегодня — «Сохранено 19.10.2026 в 14:30».
export function savedAtText(value: Date | string = new Date(), now = new Date()) {
  const date = typeof value === 'string' ? new Date(value) : value
  if (Number.isNaN(date.getTime())) return 'Сохранено'
  const sameDay = date.toDateString() === now.toDateString()
  return `Сохранено ${sameDay ? '' : `${date.toLocaleDateString('ru-RU')} `}в ${formatRuTime(date)}`
}
