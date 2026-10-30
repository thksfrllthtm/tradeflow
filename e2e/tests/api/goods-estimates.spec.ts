import { test, expect } from '../../support/fixtures'
import { createEstimate, tinyPngDataUrl, row, calcInput } from '../../support/goods-helpers'
import { E2E, uid, users } from '../../support/users'
import type { APIRequestContext } from '@playwright/test'

// Сметы товаров через API: создание, правка, автосохранение частями, удаление и владение.

const created: Array<{ api: APIRequestContext; id: number }> = []
async function make(api: APIRequestContext, extra: Record<string, unknown> = {}) {
  const estimate = await createEstimate(api, extra)
  created.push({ api, id: estimate.id })
  return estimate
}
test.afterEach(async () => {
  for (const { api, id } of created.splice(0)) await api.delete(`/api/goods-estimates/${id}`)
})

test.describe('CRUD', () => {
  test('создание: 201, поля нормализованы, статус «Черновик», смета в списке владельца', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const name = `${E2E}-${uid()}`
    const estimate = await make(api, { name: `  ${name}  `, client: '  ТОО Клиент  ', assessmentDate: '2026-10-01' })
    expect(estimate).toMatchObject({ name, client: 'ТОО Клиент', assessmentDate: '2026-10-01', status: 'Черновик' })
    const list = (await (await api.get('/api/goods-estimates')).json()).estimates as Array<{ id: number }>
    expect(list.map((item) => item.id)).toContain(estimate.id)
  })

  test('без даты — сегодняшняя дата', async ({ apiAs }) => {
    const estimate = await make(await apiAs('invoicer'), { assessmentDate: '' })
    expect(estimate.assessmentDate).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  for (const [title, body, field] of [
    ['пустое название', { name: '   ' }, 'name'],
    ['название длиннее 200', { name: 'я'.repeat(201) }, 'name'],
    ['клиент длиннее 200', { name: 'x', client: 'я'.repeat(201) }, 'client'],
    ['дата не ГГГГ-ММ-ДД', { name: 'x', assessmentDate: '20.10.2026' }, 'assessmentDate'],
  ] as const) {
    test(`валидация создания: ${title} → 400 (${field})`, async ({ apiAs }) => {
      const response = await (await apiAs('invoicer')).post('/api/goods-estimates', { data: body })
      expect(response.status()).toBe(400)
      const error = (await response.json()).error
      expect(error.field).toBe(field)
      expect(error.message).toBeTruthy()
    })
  }

  test('ровно 200 символов в названии допустимо', async ({ apiAs }) => {
    const estimate = await make(await apiAs('invoicer'), { name: `${E2E}${'я'.repeat(197)}` })
    expect([...estimate.name]).toHaveLength(200)
  })

  test('битый JSON → 400', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/goods-estimates', { data: '{', headers: { 'Content-Type': 'application/json' } })
    expect(response.status()).toBe(400)
  })

  test('правка: PATCH меняет название, клиента и дату; GET отдаёт новое', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const name = `${E2E}-renamed-${uid()}`
    const patched = await api.patch(`/api/goods-estimates/${estimate.id}`, { data: { name, client: 'Новый клиент', assessmentDate: '2026-09-30' } })
    expect(patched.status()).toBe(200)
    const got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got).toMatchObject({ id: estimate.id, name, client: 'Новый клиент', assessmentDate: '2026-09-30' })
  })

  test('правка с пустым названием → 400, смета не изменилась', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    expect((await api.patch(`/api/goods-estimates/${estimate.id}`, { data: { name: '' } })).status()).toBe(400)
    expect((await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate.name).toBe(estimate.name)
  })

  test('удаление: 204, затем 404 на GET и повторное удаление', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await createEstimate(api)
    expect((await api.delete(`/api/goods-estimates/${estimate.id}`)).status()).toBe(204)
    expect((await api.get(`/api/goods-estimates/${estimate.id}`)).status()).toBe(404)
    expect((await api.delete(`/api/goods-estimates/${estimate.id}`)).status()).toBe(404)
  })

  for (const bad of ['0', '-1', 'abc']) {
    test(`некорректный id «${bad}» → 400`, async ({ apiAs }) => {
      expect((await (await apiAs('invoicer')).get(`/api/goods-estimates/${bad}`)).status()).toBe(400)
    })
  }
})

test.describe('автосохранение черновика (PATCH /draft)', () => {
  test('строки и параметры сохраняются, фото строки — отдельно и возвращаются в строке', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const items = [row('0101291000', '10', '12.5', '100', '90'), row('6401100000', '20', '3', '50', '40')]
    const saved = await api.patch(`/api/goods-estimates/${estimate.id}/draft`, {
      data: { input: { usdKzt: '471.25', deliveryMode: 'none', items, assessmentDate: '2026-10-15' }, photos: { [items[0].id]: [tinyPngDataUrl] } },
    })
    expect(saved.status(), await saved.text()).toBe(200)
    const got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.input.usdKzt).toBe('471.25')
    expect(got.assessmentDate, 'дата расчёта меняет и дату сметы').toBe('2026-10-15')
    expect(got.input.items.map((item: { id: string }) => item.id)).toEqual(items.map((item) => item.id))
    expect(got.input.items[0].photoUrls).toEqual([tinyPngDataUrl])
    expect(got.input.items[0].photoUrl).toBe(tinyPngDataUrl)
    expect(got.input.items[1].photoUrls ?? []).toEqual([])
  })

  test('частичное сохранение не стирает остальные части', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const items = [row('0101291000', '10', '12.5', '100', '90')]
    await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input: { usdKzt: '471.25', items }, photos: { [items[0].id]: [tinyPngDataUrl] } } })
    await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input: { cashUsd: '1000' } } })
    const got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.input.usdKzt).toBe('471.25')
    expect(got.input.cashUsd).toBe('1000')
    expect(got.input.items).toHaveLength(1)
    expect(got.input.items[0].photoUrls).toEqual([tinyPngDataUrl])
  })

  test('фото в теле строки не сохраняется в строках — только через photos', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const item = row('0101291000', '10', '12.5', '100', '90', { photoUrl: tinyPngDataUrl, photoUrls: [tinyPngDataUrl] })
    await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input: { items: [item] } } })
    const got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.input.items[0].photoUrl ?? '').toBe('')
  })

  test('пустой список фото удаляет фото строки', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const items = [row('0101291000', '10', '12.5', '100', '90')]
    await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input: { items }, photos: { [items[0].id]: [tinyPngDataUrl] } } })
    await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { photos: { [items[0].id]: [] } } })
    const got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.input.items[0].photoUrls ?? []).toEqual([])
  })

  test('результат: сохраняется → статус «Рассчитано», null → снова «Черновик»', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const input = calcInput([row('0101291000', '10', '12.5', '100', '90')])
    const result = await (await api.post('/api/goods/calculate', { data: input })).json()
    expect((await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input, result } })).status()).toBe(200)
    let got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.status).toBe('Рассчитано')
    expect(got.result.totals.customsPaymentsKzt).toBe(result.totals.customsPaymentsKzt)
    expect((await (await api.get('/api/goods-estimates')).json()).estimates.find((item: { id: number }) => item.id === estimate.id).status).toBe('Рассчитано')
    await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { result: null } })
    got = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.status).toBe('Черновик')
    expect(got.result ?? null).toBeNull()
  })

  for (const [title, data, field] of [
    ['неизвестное поле сметы', { input: { hacker: 1 } }, 'input'],
    ['строка без id', { input: { items: [{ name: 'x' }] } }, 'items'],
    ['повтор id строк', { input: { items: [{ id: 'a' }, { id: 'a' }] } }, 'items'],
    ['фото не картинка', { photos: { a: ['data:text/plain;base64,aGk='] } }, 'photos'],
    ['фото GIF', { photos: { a: ['data:image/gif;base64,R0lGODlhAQABAAAAACw='] } }, 'photos'],
    ['фото с битым base64', { photos: { a: ['data:image/png;base64,%%%'] } }, 'photos'],
    ['пустое фото', { photos: { a: [' '] } }, 'photos'],
    ['больше 20 фото у строки', { photos: { a: Array(21).fill(tinyPngDataUrl) } }, 'photos'],
    ['фото без строки', { photos: { '': [tinyPngDataUrl] } }, 'photos'],
    ['дата расчёта не ГГГГ-ММ-ДД', { input: { assessmentDate: '01.10.2026' } }, 'assessmentDate'],
    ['результат не объект', { result: '"x"' }, 'result'],
  ] as const) {
    test(`валидация черновика: ${title} → 400`, async ({ apiAs }) => {
      const api = await apiAs('invoicer')
      const estimate = await make(api)
      const payload = title === 'результат не объект' ? '{"result":"x"}' : JSON.stringify(data)
      const response = await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: payload, headers: { 'Content-Type': 'application/json' } })
      expect(response.status(), await response.text()).toBe(400)
      expect((await response.json()).error.field).toBe(field)
    })
  }

  test('больше 5000 строк в черновике → 400', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimate = await make(api)
    const items = Array.from({ length: 5001 }, (_, index) => ({ id: `r${index}` }))
    const response = await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input: { items } } })
    expect(response.status()).toBe(400)
    expect((await response.json()).error.field).toBe('items')
  })
})

test.describe('владение сметами', () => {
  test('другой инвойсер не видит, не меняет, не сохраняет черновик и не удаляет чужую смету (404)', async ({ apiAs }) => {
    const owner = await apiAs('invoicer')
    const other = await apiAs('invoicer2')
    const estimate = await make(owner)
    expect((await other.get(`/api/goods-estimates/${estimate.id}`)).status()).toBe(404)
    expect((await other.patch(`/api/goods-estimates/${estimate.id}`, { data: { name: 'взлом' } })).status()).toBe(404)
    expect((await other.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { input: { usdKzt: '1' } } })).status()).toBe(404)
    expect((await other.delete(`/api/goods-estimates/${estimate.id}`)).status()).toBe(404)
    const list = (await (await other.get('/api/goods-estimates')).json()).estimates as Array<{ id: number }>
    expect(list.map((item) => item.id)).not.toContain(estimate.id)
    // Смета владельца не тронута.
    const got = (await (await owner.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
    expect(got.name).toBe(estimate.name)
    expect(got.input.usdKzt ?? '').toBe('')
  })

  test('инвойсер с ?userId= чужого получает только свои сметы', async ({ apiAs }) => {
    const owner = await apiAs('invoicer')
    const other = await apiAs('invoicer2')
    const estimate = await make(owner)
    const ownerId = (await (await owner.get('/api/auth/me')).json()).user.id
    const response = await other.get(`/api/goods-estimates?userId=${ownerId}`)
    expect(response.status()).toBe(200)
    const ids = ((await response.json()).estimates as Array<{ id: number }>).map((item) => item.id)
    expect(ids).not.toContain(estimate.id)
  })

  test('администратор открывает смету сотрудника, видит владельца и список по ?userId=', async ({ apiAs }) => {
    const owner = await apiAs('invoicer')
    const admin = await apiAs('admin')
    const estimate = await make(owner)
    const ownerId = (await (await owner.get('/api/auth/me')).json()).user.id
    const got = await admin.get(`/api/goods-estimates/${estimate.id}`)
    expect(got.status()).toBe(200)
    expect((await got.json()).estimate).toMatchObject({ id: estimate.id, ownerId, ownerName: users.invoicer.name })
    const list = (await (await admin.get(`/api/goods-estimates?userId=${ownerId}`)).json()).estimates as Array<{ id: number }>
    expect(list.map((item) => item.id)).toContain(estimate.id)
    // Собственный список администратора не содержит сметы сотрудника.
    const own = (await (await admin.get('/api/goods-estimates')).json()).estimates as Array<{ id: number }>
    expect(own.map((item) => item.id)).not.toContain(estimate.id)
  })

  test('администратор может переименовать и удалить смету сотрудника', async ({ apiAs }) => {
    const owner = await apiAs('invoicer')
    const admin = await apiAs('admin')
    const estimate = await createEstimate(owner)
    const name = `${E2E}-by-admin-${uid()}`
    expect((await admin.patch(`/api/goods-estimates/${estimate.id}`, { data: { name } })).status()).toBe(200)
    expect((await (await owner.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate.name).toBe(name)
    expect((await admin.delete(`/api/goods-estimates/${estimate.id}`)).status()).toBe(204)
    expect((await owner.get(`/api/goods-estimates/${estimate.id}`)).status()).toBe(404)
  })

  test('некорректный ?userId= у администратора → 400', async ({ apiAs }) => {
    expect((await (await apiAs('admin')).get('/api/goods-estimates?userId=abc')).status()).toBe(400)
  })
})

test.describe('история цен (price-history)', () => {
  async function estimateWithPrice(api: APIRequestContext, code: string, price: string) {
    const estimate = await make(api)
    const result = { items: [{ code, pairPriceUsd: price, excluded: false }], totals: {} }
    expect((await api.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { result } })).status()).toBe(200)
    return estimate
  }

  const prices = async (api: APIRequestContext, codes: string[], estimateId = 0) => {
    const response = await api.post('/api/estimates/price-history', { data: { estimateId, codes } })
    expect(response.status(), await response.text()).toBe(200)
    return (await response.json()).prices
  }

  test('инвойсер: цена кода из своей прошлой сметы; текущая исключается; чужие не видны', async ({ apiAs }) => {
    const owner = await apiAs('invoicer')
    const code = `E2E${uid()}`
    const estimate = await estimateWithPrice(owner, code, '7.25')
    expect(await prices(owner, [code, code, ' '])).toEqual([{ code, unitPrice: 7.25 }])
    expect(await prices(owner, [code], estimate.id), 'сама смета в истории не участвует').toEqual([])
    expect(await prices(await apiAs('invoicer2'), [code]), 'цены чужих смет не видны').toEqual([])
  })

  test('администратор видит цену из сметы сотрудника; текущая смета исключается', async ({ apiAs }) => {
    const owner = await apiAs('invoicer')
    const admin = await apiAs('admin')
    const code = `E2E${uid()}`
    const estimate = await estimateWithPrice(owner, code, '7.25')
    expect(await prices(admin, [code, code, ' '])).toEqual([{ code, unitPrice: 7.25 }])
    expect(await prices(admin, [code], estimate.id)).toEqual([])
  })

  test('берётся цена из последней изменённой сметы', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const code = `E2E${uid()}`
    await estimateWithPrice(admin, code, '1.5')
    await estimateWithPrice(admin, code, '2.75')
    expect(await prices(admin, [code])).toEqual([{ code, unitPrice: 2.75 }])
  })

  test('строки-исключения и нулевые цены в историю не попадают', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const code = `E2E${uid()}`
    const estimate = await make(admin)
    const result = { items: [{ code, pairPriceUsd: '9', excluded: true }, { code, pairPriceUsd: '0' }], totals: {} }
    await admin.patch(`/api/goods-estimates/${estimate.id}/draft`, { data: { result } })
    expect(await prices(admin, [code])).toEqual([])
  })

  test('пустой список кодов → пустой ответ', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/estimates/price-history', { data: { codes: [] } })
    expect(response.status()).toBe(200)
    expect((await response.json()).prices).toEqual([])
  })
})
