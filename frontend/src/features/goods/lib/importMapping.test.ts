import { describe, expect, it } from 'vitest'
import { buildGloveItems, emptyMapping, upgradeLegacyMapping } from './importMapping'

const row = (sourceRow: number, values: Record<string, string>) => ({ sourceRow, included: true, values })

describe('«Данные» из файла для подбора кода', () => {
  it('значения колонок «Данные» — с подписью из заголовка, без заголовка — только значение, пустые пропускаются', () => {
    const mapping = { ...emptyMapping(), name: '名称', data1: 'Материал', data2: 'Колонка 5', data3: 'Модель' }
    const [first, second] = buildGloveItems([
      row(2, { '名称': 'Лопата штыковая', 'Материал': 'сталь', 'Колонка 5': 'для сада', 'Модель': '' }),
      row(3, { '名称': 'Грабли', 'Материал': '', 'Колонка 5': '', 'Модель': '' }),
    ], [], mapping, '0', 0)
    expect(first.name).toBe('Лопата штыковая')
    expect(first.details).toEqual(['Материал: сталь', 'для сада'])
    expect(second.details).toBeUndefined()
  })

  it('старое сопоставление «Наименование 2/3» становится «Данные 1/2»', () => {
    const upgraded = upgradeLegacyMapping({ ...emptyMapping(), name: 'A', name2: 'B', name3: 'C' })
    expect(upgraded).toMatchObject({ name: 'A', data1: 'B', data2: 'C' })
    expect('name2' in upgraded).toBe(false)
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
