// Подписи главной: приветствие по времени суток, дата в заголовке, время изменения сметы.

export function greeting(hour: number) {
  if (hour >= 5 && hour < 12) return 'Доброе утро'
  if (hour >= 12 && hour < 18) return 'Добрый день'
  if (hour >= 18 && hour < 23) return 'Добрый вечер'
  return 'Доброй ночи'
}

// «Понедельник, 28 сентября»
export function longDate(date: Date) {
  const text = new Intl.DateTimeFormat('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' }).format(date)
  return text.charAt(0).toUpperCase() + text.slice(1)
}

// Как показать смету в списке, не меняя её название. Название по умолчанию «Смета товаров · дата»
// ничего не говорит (дата есть во времени изменения): вместо него — клиент, а без клиента —
// «Смета №id». Своё название сотрудника показывается как есть.
export function estimateLabel(estimate: { id: number; name: string; client: string }, title: string) {
  const client = estimate.client.trim()
  const generic = /^Смета товаров(\s*·\s*\d{4}-\d{2}-\d{2})?$/.test(title.trim())
  if (!generic) return { title, detail: client || 'Без клиента', untitled: false }
  return client ? { title: client, detail: `Смета №${estimate.id}`, untitled: false } : { title: `Смета №${estimate.id}`, detail: '', untitled: true }
}

// «сегодня, 14:20», «вчера, 09:05», иначе «21.09.2026».
export function updatedText(value: string, now = new Date()) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const time = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' }).format(date)
  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const days = Math.round((day(now) - day(date)) / 86_400_000)
  if (days === 0) return `сегодня, ${time}`
  if (days === 1) return `вчера, ${time}`
  return new Intl.DateTimeFormat('ru-RU').format(date)
}
