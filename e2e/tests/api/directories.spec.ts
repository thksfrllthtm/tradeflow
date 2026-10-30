import { test, expect } from '../../support/fixtures'
import type { APIRequestContext } from '@playwright/test'
import { E2E, uid } from '../../support/users'
import { archiveProducts, CODE_LAPTOP, CODE_PORCELAIN, findProduct, productColumns, readXlsx, upload, xlsx } from '../../support/directories-helpers'

// Справочники через API: номенклатура ТН ВЭД, справочник товаров, ТРОИС, СУР, антидемпинг, перечень РК.
// Общие справочники (номенклатура, СУР, ТРОИС, перечень РК) настоящими файлами не загружаются —
// только заведомо негодные файлы, которые сервер отклоняет, ничего не меняя.

const garbage = Buffer.from('это не excel')

test.describe('номенклатура ТН ВЭД', () => {
  test('поиск по началу кода с пробелами', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const body = await (await api.get('/api/tnved-nomenclature?q=8471%2030')).json()
    expect(body.total).toBeGreaterThan(0)
    for (const item of body.items) expect(item.code).toMatch(/^847130\d{4}$/)
    const laptop = body.items.find((item: { code: string }) => item.code === CODE_LAPTOP)
    expect(laptop).toMatchObject({ tariffText: '0%', vatPercent: '16', duty: { type: 'percentage', percent: '0' } })
    expect(laptop.detailsUrl).toMatch(/^https:\/\//)
  })

  test('поиск по словам наименования находит другие формы слова', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const body = await (await api.get(`/api/tnved-nomenclature?q=${encodeURIComponent('фарфоровые')}&limit=200`)).json()
    expect(body.total).toBeGreaterThan(0)
    for (const item of body.items) expect(item.name.toLowerCase()).toContain('фарфор')
    expect(body.items.map((item: { code: string }) => item.code)).toContain(CODE_PORCELAIN)
  })

  test('без запроса — вся номенклатура, total равен числу кодов последней загрузки', async ({ apiAs }) => {
    const body = await (await (await apiAs('invoicer')).get('/api/tnved-nomenclature?limit=1')).json()
    expect(body.import, 'номенклатура загружена').toBeTruthy()
    expect(body.total).toBe(body.import.codeCount)
    expect(body.import.source).toBe('TWS.BY')
  })

  test('постраничный вывод: страницы не пересекаются, лимит не больше 200', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const first = await (await api.get('/api/tnved-nomenclature?q=84&limit=5&offset=0')).json()
    const second = await (await api.get('/api/tnved-nomenclature?q=84&limit=5&offset=5')).json()
    expect(first.items).toHaveLength(5)
    expect(second.items).toHaveLength(5)
    const codes = new Set(first.items.map((item: { code: string }) => item.code))
    for (const item of second.items) expect(codes.has(item.code)).toBe(false)
    expect(first.items[4].code < second.items[0].code, 'список отсортирован по коду').toBe(true)
    const huge = await (await api.get('/api/tnved-nomenclature?limit=100000')).json()
    expect(huge.items.length).toBeLessThanOrEqual(200)
    const negative = await api.get('/api/tnved-nomenclature?limit=-5&offset=-5')
    expect(negative.status()).toBe(200)
  })

  test('ничего не найдено — пустой список', async ({ apiAs }) => {
    const body = await (await (await apiAs('invoicer')).get(`/api/tnved-nomenclature?q=${encodeURIComponent('zzzнесуществующеесловоqqq')}`)).json()
    expect(body.total).toBe(0)
    expect(body.items).toEqual([])
  })

  test('формат ставок у каждого кода', async ({ apiAs }) => {
    const body = await (await (await apiAs('invoicer')).get('/api/tnved-nomenclature?q=61&limit=200')).json()
    expect(body.items.length).toBeGreaterThan(0)
    for (const item of body.items) {
      expect(item.code).toMatch(/^\d{10}$/)
      expect(typeof item.tariffText).toBe('string')
      expect(['percentage', 'specific', 'combined_minimum', 'combined_plus', 'unknown']).toContain(item.duty.type)
      if (item.duty.type !== 'unknown' && item.duty.type !== 'specific') expect(Number(item.duty.percent)).not.toBeNaN()
      if (item.duty.type === 'specific' || item.duty.type.startsWith('combined')) expect(item.duty.currency, item.code).toBeTruthy()
      expect(item.vatPercent === '' || !Number.isNaN(Number(item.vatPercent))).toBe(true)
      expect(Array.isArray(item.antiDumping)).toBe(true)
    }
  })

  test('проверка списка кодов: только существующие, без повторов', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const response = await api.post('/api/tnved-nomenclature/lookup', { data: { codes: [CODE_LAPTOP, '123', '0000000000', ` ${CODE_LAPTOP}`, CODE_LAPTOP, CODE_PORCELAIN, 'abcdefghij'] } })
    expect(response.status()).toBe(200)
    const codes = (await response.json()).items.map((item: { code: string }) => item.code).sort()
    expect(codes).toEqual([CODE_PORCELAIN, CODE_LAPTOP].sort())
  })

  test('проверка пустого списка и слишком длинного', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const empty = await api.post('/api/tnved-nomenclature/lookup', { data: { codes: [] } })
    expect(empty.status()).toBe(200)
    expect((await empty.json()).items).toEqual([])
    const tooMany = await api.post('/api/tnved-nomenclature/lookup', { data: { codes: Array.from({ length: 2001 }, () => CODE_LAPTOP) } })
    expect(tooMany.status()).toBe(400)
    expect((await tooMany.json()).error.field).toBe('codes')
  })

  test('загрузка негодного файла номенклатуры отклоняется и ничего не меняет', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const before = (await (await api.get('/api/tnved-nomenclature?limit=1')).json()).import
    const noFile = await api.post('/api/tnved-nomenclature/import', { multipart: { other: 'x' } })
    expect(noFile.status()).toBe(400)
    const bad = await upload(api, '/api/tnved-nomenclature/import', garbage)
    expect(bad.status()).toBe(400)
    const wrongSheet = await upload(api, '/api/tnved-nomenclature/import', await xlsx([['Совсем', 'не', 'TWS'], ['1', '2', '3']]))
    expect(wrongSheet.status()).toBe(400)
    const after = (await (await api.get('/api/tnved-nomenclature?limit=1')).json()).import
    expect(after).toEqual(before)
  })
})

test.describe('справочник товаров', () => {
  const prefix = `${E2E} товар ${uid()}`
  test.afterAll(async ({ apiAs }) => { await archiveProducts(await apiAs('admin'), prefix) })

  test('полный цикл карточки: создание, изменение, копия, архив, восстановление', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const name = `${prefix} цикл`
    const created = await api.post('/api/product-directory', { data: { name: `  ${name}  `, code: '8471 30 000 0', synonymsRu: ['ноутбук', ' ноутбук ', 'лэптоп'] } })
    expect(created.status(), await created.text()).toBe(201)
    const item = (await created.json()).item
    expect(item).toMatchObject({ name, code: CODE_LAPTOP, unit: 'шт.', category: 'Товары', verificationStatus: 'needs_review', archivedAt: '' })
    expect(item.synonymsRu).toContain('ноутбук')

    const found = await (await api.get(`/api/product-directory?q=${encodeURIComponent('лэптоп')}`)).json()
    expect(found.items.map((entry: { id: number }) => entry.id)).toContain(item.id)

    const patched = await api.patch(`/api/product-directory/${item.id}`, { data: { name, code: CODE_PORCELAIN, companyCoefficient: '0.65', verificationStatus: 'operator_verified', unit: 'кг' } })
    expect(patched.status(), await patched.text()).toBe(200)
    expect((await patched.json()).item).toMatchObject({ code: CODE_PORCELAIN, companyCoefficient: '0.65', verificationStatus: 'operator_verified', unit: 'кг' })

    const verified = await (await api.get(`/api/product-directory?status=operator_verified&q=${encodeURIComponent(name)}`)).json()
    expect(verified.items.map((entry: { id: number }) => entry.id)).toContain(item.id)
    const review = await (await api.get(`/api/product-directory?status=needs_review&q=${encodeURIComponent(name)}`)).json()
    expect(review.items.map((entry: { id: number }) => entry.id)).not.toContain(item.id)

    const copy = await api.post(`/api/product-directory/${item.id}/copy`)
    expect(copy.status()).toBe(201)
    expect((await copy.json()).item).toMatchObject({ name: `${name} (копия)`, code: CODE_PORCELAIN, companyCoefficient: '0.65' })
    const copyAgain = await api.post(`/api/product-directory/${item.id}/copy`)
    expect(copyAgain.status(), 'вторая копия с тем же именем — конфликт').toBe(409)

    expect((await api.post(`/api/product-directory/${item.id}/archive`)).status()).toBe(204)
    expect(await findProduct(api, name), 'архив не в обычном списке').toBeUndefined()
    const archived = await findProduct(api, name, true)
    expect(archived?.archivedAt).toBeTruthy()

    // В архиве имя свободно: можно завести новую карточку с тем же именем, и тогда восстановить старую нельзя.
    const twin = await api.post('/api/product-directory', { data: { name, code: CODE_LAPTOP } })
    expect(twin.status(), await twin.text()).toBe(201)
    expect((await api.post(`/api/product-directory/${item.id}/restore`)).status()).toBe(409)
    await api.post(`/api/product-directory/${(await twin.json()).item.id}/archive`)
    expect((await api.post(`/api/product-directory/${item.id}/restore`)).status()).toBe(204)
    expect((await findProduct(api, name))?.archivedAt).toBe('')
  })

  test('то же наименование в той же категории — конфликт 409', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const name = `${prefix} дубль`
    expect((await api.post('/api/product-directory', { data: { name, code: CODE_LAPTOP } })).status()).toBe(201)
    const duplicate = await api.post('/api/product-directory', { data: { name, code: CODE_PORCELAIN } })
    expect(duplicate.status()).toBe(409)
    expect((await duplicate.json()).error.code).toBe('product_directory_duplicate')
  })

  const invalid: Array<[string, Record<string, unknown>, string, RegExp?]> = [
    ['без наименования', { name: '  ', code: CODE_LAPTOP }, 'name'],
    ['без кода', { name: 'x' }, 'code'],
    ['код из 9 цифр', { name: 'x', code: '847130000' }, 'code', /10 цифр/],
    ['код с буквами', { name: 'x', code: '84713000AB' }, 'code'],
    ['кода нет в номенклатуре', { name: 'x', code: '0000000000' }, 'code', /нет в номенклатуре/],
    ['отрицательный ИТС', { name: 'x', code: CODE_LAPTOP, companyCoefficient: '-1' }, 'companyCoefficient'],
    ['ИТС не число', { name: 'x', code: CODE_LAPTOP, companyCoefficient: 'много' }, 'companyCoefficient'],
    ['неизвестный статус', { name: 'x', code: CODE_LAPTOP, verificationStatus: 'approved' }, 'verificationStatus'],
    ['наименование длиннее 300', { name: 'я'.repeat(301), code: CODE_LAPTOP }, 'name'],
  ]
  for (const [title, data, field, message] of invalid) {
    test(`проверка полей: ${title} → 400`, async ({ apiAs }) => {
      const response = await (await apiAs('admin')).post('/api/product-directory', { data: { ...data, name: data.name === 'x' ? `${prefix} ${title}` : data.name } })
      expect(response.status(), await response.text()).toBe(400)
      const error = (await response.json()).error
      expect(error.field).toBe(field)
      if (message) expect(error.message).toMatch(message)
    })
  }

  test('несуществующая карточка → 404, кривой id → 400', async ({ apiAs }) => {
    const api = await apiAs('admin')
    expect((await api.patch('/api/product-directory/999999999', { data: { name: `${prefix} нет`, code: CODE_LAPTOP } })).status()).toBe(404)
    expect((await api.post('/api/product-directory/999999999/copy')).status()).toBe(404)
    expect((await api.post('/api/product-directory/abc/archive')).status()).toBe(400)
  })

  test('инвойсер видит справочник, но не меняет его', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const name = `${prefix} только чтение`
    const item = (await (await admin.post('/api/product-directory', { data: { name, code: CODE_LAPTOP } })).json()).item
    const invoicer = await apiAs('invoicer')
    expect((await invoicer.get('/api/product-directory')).status()).toBe(200)
    expect(await findProduct(invoicer, name)).toBeTruthy()
    expect((await invoicer.post('/api/product-directory', { data: { name: `${name} 2`, code: CODE_LAPTOP } })).status()).toBe(403)
    expect((await invoicer.patch(`/api/product-directory/${item.id}`, { data: { name, code: CODE_PORCELAIN } })).status()).toBe(403)
    expect((await invoicer.post(`/api/product-directory/${item.id}/archive`)).status()).toBe(403)
    expect((await invoicer.post(`/api/product-directory/${item.id}/copy`)).status()).toBe(403)
    expect((await invoicer.get('/api/product-directory/export')).status()).toBe(403)
    expect((await findProduct(admin, name))?.code, 'карточка не изменилась').toBe(CODE_LAPTOP)
  })

  test('выгрузка в Excel: заголовки и созданная карточка', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const name = `${prefix} выгрузка`
    await api.post('/api/product-directory', { data: { name, nameZh: '笔记本', code: CODE_LAPTOP, companyCoefficient: '1.5', synonymsRu: ['ноут', 'лэптоп'], verificationStatus: 'operator_verified' } })
    const response = await api.get('/api/product-directory/export')
    expect(response.status()).toBe(200)
    expect(response.headers()['content-type']).toContain('spreadsheetml')
    expect(decodeURIComponent(response.headers()['content-disposition'])).toContain('Справочник товаров')
    const rows = await readXlsx(await response.body())
    expect(rows[0]).toEqual(productColumns)
    const row = rows.find((cells) => cells[0] === name)
    expect(row).toEqual([name, '笔记本', 'Товары', 'шт.', CODE_LAPTOP, '1.5', 'ноут; лэптоп', '', '', 'Проверено оператором'])
  })

  test('выгрузка → загрузка без обновления: ничего не меняется', async ({ apiAs }) => {
    const api = await apiAs('admin')
    await api.post('/api/product-directory', { data: { name: `${prefix} круг`, code: CODE_LAPTOP } })
    // Карточки E2E других тестов меняются параллельно (переименование, архив) — в файл идут
    // только постоянные карточки и своя.
    const all = await readXlsx(await (await api.get('/api/product-directory/export')).body())
    const rows = all.slice(1).filter((cells) => cells.some(Boolean)).filter((cells) => !String(cells[0]).startsWith(E2E) || String(cells[0]).startsWith(prefix))
    const response = await upload(api, '/api/product-directory/import?update=false', await xlsx([all[0], ...rows]))
    expect(response.status(), await response.text()).toBe(200)
    const result = await response.json()
    expect(result).toMatchObject({ created: 0, updated: 0, errors: [] })
    expect(result.skipped).toBeGreaterThanOrEqual(rows.length - 5)
  })

  test('загрузка из Excel: создание, пропуск, обновление', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const a = `${prefix} импорт А`, b = `${prefix} импорт Б`
    const file = (its: string) => xlsx([productColumns, [a, '', '', 'пара', CODE_LAPTOP, its, 'один; два', '', '', 'Проверено оператором'], [b, '', 'Обувь', '', CODE_PORCELAIN, '', '', '', '', '']])
    const created = await upload(api, '/api/product-directory/import', await file('2'))
    expect(created.status(), await created.text()).toBe(200)
    expect(await created.json()).toMatchObject({ created: 2, updated: 0, skipped: 0 })
    expect(await findProduct(api, a)).toMatchObject({ unit: 'пара', code: CODE_LAPTOP, companyCoefficient: '2', synonymsRu: ['один', 'два'], verificationStatus: 'operator_verified' })
    expect(await findProduct(api, b)).toMatchObject({ category: 'Обувь', unit: 'шт.', verificationStatus: 'needs_review' })

    const skipped = await upload(api, '/api/product-directory/import?update=false', await file('3'))
    expect(await skipped.json()).toMatchObject({ created: 0, updated: 0, skipped: 2 })
    expect((await findProduct(api, a) as { companyCoefficient?: string }).companyCoefficient).toBe('2')

    const updated = await upload(api, '/api/product-directory/import', await file('3.25'))
    expect(await updated.json()).toMatchObject({ created: 0, updated: 2 })
    expect((await findProduct(api, a) as { companyCoefficient?: string }).companyCoefficient).toBe('3.25')
  })

  test('загрузка: код без ведущего нуля (Excel хранил число) дополняется', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const name = `${prefix} ведущий ноль`
    const code = (await (await api.get('/api/tnved-nomenclature?q=0&limit=1')).json()).items[0].code as string
    expect(code.startsWith('0')).toBe(true)
    const response = await upload(api, '/api/product-directory/import', await xlsx([['Наименование', 'Код ТН ВЭД'], [name, Number(code)]]))
    expect(response.status(), await response.text()).toBe(200)
    expect((await findProduct(api, name))?.code).toBe(code)
  })

  test('загрузка с ошибками в строках не сохраняет ничего', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const good = `${prefix} хорошая строка`
    const response = await upload(api, '/api/product-directory/import', await xlsx([
      ['Наименование', 'Код ТН ВЭД', 'Статус'],
      [good, CODE_LAPTOP, ''],
      [`${prefix} плохой код`, '0000000000', ''],
      ['', CODE_LAPTOP, ''],
      [good, CODE_LAPTOP, ''],
      [`${prefix} плохой статус`, CODE_LAPTOP, 'может быть'],
    ]))
    expect(response.status()).toBe(400)
    const errors = (await response.json()).result.errors as Array<{ row: number; message: string }>
    expect(errors.map((error) => error.row)).toEqual([3, 4, 5, 6])
    expect(errors[0].message).toMatch(/нет в номенклатуре/)
    expect(errors[1].message).toMatch(/нет наименования/)
    expect(errors[2].message).toMatch(/уже есть в строке 2/)
    expect(errors[3].message).toMatch(/не распознан/)
    expect(await findProduct(api, good), 'ни одна строка не сохранена').toBeUndefined()
  })

  test('загрузка негодных файлов → 400', async ({ apiAs }) => {
    const api = await apiAs('admin')
    expect((await upload(api, '/api/product-directory/import', garbage)).status()).toBe(400)
    const noName = await upload(api, '/api/product-directory/import', await xlsx([['Код ТН ВЭД'], [CODE_LAPTOP]]))
    expect(noName.status()).toBe(400)
    expect((await noName.json()).error.message).toContain('Наименование')
    const empty = await upload(api, '/api/product-directory/import', await xlsx([]))
    expect(empty.status()).toBe(400)
  })
})

test.describe('ТРОИС', () => {
  type Mark = { regNo: string; name: string; codes: string[]; hasLogo: boolean }
  async function registry(api: APIRequestContext) {
    const body = await (await api.get('/api/trademarks')).json()
    return body as { summary: { total: number; logos: number }; items: Mark[]; total: number }
  }

  test('список реестра и поиск по номеру', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const list = await registry(api)
    test.skip(list.summary.total === 0, 'реестр ТРОИС не загружен')
    expect(list.total).toBe(list.summary.total)
    const mark = list.items[0]
    const found = await (await api.get(`/api/trademarks?q=${encodeURIComponent(mark.regNo)}`)).json()
    expect(found.items.map((item: Mark) => item.regNo)).toContain(mark.regNo)
    const page2 = await (await api.get('/api/trademarks?page=1')).json()
    if (list.total > list.items.length) expect(page2.items[0].regNo).not.toBe(mark.regNo)
  })

  test('проверка наименований: знак из реестра находится, обычный товар — нет', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const list = await registry(api)
    const mark = list.items.find((item) => /^[\p{L}\s&]{4,}$/u.test(item.name) && item.codes.length > 0)
    test.skip(!mark, 'в реестре нет знака со словесным наименованием')
    const code = mark!.codes[0].padEnd(10, '0')
    const response = await api.post('/api/trademarks/check', { data: { rows: [{ texts: [`Товар ${mark!.name} новый`], code }, { texts: ['Кружка керамическая белая без надписей'], code: CODE_PORCELAIN }, { texts: [], code: '' }] } })
    expect(response.status()).toBe(200)
    const results = (await response.json()).results
    expect(results).toHaveLength(3)
    expect(results[0].map((hit: { regNo: string }) => hit.regNo)).toContain(mark!.regNo)
    expect(results[0][0]).toMatchObject({ sameGoods: true })
    expect(results[1]).toEqual([])
    expect(results[2]).toEqual([])
  })

  test('похожее написание латиницей вместо кириллицы → lookalike', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const response = await api.post('/api/trademarks/check', { data: { rows: [{ texts: ['Футболка мужская PULL & BEAR'], code: '6109100000' }] } })
    const hits = (await response.json()).results[0] as Array<{ name: string; kind: string }>
    test.skip(hits.length === 0, 'знака PULL & BEAR нет в загруженном реестре')
    expect(['brand', 'lookalike']).toContain(hits[0].kind)
  })

  test('проверка: слишком много строк и битые данные → 400', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const tooMany = await api.post('/api/trademarks/check', { data: { rows: Array.from({ length: 1001 }, () => ({ texts: ['x'], code: '' })) } })
    expect(tooMany.status()).toBe(400)
    const broken = await api.post('/api/trademarks/check', { data: '{"rows":', headers: { 'Content-Type': 'application/json' } })
    expect(broken.status()).toBe(400)
  })

  test('логотип: без номера и с чужим номером → 404, с логотипом → картинка', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    expect((await api.get('/api/trademarks/logo')).status()).toBe(404)
    expect((await api.get('/api/trademarks/logo?reg=нет-такого')).status()).toBe(404)
    const withLogo = (await (await api.get(`/api/trademarks?unnamed=0`)).json()).items.find((item: Mark) => item.hasLogo) as Mark | undefined
    test.skip(!withLogo, 'в реестре нет знаков с логотипом')
    const logo = await api.get(`/api/trademarks/logo?reg=${encodeURIComponent(withLogo!.regNo)}`)
    expect(logo.status()).toBe(200)
    expect(logo.headers()['content-type']).toMatch(/^image\//)
  })

  test('загрузка негодного файла реестра отклоняется и ничего не меняет', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const before = (await registry(api)).summary
    const response = await upload(api, '/api/trademarks/import', garbage, 'registry.xls')
    expect(response.status(), await response.text()).toBe(400)
    expect((await registry(api)).summary).toEqual(before)
    const badName = await api.patch('/api/trademarks/logo-name', { data: { regNo: 'нет-такого', name: 'X' } })
    expect([400, 404]).toContain(badName.status())
  })
})

test.describe('СУР, антидемпинг, перечень РК', () => {
  test('СУР: профили риска со ссылкой в номенклатуре', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const body = await (await api.get('/api/risk-indicators')).json()
    expect(Array.isArray(body.profiles)).toBe(true)
    test.skip(body.profiles.length === 0, 'профили СУР не загружены')
    for (const profile of body.profiles) {
      expect(profile.number).toBeTruthy()
      expect(profile.codeCount).toBeGreaterThan(0)
      expect(Array.isArray(profile.groups)).toBe(true)
    }
    const withSir = (await (await api.get('/api/tnved-nomenclature?q=8471&limit=200')).json()).items.find((item: { sir: unknown }) => item.sir)
    if (withSir) {
      expect(Number(withSir.sir.priceUsd)).toBeGreaterThan(0)
      expect(body.profiles.map((profile: { number: string }) => profile.number)).toContain(withSir.sir.profile)
    }
  })

  test('СУР: негодный файл и чужой профиль → ошибки без изменений', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const before = (await (await api.get('/api/risk-indicators')).json()).profiles
    expect((await upload(api, '/api/risk-indicators/import', garbage)).status()).toBe(400)
    expect((await upload(api, '/api/risk-indicators/import', await xlsx([['не', 'профиль'], ['1', '2']]))).status()).toBe(400)
    expect((await api.delete('/api/risk-indicators/999999999')).status()).toBe(404)
    expect((await (await api.get('/api/risk-indicators')).json()).profiles).toEqual(before)
  })

  test('антидемпинг: меры и их коды в номенклатуре', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const body = await (await api.get('/api/tnved-antidumping')).json()
    expect(Array.isArray(body.measures)).toBe(true)
    test.skip(body.measures.length === 0, 'антидемпинговых мер нет')
    for (const measure of body.measures) {
      expect(measure.number).toBeTruthy()
      expect(Array.isArray(measure.codes)).toBe(true)
      expect(Array.isArray(measure.countries)).toBe(true)
    }
    const measure = body.measures.find((item: { codes: string[]; suspended: boolean }) => !item.suspended && item.codes.some((code) => code.length === 10))
    test.skip(!measure, 'нет меры с полным кодом')
    const code = measure.codes.find((item: string) => item.length === 10)
    const found = await (await api.post('/api/tnved-nomenclature/lookup', { data: { codes: [code] } })).json()
    test.skip(found.items.length === 0, `кода меры ${code} нет в номенклатуре`)
    expect(found.items[0].antiDumping.map((ref: { number: string }) => ref.number)).toContain(measure.number)
  })

  test('перечень РК: Excel из строк PDF', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const response = await api.post('/api/tnved-kz-rates/excel', { data: { fileName: 'Перечень E2E.pdf', rows: [{ code: CODE_LAPTOP, name: 'Машины', rate: '0', notes: [], page: 1 }], footnotes: {} } })
    expect(response.status(), await response.text()).toBe(200)
    expect(response.headers()['content-type']).toContain('spreadsheetml')
    expect(decodeURIComponent(response.headers()['content-disposition'])).toContain('Перечень E2E.xlsx')
    const rows = await readXlsx(await response.body())
    expect(rows[0].join('|')).toContain('Код ТН ВЭД')
    expect(rows[0].join('|')).toContain('Ставка РК')
    expect(rows.some((cells) => cells.includes(CODE_LAPTOP))).toBe(true)
  })

  test('перечень РК: ошибки в строках и пустой PDF → 400, перечень не меняется', async ({ apiAs }) => {
    const api = await apiAs('admin')
    const before = (await (await api.get('/api/tnved-nomenclature?limit=1')).json()).kzImport
    const badRow = await api.post('/api/tnved-kz-rates/excel', { data: { fileName: 'x.pdf', rows: [{ code: 'abc', name: '', rate: 'zz', notes: [], page: 1 }], footnotes: {} } })
    expect(badRow.status()).toBe(400)
    expect((await badRow.json()).result.errors[0]).toMatchObject({ row: 1 })
    const emptyPdf = await api.post('/api/tnved-kz-rates/import-pdf', { data: { fileName: 'x.pdf', rows: [], footnotes: {} } })
    expect(emptyPdf.status()).toBe(400)
    const badPdfRow = await api.post('/api/tnved-kz-rates/import-pdf', { data: { fileName: 'x.pdf', rows: [{ code: '12', name: '', rate: '', notes: [], page: 1 }], footnotes: {} } })
    expect(badPdfRow.status()).toBe(400)
    expect((await upload(api, '/api/tnved-kz-rates/import', garbage)).status()).toBe(400)
    expect((await upload(api, '/api/tnved-kz-rates/import', await xlsx([['Нет', 'нужных колонок'], ['1', '2']]))).status()).toBe(400)
    expect((await (await api.get('/api/tnved-nomenclature?limit=1')).json()).kzImport).toEqual(before)
  })
})
