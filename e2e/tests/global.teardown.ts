import { request, test as teardown } from '@playwright/test'
import { existsSync } from 'node:fs'
import { authFile, E2E, MAX_WORKERS, workerInvoicer } from '../support/users'

// Учётки E2E — только для тестов, поэтому их сметы удаляются целиком. Из общих таблиц
// удаляются только записи с префиксом E2E.
teardown('уборка данных E2E', async ({ baseURL }) => {
  teardown.skip(!!process.env.E2E_SKIP_TEARDOWN, 'E2E_SKIP_TEARDOWN: данные оставлены')
  const owners = ['invoicer', 'invoicer2', 'admin', ...Array.from({ length: MAX_WORKERS }, (_, index) => workerInvoicer(index).key)] as const
  for (const key of owners) {
    if (!existsSync(authFile(key))) continue
    const api = await request.newContext({ baseURL, storageState: authFile(key) })
    const response = await api.get('/api/goods-estimates')
    if (response.ok()) {
      for (const estimate of (await response.json()).estimates ?? []) await api.delete(`/api/goods-estimates/${estimate.id}`)
    }
    await api.dispose()
  }
  const admin = await request.newContext({ baseURL, storageState: authFile('admin') })
  // Тесты пишут решения с несуществующими кодами 99xxxxxxxx. Искать только цифрами:
  // запрос с буквами сервер отправляет в платные эмбеддинги Gemini.
  const decisions = await admin.get('/api/code-decisions?q=99&limit=500')
  if (decisions.ok()) {
    for (const item of (await decisions.json()).items ?? []) {
      if (String(item.code ?? '').startsWith('99') && JSON.stringify(item).includes(E2E)) await admin.delete(`/api/code-decisions/${item.id}`)
    }
  }
  await admin.dispose()
})
