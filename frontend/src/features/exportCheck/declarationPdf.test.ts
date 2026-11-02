import { describe, expect, it } from 'vitest'
import { parseDeclarationPages, PdfText } from './declarationPdf'
import full from './testdata/3807920.json'
import part1 from './testdata/1-50.json'
import part2 from './testdata/51-100.json'
import part3 from './testdata/101-107.json'

// Снимки текстового слоя настоящих деклараций: [надпись, x, y, ширина] по страницам.
type Fixture = { fileName: string; pages: [string, number, number, number][][] }
const pagesOf = (fixture: Fixture): PdfText[][] => fixture.pages.map((page) => page.map(([str, x, y, w]) => ({ str, x, y, w })))
const parse = (fixture: Fixture) => parseDeclarationPages(pagesOf(fixture), fixture.fileName)

describe('parseDeclarationPages', () => {
  it('reads the header and all 46 goods of declaration 3807920 (invoice 357-49)', () => {
    const declaration = parse(full as Fixture)
    expect(declaration).toMatchObject({
      fileName: '3807920 改抬头 改焊丝成交单位.pdf', billNo: '3807920', sender: '北京焰耀贸易有限公司', receiver: 'ТОО «357»',
      contract: 'JJ-1 от 15.09.22', places: '2737', grossKg: '63926.4', netKg: '62557.9', totalAmount: '38233', totalQty: '390970', warnings: [],
    })
    // Сумма количеств сделки из строки tips сходится с прочитанными строками — разбор полный.
    expect(declaration.items.reduce((sum, item) => sum + Number(item.qty), 0)).toBe(390970)
    expect(declaration.items).toHaveLength(46)
    expect(declaration.items.map((item) => item.no)).toEqual(Array.from({ length: 46 }, (_, index) => index + 1))
    expect(declaration.items[0]).toEqual({ no: 1, code: '8205510000', name: '胶枪', qty: '600', qtyUnit: '个', netKg: '172.5', price: '0.1700', amount: '102.00', currency: 'USD' })
    // Тряпки: 9300 шт, 558 кг, строка на границе страницы.
    expect(declaration.items[5]).toMatchObject({ no: 6, code: '6307100000', qty: '9300', qtyUnit: '个', netKg: '558', price: '0.1000', amount: '930.00' })
    // Проволока: количество сделки в штуках (件), вес отдельно.
    expect(declaration.items[20]).toMatchObject({ no: 21, code: '8311200000', name: '焊丝', qty: '5000', qtyUnit: '件', netKg: '5322.5', price: '0.2800', amount: '1400.00' })
    expect(declaration.items[45]).toMatchObject({ no: 46, code: '8207509000', qty: '1200', netKg: '442', price: '0.1500', amount: '180.00' })
  })

  it('reads a declaration split by 50 goods: numbering restarts, the range is in 提运单号', () => {
    const [first, second, third] = [part1, part2, part3].map((fixture) => parse(fixture as Fixture))
    expect(first).toMatchObject({ billNo: '1-50', receiver: 'ТОО «Leader Trans Logistics»', contract: '№LT-24/03 от «03»', places: '1374', grossKg: '14537.93', netKg: '14125.73' })
    expect(first.items).toHaveLength(50)
    expect(first.items[0]).toMatchObject({ no: 1, code: '6404199000', qty: '192', qtyUnit: '双', netKg: '163.2', price: '4.0700', amount: '781.44', currency: 'USD' })
    expect(second).toMatchObject({ billNo: '51-100', places: '186', grossKg: '3064.9', netKg: '3009.1' })
    expect(second.items).toHaveLength(50)
    expect(second.items[0]).toMatchObject({ no: 1, qty: '24', netKg: '32.4', price: '6.1000', amount: '146.40' })
    expect(third).toMatchObject({ billNo: '101-107', places: '36', grossKg: '599.4', netKg: '588.6' })
    expect(third.items.map((item) => item.no)).toEqual([1, 2, 3, 4, 5, 6, 7])
  })

  it('fails clearly on a PDF without the goods table', () => {
    expect(() => parseDeclarationPages([[{ str: 'Инвойс', x: 10, y: 10, w: 30 }]], 'scan.pdf')).toThrow(/scan\.pdf: не найдена таблица товаров/)
  })
})
