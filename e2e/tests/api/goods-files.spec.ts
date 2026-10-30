import path from 'node:path'
import { readFileSync } from 'node:fs'
import { test, expect } from '../../support/fixtures'
import { buildWorkbook, readWorkbook } from '../../support/xlsx'
import { calcInput, lookupCodes, row, tinyPng, tinyPngDataUrl } from '../../support/goods-helpers'
import type { APIRequestContext } from '@playwright/test'

// Разбор Excel поставщика (/api/goods/parse-xlsx) и выгрузка расчёта в Excel (/api/goods/export).

const repo = path.join(import.meta.dirname, '..', '..', '..')
const XLSX = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'

type Parsed = {
  sheets: Array<{ name: string; rows: number }>
  headers: string[]
  rows: Array<{ sourceRow: number; values: Record<string, string>; included: boolean; warnings?: string[]; merged?: Record<string, number> }>
  images: Array<{ sourceRow: number; dataIndex: number; mimeType: string }>
  imageData: string[]
  warnings?: string[]
}

async function parse(api: APIRequestContext, buffer: Buffer, name = 'supplier.xlsx', query = '') {
  const response = await api.post(`/api/goods/parse-xlsx${query}`, { multipart: { file: { name, mimeType: XLSX, buffer } } })
  return { response, body: await response.json().catch(() => ({})) as Parsed & { error?: { message: string; field?: string } } }
}

test.describe('разбор Excel поставщика', () => {
  test('заголовки, строки с номерами строк листа, список листов', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Инвойс', rows: [['Наименование', 'Код', 'Кол-во', 'Цена'], ['Перчатки', '4015190000', 100, 1.25], ['Чашка', '6912002300', 20, 3]] }])
    const { response, body } = await parse(await apiAs('invoicer'), book)
    expect(response.status(), JSON.stringify(body)).toBe(200)
    expect(body.headers).toEqual(['Наименование', 'Код', 'Кол-во', 'Цена'])
    expect(body.sheets).toEqual([{ name: 'Инвойс', rows: 3 }])
    expect(body.rows.map((item) => item.sourceRow)).toEqual([2, 3])
    expect(body.rows[0]).toMatchObject({ included: true, values: { Наименование: 'Перчатки', Код: '4015190000', 'Кол-во': '100', Цена: '1.25' } })
    expect(body.images).toEqual([])
    expect(body.imageData).toEqual([])
  })

  test('пустые строки пропускаются, номера строк листа сохраняются', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Лист1', rows: [['Наименование', 'Кол-во'], ['А', 1], [null, null], ['Б', 2], [], ['В', 3]] }])
    const { body } = await parse(await apiAs('invoicer'), book)
    expect(body.rows.map((item) => [item.sourceRow, item.values['Наименование']])).toEqual([[2, 'А'], [4, 'Б'], [6, 'В']])
  })

  test('строка «Итого» исключается с предупреждением', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Лист1', rows: [['Наименование', 'Кол-во'], ['Перчатки', 10], ['Итого', 10]] }])
    const { body } = await parse(await apiAs('invoicer'), book)
    expect(body.rows.map((item) => item.included)).toEqual([true, false])
    expect(body.rows[1].warnings?.join(' ')).toContain('итог')
    expect(body.warnings?.join(' ')).toContain('Строка 3')
  })

  test('пустые и повторяющиеся заголовки получают имена «Колонка N» и «(2)»', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Лист1', rows: [['Наименование', null, 'Наименование'], ['Перчатки', '10', 'gloves']] }])
    const { body } = await parse(await apiAs('invoicer'), book)
    expect(body.headers).toEqual(['Наименование', 'Колонка 2', 'Наименование (2)'])
    expect(body.rows[0].values).toMatchObject({ 'Колонка 2': '10', 'Наименование (2)': 'gloves' })
  })

  test('шапка не в первой строке: headerRow / dataStartRow', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Лист1', rows: [['INVOICE № 15'], ['Поставщик: Ningbo Co'], ['Name', 'Qty'], ['Cup', 5], ['Plate', 6]] }])
    const { body } = await parse(await apiAs('invoicer'), book, 'invoice.xlsx', '?headerRow=3&dataStartRow=4')
    expect(body.headers).toEqual(['Name', 'Qty'])
    expect(body.rows.map((item) => item.values.Name)).toEqual(['Cup', 'Plate'])
  })

  test('несколько листов: выбор листа по имени; неизвестный лист — первый с предупреждением', async ({ apiAs }) => {
    const book = await buildWorkbook([
      { name: 'Обложка', rows: [['Коммерческое предложение']] },
      { name: 'Товары', rows: [['Наименование'], ['Ложка'], ['Вилка']] },
    ])
    const api = await apiAs('invoicer')
    const { body } = await parse(api, book, 'two.xlsx', `?sheet=${encodeURIComponent('Товары')}`)
    expect(body.sheets.map((item) => item.name)).toEqual(['Обложка', 'Товары'])
    expect(body.rows.map((item) => item.values['Наименование'])).toEqual(['Ложка', 'Вилка'])
    const missing = await parse(api, book, 'two.xlsx', '?sheet=Нет')
    expect(missing.response.status()).toBe(200)
    expect(missing.body.warnings?.join(' ')).toContain('Лист "Нет" не найден')
    expect(missing.body.rows, 'на обложке под шапкой нет строк').toEqual([])
  })

  test('объединённые ячейки набора: строки отмечены номером первой строки', async ({ apiAs }) => {
    const book = await buildWorkbook([{
      name: 'Лист1',
      rows: [['Наименование', 'Количество', 'Брутто'], ['三件套小刀叉', 1200, 16], ['动物模具', null, null], ['两件套铲子', null, null], ['Лопата', 10, 5]],
      merges: ['B2:B4', 'C2:C4'],
    }])
    const { body } = await parse(await apiAs('invoicer'), book)
    expect(body.rows).toHaveLength(4)
    expect(body.rows[0].merged ?? null).toBeNull()
    expect(body.rows[1].merged).toEqual({ Количество: 2, Брутто: 2 })
    expect(body.rows[2].merged).toEqual({ Количество: 2, Брутто: 2 })
    expect(body.rows[3].merged ?? null).toBeNull()
  })

  test('встроенное фото: строка получает картинку PNG (base64)', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Лист1', rows: [['Наименование', 'Фото'], ['Кружка', null], ['Тарелка', null]], images: [{ tl: 'B3', br: 'C4', png: tinyPng }] }])
    const { response, body } = await parse(await apiAs('invoicer'), book)
    expect(response.status()).toBe(200)
    expect(body.images).toHaveLength(1)
    expect(body.images[0]).toMatchObject({ sourceRow: 3, mimeType: 'image/png' })
    expect(Buffer.from(body.imageData[body.images[0].dataIndex], 'base64').subarray(1, 4).toString()).toBe('PNG')
  })

  test('shareImages: фото в объединённой ячейке получают все её строки, картинка передаётся один раз', async ({ apiAs }) => {
    const book = await buildWorkbook([{
      name: 'Лист1',
      rows: [['Наименование', 'Фото'], ['Цепь 3*2,3'], ['Цепь 4*2,3'], ['Цепь 5*2,3'], ['Цепь 10*2,3'], ['Брусок']],
      merges: ['B2:B5'],
      images: [{ tl: 'B2', br: 'C6' , png: tinyPng }],
    }])
    const { body } = await parse(await apiAs('invoicer'), book, 'chains.xlsx', '?shareImages=true')
    expect(body.images.map((item) => item.sourceRow).sort()).toEqual([2, 3, 4, 5])
    expect(body.imageData).toHaveLength(1)
  })

  test('реальный .xls 97-2003 (набор с объединёнными ячейками)', async ({ apiAs }) => {
    const data = readFileSync(path.join(repo, 'backend/internal/service/goods/testdata/set.xls'))
    const response = await (await apiAs('invoicer')).post(`/api/goods/parse-xlsx?sheet=${encodeURIComponent('Лист1')}`, { multipart: { file: { name: 'Набор.XLS', mimeType: 'application/vnd.ms-excel', buffer: data } } })
    expect(response.status()).toBe(200)
    const body = await response.json() as Parsed
    expect(body.sheets.map((item) => item.name)).toEqual(['Лист1', 'Итог'])
    expect(body.rows).toHaveLength(4)
    expect(body.rows[0].values).toMatchObject({ Наименование: '三件套小刀叉', Количество: '1200', Брутто: '16.5', Цена: '2.40', Артикул: 'мх08199' })
    expect(body.rows[1].merged).toMatchObject({ Количество: 2, Брутто: 2 })
  })

  test('реальный .xlsx из другой части проекта читается без ошибок', async ({ apiAs }) => {
    const data = readFileSync(path.join(repo, 'backend/internal/service/translator/testdata/c106.xlsx'))
    const { response, body } = await parse(await apiAs('invoicer'), data, 'c106.xlsx')
    expect(response.status(), JSON.stringify(body).slice(0, 300)).toBe(200)
    expect(body.sheets.length).toBeGreaterThan(0)
  })

  for (const [title, name, buffer, message] of [
    ['не Excel по расширению', 'invoice.pdf', Buffer.from('%PDF-1.4'), 'Поддерживаются только файлы Excel'],
    ['повреждённый .xlsx', 'broken.xlsx', Buffer.from('not an xlsx at all'), 'Файл повреждён'],
    ['повреждённый .xls', 'broken.xls', Buffer.from('not an xls'), 'Файл повреждён'],
  ] as const) {
    test(`ошибка: ${title} → 400 с текстом`, async ({ apiAs }) => {
      const { response, body } = await parse(await apiAs('invoicer'), buffer, name)
      expect(response.status()).toBe(400)
      expect(body.error?.message).toContain(message)
      expect(body.error?.field).toBe('file')
    })
  }

  test('ошибка: без файла → 400', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/goods/parse-xlsx', { multipart: { other: 'x' } })
    expect(response.status()).toBe(400)
    expect((await response.json()).error.field).toBe('file')
  })

  test('ошибка: строка заголовков за пределами листа и данные выше шапки → 400', async ({ apiAs }) => {
    const book = await buildWorkbook([{ name: 'Лист1', rows: [['Наименование'], ['Перчатки']] }])
    const api = await apiAs('invoicer')
    const outside = await parse(api, book, 'a.xlsx', '?headerRow=50&dataStartRow=51')
    expect(outside.response.status()).toBe(400)
    expect(outside.body.error?.field).toBe('headerRow')
    const above = await parse(api, book, 'a.xlsx', '?headerRow=2&dataStartRow=2')
    expect(above.response.status()).toBe(400)
    expect(above.body.error?.field).toBe('dataStartRow')
  })
})

test.describe('выгрузка расчёта в Excel', () => {
  test('книга «Расчёт» + «Итоги»: строки, ставки, фото, итог растаможки как в расчёте', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const rates = await lookupCodes(api, ['0101291000'])
    test.skip(rates.get('0101291000')?.duty.percent !== '5', 'ставка 0101291000 изменилась')
    const input = calcInput([
      row('0101291000', '7', '12.3456', '100', '90', { name: 'Кружка из Excel', supplierName: 'Кружка из Excel', photoUrl: tinyPngDataUrl }),
      row('0101291000', '3', '10', '20', '19', { name: 'Тарелка', supplierName: 'Тарелка' }),
    ], { mrp: '4325', feeMultiplier: '6' })
    const calculated = await (await api.post('/api/goods/calculate', { data: input })).json()

    const response = await api.post('/api/goods/export', { data: input })
    expect(response.status()).toBe(200)
    expect(response.headers()['content-type']).toContain(XLSX)
    expect(response.headers()['content-disposition']).toContain('goods-calculation.xlsx')
    const book = await readWorkbook(await response.body())
    expect(book.worksheets.map((sheet) => sheet.name)).toEqual(['Расчёт', 'Итоги'])

    const calc = book.getWorksheet('Расчёт')!
    const summary = book.getWorksheet('Итоги')!
    expect(calc.getCell('C1').text).toBe('Наименование')
    expect(calc.getCell('C2').text).toBe('Кружка из Excel')
    expect(calc.getCell('C3').text).toBe('Тарелка')
    expect(calc.getCell('F2').text).toBe('5%')
    expect(calc.getCell('G2').text).toBe('16%')
    expect(Number(calc.getCell('K2').value)).toBe(7)
    expect(Number(calc.getCell('O2').value)).toBe(12.3456)
    expect(calc.getCell('C4').text).toBe('Итого')
    expect(calc.getImages().length, 'фото первой строки встроено').toBe(1)

    expect(summary.getCell('A18').text).toBe('Растаможка к оплате')
    expect(Number(summary.getCell('B18').value)).toBeCloseTo(Number(calculated.totals.customsPaymentsKzt), 2)
    expect(Number(summary.getCell('B18').value).toFixed(2)).toBe(calculated.totals.customsPaymentsKzt)
    expect(summary.getCell('B9').text).toBe('МРП 4 325 × 6')
  })

  test('ошибочный ввод → 400, а не файл', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/goods/export', { data: calcInput([], { assessmentDate: 'вчера' }) })
    expect(response.status()).toBe(400)
    expect((await response.json()).error.field).toBe('assessmentDate')
  })
})
