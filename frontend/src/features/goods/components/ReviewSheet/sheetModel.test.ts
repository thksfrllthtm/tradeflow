import { describe, expect, it } from 'vitest'
import { emptyItem } from '../../lib/rowActions'
import { applySheetChanges, columnIndex, copyValue, normalizeNumber, parseFlag, sheetRow } from './sheetModel'

const item = (id: string, patch = {}) => ({ ...emptyItem(id, '0.3'), ...patch })
const at = (key: Parameters<typeof columnIndex>[0], y: number, value: unknown) => ({ x: columnIndex(key), y, value })
const newId = (index: number) => `new-${index}`

describe('normalizeNumber', () => {
  it('принимает числа из русского Excel', () => {
    expect(normalizeNumber('1 680,5')).toBe('1680.5')
    expect(normalizeNumber('1 680')).toBe('1680')
    expect(normalizeNumber(' 0.3 ')).toBe('0.3')
    expect(normalizeNumber('')).toBe('')
  })
  it('не число оставляет как введено', () => {
    expect(normalizeNumber('12 шт')).toBe('12 шт')
  })
})

describe('parseFlag и copyValue', () => {
  it('понимает флажки из Excel и Delete не считает флажком', () => {
    expect(parseFlag('ИСТИНА')).toBe(true)
    expect(parseFlag('нет')).toBe(false)
    expect(parseFlag(true)).toBe(true)
    expect(parseFlag('')).toBeNull()
  })
  it('копирует числа с запятой и не копирует служебные колонки', () => {
    expect(copyValue('grossKg', '1680.5')).toBe('1680,5')
    expect(copyValue('included', true)).toBe('да')
    expect(copyValue('status', 'x')).toBe('')
    expect(copyValue('name', 'Лопата\tштыковая')).toBe('Лопата штыковая')
  })
})

describe('applySheetChanges', () => {
  it('вставка блока чисел меняет поля, нетто становится ручным', () => {
    const items = [item('a'), item('b')]
    const { items: next } = applySheetChanges(items, [at('grossKg', 0, '1 680'), at('netKg', 0, '1580,5'), at('places', 1, '200')], '0.3', newId)
    expect(next?.[0]).toMatchObject({ grossKg: '1680', netKg: '1580.5', netSource: 'manual' })
    expect(next?.[1].places).toBe('200')
  })

  it('Delete по нетто возвращает авто-нетто, по флажку «В расчёте» — ничего не меняет', () => {
    const items = [item('a', { netKg: '10', netSource: 'manual' })]
    const { items: next } = applySheetChanges(items, [at('netKg', 0, ''), at('included', 0, '')], '0.3', newId)
    expect(next?.[0]).toMatchObject({ netKg: '', netSource: '', excluded: false })
  })

  it('без изменений возвращает null', () => {
    const items = [item('a', { grossKg: '5' })]
    expect(applySheetChanges(items, [at('grossKg', 0, '5'), at('included', 0, true)], '0.3', newId).items).toBeNull()
  })

  it('вставка ниже последней строки добавляет новые строки', () => {
    const items = [item('a')]
    const { items: next } = applySheetChanges(items, [at('name', 0, 'Лопата'), at('name', 2, 'Грабли'), at('pairs', 2, '12')], '0.5', newId)
    expect(next?.map((row) => row.id)).toEqual(['a', 'new-1', 'new-2'])
    expect(next?.[2]).toMatchObject({ name: 'Грабли', supplierName: 'Грабли', pairs: '12', packageWeightKg: '0.5' })
  })

  it('новый код снимает товар справочника и просит ставки из номенклатуры', () => {
    const items = [item('a', { code: '6116109500', coefficient: '0.65', productDirectoryId: 7, directorySnapshot: { code: '6116109500', rateSource: 'nomenclature' } })]
    const { items: next, codes } = applySheetChanges(items, [at('code', 0, '8201 10 000 0')], '0.3', newId)
    expect(next?.[0]).toMatchObject({ code: '8201100000', coefficient: '', productDirectoryId: undefined, directorySnapshot: { code: '8201100000' } })
    expect(codes.get('a')).toBe('8201100000')
  })

  it('флажок выбора меняет отметку, а не строку', () => {
    const { items: next, selected } = applySheetChanges([item('a')], [at('select', 0, true)], '0.3', newId)
    expect(next).toBeNull()
    expect(selected.get('a')).toBe(true)
  })

  it('строка листа совпадает с колонками', () => {
    const row = sheetRow(item('a', { name: 'Лопата', grossKg: '5', excluded: true }), true)
    expect(row[columnIndex('select')]).toBe(true)
    expect(row[columnIndex('name')]).toBe('Лопата')
    expect(row[columnIndex('grossKg')]).toBe('5')
    expect(row[columnIndex('included')]).toBe(false)
    expect(row[columnIndex('status')]).toBe('')
  })
})
