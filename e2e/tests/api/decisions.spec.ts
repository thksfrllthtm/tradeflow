import type { APIRequestContext } from '@playwright/test'
import { test, expect } from '../../support/fixtures'
import { codePrefix, createDecision, decisionName, DecisionTracker, realCode } from '../../support/decisions-helpers'
import { E2E, uid, users } from '../../support/users'

// База решений через API: запись сотрудника «Не проверено», проверка/отклонение/правка/удаление
// администратором, список с фильтрами, импорт и связь со сметой.

// Имя e2e-администратора может быть пустым (bootstrap-admin не задаёт имя) — тогда показывается email.
const adminNames: string[] = [users.admin.name, users.admin.email]

let tracker: DecisionTracker
let prefix: string
test.beforeEach(({ apiAs }) => { tracker = new DecisionTracker(() => apiAs('admin')); prefix = codePrefix() })
test.afterEach(async () => { await tracker.cleanup(); await tracker.cleanupByCode(prefix) })

test.describe('создание', () => {
  test('инвойсер добавляет запись: «Не проверено», источник «вручную», код нормализуется', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const spaced = `${prefix.slice(0, 4)} ${prefix.slice(4, 6)} ${prefix.slice(6, 9)} 1`
    const { response, body } = await createDecision(api, { invoiceName: decisionName('create'), supplierName: 'Supplier E2E', code: spaced, reason: 'причина', quote: 'цитата', unitPriceUsd: '1,25', unitNetKg: '0.5' })
    expect(response.status()).toBe(201)
    tracker.track(body.id)
    expect(body).toMatchObject({ status: 'pending', source: 'manual', code: `${prefix}1`, invoiceName: decisionName('create'), supplierName: 'Supplier E2E', reason: 'причина', quote: 'цитата', unitPriceUsd: '1.25', unitNetKg: '0.5', createdByName: users.invoicer.name, photos: [], rejectedCodes: [] })
    expect(body.nomenclature, 'кода 99… нет в номенклатуре').toBeNull()
    expect(body.verifiedAt).toBeNull()
  })

  test('код из номенклатуры — запись получает ставки номенклатуры', async ({ apiAs }) => {
    const { response, body } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('nomen'), code: realCode })
    expect(response.status()).toBe(201)
    tracker.track(body.id)
    expect(body.nomenclature).toMatchObject({ code: realCode })
    expect(body.nomenclature.vatPercent).toBeTruthy()
  })

  test('только итоговое наименование — тоже годится', async ({ apiAs }) => {
    const { response, body } = await createDecision(await apiAs('invoicer'), { invoiceName: '', finalName: decisionName('final'), code: `${prefix}2` })
    expect(response.status()).toBe(201)
    tracker.track(body.id)
    expect(body.finalName).toBe(decisionName('final'))
  })

  test('отвергнутые коды: свой код и негодные отброшены, не больше пяти', async ({ apiAs }) => {
    const rejected = [
      { code: `${prefix}3`, why: 'тот же код' },
      { code: '123', why: 'негодный' },
      ...[0, 1, 2, 3, 4, 5].map((n) => ({ code: `98000000${n}0`, why: `причина ${n}` })),
    ]
    const { response, body } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('rej'), code: `${prefix}3`, rejectedCodes: rejected })
    expect(response.status()).toBe(201)
    tracker.track(body.id)
    expect(body.rejectedCodes).toHaveLength(5)
    expect(body.rejectedCodes.map((item: { code: string }) => item.code)).not.toContain(`${prefix}3`)
    expect(body.rejectedCodes[0]).toEqual({ code: '9800000000', why: 'причина 0' })
  })

  test('негодный код агента не сохраняется', async ({ apiAs }) => {
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('agent'), code: `${prefix}4`, agentCode: '12345' })
    tracker.track(body.id)
    expect(body.agentCode).toBe('')
  })

  const invalid: Array<[string, Record<string, unknown>, string]> = [
    ['код из 9 цифр', { invoiceName: 'x', code: '123456789' }, 'code'],
    ['код с буквами', { invoiceName: 'x', code: '84713000AB' }, 'code'],
    ['пустой код', { invoiceName: 'x', code: '' }, 'code'],
    ['нет ни одного наименования', { invoiceName: '  ', finalName: '', supplierName: '', code: realCode }, 'invoiceName'],
    ['отрицательная цена', { invoiceName: 'x', code: realCode, unitPriceUsd: '-1' }, 'unitPriceUsd'],
    ['цена не число', { invoiceName: 'x', code: realCode, unitPriceUsd: 'abc' }, 'unitPriceUsd'],
    ['нетто больше 10^8', { invoiceName: 'x', code: realCode, unitNetKg: '100000000' }, 'unitNetKg'],
    ['фото не картинка', { invoiceName: 'x', code: realCode, photos: ['data:text/plain;base64,aGVsbG8='] }, 'photos'],
  ]
  for (const [title, input, field] of invalid) {
    test(`отказ 400: ${title}`, async ({ apiAs }) => {
      const { response, body } = await createDecision(await apiAs('invoicer'), input)
      expect(response.status(), JSON.stringify(body)).toBe(400)
      expect(body.error.field).toBe(field)
      expect(body.error.message).toBeTruthy()
    })
  }

  test('битый JSON → 400', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/code-decisions', { data: '{"code":', headers: { 'Content-Type': 'application/json' } })
    expect(response.status()).toBe(400)
  })

  test('длинные тексты обрезаются, а не роняют сервер', async ({ apiAs }) => {
    const { response, body } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('long') + 'я'.repeat(5000), reason: 'r'.repeat(20000), code: `${prefix}5` })
    expect(response.status()).toBe(201)
    tracker.track(body.id)
    expect(body.invoiceName.length).toBeLessThan(5000)
    expect(body.reason.length).toBeLessThan(20000)
  })
})

test.describe('чтение и список', () => {
  test('карточка по id и 404 на несуществующую и негодный id', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const { body: created } = await createDecision(api, { invoiceName: decisionName('get'), code: `${prefix}1` })
    tracker.track(created.id)
    const got = await api.get(`/api/code-decisions/${created.id}`)
    expect(got.status()).toBe(200)
    expect(await got.json()).toMatchObject({ id: created.id, code: `${prefix}1`, invoiceName: decisionName('get') })
    expect((await api.get('/api/code-decisions/999999999')).status()).toBe(404)
    expect((await api.get('/api/code-decisions/abc')).status()).toBe(404)
    expect((await api.get('/api/code-decisions/-5')).status()).toBe(404)
  })

  test('поиск по началу кода, фильтр статуса, limit/offset и порядок «новые сверху»', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const ids: number[] = []
    for (const n of [1, 2, 3]) {
      const { body } = await createDecision(api, { invoiceName: decisionName(`list${n}`), code: `${prefix}${n}` })
      ids.push(tracker.track(body.id))
    }
    const all = await (await api.get(`/api/code-decisions?q=${prefix}`)).json()
    expect(all.total).toBe(3)
    expect(all.items.map((item: { id: number }) => item.id)).toEqual([...ids].reverse())
    expect(all.stats).toEqual(expect.objectContaining({ pending: expect.any(Number), verified: expect.any(Number), rejected: expect.any(Number) }))

    const spaced = await (await api.get(`/api/code-decisions?q=${encodeURIComponent(`${prefix.slice(0, 4)} ${prefix.slice(4)}`)}`)).json()
    expect(spaced.total, 'пробелы в коде запроса не мешают').toBe(3)

    const page1 = await (await api.get(`/api/code-decisions?q=${prefix}&limit=2&offset=0`)).json()
    const page2 = await (await api.get(`/api/code-decisions?q=${prefix}&limit=2&offset=2`)).json()
    expect(page1.items).toHaveLength(2)
    expect(page2.items).toHaveLength(1)
    expect(page1.total).toBe(3)
    expect(new Set([...page1.items, ...page2.items].map((item: { id: number }) => item.id)).size).toBe(3)

    expect((await (await api.get(`/api/code-decisions?q=${prefix}&status=pending`)).json()).total).toBe(3)
    expect((await (await api.get(`/api/code-decisions?q=${prefix}&status=verified`)).json()).total).toBe(0)
    expect((await (await api.get(`/api/code-decisions?q=${prefix}&status=bogus`)).json()).total, 'неизвестный статус — как «все»').toBe(3)
    expect((await (await api.get(`/api/code-decisions?q=${prefix}&agentWrong=true`)).json()).total).toBe(0)
  })

  test('негодные limit/offset не ломают список', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    for (const query of ['limit=-1', 'limit=100000', 'offset=-10', 'limit=abc&offset=xyz']) {
      const response = await api.get(`/api/code-decisions?q=${prefix}&${query}`)
      expect(response.status(), query).toBe(200)
    }
    const big = await (await api.get('/api/code-decisions?limit=100000')).json()
    expect(big.items.length, 'потолок 200 → 50 по умолчанию').toBeLessThanOrEqual(200)
  })

  test('статистика растёт после добавления записи', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const before = await (await api.get('/api/code-decisions/stats')).json()
    expect(before).toEqual(expect.objectContaining({ pending: expect.any(Number), verified: expect.any(Number), rejected: expect.any(Number), agentChecked: expect.any(Number), agentCorrect: expect.any(Number) }))
    const { body } = await createDecision(api, { invoiceName: decisionName('stats'), code: `${prefix}1` })
    tracker.track(body.id)
    const after = await (await api.get('/api/code-decisions/stats')).json()
    expect(after.pending).toBeGreaterThanOrEqual(before.pending + 1)
  })
})

test.describe('проверка администратором', () => {
  test('«Проверено» с правкой кода: статус, кто проверил, ИИ ошибся', async ({ apiAs }) => {
    const { body: created } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('verify'), code: `${prefix}1`, agentCode: `${prefix}1` })
    tracker.track(created.id)
    const admin = await apiAs('admin')
    const response = await admin.post(`/api/code-decisions/${created.id}/verify`, { data: { code: `${prefix}2`, finalName: 'итог / 中文', unitPriceUsd: '2.5' } })
    expect(response.status()).toBe(200)
    const verified = await response.json()
    expect(verified).toMatchObject({ status: 'verified', code: `${prefix}2`, finalName: 'итог / 中文', unitPriceUsd: '2.5', agentCorrect: false })
    expect(adminNames).toContain(verified.verifiedByName)
    expect(verified.verifiedAt).toBeTruthy()
    const filtered = await (await admin.get(`/api/code-decisions?q=${prefix}&agentWrong=true`)).json()
    expect(filtered.items.map((item: { id: number }) => item.id)).toContain(created.id)
  })

  test('«Проверено» без правки: код агента верный', async ({ apiAs }) => {
    const { body: created } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('agentok'), code: `${prefix}1`, agentCode: `${prefix}1` })
    tracker.track(created.id)
    const response = await (await apiAs('admin')).post(`/api/code-decisions/${created.id}/verify`)
    expect(response.status()).toBe(200)
    expect(await response.json()).toMatchObject({ status: 'verified', agentCorrect: true, code: `${prefix}1` })
  })

  test('правка без смены статуса; пустая цена стирает цену; негодный код → 400', async ({ apiAs }) => {
    const { body: created } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('patch'), code: `${prefix}1`, unitPriceUsd: '3' })
    tracker.track(created.id)
    const admin = await apiAs('admin')
    const patched = await admin.patch(`/api/code-decisions/${created.id}`, { data: { finalName: 'новое', unitPriceUsd: '', note: 'заметка' } })
    expect(patched.status()).toBe(200)
    expect(await patched.json()).toMatchObject({ status: 'pending', finalName: 'новое', unitPriceUsd: '', note: 'заметка' })
    const bad = await admin.patch(`/api/code-decisions/${created.id}`, { data: { code: '12' } })
    expect(bad.status()).toBe(400)
    expect((await bad.json()).error.field).toBe('code')
    expect((await admin.patch(`/api/code-decisions/${created.id}`, { data: { unitPriceUsd: '-3' } })).status()).toBe(400)
    expect((await admin.patch('/api/code-decisions/999999999', { data: { note: 'x' } })).status()).toBe(404)
  })

  test('«Отклонить» с заметкой', async ({ apiAs }) => {
    const { body: created } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('reject'), code: `${prefix}1` })
    tracker.track(created.id)
    const admin = await apiAs('admin')
    const response = await admin.post(`/api/code-decisions/${created.id}/reject`, { data: { note: 'не тот товар' } })
    expect(response.status()).toBe(200)
    expect(await response.json()).toMatchObject({ status: 'rejected', note: 'не тот товар' })
    expect((await (await admin.get(`/api/code-decisions?q=${prefix}&status=rejected`)).json()).total).toBe(1)
    expect((await admin.post('/api/code-decisions/999999999/reject')).status()).toBe(404)
  })

  test('удаление: 204, затем 404', async ({ apiAs }) => {
    const { body: created } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('delete'), code: `${prefix}1` })
    const admin = await apiAs('admin')
    expect((await admin.delete(`/api/code-decisions/${created.id}`)).status()).toBe(204)
    expect((await admin.get(`/api/code-decisions/${created.id}`)).status()).toBe(404)
    expect((await admin.delete(`/api/code-decisions/${created.id}`)).status()).toBe(404)
  })

  test('инвойсер не может проверить, отклонить, править и удалить — запись не меняется', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const { body: created } = await createDecision(api, { invoiceName: decisionName('deny'), code: `${prefix}1` })
    tracker.track(created.id)
    expect((await api.post(`/api/code-decisions/${created.id}/verify`, { data: { code: `${prefix}9` } })).status()).toBe(403)
    expect((await api.post(`/api/code-decisions/${created.id}/reject`, { data: { note: 'x' } })).status()).toBe(403)
    expect((await api.patch(`/api/code-decisions/${created.id}`, { data: { code: `${prefix}9` } })).status()).toBe(403)
    expect((await api.delete(`/api/code-decisions/${created.id}`)).status()).toBe(403)
    expect(await (await api.get(`/api/code-decisions/${created.id}`)).json()).toMatchObject({ status: 'pending', code: `${prefix}1` })
  })

  test('переводчик не видит базу решений', async ({ apiAs }) => {
    const api = await apiAs('translator')
    expect((await api.get(`/api/code-decisions?q=${prefix}`)).status()).toBe(403)
    expect((await createDecision(api, { invoiceName: decisionName('tr'), code: `${prefix}1` })).response.status()).toBe(403)
  })
})

test.describe('импорт', () => {
  test('пустой список, битый JSON и слишком много строк → 400', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const empty = await admin.post('/api/code-decisions/import', { data: { rows: [] } })
    expect(empty.status()).toBe(400)
    expect((await empty.json()).error.message).toContain('Нет строк')
    expect((await admin.post('/api/code-decisions/import', { data: {} })).status()).toBe(400)
    expect((await admin.post('/api/code-decisions/import', { data: '{"rows":[', headers: { 'Content-Type': 'application/json' } })).status()).toBe(400)
    expect((await admin.post('/api/code-decisions/import', { data: { rows: 'строка' } })).status()).toBe(400)
    const many = Array.from({ length: 201 }, (_, n) => ({ invoiceName: `x${n}`, code: '9900000000' }))
    const tooMany = await admin.post('/api/code-decisions/import', { data: { rows: many } })
    expect(tooMany.status(), 'лимит строк за раз').toBe(400)
  })

  test('строки с ошибками пропускаются с номером, годные — сразу «Проверено», повтор пропускается', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const rows = [
      { invoiceName: decisionName('imp1'), code: `${prefix}1` },
      { invoiceName: decisionName('imp2'), code: '12' },
      { invoiceName: '', code: `${prefix}3` },
      { invoiceName: decisionName('imp4'), code: `${prefix}4`, unitPriceUsd: '7' },
    ]
    const response = await admin.post('/api/code-decisions/import', { data: { rows } })
    expect(response.status()).toBe(200)
    const result = await response.json()
    expect(result.created).toBe(2)
    expect(result.skipped).toBe(0)
    expect(result.errors.map((error: { row: number }) => error.row)).toEqual([2, 3])

    const list = await (await admin.get(`/api/code-decisions?q=${prefix}`)).json()
    expect(list.total).toBe(2)
    for (const item of list.items) {
      expect(item).toMatchObject({ status: 'verified', source: 'import' })
      expect(adminNames).toContain(item.verifiedByName)
    }

    const again = await (await admin.post('/api/code-decisions/import', { data: { rows: [rows[0]] } })).json()
    expect(again).toMatchObject({ created: 0, skipped: 1 })
    const kept = await (await admin.post('/api/code-decisions/import', { data: { rows: [rows[0]], keepDuplicates: true } })).json()
    expect(kept, 'keepDuplicates — повтор загружается').toMatchObject({ created: 1, skipped: 0 })
  })
})

test.describe('связь со сметой', () => {
  async function estimate(api: APIRequestContext, tag: string) {
    const response = await api.post('/api/goods-estimates', { data: { name: `${E2E}-${tag}-${uid()}`, client: '', assessmentDate: '' } })
    expect(response.status()).toBe(201)
    return (await response.json()).estimate.id as number
  }

  test('статус строк своей сметы — 200, чужой — 404, администратору — 200', async ({ apiAs }) => {
    const own = await apiAs('invoicer')
    const other = await apiAs('invoicer2')
    const ownId = await estimate(own, 'dec-own')
    const otherId = await estimate(other, 'dec-other')
    try {
      const mine = await own.get(`/api/code-decisions/estimate/${ownId}`)
      expect(mine.status()).toBe(200)
      expect(await mine.json()).toEqual({ items: {} })
      expect((await own.get(`/api/code-decisions/estimate/${otherId}`)).status(), 'чужая смета не видна').toBe(404)
      expect((await (await apiAs('admin')).get(`/api/code-decisions/estimate/${otherId}`)).status()).toBe(200)
      expect((await own.get('/api/code-decisions/estimate/abc')).status()).toBe(404)
    } finally {
      await own.delete(`/api/goods-estimates/${ownId}`)
      await other.delete(`/api/goods-estimates/${otherId}`)
    }
  })

  test('тот же товар той же сметы второй раз не записывается — возвращается прежняя запись', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const estimateId = await estimate(api, 'dec-dup')
    try {
      const input = { invoiceName: decisionName('dup'), code: `${prefix}1`, estimateId, itemId: 'row-1' }
      const first = await createDecision(api, input)
      expect(first.response.status()).toBe(201)
      tracker.track(first.body.id)
      const second = await createDecision(api, { ...input, itemId: 'row-2' })
      expect(second.response.status()).toBe(200)
      expect(second.body).toMatchObject({ id: first.body.id, duplicate: true })
      const third = await createDecision(api, { ...input, code: `${prefix}2`, itemId: 'row-3' })
      expect(third.response.status(), 'другой код — новая запись').toBe(201)
      tracker.track(third.body.id)
    } finally {
      await api.delete(`/api/goods-estimates/${estimateId}`)
    }
  })
})

test.describe('статус и расход ИИ-агента', () => {
  for (const who of ['admin', 'invoicer', 'translator'] as const) {
    test(`статус агента для ${who}: настроен ли, лимит и сколько использовано`, async ({ apiAs }) => {
      const response = await (await apiAs(who)).get('/api/tnved-agent/status')
      expect(response.status()).toBe(200)
      const status = await response.json()
      expect(status).toEqual(expect.objectContaining({ configured: expect.any(Boolean), dailyLimit: expect.any(Number), usedToday: expect.any(Number) }))
      expect(status.usedToday).toBeGreaterThanOrEqual(0)
    })
  }

  test('расход агента — только администратору', async ({ apiAs }) => {
    const response = await (await apiAs('admin')).get('/api/tnved-agent/usage')
    expect(response.status()).toBe(200)
    expect(await response.json()).toBeTruthy()
    expect((await (await apiAs('invoicer')).get('/api/tnved-agent/usage')).status()).toBe(403)
  })

  test('история справочника — своя у каждого; удалить несуществующий вопрос → 404', async ({ apiAs }) => {
    for (const who of ['invoicer', 'translator'] as const) {
      const api = await apiAs(who)
      const history = await api.get('/api/tnved-agent/reference')
      expect(history.status()).toBe(200)
      expect(Array.isArray((await history.json()).items)).toBe(true)
      expect((await api.delete('/api/tnved-agent/reference/999999999')).status()).toBe(404)
    }
  })

  test('чужой вопрос справочника не открыть и не удалить', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const adminHistory = (await (await admin.get('/api/tnved-agent/reference')).json()).items as Array<{ id: number }>
    const invoicerHistory = (await (await (await apiAs('invoicer')).get('/api/tnved-agent/reference')).json()).items as Array<{ id: number }>
    const foreign = adminHistory.find((item) => !invoicerHistory.some((own) => own.id === item.id))
    test.skip(!foreign, 'у e2e-администратора нет вопросов в истории — проверять нечего')
    const api = await apiAs('invoicer2')
    expect([403, 404]).toContain((await api.get(`/api/tnved-agent/sessions/${foreign!.id}`)).status())
    expect([403, 404]).toContain((await api.delete(`/api/tnved-agent/reference/${foreign!.id}`)).status())
  })
})
