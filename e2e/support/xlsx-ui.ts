import { deflateSync } from 'node:zlib'
import ExcelJS from 'exceljs'

// Excel поставщика для UI-тестов смет: заголовки, строки товаров и фото в ячейке строки.

// Сплошной PNG size×size: настоящий файл без сторонних картинок в репозитории.
export function png(size = 48, rgb: [number, number, number] = [220, 60, 40]) {
  const crcTable = Array.from({ length: 256 }, (_, n) => { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; return c >>> 0 })
  const crc = (buf: Buffer) => { let c = 0xffffffff; for (const b of buf) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0 }
  const chunk = (type: string, data: Buffer) => {
    const head = Buffer.alloc(4); head.writeUInt32BE(data.length)
    const body = Buffer.concat([Buffer.from(type), data])
    const tail = Buffer.alloc(4); tail.writeUInt32BE(crc(body))
    return Buffer.concat([head, body, tail])
  }
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(size, 0); ihdr.writeUInt32BE(size, 4); ihdr[8] = 8; ihdr[9] = 2
  const raw = Buffer.alloc((size * 3 + 1) * size)
  for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) raw.set(rgb, y * (size * 3 + 1) + 1 + x * 3)
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0))])
}

export type SupplierRow = { name: string; code: string; qty: number; gross: number; net: number; price: number; photo?: boolean }

export const HEADERS = ['Наименование', 'Код ТН ВЭД', 'Количество', 'Брутто', 'Нетто', 'Цена', 'Фото']

// Книга: лист «Инвойс» с товарами (фото в колонке G) и второй лист «Прочее», чтобы был выбор листа.
export async function supplierXlsx(rows: SupplierRow[]): Promise<Buffer> {
  const book = new ExcelJS.Workbook()
  const sheet = book.addWorksheet('Инвойс')
  sheet.addRow(HEADERS)
  rows.forEach((row) => sheet.addRow([row.name, row.code, row.qty, row.gross, row.net, row.price, '']))
  rows.forEach((row, index) => {
    if (!row.photo) return
    const image = book.addImage({ buffer: png(48, [40 + index * 60, 120, 200]) as unknown as ExcelJS.Buffer, extension: 'png' })
    // Строка Excel index+2 (после заголовка) → в exceljs строки с нуля: index+1.
    sheet.addImage(image, { tl: { col: 6.1, row: index + 1.1 }, ext: { width: 30, height: 15 } })
    sheet.getRow(index + 2).height = 24
  })
  book.addWorksheet('Прочее').addRow(['Не товары'])
  return Buffer.from(await book.xlsx.writeBuffer())
}
