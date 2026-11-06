import type { APIRequestContext } from '@playwright/test'
import { test, expect } from '../../support/fixtures'
import { E2E, uid } from '../../support/users'

// Керамика: база марок (наименование, марка, цена), точный поиск марок и цена типа/размера —
// без ИИ. /analyze (ИИ) не вызывается. /price читает только базу (price.go: Price → catalog.go: loadContext).

type Product = { id: number; mark: string; name: string; price: string; productType: string; sizeValue: string; sizeUnit: string; chineseName: string }

// Марки этого теста: после него администратор удаляет только их (тесты идут параллельно,
// общий поиск по префиксу удалил бы записи соседнего теста).
const used: string[] = []
const mark = () => { const value = `${E2E}-CER-${uid()}`.toUpperCase(); used.push(value); return value }

async function cleanup(admin: APIRequestContext) {
  for (const value of used.splice(0)) {
    const list = await (await admin.get(`/api/ceramics/products?q=${encodeURIComponent(value)}`)).json()
    for (const item of list.items as Product[]) if (item.mark === value) await admin.delete(`/api/ceramics/products/${item.id}`)
  }
}

test.describe('керамика API', () => {
  test.afterEach(async ({ apiAs }) => cleanup(await apiAs('admin')))

  test('статус ИИ, типы, список базы', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const status = await (await api.get('/api/ceramics/status')).json()
    expect(typeof status.ai.configured).toBe('boolean')
    expect(typeof status.ai.vision).toBe('boolean')

    const types = await api.get('/api/ceramics/types')
    expect(types.status()).toBe(200)
    expect(Array.isArray((await types.json()).items)).toBe(true)

    const list = await (await api.get('/api/ceramics/products?limit=5')).json()
    expect(list.items.length).toBeLessThanOrEqual(5)
    expect(typeof list.total).toBe('number')
  })

  test('инвойсер добавляет и правит марку; тип и размер — из наименования', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const code = mark()
    const created = await api.post('/api/ceramics/products', { data: { mark: code, name: 'Тарелка керамическая 8 дюймов/ 陶瓷盘8寸', price: '1,25' } })
    expect(created.status(), await created.text()).toBe(201)
    const item = (await created.json()).item as Product
    expect(item).toMatchObject({ mark: code, price: '1.25', sizeValue: '8', sizeUnit: 'inch' })
    expect(item.productType).toBeTruthy()
    expect(item.chineseName).toContain('陶瓷盘')

    // Поиск по базе находит новую марку.
    const found = await (await api.get(`/api/ceramics/products?q=${code}`)).json()
    expect(found.items.map((row: Product) => row.id)).toContain(item.id)

    const updated = await api.patch(`/api/ceramics/products/${item.id}`, { data: { mark: code, name: 'Тарелка керамическая 10 дюймов/ 陶瓷盘10寸', price: '2.5' } })
    expect(updated.status(), await updated.text()).toBe(200)
    expect((await updated.json()).item).toMatchObject({ id: item.id, price: '2.5', sizeValue: '10' })
  })

  test('повтор марки → 400; правка чужой записи в занятую марку → 400', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const first = mark()
    const second = mark()
    const a = (await (await api.post('/api/ceramics/products', { data: { mark: first, name: 'Кружка керамическая', price: '1' } })).json()).item as Product
    await api.post('/api/ceramics/products', { data: { mark: second, name: 'Кружка керамическая', price: '1' } })

    const duplicate = await api.post('/api/ceramics/products', { data: { mark: first.toLowerCase(), name: 'Кружка', price: '1' } })
    expect(duplicate.status()).toBe(400)
    expect((await duplicate.json()).error).toMatchObject({ field: 'mark' })

    const clash = await api.patch(`/api/ceramics/products/${a.id}`, { data: { mark: second, name: 'Кружка', price: '1' } })
    expect(clash.status()).toBe(400)
    expect((await clash.json()).error.message).toContain('другой записи')
  })

  const invalid: Array<[string, object, string]> = [
    ['пустая марка', { mark: '  ', name: 'Чашка', price: '1' }, 'mark'],
    ['пустое наименование', { mark: 'X1', name: '', price: '1' }, 'name'],
    ['цена 0', { mark: 'X1', name: 'Чашка', price: '0' }, 'price'],
    ['цена не число', { mark: 'X1', name: 'Чашка', price: 'abc' }, 'price'],
    ['цена с 5 знаками', { mark: 'X1', name: 'Чашка', price: '1.12345' }, 'price'],
    ['наименование длиннее предела', { mark: 'X1', name: 'я'.repeat(5000), price: '1' }, 'name'],
  ]
  for (const [name, data, field] of invalid) {
    test(`валидация записи: ${name} → 400 (${field})`, async ({ apiAs }) => {
      const response = await (await apiAs('invoicer')).post('/api/ceramics/products', { data })
      expect(response.status(), await response.text()).toBe(400)
      expect((await response.json()).error.field).toBe(field)
    })
  }

  test('правка несуществующей записи → 404, некорректный id → 400', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    expect((await api.patch('/api/ceramics/products/999999999', { data: { mark: mark(), name: 'Чашка', price: '1' } })).status()).toBe(404)
    expect((await api.patch('/api/ceramics/products/abc', { data: { mark: 'X', name: 'Чашка', price: '1' } })).status()).toBe(400)
  })

  test('удаляет только администратор', async ({ apiAs }) => {
    const invoicer = await apiAs('invoicer')
    const item = (await (await invoicer.post('/api/ceramics/products', { data: { mark: mark(), name: 'Ваза керамическая', price: '3' } })).json()).item as Product
    expect((await invoicer.delete(`/api/ceramics/products/${item.id}`)).status()).toBe(403)
    const admin = await apiAs('admin')
    expect((await admin.delete(`/api/ceramics/products/${item.id}`)).status()).toBe(204)
    expect((await admin.delete(`/api/ceramics/products/${item.id}`)).status()).toBe(404)
  })

  test('поиск марок без ИИ: найденные — FOUND_BY_MARK, неизвестные — не в ответе', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const code = mark()
    await api.post('/api/ceramics/products', { data: { mark: code, name: 'Салатник керамический 6 дюймов', price: '4.4' } })
    const response = await api.post('/api/ceramics/lookup', { data: { marks: [code.toLowerCase(), ` ${code} `, `${code}-NONE`] } })
    expect(response.status()).toBe(200)
    const items = (await response.json()).items
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ status: 'FOUND_BY_MARK', mark: code, aiUsed: false, price: { status: 'MATCHED', price: '4.4' } })
    expect(items[0].product.name).toContain('Салатник')

    const empty = await (await api.post('/api/ceramics/lookup', { data: { marks: [] } })).json()
    expect(empty.items).toEqual([])
  })

  test('поиск марок: больше предела за раз → 400', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/ceramics/lookup', { data: { marks: Array.from({ length: 5001 }, (_, index) => `M${index}`) } })
    expect(response.status()).toBe(400)
    expect((await response.json()).error.field).toBe('marks')
  })

  test('цена типа и размера из базы: самая частая цена; неизвестный тип → 400', async ({ apiAs }) => {
    const api = await apiAs('invoicer')
    const items = await Promise.all(['5', '5', '7'].map(async (price) =>
      (await (await api.post('/api/ceramics/products', { data: { mark: mark(), name: `${E2E}чашник керамический 9 дюймов`, price } })).json()).item as Product))
    const type = items[0].productType
    const response = await api.post('/api/ceramics/price', { data: { productType: type, sizeValue: '9', sizeUnit: 'inch' } })
    expect(response.status(), await response.text()).toBe(200)
    const price = (await response.json()).price
    expect(price.price).toBe('5')
    expect(price.total).toBe(3)
    expect(price.options).toEqual(expect.arrayContaining([expect.objectContaining({ price: '5', count: 2 }), expect.objectContaining({ price: '7', count: 1 })]))

    const unknown = await api.post('/api/ceramics/price', { data: { productType: `${E2E}-нет-такого-${uid()}`, sizeValue: '', sizeUnit: '' } })
    expect(unknown.status()).toBe(400)
    expect((await unknown.json()).error.field).toBe('productType')

    const badSize = await api.post('/api/ceramics/price', { data: { productType: type, sizeValue: '-1', sizeUnit: 'inch' } })
    expect(badSize.status()).toBe(400)
    expect((await badSize.json()).error.field).toBe('sizeValue')
  })

  test('загрузка базы администратором: создание, повтор без перезаписи — пропуск, с перезаписью — обновление', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const [a, b] = [mark(), mark()]
    const items = [{ mark: a, name: 'Блюдо керамическое 12 дюймов', price: '10' }, { mark: b, name: 'Блюдо керамическое 14 дюймов', price: '12' }]
    const first = await admin.post('/api/ceramics/products/import', { data: { items, overwrite: false } })
    expect(first.status(), await first.text()).toBe(200)
    expect(await first.json()).toEqual({ created: 2, updated: 0, skipped: 0 })

    const again = await (await admin.post('/api/ceramics/products/import', { data: { items, overwrite: false } })).json()
    expect(again).toEqual({ created: 0, updated: 0, skipped: 2 })

    const overwrite = await (await admin.post('/api/ceramics/products/import', { data: { items: [{ ...items[0], price: '11' }], overwrite: true } })).json()
    expect(overwrite).toEqual({ created: 0, updated: 1, skipped: 0 })
    const found = await (await admin.get(`/api/ceramics/products?q=${a}`)).json()
    expect(found.items[0].price).toBe('11')
  })

  const badImports: Array<[string, object, string]> = [
    ['пустой список', { items: [] }, 'Нет строк'],
    ['повтор марки в файле', { items: [{ mark: 'E2E-DUP', name: 'Чашка', price: '1' }, { mark: 'e2e-dup', name: 'Чашка', price: '2' }] }, 'повторяется'],
    ['плохая цена в строке 2', { items: [{ mark: 'E2E-OK1', name: 'Чашка', price: '1' }, { mark: 'E2E-OK2', name: 'Чашка', price: 'x' }] }, 'Строка 2'],
  ]
  for (const [name, data, message] of badImports) {
    test(`загрузка базы: ${name} → 400, ничего не записано`, async ({ apiAs }) => {
      const admin = await apiAs('admin')
      const response = await admin.post('/api/ceramics/products/import', { data })
      expect(response.status(), await response.text()).toBe(400)
      expect((await response.json()).error.message).toContain(message)
      const left = await (await admin.get('/api/ceramics/products?q=E2E-OK1')).json()
      expect(left.total).toBe(0)
    })
  }

  test('загрузка базы инвойсером → 403', async ({ apiAs }) => {
    const response = await (await apiAs('invoicer')).post('/api/ceramics/products/import', { data: { items: [{ mark: mark(), name: 'Чашка', price: '1' }], overwrite: false } })
    expect(response.status()).toBe(403)
  })
})
