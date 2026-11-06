import { describe, expect, it } from 'vitest'
import { shownValue } from './shownValue'

// Показ чисел: столько знаков, сколько сравнивает сервер, чтобы «280.00» и «280» выглядели одинаково,
// а найденное расхождение всегда было видно.
describe('shownValue', () => {
  it('formats like the server compares', () => {
    expect(shownValue('Нетто, кг', '280.00')).toBe(shownValue('Нетто, кг', '280'))
    expect(shownValue('Нетто, кг', '1057.8')).toBe('1 057,8')
    expect(shownValue('Нетто, кг', '0.0005')).toBe('0,001')
    expect(shownValue('Цена, USD', '0.17')).toBe('0,1700')
    expect(shownValue('Сумма, USD', '11568.42')).toBe('11 568,42')
    expect(shownValue('Общая стоимость, USD', '38233')).toBe('38 233,00')
    expect(shownValue('Штук', '141000')).toBe('141 000')
    expect(shownValue('Количество мест', '2737')).toBe('2 737')
  })
  it('keeps non-numbers as they are', () => {
    expect(shownValue('Штук', '')).toBe('—')
    expect(shownValue('Строка', '№ 31')).toBe('№ 31')
  })
})
