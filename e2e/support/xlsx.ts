import ExcelJS from 'exceljs'

// Книги Excel «как у поставщиков» для тестов разбора: листы из массивов строк, объединения,
// картинки в ячейках.
export type SheetSpec = {
  name: string
  rows: Array<Array<string | number | null>>
  merges?: string[]
  // Картинка с верхним левым углом в ячейке (A1-нотация) и нижним правым — в br.
  images?: Array<{ tl: string; br: string; png: Buffer }>
}

const cellIndex = (ref: string) => {
  const [, letters, digits] = /^([A-Z]+)(\d+)$/.exec(ref)!
  const col = [...letters].reduce((sum, ch) => sum * 26 + ch.charCodeAt(0) - 64, 0) - 1
  return { col, row: Number(digits) - 1 }
}

export async function buildWorkbook(sheets: SheetSpec[]): Promise<Buffer> {
  const book = new ExcelJS.Workbook()
  for (const spec of sheets) {
    const sheet = book.addWorksheet(spec.name)
    spec.rows.forEach((values, index) => { sheet.getRow(index + 1).values = values.map((value) => value ?? null) })
    for (const range of spec.merges ?? []) sheet.mergeCells(range)
    for (const image of spec.images ?? []) {
      const id = book.addImage({ buffer: image.png as unknown as ExcelJS.Buffer, extension: 'png' })
      sheet.addImage(id, { tl: cellIndex(image.tl), br: cellIndex(image.br) } as unknown as ExcelJS.ImageRange)
    }
  }
  return Buffer.from(await book.xlsx.writeBuffer())
}

export async function readWorkbook(data: Buffer) {
  const book = new ExcelJS.Workbook()
  await book.xlsx.load(data as unknown as ExcelJS.Buffer)
  return book
}
