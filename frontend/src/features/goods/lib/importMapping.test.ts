import { describe, expect, it } from 'vitest'
import { buildGloveItems, emptyMapping } from './importMapping'

const row = (sourceRow: number, values: Record<string, string>) => ({ sourceRow, included: true, values })

describe('несколько колонок наименования', () => {
  it('первое непустое — наименование, остальные — дополнительные', () => {
    const mapping = { ...emptyMapping(), name: '名称', name2: 'НАИМЕНОВАНИЕ' }
    const [first, second, third] = buildGloveItems([
      row(2, { '名称': '桶红本色头，0.7公斤', 'НАИМЕНОВАНИЕ': '0,7 Лопата оран штыковой B-1' }),
      row(3, { '名称': '', 'НАИМЕНОВАНИЕ': '0,7 Лопата оран совковый B-2' }),
      row(4, { '名称': 'Лопата', 'НАИМЕНОВАНИЕ': 'Лопата' }),
    ], [], mapping, '0', 0)
    expect(first.name).toBe('桶红本色头，0.7公斤')
    expect(first.otherNames).toEqual(['0,7 Лопата оран штыковой B-1'])
    expect(second.name).toBe('0,7 Лопата оран совковый B-2')
    expect(second.otherNames).toBeUndefined()
    expect(third.otherNames).toBeUndefined()
  })
})

describe('набор из объединённых строк Excel', () => {
  it('строки, где количество объединено, склеиваются в первую: имена через «+», фото все', () => {
    const mapping = { ...emptyMapping(), name: 'Наименование', pairs: 'Количество', grossKg: 'Брутто' }
    const items = buildGloveItems([
      row(4, { 'Наименование': '三件套小刀叉蓝色', 'Количество': '1200', 'Брутто': '16' }),
      { ...row(5, { 'Наименование': '动物模具', 'Количество': '', 'Брутто': '' }), merged: { 'Количество': 4, 'Брутто': 4 } },
      { ...row(6, { 'Наименование': '两件套铲子', 'Количество': '', 'Брутто': '' }), merged: { 'Количество': 4, 'Брутто': 4 } },
      row(7, { 'Наименование': 'Лопата', 'Количество': '10', 'Брутто': '5' }),
    ], [{ sourceRow: 4, data: 'AAA', mimeType: 'image/png' }, { sourceRow: 5, data: 'BBB', mimeType: 'image/png' }], mapping, '0', 0)
    expect(items.map((item) => item.name)).toEqual(['三件套小刀叉蓝色 + 动物模具 + 两件套铲子', 'Лопата'])
    expect(items[0]).toMatchObject({ pairs: '1200', grossKg: '16', supplierName: '三件套小刀叉蓝色 + 动物模具 + 两件套铲子' })
    expect(items[0].photoUrls).toEqual(['data:image/png;base64,AAA', 'data:image/png;base64,BBB'])
  })

  it('объединён только вес (общая коробка) — строки остаются отдельными', () => {
    const mapping = { ...emptyMapping(), name: 'Наименование', pairs: 'Количество', grossKg: 'Брутто' }
    const items = buildGloveItems([
      row(4, { 'Наименование': 'Ложка', 'Количество': '100', 'Брутто': '20' }),
      { ...row(5, { 'Наименование': 'Вилка', 'Количество': '200', 'Брутто': '' }), merged: { 'Брутто': 4 } },
    ], [], mapping, '0', 0)
    expect(items).toHaveLength(2)
  })
})
