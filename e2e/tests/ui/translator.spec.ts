import { readFileSync } from 'node:fs'
import path from 'node:path'
import JSZip from 'jszip'
import type { APIRequestContext, Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { E2E, uid } from '../../support/users'

// «Переводчик» (/translator, /translator/:id). Загрузка только разбирает файл; перевод (DeepSeek)
// подменяется: POST translate и опрос документа отвечают моками.

const testdata = path.join(import.meta.dirname, '..', '..', '..', 'backend', 'internal', 'service', 'translator', 'testdata')
const DOCX = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'

// Документы этого теста; после теста администратор удаляет их.
const created: number[] = []
test.afterEach(async ({ apiAs }) => {
  const admin = await apiAs('admin')
  for (const id of created.splice(0)) await admin.delete(`/api/translator/documents/${id}`)
})

const docId = (page: Page) => Number(/\/translator\/(\d+)$/.exec(page.url())?.[1])

async function uploadViaUI(page: Page, name: string, fixture = 'simple.docx') {
  await page.goto('/translator')
  await page.getByRole('button', { name: 'Загрузить документ' }).click()
  const drawer = page.getByRole('dialog', { name: 'Загрузить документ' })
  await drawer.locator('input[type=file]').setInputFiles({ name: `${name}.docx`, mimeType: DOCX, buffer: readFileSync(path.join(testdata, fixture)) })
  await expect(drawer).toContainText(`${name}.docx`)
  await drawer.getByRole('button', { name: 'Загрузить', exact: true }).click()
  await expect(page).toHaveURL(/\/translator\/\d+$/)
  created.push(docId(page))
  await expect(page.getByRole('heading', { level: 1 })).toContainText(name)
}

async function uploadViaAPI(api: APIRequestContext, name: string) {
  const response = await api.post('/api/translator/documents', { multipart: { file: { name: `${name}.docx`, mimeType: DOCX, buffer: readFileSync(path.join(testdata, 'simple.docx')) }, sourceLang: 'en', targetLang: 'kk' } })
  expect(response.status()).toBe(201)
  const id = (await response.json()).id as number
  created.push(id)
  return id
}

const toast = (page: Page) => page.locator('.tr-toast')

test.describe('переводчик: работа с документом', () => {
  test.use(as('translator'))

  test('загрузка документа: открывается документ «не переведён», сегменты оригинала', async ({ page }) => {
    const name = `${E2E} ${uid()} договор`
    await uploadViaUI(page, name)
    await expect(page.locator('h1 .tr-status')).toHaveText('Не переведён')
    await expect(page.getByText('Китайский → Русский · загрузил E2E Переводчик')).toBeVisible()
    await expect(page.locator('.tr-doc-progress')).toContainText(/Переведено 0 из \d+/)
    await expect(page.getByRole('button', { name: 'Скачать перевод' })).toBeDisabled()
    await expect(page.getByRole('button', { name: 'Перевести', exact: true })).toBeEnabled()
    expect(await page.locator('.tr-seg:not(.tr-seg-head)').count()).toBeGreaterThan(0)

    // В списке документов — с прогрессом и статусом; поиск по названию.
    await page.getByRole('link', { name: '← Все документы' }).click()
    await page.getByLabel('Поиск документов').fill(name)
    const row = page.locator('tbody tr', { hasText: name })
    await expect(row).toHaveCount(1)
    await expect(row).toContainText('Китайский → Русский')
    await expect(row).toContainText('Не переведён')
    await expect(row).toContainText(/0 из \d+/)
    await page.getByLabel('Поиск документов').fill(`${name} zzqqxx`)
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
  })

  test('правка сегмента: автосохранение, подтверждение, история, возврат версии', async ({ page }) => {
    await uploadViaUI(page, `${E2E} ${uid()} правка`)
    await page.getByRole('button', { name: /Нет перевода — щёлкните, чтобы ввести/ }).first().click()
    const editor = page.getByLabel('Перевод сегмента')
    await editor.click()
    await page.keyboard.type('E2E первый вариант')
    await expect(toast(page)).toHaveText('Сохранено')
    await expect(page.locator('.tr-count.st-edited b')).toHaveText('1')

    // Тост от первого сохранения ещё может висеть — ждём сам запрос второй правки.
    await editor.click()
    await page.keyboard.press('Control+A')
    const saved = page.waitForResponse((response) => response.url().includes('/api/translator/segments/') && response.request().method() === 'PATCH'
      && (response.request().postData() ?? '').includes('второй'))
    await page.keyboard.type('E2E второй вариант')
    expect((await saved).status()).toBe(200)

    await page.getByRole('button', { name: 'История' }).click()
    const history = page.getByRole('dialog', { name: 'История перевода' })
    await expect(history.locator('li')).toHaveCount(2)
    await expect(history.locator('li').first()).toContainText('E2E второй вариант')
    await expect(history.locator('li').first()).toContainText('Текущая')
    await expect(history.locator('li').nth(1)).toContainText('E2E Переводчик')
    await history.locator('li').nth(1).getByRole('button', { name: 'Вернуть' }).click()
    await expect(toast(page)).toHaveText('Версия возвращена')
    await expect(history).toHaveCount(0)

    await page.getByRole('button', { name: 'Подтвердить' }).click()
    await expect(toast(page)).toHaveText('Подтверждено')
    await expect(page.locator('.tr-count.st-confirmed b')).toHaveText('1')
    await expect(page.locator('.tr-seg.st-confirmed')).toContainText('E2E первый вариант')

    // Фильтр «Неподтверждённые» скрывает подтверждённый сегмент.
    await page.keyboard.press('Escape')
    await page.getByRole('radio', { name: 'Неподтверждённые' }).click()
    await expect(page.locator('.tr-seg.st-confirmed')).toHaveCount(0)
    await page.getByRole('radio', { name: 'Все' }).click()
    await expect(page.locator('.tr-seg.st-confirmed')).toHaveCount(1)
  })

  test('выгрузка: предупреждение о неподтверждённых, файлы .docx с переводом', async ({ page }) => {
    const name = `${E2E} ${uid()} выгрузка`
    await uploadViaUI(page, name)
    await page.getByRole('button', { name: /Нет перевода — щёлкните, чтобы ввести/ }).first().click()
    await page.getByLabel('Перевод сегмента').click()
    await page.keyboard.type('E2E текст для выгрузки')
    await expect(toast(page)).toHaveText('Сохранено')
    await page.keyboard.press('Escape')

    // Отказ в подтверждении — скачивания нет.
    page.once('dialog', (dialog) => { expect(dialog.message()).toMatch(/Не подтверждено \d+ сегмент/); void dialog.dismiss() })
    let downloaded = false
    page.once('download', () => { downloaded = true })
    await page.getByRole('button', { name: 'Скачать перевод' }).click()
    await page.waitForTimeout(500)
    expect(downloaded).toBe(false)

    for (const [button, suffix] of [['Скачать перевод', 'ru'], ['Скачать двуязычный', 'zh-ru']] as const) {
      page.once('dialog', (dialog) => void dialog.accept())
      const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: button }).click()])
      expect(download.suggestedFilename()).toBe(`${name}_${suffix}.docx`)
      const zip = await JSZip.loadAsync(readFileSync(await download.path()))
      expect(await zip.file('word/document.xml')!.async('string')).toContain('E2E текст для выгрузки')
    }
  })

  test('перевод документа (мок ИИ): сегменты заполняются, статус «Переведён»', async ({ page }) => {
    await uploadViaUI(page, `${E2E} ${uid()} перевод`)
    const id = docId(page)
    let translateCalls = 0
    await page.route(`**/api/translator/documents/${id}/translate`, (route) => { translateCalls++; return route.fulfill({ status: 204 }) })
    // Опрос во время перевода: настоящий документ целиком + машинный перевод каждого сегмента.
    await page.route(new RegExp(`/api/translator/documents/${id}\\?since=`), async (route) => {
      const real = await (await route.fetch({ url: new URL(`/api/translator/documents/${id}`, route.request().url()).toString() })).json()
      real.document.status = 'translated'
      real.segments = real.segments.map((segment: { id: number }, index: number) => ({ ...segment, mtText: `E2E машинный ${index + 1}` }))
      await route.fulfill({ json: real })
    })
    await page.getByRole('button', { name: 'Перевести', exact: true }).click()
    await expect(page.locator('h1 .tr-status')).toHaveText('Переведён')
    expect(translateCalls).toBe(1)
    const total = await page.locator('.tr-seg:not(.tr-seg-head)').count()
    await expect(page.locator('.tr-doc-progress')).toContainText(`Переведено ${total} из ${total}`)
    await expect(page.getByText('E2E машинный 1', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Перевести', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Скачать перевод' })).toBeEnabled()
  })

  test('перевод недоступен (ИИ выключен): ошибка на странице', async ({ page, aiBlocked }) => {
    await uploadViaUI(page, `${E2E} ${uid()} ошибка`)
    await page.getByRole('button', { name: 'Перевести', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('ИИ отключён в E2E')
    expect(aiBlocked.some((url) => url.endsWith('/translate'))).toBe(true)
    await expect(page.locator('h1 .tr-status')).toHaveText('Не переведён')
  })

  test('«Перевести заново» без ИИ: ошибка во всплывающем сообщении', async ({ page }) => {
    await uploadViaUI(page, `${E2E} ${uid()} заново`)
    await page.getByRole('button', { name: /Нет перевода — щёлкните, чтобы ввести/ }).first().click()
    await page.getByRole('button', { name: 'Перевести заново' }).click()
    await expect(toast(page)).toHaveText('ИИ отключён в E2E')
  })

  test('удаление своего документа: подтверждение, возврат к списку', async ({ page }) => {
    const name = `${E2E} ${uid()} удалить`
    await uploadViaUI(page, name)
    page.once('dialog', (dialog) => void dialog.dismiss())
    await page.getByRole('button', { name: 'Удалить' }).click()
    await expect(page).toHaveURL(/\/translator\/\d+$/)
    page.once('dialog', (dialog) => { expect(dialog.message()).toContain(name); void dialog.accept() })
    await page.getByRole('button', { name: 'Удалить' }).click()
    await expect(page).toHaveURL(/\/translator$/)
    await page.getByLabel('Поиск документов').fill(name)
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
  })

  test('чужой документ: можно открыть, кнопки «Удалить» нет', async ({ page, apiAs }) => {
    const name = `${E2E} ${uid()} чужой`
    const id = await uploadViaAPI(await apiAs('admin'), name)
    await page.goto(`/translator/${id}`)
    await expect(page.getByRole('heading', { level: 1 })).toContainText(name)
    // Имя администратора bootstrap-admin пустое — проверяем только языки и «загрузил».
    await expect(page.getByText(/Английский → Казахский · загрузил/)).toBeVisible()
    await expect(page.getByRole('button', { name: 'Удалить' })).toHaveCount(0)
  })

  test('несуществующий документ — сообщение об ошибке', async ({ page }) => {
    await page.goto('/translator/999999999')
    await expect(page.getByRole('alert')).toContainText('Документ не найден')
  })

  test('загрузка: неподдерживаемый файл и выбор языков', async ({ page }) => {
    await page.goto('/translator')
    await page.getByRole('button', { name: 'Загрузить документ' }).click()
    const drawer = page.getByRole('dialog', { name: 'Загрузить документ' })
    const input = drawer.locator('input[type=file]')
    await input.setInputFiles({ name: 'e2e.txt', mimeType: 'text/plain', buffer: Buffer.from('x') })
    await expect(drawer.getByRole('alert')).toHaveText('Выберите документ Word (.docx) или книгу Excel (.xlsx)')
    await expect(drawer.getByRole('button', { name: 'Загрузить', exact: true })).toBeDisabled()
    await input.setInputFiles({ name: 'e2e.doc', mimeType: 'application/msword', buffer: Buffer.from('x') })
    await expect(drawer.getByRole('alert')).toContainText('Файлы .doc не поддерживаются')

    // Тот же язык с другой стороны — языки меняются местами.
    await drawer.getByLabel('Язык оригинала').selectOption('ru')
    await expect(drawer.getByLabel('Перевести на')).toHaveValue('zh')

    // Битый .docx проходит проверку в браузере, но сервер отвечает ошибкой.
    await input.setInputFiles({ name: 'битый.docx', mimeType: DOCX, buffer: Buffer.from('not a zip') })
    await drawer.getByRole('button', { name: 'Загрузить', exact: true }).click()
    await expect(drawer.getByRole('alert')).toContainText('Не удалось прочитать файл')

    await drawer.getByRole('button', { name: 'Отмена' }).click()
    await expect(drawer).toHaveCount(0)
  })
})

test.describe('переводчик: доступ по ролям', () => {
  test.describe('переводчик', () => {
    test.use(as('translator'))

    test('меню: только Главная, Номенклатура, Утильсбор, Переводчик', async ({ page }) => {
      await page.goto('/translator')
      const nav = page.getByRole('navigation', { name: 'Разделы' })
      await expect(nav.getByRole('link', { name: 'Переводчик' })).toBeVisible()
      await expect(nav.getByRole('link', { name: 'Сметы' })).toHaveCount(0)
      await nav.getByText('Справочники').click()
      await expect(nav.getByRole('link', { name: 'Номенклатура' })).toBeVisible()
      await expect(nav.getByRole('link', { name: 'Утильсбор' })).toBeVisible()
      for (const hidden of ['Справочник товаров', 'База решений', 'Керамика']) await expect(nav.getByRole('link', { name: hidden })).toHaveCount(0)
    })

    for (const url of ['/ceramics', '/ceramics/base', '/estimates/goods', '/directory/products', '/decisions']) {
      test(`закрытый раздел ${url} → главная`, async ({ page }) => {
        await page.goto(url)
        await expect(page).toHaveURL(/\/$/)
      })
    }
  })

  test.describe('инвойсер', () => {
    test.use(as('invoicer'))
    test('раздел «Переводчик» закрыт: в меню нет, по адресу — на главную', async ({ page }) => {
      await page.goto('/translator')
      await expect(page).toHaveURL(/\/$/)
      await expect(page.getByRole('navigation', { name: 'Разделы' }).getByRole('link', { name: 'Переводчик' })).toHaveCount(0)
    })
  })

  test.describe('администратор', () => {
    test.use(as('admin'))
    test('видит раздел и удаляет чужой документ', async ({ page, apiAs }) => {
      const name = `${E2E} ${uid()} админ`
      const response = await (await apiAs('translator')).post('/api/translator/documents', { multipart: { file: { name: `${name}.docx`, mimeType: DOCX, buffer: readFileSync(path.join(testdata, 'simple.docx')) }, sourceLang: 'zh', targetLang: 'ru' } })
      const id = (await response.json()).id as number
      created.push(id)
      await page.goto(`/translator/${id}`)
      page.once('dialog', (dialog) => void dialog.accept())
      await page.getByRole('button', { name: 'Удалить' }).click()
      await expect(page).toHaveURL(/\/translator$/)
    })
  })
})
