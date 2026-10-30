import { readFileSync } from 'node:fs'
import path from 'node:path'
import JSZip from 'jszip'
import type { APIRequestContext } from '@playwright/test'
import { test, expect } from '../../support/fixtures'
import { E2E, uid } from '../../support/users'
import { createTempUser, deleteUser } from '../../support/auth-helpers'
import { loginAs } from '../../support/employees-helpers'

// «Переводчик»: загрузка .docx/.xlsx разбирает файл на сегменты без перевода (status parsed —
// Upload в service/translator не запускает ИИ). Перевод (translate/retranslate) не вызывается.
// Документы общие: видят и правят все переводчики, удаляет загрузивший или администратор.

const testdata = path.join(import.meta.dirname, '..', '..', '..', 'backend', 'internal', 'service', 'translator', 'testdata')
const DOCX = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
const file = (name: string, as = `${E2E} ${uid()} ${name}`) => ({ name: as, mimeType: DOCX, buffer: readFileSync(path.join(testdata, name)) })

type Segment = { id: number; kind: string; sourceText: string; mtText: string; targetText: string; status: string; locked: boolean; qa: unknown[] }

// Документы этого теста — администратор удаляет их после теста.
const created: number[] = []

async function uploaded(api: APIRequestContext, name = 'simple.docx') {
  const response = await api.post('/api/translator/documents', { multipart: { file: file(name), sourceLang: 'zh', targetLang: 'ru' } })
  expect(response.status(), await response.text()).toBe(201)
  const id = (await response.json()).id as number
  created.push(id)
  return id
}

test.describe('переводчик API', () => {
  test.afterEach(async ({ apiAs }) => {
    const admin = await apiAs('admin')
    for (const id of created.splice(0)) await admin.delete(`/api/translator/documents/${id}`)
  })

  test('загрузка .docx: документ «не переведён», сегменты с оригиналом, без перевода', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const name = `${E2E} ${uid()} простой`
    const response = await api.post('/api/translator/documents', { multipart: { file: file('simple.docx', `${name}.docx`), sourceLang: 'zh', targetLang: 'ru' } })
    expect(response.status(), await response.text()).toBe(201)
    const id = (await response.json()).id as number
    created.push(id)

    const view = await (await api.get(`/api/translator/documents/${id}`)).json()
    expect(view.document).toMatchObject({ id, name, format: 'docx', sourceLang: 'zh', targetLang: 'ru', status: 'parsed', createdByName: 'E2E Переводчик', confirmedCount: 0 })
    expect(view.document.segmentCount).toBeGreaterThan(0)
    expect(view.segments).toHaveLength(view.document.segmentCount)
    for (const segment of view.segments as Segment[]) {
      expect(segment.sourceText).toBeTruthy()
      expect(segment).toMatchObject({ mtText: '', targetText: '', status: 'machine', locked: false })
    }
    expect(Date.parse(view.now)).not.toBeNaN()

    // Список и поиск по названию.
    const list = await (await api.get(`/api/translator/documents?q=${encodeURIComponent(name)}`)).json()
    expect(list.items.map((item: { id: number }) => item.id)).toEqual([id])
    const none = await (await api.get(`/api/translator/documents?q=${encodeURIComponent(`${name} zzqqxxнет`)}`)).json()
    expect(none.items).toEqual([])
  })

  test('опрос since: без изменений — пустой список сегментов', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const id = await uploaded(api)
    const future = new Date(Date.now() + 60_000).toISOString()
    const view = await (await api.get(`/api/translator/documents/${id}?since=${encodeURIComponent(future)}`)).json()
    expect(view.segments).toEqual([])
    expect(view.document.status).toBe('parsed')
  })

  test('таблица в .docx: сегменты ячеек; книга .xlsx тоже разбирается', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const docx = await (await api.get(`/api/translator/documents/${await uploaded(api, 'table.docx')}`)).json()
    expect(docx.segments.some((segment: Segment) => segment.kind === 'table_cell')).toBe(true)

    const response = await api.post('/api/translator/documents', { multipart: {
      file: { name: `${E2E} ${uid()} c106.xlsx`, mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: readFileSync(path.join(testdata, 'c106.xlsx')) },
      sourceLang: 'zh', targetLang: 'ru' } })
    expect(response.status(), await response.text()).toBe(201)
    const id = (await response.json()).id
    created.push(id)
    const xlsx = await (await api.get(`/api/translator/documents/${id}`)).json()
    expect(xlsx.document.format).toBe('xlsx')
    expect(xlsx.document.segmentCount).toBeGreaterThan(0)
  })

  test('правка сегмента: «исправлен», история, подтверждение, счётчик подтверждённых', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const id = await uploaded(api)
    const [segment] = (await (await api.get(`/api/translator/documents/${id}`)).json()).segments as Segment[]

    const edited = await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: 'E2E перевод один' } })
    expect(edited.status(), await edited.text()).toBe(200)
    expect(await edited.json()).toMatchObject({ id: segment.id, targetText: 'E2E перевод один', status: 'edited', locked: true })

    await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: 'E2E перевод два' } })
    const history = await (await api.get(`/api/translator/segments/${segment.id}/history`)).json()
    expect(history.items.map((item: { targetText: string }) => item.targetText)).toEqual(['E2E перевод два', 'E2E перевод один'])
    expect(history.items[0].changedByName).toBe('E2E Переводчик')

    // Тот же текст без статуса — ничего не сохраняется, история не растёт.
    await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: 'E2E перевод два' } })
    expect((await (await api.get(`/api/translator/segments/${segment.id}/history`)).json()).items).toHaveLength(2)

    const confirmed = await (await api.patch(`/api/translator/segments/${segment.id}`, { data: { status: 'confirmed' } })).json()
    expect(confirmed).toMatchObject({ status: 'confirmed', targetText: 'E2E перевод два' })
    const view = await (await api.get(`/api/translator/documents/${id}`)).json()
    expect(view.document.confirmedCount).toBe(1)

    // Снять подтверждение.
    expect((await (await api.patch(`/api/translator/segments/${segment.id}`, { data: { status: 'edited' } })).json()).status).toBe('edited')
  })

  test('правка сегмента: пустой перевод, неизвестный статус, нет сегмента', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const id = await uploaded(api)
    const [segment] = (await (await api.get(`/api/translator/documents/${id}`)).json()).segments as Segment[]

    const empty = await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: '' } })
    expect(empty.status()).toBe(400)
    expect((await empty.json()).error.field).toBe('targetText')
    // Подтвердить сегмент без перевода нельзя: перевод пустой.
    expect((await api.patch(`/api/translator/segments/${segment.id}`, { data: { status: 'confirmed' } })).status()).toBe(400)

    const status = await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: 'x', status: 'approved' } })
    expect(status.status()).toBe(400)
    expect((await status.json()).error.field).toBe('status')

    const long = await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: 'я'.repeat(20001) } })
    expect(long.status()).toBe(400)

    expect((await api.patch('/api/translator/segments/999999999', { data: { targetText: 'x' } })).status()).toBe(404)
    expect((await api.get('/api/translator/segments/999999999/history')).status()).toBe(404)
    expect((await api.get('/api/translator/segments/abc/history')).status()).toBe(404)
  })

  test('правка сегмента: перевод из одних пробелов → 400', async ({ apiAs }) => {
    // Страница такую правку не шлёт (!text.trim()), но сервер обязан отклонять её сам.
    const api = await apiAs('translator')
    const id = await uploaded(api)
    const [segment] = (await (await api.get(`/api/translator/documents/${id}`)).json()).segments as Segment[]
    const blank = await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: '   ' } })
    expect(blank.status()).toBe(400)
  })

  test('выгрузка .docx: перевод и двуязычный — настоящий Word с исправленным текстом', async ({ apiAs }) => {
    const api = await apiAs('translator')
    const id = await uploaded(api)
    const [segment] = (await (await api.get(`/api/translator/documents/${id}`)).json()).segments as Segment[]
    await api.patch(`/api/translator/segments/${segment.id}`, { data: { targetText: 'E2E выгрузка' } })

    for (const mode of ['target', 'bilingual']) {
      const response = await api.get(`/api/translator/documents/${id}/export?mode=${mode}`)
      expect(response.status(), mode).toBe(200)
      expect(response.headers()['content-type']).toBe(DOCX)
      expect(response.headers()['content-disposition']).toContain('attachment')
      const zip = await JSZip.loadAsync(await response.body())
      const xml = await zip.file('word/document.xml')?.async('string')
      expect(xml, `${mode}: word/document.xml`).toBeTruthy()
      expect(xml).toContain('E2E выгрузка')
      if (mode === 'bilingual') expect(xml).toContain(segment.sourceText.replace(/<[^>]+>/g, '').slice(0, 5))
    }
  })

  const badUploads: Array<[string, () => object, string]> = [
    ['текстовый файл', () => ({ file: { name: 'e2e.txt', mimeType: 'text/plain', buffer: Buffer.from('hello') }, sourceLang: 'zh', targetLang: 'ru' }), 'Word'],
    ['старый .doc', () => ({ file: { name: 'e2e.doc', mimeType: 'application/msword', buffer: Buffer.from('hello') }, sourceLang: 'zh', targetLang: 'ru' }), '.doc'],
    ['старый .xls', () => ({ file: { name: 'e2e.xls', mimeType: 'application/vnd.ms-excel', buffer: Buffer.from('hello') }, sourceLang: 'zh', targetLang: 'ru' }), '.xls'],
    ['битый .docx', () => ({ file: { name: 'e2e.docx', mimeType: DOCX, buffer: Buffer.from('not a zip') }, sourceLang: 'zh', targetLang: 'ru' }), 'Не удалось прочитать'],
    ['одинаковые языки', () => ({ file: file('simple.docx'), sourceLang: 'ru', targetLang: 'ru' }), 'отличаться'],
    ['неизвестный язык оригинала', () => ({ file: file('simple.docx'), sourceLang: 'de', targetLang: 'ru' }), 'оригинала'],
    ['без языка перевода', () => ({ file: file('simple.docx'), sourceLang: 'zh' }), 'перевода'],
    ['без файла', () => ({ sourceLang: 'zh', targetLang: 'ru' }), 'Word'],
  ]
  for (const [name, data, message] of badUploads) {
    test(`загрузка: ${name} → 400`, async ({ apiAs }) => {
      const response = await (await apiAs('translator')).post('/api/translator/documents', { multipart: data() as never })
      expect(response.status(), await response.text()).toBe(400)
      expect((await response.json()).error.message).toContain(message)
    })
  }

  test('документы общие: другой переводчик видит и правит, но удалить не может; удаляет загрузивший', async ({ apiAs, baseURL }) => {
    const owner = await apiAs('translator')
    const id = await uploaded(owner)
    const admin = await apiAs('admin')
    const temp = await createTempUser(admin, 'translator')
    try {
      const { context: other, status } = await loginAs(baseURL, temp.email, temp.password)
      expect(status).toBe(200)
      const list = await (await other.get('/api/translator/documents')).json()
      expect(list.items.map((item: { id: number }) => item.id)).toContain(id)
      const view = await (await other.get(`/api/translator/documents/${id}`)).json()
      const edit = await other.patch(`/api/translator/segments/${view.segments[0].id}`, { data: { targetText: 'E2E чужая правка' } })
      expect(edit.status()).toBe(200)
      const history = await (await owner.get(`/api/translator/segments/${view.segments[0].id}/history`)).json()
      expect(history.items[0].changedByName).toBe(temp.name)

      const denied = await other.delete(`/api/translator/documents/${id}`)
      expect(denied.status()).toBe(403)
      expect((await denied.json()).error.code).toBe('forbidden')
      await other.dispose()

      expect((await owner.delete(`/api/translator/documents/${id}`)).status()).toBe(204)
      expect((await owner.get(`/api/translator/documents/${id}`)).status()).toBe(404)
      expect((await owner.delete(`/api/translator/documents/${id}`)).status()).toBe(404)
    } finally {
      await deleteUser(admin, temp.id)
    }
  })

  test('администратор удаляет чужой документ', async ({ apiAs }) => {
    const id = await uploaded(await apiAs('translator'))
    const admin = await apiAs('admin')
    expect((await admin.delete(`/api/translator/documents/${id}`)).status()).toBe(204)
    expect((await admin.get(`/api/translator/documents/${id}`)).status()).toBe(404)
  })

  test('несуществующий документ: 404 на чтение, выгрузку; некорректный id — 404', async ({ apiAs }) => {
    const api = await apiAs('translator')
    expect((await api.get('/api/translator/documents/999999999')).status()).toBe(404)
    expect((await api.get('/api/translator/documents/999999999/export')).status()).toBe(404)
    expect((await api.get('/api/translator/documents/abc')).status()).toBe(404)
    expect((await api.get('/api/translator/documents/-1')).status()).toBe(404)
  })
})

