import { test, expect } from '../../support/fixtures'

// Утильсбор: ожидаемые суммы — из правил backend/internal/service/utilfee и его Go-тестов
// (масса, т × МРП × k; упаковка — по материалу; транспорт — 50 МРП × коэффициент).

const MRP = '4325'

test.describe('утильсбор: перечень', () => {
  test('весь перечень: 19 разделов, позиции на месте, 5 материалов', async ({ apiAs }) => {
    const response = await (await apiAs('translator')).get('/api/util-fee')
    expect(response.status()).toBe(200)
    const body = await response.json()
    expect(body.edition).toContain('689')
    expect(body.sections).toHaveLength(19)
    const counts = Object.fromEntries(body.sections.map((section: { number: number; items: unknown[] }) => [section.number, section.items.length]))
    expect(counts).toMatchObject({ 1: 15, 2: 13, 6: 12, 16: 19, 18: 404, 19: 7 })
    expect(body.materials.map((material: { id: string }) => material.id)).toEqual(['paper', 'glass', 'polymer', 'metal', 'combined'])
    // У пунктов разд. 6 k — по пункту.
    const sec6 = body.sections[5]
    expect(sec6.items[0].k).toBe('1.40')
    expect(sec6.items[7]).toMatchObject({ item: '6.8', k: '1.14' })
  })
})

test.describe('утильсбор: один код', () => {
  const calc = async (apiAs: (who: 'invoicer') => Promise<import('@playwright/test').APIRequestContext>, data: object) =>
    (await apiAs('invoicer')).post('/api/util-fee', { data })

  test('микроволновка: товар (разд. 16) + две упаковки (разд. 18), пустая строка упаковки пропущена', async ({ apiAs }) => {
    const response = await calc(apiAs, { code: '8516 50 000 0', mrp: MRP, massKg: '500',
      packaging: [{ material: 'paper', massKg: '120' }, { material: 'polymer', massKg: '15,5' }, { material: 'glass', massKg: '' }] })
    expect(response.status()).toBe(200)
    const body = await response.json()
    expect(body.code).toBe('8516500000')
    expect(body.matches.map((match: { item: string }) => match.item)).toEqual(['16.13', '18.353'])
    expect(body.product).toMatchObject({ amount: '4325.00', k: '2.00' }) // 0,5 т × 4325 × 2,00
    expect(body.packaging).toEqual([
      expect.objectContaining({ index: 0, material: 'paper', amount: '913.44' }), // 0,12 × 4325 × 1,76
      expect.objectContaining({ index: 1, material: 'polymer', amount: '93.85' }), // 0,0155 × 4325 × 1,40 = 93,8525
    ])
    expect(body.total).toBe('5332.29')
  })

  test('плитка: сама не облагается, только упаковка', async ({ apiAs }) => {
    const body = await (await calc(apiAs, { code: '6907210000', mrp: MRP, massKg: '20000', packaging: [{ material: 'paper', massKg: '120' }] })).json()
    expect(body.matches.map((match: { section: number }) => match.section)).toEqual([18])
    expect(body.product).toBeNull()
    expect(body.total).toBe('913.44')
  })

  test('транспорт: коэффициент зависит от ввоза из РБ/РФ', async ({ apiAs }) => {
    const notEaeu = await (await calc(apiAs, { code: '8703231940', mrp: MRP, vehicleCategory: 'm1-2000' })).json()
    expect(notEaeu.vehicle).toMatchObject({ coefficient: '3.5', amount: '756875.00' }) // 50 × 4325 × 3,5
    expect(notEaeu.total).toBe('756875.00')
    expect(notEaeu.vehicleCategories.every((category: { group: string }) => category.group === 'm1')).toBe(true)

    const eaeu = await (await calc(apiAs, { code: '8703231940', mrp: MRP, vehicleCategory: 'm1-2000', fromBelarusRussia: true })).json()
    expect(eaeu.vehicle.amount).toBe('29410000.00') // 50 × 4325 × 136
  })

  test('только код: позиции перечня без суммы', async ({ apiAs }) => {
    const body = await (await calc(apiAs, { code: '8471300000' })).json()
    expect(body.matches.length).toBeGreaterThan(0)
    expect(body.matches[0]).toMatchObject({ section: 16, kind: 'product', k: '2.00' })
    expect(body.total).toBe('')
    expect(body.product).toBeNull()
  })

  test('неполный код: позиции, зависящие от полного кода', async ({ apiAs }) => {
    const body = await (await calc(apiAs, { code: '4011' })).json()
    expect(body.possible.some((match: { section: number }) => match.section === 2)).toBe(true)
    expect(body.matches.some((match: { section: number }) => match.section === 2)).toBe(false)
  })

  test('код не из перечня: ничего не платится', async ({ apiAs }) => {
    const body = await (await calc(apiAs, { code: '0101210000', mrp: MRP, massKg: '100' })).json()
    expect(body.matches).toEqual([])
    expect(body.total).toBe('')
  })

  const invalid: Array<[string, object, string]> = [
    ['короткий код', { code: '12' }, 'code'],
    ['код с буквами', { code: '84AB' }, 'code'],
    ['код длиннее 10 цифр', { code: '84713000001' }, 'code'],
    ['отрицательная масса', { code: '8471300000', mrp: MRP, massKg: '-5' }, 'massKg'],
    ['масса без МРП', { code: '8471300000', massKg: '100' }, 'mrp'],
    ['МРП не число', { code: '8471300000', mrp: 'abc', massKg: '100' }, 'mrp'],
    ['неизвестный материал', { code: '6907210000', mrp: MRP, packaging: [{ material: 'wood', massKg: '10' }] }, 'packaging.0.material'],
    ['категория чужой группы', { code: '8704211000', mrp: MRP, vehicleCategory: 'm1-2000' }, 'vehicleCategory'],
  ]
  for (const [name, data, field] of invalid) {
    test(`валидация: ${name} → 400 (${field})`, async ({ apiAs }) => {
      const response = await calc(apiAs, data)
      expect(response.status(), await response.text()).toBe(400)
      const error = (await response.json()).error
      expect(error.field).toBe(field)
      expect(error.message).toBeTruthy()
    })
  }
})

test.describe('утильсбор: таблица строк', () => {
  const rows = async (apiAs: (who: 'admin') => Promise<import('@playwright/test').APIRequestContext>, data: object) =>
    (await apiAs('admin')).post('/api/util-fee/rows', { data })

  test('строки из Excel: нетто по % упаковки, своё нетто, товар+упаковка, код вне перечня; итог и USD', async ({ apiAs }) => {
    const response = await rows(apiAs, { mrp: MRP, material: 'paper', usdRate: '500', rows: [
      { code: '4202929800', grossKg: '686' },
      { code: '6307 90 980 0', grossKg: '552', netKg: '450' },
      { code: '3924100000', grossKg: '100' },
      { code: '0101210000', grossKg: '1 800,5' },
    ] })
    expect(response.status()).toBe(200)
    const body = await response.json()
    expect(body.rows[0]).toMatchObject({ netKg: '548.800', netCalculated: true, packagingKg: '137.200', product: '', packaging: '1044.37', total: '1044.37' })
    expect(body.rows[1]).toMatchObject({ netKg: '450.000', netCalculated: false, packagingKg: '102.000', packaging: '776.42', total: '776.42' })
    expect(body.rows[2]).toMatchObject({ product: '484.40', packaging: '152.24', total: '636.64' })
    expect(body.rows[3]).toMatchObject({ matches: [], total: '0.00', error: '' })
    expect(body.total).toBe('2457.43')
    expect(body.totalUsd).toBe('4.91') // 2457,43 / 500
    expect(body.rows[0].totalUsd).toBe('2.09') // 1044,37 / 500
  })

  test('сбор за товар от брутто: упаковка считается отдельно', async ({ apiAs }) => {
    const body = await (await rows(apiAs, { mrp: MRP, material: 'paper', productWeight: 'gross', rows: [{ code: '3924100000', grossKg: '100' }] })).json()
    expect(body.rows[0]).toMatchObject({ product: '605.50', packaging: '152.24', total: '757.74' }) // 0,1 × 4325 × 1,40
    expect(body.totalUsd).toBe('')
  })

  test('другой % упаковки и материал', async ({ apiAs }) => {
    const body = await (await rows(apiAs, { mrp: MRP, material: 'polymer', packagingPercent: '10', rows: [{ code: '4202929800', grossKg: '1000' }] })).json()
    // нетто 900, упаковка 100 кг → 0,1 × 4325 × 1,40 = 605,50
    expect(body.rows[0]).toMatchObject({ netKg: '900.000', packagingKg: '100.000', packaging: '605.50', total: '605.50' })
  })

  test('ошибки строки остаются в строке, остальные считаются', async ({ apiAs }) => {
    const body = await (await rows(apiAs, { mrp: MRP, material: 'paper', rows: [
      { code: '3924100000', grossKg: '' },
      { code: '3924100000', grossKg: '10', netKg: '20' },
      { code: '3924100000', grossKg: 'abc' },
      { code: '', grossKg: '', netKg: '' },
      { code: '3924100000', grossKg: '100' },
    ] })).json()
    expect(body.rows[0].error).toBe('Укажите брутто')
    expect(body.rows[1].error).toBe('Нетто больше брутто')
    expect(body.rows[2].error).toBe('Брутто должно быть положительным числом')
    expect(body.rows[3]).toMatchObject({ error: '', total: '' })
    expect(body.rows[4].total).toBe('636.64')
    expect(body.total).toBe('636.64')
  })

  test('транспорт в таблице: не считается, предупреждение', async ({ apiAs }) => {
    const body = await (await rows(apiAs, { mrp: MRP, material: 'paper', rows: [{ code: '8703231940', grossKg: '1500' }] })).json()
    expect(body.rows[0].total).toBe('')
    expect(body.rows[0].warnings.join(' ')).toContain('Один код')
  })

  const invalid: Array<[string, object, string]> = [
    ['без МРП', { material: 'paper', rows: [] }, 'mrp'],
    ['неизвестный материал', { mrp: MRP, material: 'wood', rows: [] }, 'material'],
    ['процент упаковки 100', { mrp: MRP, material: 'paper', packagingPercent: '100', rows: [] }, 'packagingPercent'],
    ['отрицательный процент', { mrp: MRP, material: 'paper', packagingPercent: '-1', rows: [] }, 'packagingPercent'],
    ['неизвестный вес товара', { mrp: MRP, material: 'paper', productWeight: 'tare', rows: [] }, 'productWeight'],
    ['курс не число', { mrp: MRP, material: 'paper', usdRate: 'x', rows: [] }, 'usdRate'],
    ['больше 1000 строк', { mrp: MRP, material: 'paper', rows: Array.from({ length: 1001 }, () => ({ code: '3924100000', grossKg: '1' })) }, 'rows'],
  ]
  for (const [name, data, field] of invalid) {
    test(`валидация таблицы: ${name} → 400 (${field})`, async ({ apiAs }) => {
      const response = await rows(apiAs, data)
      expect(response.status(), await response.text()).toBe(400)
      expect((await response.json()).error.field).toBe(field)
    })
  }
})
