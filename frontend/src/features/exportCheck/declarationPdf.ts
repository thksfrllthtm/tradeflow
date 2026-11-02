// Чтение китайской экспортной декларации (报关单, PDF из «единого окна» таможни КНР) в браузере.
// У PDF есть текстовый слой, pdf.js отдаёт каждую надпись с координатами. Страница повёрнута,
// поэтому координаты берём уже через viewport: y — строка, x — колонка, как на бумаге.
//
// Шапка: под каждой подписью (境内发货人, 境外收货人, 合同协议号, 提运单号, 件数, 毛重, 净重) значение
// стоит строкой ниже с тем же левым краем. Таблица товаров: заголовки колонок в одной строке (项号,
// 商品编号, 商品名称…, 数量及单位, 单价/总价/币制), колонка надписи — по её середине между серединами
// заголовков; товар начинается с номера в колонке 项号 и тянется до следующего номера. В клетке
// количества до трёх строк: вес в 千克 и количество сделки в 个/件/…; в клетке цены — цена, сумма,
// валюта. Дополнительные страницы (附加页) повторяют те же товары — берём первое вхождение.

export type PdfText = { str: string; x: number; y: number; w: number }

export type DeclarationItem = {
  no: number
  code: string
  name: string
  qty: string
  qtyUnit: string
  netKg: string
  price: string
  amount: string
  currency: string
}

export type Declaration = {
  fileName: string
  billNo: string
  sender: string
  receiver: string
  contract: string
  places: string
  grossKg: string
  netKg: string
  totalAmount: string // 总价 из строки tips на дополнительных страницах
  totalQty: string // 成交数量合计 оттуда же: сумма количеств сделки, для самопроверки разбора
  items: DeclarationItem[]
  warnings: string[]
}

const headerLabels = { sender: '境内发货人', receiver: '境外收货人', contract: '合同协议号', billNo: '提运单号', places: '件数', grossKg: '毛重(千克)', netKg: '净重(千克)' } as const
type Column = 'no' | 'code' | 'name' | 'qty' | 'price'
const columnLabels: Record<Column, string[]> = {
  no: ['项号'], code: ['商品编号'], name: ['商品名称及规格型号', '商品名称、规格型号'], qty: ['数量及单位'], price: ['单价/总价/币制'],
}
const footerMarker = /确认:|自报自缴|兹申明|申报单位|报关人员/

// Значение под подписью: та же левая граница, строкой ниже.
function valueBelow(items: PdfText[], label: string) {
  const mark = items.find((item) => item.str.trim() === label)
  if (!mark) return ''
  const below = items.filter((item) => Math.abs(item.x - mark.x) < 3 && item.y > mark.y + 4 && item.y < mark.y + 18).sort((a, b) => a.y - b.y)
  return below[0]?.str.trim() ?? ''
}

function quantity(line: string): { value: string; unit: string } | null {
  const match = /^([\d.,]+)\s*([^\d.,\s]+)$/.exec(line.trim())
  return match ? { value: match[1].replace(/,/g, ''), unit: match[2] } : null
}
const isKg = (unit: string) => unit === '千克' || unit === '公斤' || unit.toLowerCase() === 'kg'
const isNumber = (text: string) => /^[\d.,]+$/.test(text.trim())

// Товары одной страницы (пусто, если на странице нет таблицы).
function pageItems(items: PdfText[]): DeclarationItem[] {
  const labelOf = new Map<PdfText, Column>()
  for (const [column, labels] of Object.entries(columnLabels) as [Column, string[]][]) {
    const found = items.find((item) => labels.includes(item.str.trim()))
    if (found) labelOf.set(found, column)
  }
  const noLabel = [...labelOf].find(([, column]) => column === 'no')?.[0]
  if (!noLabel || labelOf.size < 5) return []
  // Все заголовки строки таблицы (и те, что нам не нужны) задают колонки. Заголовок стоит по центру
  // своей клетки, числа прижаты к её правому краю, текст — к левому, поэтому надпись относится к
  // последнему заголовку, начало которого левее её правого края. Короткое наименование («胶枪») так
  // попадает в зону кода — код и наименование считаем одной зоной, код в ней отличается видом (10 цифр).
  const headers = items.filter((item) => Math.abs(item.y - noLabel.y) < 3 && item.x >= noLabel.x - 10).sort((a, b) => a.x - b.x)
  const columnAt = (item: PdfText): Column | null => {
    const right = item.x + item.w + 1
    let header = headers[0]
    for (const candidate of headers) if (candidate.x <= right) header = candidate
    const column = labelOf.get(header) ?? null
    if (column !== 'code' && column !== 'name') return column
    return /^\d{8,10}$/.test(item.str.trim()) ? 'code' : 'name'
  }
  const body = items.filter((item) => item.y > noLabel.y + 5)
  const anchors = body.filter((item) => columnAt(item) === 'no' && /^\d{1,3}$/.test(item.str.trim())).sort((a, b) => a.y - b.y)
  if (anchors.length === 0) return []
  const last = anchors[anchors.length - 1]
  const footer = Math.min(...body.filter((item) => item.y > last.y + 4 && footerMarker.test(item.str)).map((item) => item.y), Infinity)

  return anchors.map((anchor, index) => {
    const end = index + 1 < anchors.length ? anchors[index + 1].y - 4 : footer
    const cells: Record<Column, string[]> = { no: [], code: [], name: [], qty: [], price: [] }
    for (const item of body.filter((item) => item !== anchor && item.y >= anchor.y - 4 && item.y < end).sort((a, b) => a.y - b.y || a.x - b.x)) {
      const column = columnAt(item)
      if (column) cells[column].push(item.str.trim())
    }
    const quantities = cells.qty.map(quantity).filter((value): value is { value: string; unit: string } => value !== null)
    const kg = quantities.find((value) => isKg(value.unit))
    const deal = quantities.find((value) => !isKg(value.unit)) ?? kg
    const numbers = cells.price.filter(isNumber).map((value) => value.replace(/,/g, ''))
    const currencyText = cells.price.find((value) => !isNumber(value)) ?? ''
    return {
      no: Number(anchor.str),
      code: cells.code.find((value) => /^\d{8,10}$/.test(value)) ?? cells.code.join(''),
      name: cells.name[0] ?? '',
      qty: deal?.value ?? '', qtyUnit: deal?.unit ?? '', netKg: kg?.value ?? '',
      price: numbers[0] ?? '', amount: numbers[1] ?? '',
      currency: currencyText.includes('美元') || currencyText.includes('USD') ? 'USD' : currencyText.replace(/[()（）]/g, ''),
    }
  })
}

const sameValues = (a: DeclarationItem, b: DeclarationItem) => a.qty === b.qty && a.netKg === b.netKg && a.price === b.price && a.amount === b.amount

// Разбор уже прочитанных страниц (надписи с координатами) — без pdf.js, для тестов.
export function parseDeclarationPages(pages: PdfText[][], fileName: string): Declaration {
  const warnings: string[] = []
  const first = pages.find((items) => items.some((item) => item.str.trim() === headerLabels.places)) ?? []
  const items = new Map<number, DeclarationItem>()
  for (const page of pages) {
    for (const item of pageItems(page)) {
      const seen = items.get(item.no)
      if (!seen) items.set(item.no, item)
      else if (!sameValues(seen, item)) warnings.push(`Товар ${item.no} повторяется в файле с другими значениями: ${seen.qty} ${seen.qtyUnit}, ${seen.netKg} кг, ${seen.price}, ${seen.amount} и ${item.qty} ${item.qtyUnit}, ${item.netKg} кг, ${item.price}, ${item.amount}`)
    }
  }
  if (items.size === 0) throw new Error(`${fileName}: не найдена таблица товаров декларации. Нужен PDF с текстом, а не скан`)
  if (first.length === 0) warnings.push('Не найдена шапка декларации: нет мест, брутто, нетто, получателя и контракта')
  const sorted = [...items.values()].sort((a, b) => a.no - b.no)
  // Неполная строка — признак, что разметка PDF отличается от ожидаемой: лучше сказать сразу.
  const incomplete = sorted.filter((item) => !item.qty || !item.price || !item.amount || !item.code)
  if (incomplete.length > 0) warnings.push(`Не все поля прочитаны у товаров: ${incomplete.map((item) => item.no).join(', ')}. Проверьте их в PDF`)
  const gaps = Array.from({ length: sorted[sorted.length - 1].no }, (_, index) => index + 1).filter((no) => !items.has(no))
  if (gaps.length > 0) warnings.push(`Номера товаров идут не подряд, нет: ${gaps.join(', ')}. Возможно, страница не прочитана`)
  const tip = (pattern: RegExp) => pages.flat().map((item) => pattern.exec(item.str)?.[1]).find(Boolean) ?? ''
  const header = Object.fromEntries(Object.entries(headerLabels).map(([key, label]) => [key, valueBelow(first, label)])) as Record<keyof typeof headerLabels, string>
  return { fileName, ...header, totalAmount: tip(/总价([\d.]+)/), totalQty: tip(/成交数量合计([\d.]+)/), items: sorted, warnings }
}

export async function readDeclarationPdf(file: File): Promise<Declaration> {
  const [pdfjs, worker] = await Promise.all([import('pdfjs-dist/legacy/build/pdf.mjs'), import('pdfjs-dist/legacy/build/pdf.worker.min.mjs?url')])
  pdfjs.GlobalWorkerOptions.workerSrc = worker.default
  const document = await pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()), verbosity: 0 }).promise
  const pages: PdfText[][] = []
  for (let number = 1; number <= document.numPages; number++) {
    const page = await document.getPage(number)
    const viewport = page.getViewport({ scale: 1 })
    const content = await page.getTextContent()
    pages.push(content.items.flatMap((item) => {
      if (!('str' in item) || !item.str.trim()) return []
      const [, , , , x, y] = pdfjs.Util.transform(viewport.transform, item.transform)
      return [{ str: item.str, x, y, w: item.width }]
    }))
  }
  return parseDeclarationPages(pages, file.name)
}
