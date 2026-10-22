import { describe, expect, it } from 'vitest'
import { estimateLabel, greeting, longDate, updatedText } from './homeText'

describe('homeText', () => {
  it('greets by the time of day', () => {
    expect(greeting(4)).toBe('Доброй ночи')
    expect(greeting(5)).toBe('Доброе утро')
    expect(greeting(12)).toBe('Добрый день')
    expect(greeting(18)).toBe('Добрый вечер')
    expect(greeting(23)).toBe('Доброй ночи')
  })

  it('writes the date with a capital weekday', () => {
    expect(longDate(new Date(2026, 8, 28))).toBe('Понедельник, 28 сентября')
  })

  it('shows today and yesterday with time, older changes as a date', () => {
    const now = new Date(2026, 8, 28, 15, 0)
    expect(updatedText(new Date(2026, 8, 28, 9, 5).toISOString(), now)).toBe('сегодня, 09:05')
    expect(updatedText(new Date(2026, 8, 27, 23, 40).toISOString(), now)).toBe('вчера, 23:40')
    expect(updatedText(new Date(2026, 8, 21, 12, 0).toISOString(), now)).toBe('21.09.2026')
    expect(updatedText('', now)).toBe('')
  })
})

describe('estimateLabel', () => {
  it('replaces the default name with the client or the number, keeping own names', () => {
    expect(estimateLabel({ id: 7, name: '', client: 'ТОО «Стройдом»' }, 'Смета товаров · 2026-10-05')).toEqual({ title: 'ТОО «Стройдом»', detail: 'Смета №7', untitled: false })
    expect(estimateLabel({ id: 8, name: '', client: ' ' }, 'Смета товаров')).toEqual({ title: 'Смета №8', detail: '', untitled: true })
    expect(estimateLabel({ id: 9, name: '', client: '' }, 'Керамика, партия 460')).toEqual({ title: 'Керамика, партия 460', detail: 'Без клиента', untitled: false })
  })
})
