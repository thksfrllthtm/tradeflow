import { test, expect } from '../../support/fixtures'
import { D, calcInput, calculate, expectedDuty, lookupCodes, row, type NomenclatureItem } from '../../support/goods-helpers'
import { request, type APIRequestContext } from '@playwright/test'
import { authFile } from '../../support/users'

// Расчёт растаможки (/api/goods/calculate) против локальной номенклатуры ЕАЭС.
// Курсы заданы явно: USD 471.25, EUR 548.60 (НБРК не запрашивается).
// Ожидаемые суммы посчитаны вручную по правилам (duty.go, item.go, allocation.go, rounding.go):
// ТС = round2(round2(цена × кол-во) × курс + доля доставки × USD); пошлина и НДС — round2;
// НДС = 16% × (ТС + пошлина + антидемпинг + доля сбора); сбор делится поровну по позициям.
// Перед проверкой сверяется ставка кода в номенклатуре: если её обновили — тест пропускается.

const USD = '471.25'
const EUR = '548.60'

type Item = Record<string, string | string[] | boolean | null | undefined | Record<string, string>> & { errors: string[]; warnings: string[] }
const issues = (item: Item) => [...(item.errors ?? []), ...(item.warnings ?? [])].join(' | ')

let invoicer: APIRequestContext
let rates: Map<string, NomenclatureItem>

const CODES = {
  percent5: '0101291000',      // 5%
  perPair: '6401100000',       // 0.75 EUR за 1 пар
  perKg: '0808101000',         // 0.06 EUR за 1 кг
  usdPerTonne: '1701121001',   // 250 USD за 1 т
  minKg: '0201100008',         // 50%, но не менее 1 EUR за 1 кг
  plusKg: '3401110001',        // 4.5% плюс 0.02 EUR за 1 кг
  minPiece: '4012110000',      // 15%, но не менее 2.02 EUR за 1 шт
  per1000: '2402201000',       // 2 EUR за 1 1000 шт
  perLitre: '2103903000',      // 2 EUR за 1 л — расчётом не поддерживается
  antiDumping: '2933610000',   // мера AD-34 для Китая
}

test.beforeAll(async ({ baseURL }) => {
  invoicer = await request.newContext({ baseURL, storageState: authFile('invoicer') })
  rates = await lookupCodes(invoicer, Object.values(CODES))
})
test.afterAll(async () => { await invoicer?.dispose() })

function requireRate(code: string, tariff: Partial<NomenclatureItem['duty']>) {
  const item = rates.get(code)
  test.skip(!item, `кода ${code} нет в локальной номенклатуре`)
  for (const [key, value] of Object.entries(tariff)) {
    test.skip(item!.duty[key as keyof typeof tariff] !== value, `ставка ${code} в номенклатуре изменилась: ${JSON.stringify(item!.duty)}`)
  }
  test.skip(item!.vatPercent !== '16', `НДС ${code} не 16%`)
}

async function calc(items: unknown[], extra: Record<string, unknown> = {}) {
  const { response, body } = await calculate(invoicer, calcInput(items, extra))
  expect(response.status(), JSON.stringify(body)).toBe(200)
  return body as { items: Item[]; totals: Record<string, string | number | boolean | string[]> } & Record<string, unknown>
}

test.describe('схемы пошлины — точные суммы', () => {
  test('процентная 5%: округление ТС «половина вверх», пошлина, НДС, итоги', async () => {
    requireRate(CODES.percent5, { type: 'percentage', percent: '5' })
    const result = await calc([row(CODES.percent5, '7', '12.3456', '100', '90')])
    const item = result.items[0]
    // 12.3456 × 7 = 86.4192 → 86.42 USD; × 471.25 = 40725.425 → 40725.43
    expect(item).toMatchObject({ goodsCost: '86.42', costUsd: '86.42', pairPrice: '12.3456', customsValueKzt: '40725.43', dutyKzt: '2036.27', antiDumpingKzt: '0.00', vatKzt: '6841.87', vatBaseKzt: '42761.70', status: 'calculated' })
    expect(issues(item)).toBe('')
    expect(result.totals).toMatchObject({
      totalCostUsd: '86.42', customsValueKzt: '40725.43', dutyKzt: '2036.27', vatKzt: '6841.87', feeKzt: '0.00',
      customsPaymentsKzt: '8878.14', expensesKzt: '8878.14', expensesUsd: '18.84', calculatedCount: 1, totalCount: 1, needsReview: false, errorCount: 0,
    })
    expect(result).toMatchObject({ usdKzt: USD, eurKzt: '548.6', assessmentDate: '2026-10-20', customsValueMode: 'invoice', deliveryMode: 'none' })
  })

  const cases = [
    { title: 'специфическая EUR за пару', code: CODES.perPair, rate: { type: 'specific', amount: '0.75', currency: 'EUR', unit: 'пар' }, qty: '120', price: '2.5', net: '40', customs: '141375.00', duty: '49374.00', vat: '30519.84' },
    { title: 'специфическая EUR за кг нетто', code: CODES.perKg, rate: { type: 'specific', amount: '0.06', currency: 'EUR', unit: 'кг' }, qty: '1', price: '500', net: '1234.5', customs: '235625.00', duty: '40634.80', vat: '44201.57' },
    { title: 'специфическая USD за тонну (нетто / 1000)', code: CODES.usdPerTonne, rate: { type: 'specific', amount: '250', currency: 'USD', unit: 'т' }, qty: '1', price: '1000', net: '2500', customs: '471250.00', duty: '294531.25', vat: '122525.00' },
    { title: '«не менее» за кг — побеждает специфическая', code: CODES.minKg, rate: { type: 'combined_minimum', percent: '50', amount: '1', unit: 'кг' }, qty: '1', price: '100', net: '1000', customs: '47125.00', duty: '548600.00', vat: '95316.00' },
    { title: '«не менее» за кг — побеждает процентная', code: CODES.minKg, rate: { type: 'combined_minimum', percent: '50', amount: '1', unit: 'кг' }, qty: '1', price: '5000', net: '10', customs: '2356250.00', duty: '1178125.00', vat: '565500.00' },
    { title: '«плюс» за кг — сумма двух частей', code: CODES.plusKg, rate: { type: 'combined_plus', percent: '4.5', amount: '0.02', unit: 'кг' }, qty: '100', price: '3', net: '250', customs: '141375.00', duty: '9104.88', vat: '24076.78' },
    { title: '«не менее» за штуку', code: CODES.minPiece, rate: { type: 'combined_minimum', percent: '15', amount: '2.02', unit: 'шт' }, qty: '10', price: '50', net: '80', customs: '235625.00', duty: '35343.75', vat: '43355.00' },
    { title: 'специфическая за 1000 шт (количество / 1000)', code: CODES.per1000, rate: { type: 'specific', amount: '2', unit: '1000 шт' }, qty: '5000', price: '0.1', net: '5', customs: '235625.00', duty: '5486.00', vat: '38577.76' },
  ] as const

  for (const c of cases) {
    test(c.title, async () => {
      requireRate(c.code, c.rate)
      const result = await calc([row(c.code, c.qty, c.price, String(Number(c.net) + 1), c.net)])
      const item = result.items[0]
      expect(item, issues(item)).toMatchObject({ customsValueKzt: c.customs, dutyKzt: c.duty, vatKzt: c.vat, status: 'calculated' })
      // Те же числа по общей формуле правил — проверка самих ожиданий.
      const duty = expectedDuty(rates.get(c.code)!.duty, D(c.customs), D(c.net), D(c.qty), D(USD), D(EUR)).round(2)
      expect(duty.fixed(2)).toBe(c.duty)
      expect(D(c.customs).add(duty).mul(16).div(100).fixed(2)).toBe(c.vat)
      expect(result.totals.customsPaymentsKzt).toBe(D(c.duty).add(c.vat).fixed(2))
    })
  }

  test('ставка за литры не поддерживается: строка с ошибкой, итог растаможки не выводится', async () => {
    requireRate(CODES.perLitre, { type: 'specific', unit: 'л' })
    const result = await calc([row(CODES.perLitre, '10', '5', '11', '10'), row(CODES.percent5, '1', '10', '2', '1')])
    expect(issues(result.items[0])).toContain('не поддерживается')
    expect(result.items[0].status).toBe('needs_review')
    expect(result.items[0].dutyKzt).toBe('')
    expect(result.totals.customsPaymentsKzt).toBe('')
    expect((result.totals.blockingErrors as string[]).join(' ')).toContain('Итог растаможки не выводится')
    expect(result.totals.needsReview).toBe(true)
  })

  test('код вне номенклатуры — понятная ошибка «Ставки ТН ВЭД», без пошлины', async () => {
    const result = await calc([row('1111111111', '1', '10', '2', '1')])
    const item = result.items[0]
    expect(item.errors.join(' ')).toMatch(/^Ставки ТН ВЭД: кода 1111111111 нет в номенклатуре/)
    expect(item).toMatchObject({ dutyKzt: '', vatKzt: '', status: 'needs_review' })
  })

  test('код с пробелами и в виде числа Excel («0101 29 100 0», «0101291000.0») распознаётся', async () => {
    requireRate(CODES.percent5, { type: 'percentage', percent: '5' })
    const result = await calc([row('0101 29 100 0', '1', '100', '2', '1'), row('0101291000.0', '1', '100', '2', '1')])
    for (const item of result.items) expect(item, issues(item)).toMatchObject({ code: CODES.percent5, dutyKzt: '2356.25', status: 'calculated' })
  })

  test('строка без кода — «укажите код ТН ВЭД», не считается', async () => {
    const result = await calc([row('', '1', '10', '2', '1')])
    expect(issues(result.items[0])).toMatch(/код ТН ВЭД/)
    expect(result.items[0].status).toBe('needs_review')
  })

  test('ставки только из номенклатуры: подложенный в запрос снимок с 0% игнорируется', async () => {
    requireRate(CODES.percent5, { type: 'percentage', percent: '5' })
    const forged = row(CODES.percent5, '1', '100', '2', '1', { directorySnapshot: { code: CODES.percent5, name: 'x', duty: { type: 'percentage', percent: '0' }, vat: { type: 'exempt' } } })
    const result = await calc([forged])
    expect(result.items[0]).toMatchObject({ customsValueKzt: '47125.00', dutyKzt: '2356.25', vatKzt: '7917.00' })
  })
})

test.describe('стоимость строки', () => {
  test.beforeEach(() => requireRate(CODES.percent5, { type: 'percentage', percent: '5' }))

  test('стоимость в EUR — по курсу EUR, в KZT — без пересчёта', async () => {
    const result = await calc([
      row(CODES.percent5, '10', '10', '2', '1', { costCurrency: 'EUR' }),
      row(CODES.percent5, '10', '10', '2', '1', { costCurrency: 'KZT' }),
    ])
    expect(result.items[0]).toMatchObject({ goodsCost: '100.00', goodsCostCurrency: 'EUR', costUsd: '', customsValueKzt: '54860.00', dutyKzt: '2743.00' })
    expect(result.items[1]).toMatchObject({ goodsCost: '100.00', goodsCostCurrency: 'KZT', customsValueKzt: '100.00', dutyKzt: '5.00' })
  })

  test('неизвестная валюта — предупреждение', async () => {
    const result = await calc([row(CODES.percent5, '10', '10', '2', '1', { costCurrency: 'GBP' })])
    expect(issues(result.items[0])).toContain('Валюта стоимости не поддерживается')
  })

  test('дополнительные расходы входят в ТС', async () => {
    const result = await calc([row(CODES.percent5, '10', '10', '2', '1', { extraExpenses: '50' })])
    // (100 + 50) × 471.25 = 70687.50; пошлина 3534.38 (3534.375 ↑); НДС 16% от 74221.88 = 11875.50
    expect(result.items[0]).toMatchObject({ customsValueKzt: '70687.50', dutyKzt: '3534.38', vatKzt: '11875.50' })
  })

  test('ТС в тенге, введённая вручную, заменяет расчёт по цене', async () => {
    const result = await calc([row(CODES.percent5, '10', '10', '2', '1', { customsValueDirectKzt: '100000' })])
    expect(result.items[0]).toMatchObject({ customsValueKzt: '100000.00', dutyKzt: '5000.00', vatKzt: '16800.00' })
  })

  test('стоимость без цены за единицу — из поля «стоимость товара»', async () => {
    const result = await calc([row(CODES.percent5, '4', '', '2', '1', { costInput: '250' })])
    // 250 / 4 = 62.5 за единицу; ТС 250 × 471.25 = 117812.50
    expect(result.items[0]).toMatchObject({ goodsCost: '250.00', pairPrice: '62.5000', customsValueKzt: '117812.50' })
  })

  test('цена за единицу округляется до 4 знаков и стоимость пересчитывается от неё', async () => {
    const result = await calc([row(CODES.percent5, '3', '0.33335', '2', '1')])
    // 0.33335 → 0.3334; × 3 = 1.0002 → 1.00 USD
    expect(result.items[0]).toMatchObject({ pairPrice: '0.3334', goodsCost: '1.00', customsValueKzt: '471.25' })
  })

  test('без цены и стоимости — «Укажите цену за единицу»', async () => {
    const result = await calc([row(CODES.percent5, '3', '', '2', '1')])
    expect(issues(result.items[0])).toContain('Укажите цену за единицу')
    expect(result.items[0].status).toBe('needs_review')
  })
})

test.describe('количество и вес', () => {
  test.beforeEach(() => requireRate(CODES.perKg, { type: 'specific', amount: '0.06', unit: 'кг' }))

  test('вес в граммах переводится в килограммы', async () => {
    const result = await calc([row(CODES.perKg, '1', '100', '5000', '4000', { grossWeightUnit: 'g', netWeightUnit: 'g' })])
    // нетто 4 кг: 0.06 × 4 × 548.60 = 131.664 → 131.66
    expect(result.items[0]).toMatchObject({ grossKg: '5', netKg: '4', grossWeightUnit: 'kg', netWeightUnit: 'kg', dutyKzt: '131.66' })
  })

  test('нетто без значения = брутто − места × вес упаковки (0.3 по умолчанию)', async () => {
    const result = await calc([row(CODES.perKg, '40', '1', '100', '', { places: '4', netSource: '' })])
    // 100 − 4 × 0.3 = 98.8 кг; пошлина 0.06 × 98.8 × 548.60 = 3252.1008 → 3252.10
    expect(result.items[0]).toMatchObject({ netKg: '98.8', netSource: 'calculated', pairsPerPlace: '10', dutyKzt: '3252.10' })
  })

  test('вес упаковки сметы меняет расчётный нетто', async () => {
    const result = await calc([row(CODES.perKg, '40', '1', '100', '', { places: '4', netSource: '' })], { packageWeightKg: '1' })
    expect(result.items[0].netKg).toBe('96')
  })

  test('пустое количество восстанавливается из мест × количества в месте', async () => {
    const result = await calc([row(CODES.perKg, '', '1', '100', '90', { places: '20', pairsPerPlace: '40' })])
    expect(result.items[0]).toMatchObject({ pairs: '800', places: '20', pairsPerPlace: '40', goodsCost: '800.00' })
  })

  for (const [title, extra, message] of [
    ['отрицательное количество', { pairs: '-5' }, 'должно быть положительным'],
    ['дробное количество', { pairs: '1.5' }, 'должно быть целым числом'],
    ['нетто больше брутто', { grossKg: '5', netKg: '10' }, 'Вес нетто не может быть больше брутто'],
    ['отрицательная цена', { pricePerPair: '-1' }, 'не может быть отрицательной'],
    ['отрицательные доп. расходы', { extraExpenses: '-1' }, 'не могут быть отрицательными'],
    ['количество ≠ места × в месте', { pairs: '10', places: '3', pairsPerPlace: '4' }, 'не совпадает'],
    ['неизвестная единица веса', { grossWeightUnit: 'lb' }, 'укажите единицу кг или г'],
    ['не число в весе', { netKg: 'много' }, ''],
  ] as const) {
    test(`ошибка строки: ${title} → 200, строка на проверку`, async () => {
      const result = await calc([row(CODES.perKg, '10', '1', '20', '10', extra)])
      const item = result.items[0]
      expect(item.status).toBe('needs_review')
      if (message) expect(issues(item)).toContain(message)
      else expect(issues(item)).not.toBe('')
      expect(result.totals.needsReview).toBe(true)
    })
  }
})

test.describe('таможенный сбор — один на декларацию', () => {
  test.beforeEach(() => requireRate(CODES.percent5, { type: 'percentage', percent: '5' }))

  test('МРП × множитель делится поровну; НДС считается с долей сбора', async () => {
    const items = [1, 2, 3].map(() => row(CODES.percent5, '1', '100', '2', '1'))
    const result = await calc(items, { mrp: '4325', feeMultiplier: '6' })
    // 4325 × 6 = 25950 → по 8650; НДС = 16% × (47125 + 2356.25 + 8650) = 16% × 58131.25 = 9301.00
    for (const item of result.items) expect(item).toMatchObject({ feeKzt: '8650.00', vatKzt: '9301.00', vatBaseKzt: '58131.25' })
    expect(result.totals.feeKzt).toBe('25950.00')
    expect(result.customsFeeKzt).toBe('25950')
    expect(result.totals.customsPaymentsKzt).toBe(D('2356.25').mul(3).add('25950').add(D('9301.00').mul(3)).fixed(2))
  })

  test('неделимый остаток — по тиыну первым позициям, сумма точно равна сбору', async () => {
    const items = Array.from({ length: 7 }, () => row(CODES.percent5, '1', '100', '2', '1'))
    const result = await calc(items, { customsFeeKzt: '25950' })
    // 25950 / 7 = 3707.142… → 3707.14 × 7 = 25949.98; остаток 0.02 → первые две по 3707.15
    expect(result.items.map((item) => item.feeKzt)).toEqual(['3707.15', '3707.15', '3707.14', '3707.14', '3707.14', '3707.14', '3707.14'])
    expect(result.totals.feeKzt).toBe('25950.00')
  })

  test('множитель 0 — сбора нет', async () => {
    const result = await calc([row(CODES.percent5, '1', '100', '2', '1')], { mrp: '4325', feeMultiplier: '0' })
    expect(result.totals.feeKzt).toBe('0.00')
  })

  test('исключённая строка не получает доли сбора и не входит в итоги', async () => {
    const items = [row(CODES.percent5, '1', '100', '2', '1'), row(CODES.percent5, '1', '100', '2', '1', { excluded: true })]
    const result = await calc(items, { customsFeeKzt: '1000' })
    expect(result.items[0].feeKzt).toBe('1000.00')
    expect(result.items[1]).toMatchObject({ excluded: true, feeKzt: '', customsValueKzt: '', dutyKzt: '' })
    expect(result.totals).toMatchObject({ totalCount: 1, calculatedCount: 1, customsValueKzt: '47125.00', feeKzt: '1000.00' })
  })
})

test('исключённая строка сохраняет статус «excluded»', async () => {
  requireRate(CODES.percent5, { type: 'percentage', percent: '5' })
  const result = await calc([row(CODES.percent5, '1', '100', '2', '1', { excluded: true })])
  expect(result.items[0].status).toBe('excluded')
})

test.describe('доставка — распределение по брутто', () => {
  test.beforeEach(() => requireRate(CODES.percent5, { type: 'percentage', percent: '5' }))

  const three = () => ['100', '200', '300'].map((gross) => row(CODES.percent5, '10', '10', gross, String(Number(gross) - 1)))

  test('общая сумма: доли 1/6, 2/6, 3/6; ТС включает точную долю; итоги доставки', async () => {
    const result = await calc(three(), { deliveryMode: 'total', deliveryTotalUsd: '1000' })
    expect(result.items.map((item) => item.deliveryUsd)).toEqual(['166.67', '333.33', '500.00'])
    expect(result.items.map((item) => item.deliveryKzt)).toEqual(['78541.67', '157083.33', '235625.00'])
    // 100 × 471.25 + 166.666… × 471.25 = 125666.666… → 125666.67
    expect(result.items.map((item) => item.customsValueKzt)).toEqual(['125666.67', '204208.33', '282750.00'])
    expect(result.items.map((item) => item.dutyKzt)).toEqual(['6283.33', '10210.42', '14137.50'])
    const payments = D('6283.33').add('10210.42').add('14137.50')
      .add(D('125666.67').add('6283.33').mul(16).div(100).round(2))
      .add(D('204208.33').add('10210.42').mul(16).div(100).round(2))
      .add(D('282750.00').add('14137.50').mul(16).div(100).round(2))
    expect(result.totals).toMatchObject({ totalDeliveryUsd: '1000.00', deliveryKzt: '471250.00', customsPaymentsKzt: payments.fixed(2) })
    expect(result.totals.expensesKzt).toBe(payments.add('471250.00').fixed(2))
    expect(result.totals.expensesUsd).toBe(payments.add('471250.00').div(USD).fixed(2))
  })

  test('вагоны: 2 × тариф 420 по умолчанию = 840 USD', async () => {
    const result = await calc(three(), { deliveryMode: 'wagons', wagons: '2' })
    expect(result).toMatchObject({ wagonTariffUsd: '420', deliveryTotalUsd: '840' })
    expect(result.totals.totalDeliveryUsd).toBe('840.00')
    expect(result.items.map((item) => item.deliveryUsd)).toEqual(['140.00', '280.00', '420.00'])
  })

  test('вагоны со своим тарифом', async () => {
    const result = await calc(three(), { deliveryMode: 'wagons', wagons: '3', wagonTariffUsd: '500.5' })
    expect(result.totals.totalDeliveryUsd).toBe('1501.50')
  })

  test('«доставка уже в цене» — доля показывается, но в ТС не добавляется', async () => {
    const items = three()
    items[0] = { ...items[0], deliveryAlreadyInCost: true } as typeof items[0]
    const result = await calc(items, { deliveryMode: 'total', deliveryTotalUsd: '1000' })
    expect(result.items[0]).toMatchObject({ deliveryUsd: '166.67', customsValueKzt: '47125.00' })
  })

  test('строка без брутто — предупреждение о распределении', async () => {
    const items = three()
    items[1] = { ...items[1], grossKg: '', netKg: '10' } as typeof items[1]
    const result = await calc(items, { deliveryMode: 'total', deliveryTotalUsd: '1000' })
    expect(issues(result.items[0])).toContain('не для всех включённых строк указан положительный брутто')
    expect(result.totals.needsReview).toBe(true)
  })

  test('ручная доставка, не равная общей, — предупреждение', async () => {
    const items = three().map((item) => ({ ...item, deliveryManual: true, deliveryUsd: '100' }))
    const result = await calc(items, { deliveryMode: 'total', deliveryTotalUsd: '1000' })
    expect(issues(result.items[0])).toContain('Ручное распределение доставки не равно общей доставке')
  })

  test('ручная доставка, равная общей, — без предупреждения; ТС по ручной доле', async () => {
    const items = three().map((item, index) => ({ ...item, deliveryManual: true, deliveryUsd: ['500', '300', '200'][index] }))
    const result = await calc(items, { deliveryMode: 'total', deliveryTotalUsd: '1000' })
    expect(result.items.map((item) => item.deliveryUsd)).toEqual(['500.00', '300.00', '200.00'])
    expect(result.items[0]).toMatchObject({ customsValueKzt: '282750.00', status: 'calculated' })
  })

  test('остаток наличных = наличные − расходы в USD', async () => {
    const result = await calc([row(CODES.percent5, '7', '12.3456', '100', '90')], { cashUsd: '100' })
    expect(result.totals.cashBalanceUsd).toBe('81.16')
  })
})

test.describe('антидемпинг', () => {
  let rate = ''
  test.beforeEach(async () => {
    const measures = (await (await invoicer.get('/api/tnved-antidumping')).json()).measures as Array<{ codes: string[]; rates: Record<string, string>; suspended?: boolean }>
    const measure = measures.find((item) => item.codes.includes(CODES.antiDumping) && item.rates.CN && !item.suspended)
    test.skip(!measure, `в реестре нет меры для ${CODES.antiDumping} и Китая`)
    test.skip(rates.get(CODES.antiDumping)?.duty.type !== 'percentage', 'код не с процентной пошлиной')
    rate = measure!.rates.CN
  })

  test('страна «Китай» — антидемпинг по ставке реестра ЕЭК и в базе НДС', async () => {
    const result = await calc([row(CODES.antiDumping, '10', '100', '20', '15', { antiDumpingChoice: '' })], { origin: 'Китай' })
    const item = result.items[0]
    const customs = D('471250.00')
    const duty = expectedDuty(rates.get(CODES.antiDumping)!.duty, customs, D(15), D(10), D(USD), D(EUR)).round(2)
    const ad = customs.mul(rate).div(100).round(2)
    expect(item).toMatchObject({ customsValueKzt: '471250.00', dutyKzt: duty.fixed(2), antiDumpingKzt: ad.fixed(2), vatKzt: customs.add(duty).add(ad).mul(16).div(100).fixed(2) })
    expect(result.totals.antiDumpingKzt).toBe(ad.fixed(2))
  })

  test('выбор «не начислять» убирает антидемпинг', async () => {
    const result = await calc([row(CODES.antiDumping, '10', '100', '20', '15', { antiDumpingChoice: 'none' })], { origin: 'Китай' })
    expect(result.items[0].antiDumpingKzt).toBe('0.00')
  })

  test('другая страна — мера не применяется', async () => {
    const result = await calc([row(CODES.antiDumping, '10', '100', '20', '15', { antiDumpingChoice: '' })], { origin: 'Германия' })
    expect(result.items[0].antiDumpingKzt).toBe('0.00')
  })
})

test.describe('ТС по СУР', () => {
  let price = ''
  test.beforeEach(async () => {
    requireRate(CODES.perPair, { type: 'specific', amount: '0.75', unit: 'пар' })
    price = rates.get(CODES.perPair)?.sir?.priceUsd ?? ''
    test.skip(!price, `нет индикатора СУР для ${CODES.perPair} (CN)`)
  })

  test('нетто × индикатор × (1 − снижение %) × курс; доставка сверху', async () => {
    const result = await calc([row(CODES.perPair, '120', '2.5', '50', '40')], { customsValueMode: 'sir', sirWeight: 'net', sirDiscountPercent: '10', sirCountry: 'CN', deliveryMode: 'total', deliveryTotalUsd: '100' })
    const perKg = D(price).mul(90).div(100)
    const valueUsd = D(40).mul(perKg).round(2)
    const customs = valueUsd.mul(USD).add(D(100).mul(USD)).round(2)
    const item = result.items[0]
    expect(item, issues(item)).toMatchObject({ customsBasis: 'sir', customsValueKzt: customs.fixed(2), dutyKzt: '49374.00' })
    expect(item.sir).toMatchObject({ country: 'CN', weight: 'net', weightKg: '40', discountPercent: '10', valueUsd: valueUsd.fixed(2), priceUsd: price })
    expect(result).toMatchObject({ customsValueMode: 'sir', sirWeight: 'net', sirDiscountPercent: '10', sirCountry: 'CN' })
  })

  test('по брутто и без цены инвойса', async () => {
    const result = await calc([row(CODES.perPair, '120', '', '50', '40')], { customsValueMode: 'sir', sirWeight: 'gross', sirCountry: 'CN' })
    const customs = D(50).mul(price).round(2).mul(USD).round(2)
    expect(result.items[0], issues(result.items[0])).toMatchObject({ customsBasis: 'sir', customsValueKzt: customs.fixed(2) })
  })

  test('код без индикатора — ТС по инвойсу с пояснением', async () => {
    requireRate(CODES.usdPerTonne, { type: 'specific' })
    test.skip(!!rates.get(CODES.usdPerTonne)?.sir, 'у кода появился индикатор')
    const result = await calc([row(CODES.usdPerTonne, '1', '1000', '2501', '2500')], { customsValueMode: 'sir', sirCountry: 'CN' })
    expect(result.items[0].customsValueKzt).toBe('471250.00')
    expect(issues(result.items[0])).toContain('индикатора нет — ТС по инвойсу')
  })

  test('СУР без веса — понятная ошибка', async () => {
    const noGross = await calc([row(CODES.perPair, '120', '2.5', '', '40')], { customsValueMode: 'sir', sirWeight: 'gross', sirCountry: 'CN' })
    expect(issues(noGross.items[0])).toContain('СУР: для ТС по индикатору нужен вес брутто')
    expect(noGross.items[0].customsValueKzt).toBe('')
  })
})

test.describe('ошибочный запрос → 400 с понятным текстом', () => {
  const ok = () => calcInput([row(CODES.percent5, '1', '10', '2', '1')])
  const cases: Array<[string, Record<string, unknown>, string]> = [
    ['нет даты', { assessmentDate: '' }, 'assessmentDate'],
    ['дата ДД.ММ.ГГГГ', { assessmentDate: '17.09.2026' }, 'assessmentDate'],
    ['нет курса USD', { usdKzt: '' }, 'usdKzt'],
    ['курс USD отрицательный', { usdKzt: '-1' }, 'usdKzt'],
    ['курс USD не число', { usdKzt: 'abc' }, 'usdKzt'],
    ['курс USD ноль', { usdKzt: '0' }, 'usdKzt'],
    ['курс EUR отрицательный', { eurKzt: '-5' }, 'eurKzt'],
    ['неизвестный режим доставки', { deliveryMode: 'plane' }, 'deliveryMode'],
    ['общая доставка без суммы', { deliveryMode: 'total', deliveryTotalUsd: '' }, 'deliveryTotalUsd'],
    ['доставка отрицательная', { deliveryMode: 'total', deliveryTotalUsd: '-1' }, 'deliveryTotalUsd'],
    ['вагоны без количества', { deliveryMode: 'wagons', wagons: '' }, 'wagons'],
    ['вагоны отрицательные', { deliveryMode: 'wagons', wagons: '-2' }, 'wagons'],
    ['вес упаковки отрицательный', { packageWeightKg: '-1' }, 'packageWeightKg'],
    ['МРП без множителя', { mrp: '4325' }, 'feeMultiplier'],
    ['множитель без МРП', { feeMultiplier: '6' }, 'mrp'],
    ['сбор отрицательный', { customsFeeKzt: '-1' }, 'customsFeeKzt'],
    ['наличные отрицательные', { cashUsd: '-1' }, 'cashUsd'],
    ['режим ТС неизвестен', { customsValueMode: 'magic' }, 'customsValueMode'],
    ['вес СУР неизвестен', { sirWeight: 'tare' }, 'sirWeight'],
    ['снижение СУР 100%', { sirDiscountPercent: '100' }, 'sirDiscountPercent'],
    ['снижение СУР не число', { sirDiscountPercent: 'много' }, 'sirDiscountPercent'],
    ['страна СУР не двухбуквенная', { sirCountry: 'CHN' }, 'sirCountry'],
    ['карточка товара не существует', { items: [row(CODES.percent5, '1', '10', '2', '1', { productDirectoryId: 999999999 })] }, 'items'],
  ]
  for (const [title, patch, field] of cases) {
    test(`${title} → 400 (${field})`, async () => {
      const { response, body } = await calculate(invoicer, { ...ok(), ...patch })
      expect(response.status(), JSON.stringify(body)).toBe(400)
      expect(body.error.message).toBeTruthy()
      expect(body.error.field).toBe(field)
    })
  }

  test('больше 5000 строк → 400', async () => {
    const items = Array.from({ length: 5001 }, () => ({ code: CODES.percent5 }))
    const { response, body } = await calculate(invoicer, calcInput(items))
    expect(response.status()).toBe(400)
    expect(body.error.field).toBe('items')
  })

  test('битый JSON → 400', async () => {
    const response = await invoicer.post('/api/goods/calculate', { data: '{"items":', headers: { 'Content-Type': 'application/json' } })
    expect(response.status()).toBe(400)
  })

  test('пустой список строк — 200, нулевые итоги', async () => {
    const result = await calc([])
    expect(result.items).toEqual([])
    expect(result.totals).toMatchObject({ totalCount: 0, customsPaymentsKzt: '0.00' })
  })
})
