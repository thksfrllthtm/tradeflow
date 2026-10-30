import ExcelJS from 'exceljs'
import type { APIRequestContext } from '@playwright/test'

// Коды, которые точно есть в номенклатуре ТН ВЭД ЕАЭС (выгрузка TWS.BY).
export const CODE_LAPTOP = '8471300000' // ЕТТ 0%
export const CODE_PORCELAIN = '6909110000' // ЕТТ 5%

// Колонки выгрузки справочника товаров (backend service/directory/transfer.go productColumns).
export const productColumns = ['Наименование', 'Наименование (кит.)', 'Категория', 'Ед. изм.', 'Код ТН ВЭД', 'ИТС компании', 'Синонимы (рус.)', 'Синонимы (кит.)', 'Синонимы (англ.)', 'Статус']

// XLSX из строк: первая строка — заголовки.
export async function xlsx(rows: (string | number)[][]) {
  const book = new ExcelJS.Workbook()
  const sheet = book.addWorksheet('Товары')
  for (const row of rows) sheet.addRow(row)
  return Buffer.from(await book.xlsx.writeBuffer())
}

// Строки первого листа XLSX как текст.
export async function readXlsx(buffer: Buffer) {
  const book = new ExcelJS.Workbook()
  await book.xlsx.load(buffer)
  const rows: string[][] = []
  book.worksheets[0].eachRow((row) => rows.push((row.values as unknown[]).slice(1).map((value) => value == null ? '' : String(value))))
  return rows
}

export const upload = (api: APIRequestContext, path: string, buffer: Buffer, name = 'file.xlsx') =>
  api.post(path, { multipart: { file: { name, mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer } } })

type Product = { id: number; name: string; category: string; archivedAt: string; code: string; companyCoefficient: string; [key: string]: unknown }

// Архивирует все активные карточки справочника товаров с этим префиксом наименования (удаления нет).
export async function archiveProducts(api: APIRequestContext, prefix: string) {
  const list = await api.get(`/api/product-directory?q=${encodeURIComponent(prefix)}`)
  for (const item of ((await list.json()).items ?? []) as Product[]) {
    if (item.name.startsWith(prefix) && !item.archivedAt) await api.post(`/api/product-directory/${item.id}/archive`)
  }
}

export async function findProduct(api: APIRequestContext, name: string, includeArchived = false) {
  const list = await api.get(`/api/product-directory?q=${encodeURIComponent(name)}&includeArchived=${includeArchived}`)
  return ((await list.json()).items as Product[]).find((item) => item.name === name)
}
