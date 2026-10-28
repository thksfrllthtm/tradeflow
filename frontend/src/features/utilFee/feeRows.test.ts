import { describe, expect, it } from 'vitest'
import { emptyRow, isTable, pasteRows, withTail } from './feeRows'

describe('pasteRows', () => {
  it('колонка из Excel заполняет строки вниз и оставляет пустую строку в конце', () => {
    const rows = pasteRows([emptyRow()], 0, 0, '6307909800\r\n4202929800\r\n')
    expect(rows.map((row) => row.code)).toEqual(['6307909800', '4202929800', ''])
  })

  it('вставка брутто рядом с кодами не трогает коды', () => {
    const codes = pasteRows([emptyRow()], 0, 0, '6307909800\n4202929800')
    const rows = pasteRows(codes, 0, 1, '552\n851,55')
    expect(rows.slice(0, 2)).toEqual([
      { code: '6307909800', grossKg: '552', netKg: '' },
      { code: '4202929800', grossKg: '851,55', netKg: '' },
    ])
  })

  it('несколько колонок — вправо, лишние справа отбрасываются', () => {
    const rows = pasteRows([emptyRow()], 1, 1, '686\t548\tлишнее')
    expect(rows[1]).toEqual({ code: '', grossKg: '686', netKg: '548' })
    expect(rows).toHaveLength(3)
  })
})

describe('withTail / isTable', () => {
  it('убирает лишние пустые строки в конце', () => {
    expect(withTail([{ code: '1', grossKg: '', netKg: '' }, emptyRow(), emptyRow()])).toHaveLength(2)
  })

  it('одно значение — не таблица, колонка или строка — таблица', () => {
    expect(isTable('552\r\n')).toBe(false)
    expect(isTable('552\n600')).toBe(true)
    expect(isTable('552\t600')).toBe(true)
  })
})
