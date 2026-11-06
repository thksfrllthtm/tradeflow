import { GoodsItemInput } from '@/shared/types/goods'
import { editItem, emptyItem } from '../../lib/rowActions'
import { normalizeTnvedCode } from '../../lib/nomenclatureRates'

// Таблица этапа 2 «как в Excel»: колонки листа и перевод правок листа (ввод, вставка, Delete,
// протягивание) в строки сметы. Строки сметы — источник правды, лист их только показывает.

export type SheetKey = 'photo' | 'name' | 'code' | 'duty' | NumberField | 'included'
export type NumberField = 'places' | 'pairsPerPlace' | 'pairs' | 'grossKg' | 'packageWeightKg' | 'netKg'

export type SheetColumn = { key: SheetKey; title: string; width: number; type: 'text' | 'checkbox'; readOnly?: boolean; align: 'left' | 'right' | 'center'; hint?: string }

export const NUMBER_FIELDS: NumberField[] = ['places', 'pairsPerPlace', 'pairs', 'grossKg', 'packageWeightKg', 'netKg']

// Выбор строк и их состояние — в ячейке номера (не колонки листа); «Фото» и «Упак.» прячутся, когда не нужны.
export const SHEET_COLUMNS: SheetColumn[] = [
  { key: 'photo', title: 'Фото', width: 58, type: 'text', readOnly: true, align: 'center' },
  { key: 'name', title: 'Товар', width: 260, type: 'text', align: 'left' },
  { key: 'code', title: 'Код ТН ВЭД', width: 204, type: 'text', align: 'left', hint: 'Код можно ввести или вставить; ставки подставятся из номенклатуры. Значки справа: справочник, подбор ИИ, база решений' },
  { key: 'duty', title: 'Пошлина', width: 92, type: 'text', readOnly: true, align: 'right', hint: 'Ставка пошлины ЕТТ по коду из номенклатуры ТН ВЭД ЕАЭС' },
  { key: 'places', title: 'Места', width: 76, type: 'text', align: 'right', hint: 'Количество мест' },
  { key: 'pairsPerPlace', title: 'Шт. в месте', width: 96, type: 'text', align: 'right', hint: 'Количество товара в одном месте' },
  { key: 'pairs', title: 'Кол-во, шт', width: 100, type: 'text', align: 'right', hint: 'Пустое из трёх полей рассчитается: места × штук в месте' },
  { key: 'grossKg', title: 'Брутто, кг', width: 100, type: 'text', align: 'right' },
  { key: 'packageWeightKg', title: 'Упак., кг', width: 90, type: 'text', align: 'right', hint: 'Вес упаковки одного места для авто-нетто. Пустое — общая упаковка из параметров расчёта' },
  { key: 'netKg', title: 'Нетто, кг', width: 100, type: 'text', align: 'right', hint: 'Пустое — рассчитается: брутто − места × вес упаковки' },
  { key: 'included', title: 'В расчёте', width: 84, type: 'checkbox', align: 'center', hint: 'Учитывать строку в расчёте' },
]

export const columnIndex = (key: SheetKey) => SHEET_COLUMNS.findIndex((column) => column.key === key)

export type SheetCell = string | boolean

// Значения строки для листа. Служебные колонки (фото, пошлина) пустые: их вид рисуется
// отдельно, а при копировании они дают пустые ячейки.
export function sheetRow(item: GoodsItemInput): SheetCell[] {
  return SHEET_COLUMNS.map(({ key }) => {
    if (key === 'included') return !item.excluded
    if (key === 'name' || key === 'code' || (NUMBER_FIELDS as string[]).includes(key)) return String(item[key as keyof GoodsItemInput] ?? '')
    return ''
  })
}

// Число из Excel или с клавиатуры: «1 680,5» → «1680.5». Не число — остаётся как введено
// (строка покажет ошибку при расчёте), пустое — пустое (авто-значение).
export function normalizeNumber(value: unknown): string {
  const text = String(value ?? '').replace(/[\s  ]/g, '')
  return /^[+-]?(\d+([.,]\d*)?|[.,]\d+)$/.test(text) ? text.replace(',', '.') : String(value ?? '').trim()
}

// Флажок из листа или из вставленного текста; null — не флажок (например, Delete): не меняем.
export function parseFlag(value: unknown): boolean | null {
  if (typeof value === 'boolean') return value
  const text = String(value ?? '').trim().toLowerCase()
  if (['true', '1', 'да', 'истина', 'yes', '+', '✓'].includes(text)) return true
  if (['false', '0', 'нет', 'ложь', 'no', '-'].includes(text)) return false
  return null
}

// Значение ячейки для буфера обмена: числа с запятой (русский Excel), флажки — «да»/«нет».
export function copyValue(key: SheetKey, value: unknown): string {
  if (key === 'photo' || key === 'duty') return ''
  if (key === 'included') return parseFlag(value) ? 'да' : 'нет'
  const text = String(value ?? '').replace(/[\t\r\n]+/g, ' ')
  return (NUMBER_FIELDS as string[]).includes(key) ? text.replace('.', ',') : text
}

export type SheetChange = { x: number; y: number; value: unknown }

export type AppliedChanges = {
  // null — строки не изменились (например, Delete по флажкам).
  items: GoodsItemInput[] | null
  // Строки с новым кодом из 10 цифр: ставки нужно взять из номенклатуры.
  codes: Map<string, string>
}

// Правки листа → новые строки сметы. Строки листа ниже последней строки сметы (вставка из Excel
// длиннее таблицы) становятся новыми строками.
export function applySheetChanges(items: GoodsItemInput[], changes: SheetChange[], packageWeightKg: string, newId: (index: number) => string): AppliedChanges {
  const next = [...items]
  const codes = new Map<string, string>()
  let changed = false
  for (const { x, y, value } of changes) {
    const key = SHEET_COLUMNS[x]?.key
    if (!key || y < 0) continue
    while (next.length <= y) { next.push(emptyItem(newId(next.length), packageWeightKg)); changed = true }
    const item = next[y]
    const id = item.id ?? ''
    let updated = item
    if (key === 'included') {
      const flag = parseFlag(value)
      if (flag !== null && flag === item.excluded) updated = editItem(item, 'excluded', !flag)
    } else if (key === 'name') {
      const name = String(value ?? '')
      if (name !== item.name) updated = editItem(item, 'name', name)
    } else if (key === 'code') {
      const text = String(value ?? '').trim()
      const code = normalizeTnvedCode(text) || text.replace(/\s+/g, '')
      if (code !== item.code) {
        // Новый код снимает выбранный товар справочника (название из справочника и ИТС).
        updated = { ...item, code, standardName: '', coefficient: '', productDirectoryId: undefined, directorySnapshot: code ? { code } : undefined }
        if (normalizeTnvedCode(code)) codes.set(id, code)
        else codes.delete(id)
      }
    } else if ((NUMBER_FIELDS as string[]).includes(key)) {
      const number = normalizeNumber(value)
      if (number !== String(item[key as NumberField] ?? '')) updated = editItem(item, key as NumberField, number)
    }
    if (updated !== item) { next[y] = updated; changed = true }
  }
  return { items: changed ? next : null, codes }
}
