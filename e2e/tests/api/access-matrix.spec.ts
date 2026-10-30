import { test, expect } from '../../support/fixtures'

// Матрица прав: каждый маршрут API × каждая роль. Проверяет, что права проверяет сервер,
// а не только скрытые кнопки. Разрешённый вызов шлёт заведомо негодные данные (битый JSON,
// несуществующий id) — обработчик отвечает 400/404 и ничего не меняет.
// probe: false — разрешённый вызов не делается (платный ИИ, внешние сайты).

type Access =
  | 'public'          // без входа
  | 'user'            // любой вошедший
  | 'admin'           // только администратор
  | 'notTranslator'   // администратор и инвойсер
  | 'translatorDesk'  // администратор и переводчик

type Route = { method: 'GET' | 'POST' | 'PATCH' | 'DELETE'; path: string; access: Access; probe?: boolean }

const X = 999999999

const routes: Route[] = [
  { method: 'GET', path: '/api/health', access: 'public' },
  { method: 'GET', path: '/api/rates/today', access: 'public' },
  { method: 'GET', path: '/api/auth/me', access: 'user' },

  { method: 'GET', path: '/api/employees', access: 'admin' },
  { method: 'POST', path: '/api/employees', access: 'admin' },
  { method: 'PATCH', path: `/api/employees/${X}`, access: 'admin' },
  { method: 'DELETE', path: `/api/employees/${X}`, access: 'admin' },

  { method: 'POST', path: '/api/estimates/price-weights', access: 'notTranslator', probe: false },
  { method: 'POST', path: '/api/estimates/price-history', access: 'notTranslator' },

  { method: 'GET', path: '/api/tnved-nomenclature?q=8471', access: 'user' },
  { method: 'POST', path: '/api/tnved-nomenclature/lookup', access: 'user' },
  { method: 'POST', path: '/api/tnved-nomenclature/import', access: 'admin' },

  { method: 'GET', path: '/api/tnved-antidumping', access: 'user' },
  { method: 'POST', path: '/api/tnved-antidumping/refresh', access: 'admin', probe: false },

  { method: 'GET', path: '/api/risk-indicators', access: 'user' },
  { method: 'POST', path: '/api/risk-indicators/import', access: 'admin' },
  { method: 'DELETE', path: `/api/risk-indicators/${X}`, access: 'admin' },

  { method: 'GET', path: '/api/trademarks', access: 'user' },
  { method: 'GET', path: '/api/trademarks/logo', access: 'user' },
  { method: 'POST', path: '/api/trademarks/check', access: 'user' },
  { method: 'POST', path: '/api/trademarks/import', access: 'admin' },
  { method: 'PATCH', path: '/api/trademarks/logo-name', access: 'admin' },

  { method: 'GET', path: '/api/util-fee', access: 'user' },
  { method: 'POST', path: '/api/util-fee', access: 'user' },
  { method: 'POST', path: '/api/util-fee/rows', access: 'user' },

  { method: 'POST', path: '/api/tnved-kz-rates/excel', access: 'admin' },
  { method: 'POST', path: '/api/tnved-kz-rates/import-pdf', access: 'admin' },
  { method: 'POST', path: '/api/tnved-kz-rates/import', access: 'admin' },

  { method: 'GET', path: '/api/product-directory', access: 'notTranslator' },
  { method: 'GET', path: '/api/product-directory/export', access: 'admin' },
  { method: 'POST', path: '/api/product-directory/import', access: 'admin' },
  { method: 'POST', path: '/api/product-directory', access: 'admin' },
  { method: 'PATCH', path: `/api/product-directory/${X}`, access: 'admin' },
  { method: 'POST', path: `/api/product-directory/${X}/archive`, access: 'admin' },
  { method: 'POST', path: `/api/product-directory/${X}/restore`, access: 'admin' },
  { method: 'POST', path: `/api/product-directory/${X}/copy`, access: 'admin' },

  { method: 'POST', path: '/api/goods/parse-xlsx', access: 'notTranslator' },
  { method: 'POST', path: '/api/goods/calculate', access: 'notTranslator' },
  { method: 'POST', path: '/api/goods/export', access: 'notTranslator' },

  { method: 'GET', path: '/api/goods-estimates', access: 'notTranslator' },
  { method: 'POST', path: '/api/goods-estimates', access: 'notTranslator' },
  { method: 'GET', path: `/api/goods-estimates/${X}`, access: 'notTranslator' },
  { method: 'PATCH', path: `/api/goods-estimates/${X}`, access: 'notTranslator' },
  { method: 'DELETE', path: `/api/goods-estimates/${X}`, access: 'notTranslator' },
  { method: 'PATCH', path: `/api/goods-estimates/${X}/draft`, access: 'notTranslator' },

  { method: 'GET', path: '/api/rates/nbk', access: 'user', probe: false },

  { method: 'GET', path: '/api/code-decisions', access: 'notTranslator' },
  { method: 'GET', path: '/api/code-decisions/stats', access: 'notTranslator' },
  { method: 'GET', path: `/api/code-decisions/estimate/${X}`, access: 'notTranslator' },
  { method: 'GET', path: `/api/code-decisions/${X}`, access: 'notTranslator' },
  { method: 'POST', path: '/api/code-decisions', access: 'notTranslator' },
  { method: 'POST', path: '/api/code-decisions/import', access: 'admin' },
  { method: 'PATCH', path: `/api/code-decisions/${X}`, access: 'admin' },
  { method: 'POST', path: `/api/code-decisions/${X}/verify`, access: 'admin' },
  { method: 'POST', path: `/api/code-decisions/${X}/reject`, access: 'admin' },
  { method: 'DELETE', path: `/api/code-decisions/${X}`, access: 'admin' },

  { method: 'GET', path: '/api/tnved-agent/status', access: 'user' },
  { method: 'GET', path: '/api/tnved-agent/usage', access: 'admin' },
  { method: 'GET', path: '/api/tnved-agent/sessions', access: 'notTranslator' },
  { method: 'POST', path: '/api/tnved-agent/sessions', access: 'notTranslator', probe: false },
  { method: 'GET', path: `/api/tnved-agent/sessions/${X}`, access: 'user' },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/begin`, access: 'notTranslator', probe: false },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/messages`, access: 'user', probe: false },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/rerun`, access: 'notTranslator', probe: false },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/documents`, access: 'user', probe: false },
  { method: 'GET', path: '/api/tnved-agent/documents/8471300000', access: 'user' },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/cancel`, access: 'notTranslator' },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/apply`, access: 'notTranslator', probe: false },
  { method: 'POST', path: `/api/tnved-agent/sessions/${X}/name`, access: 'user', probe: false },
  { method: 'GET', path: '/api/tnved-agent/reference', access: 'user' },
  { method: 'POST', path: '/api/tnved-agent/reference', access: 'user', probe: false },
  { method: 'POST', path: '/api/tnved-agent/reference/trademarks', access: 'user', probe: false },
  { method: 'DELETE', path: `/api/tnved-agent/reference/${X}`, access: 'user' },

  { method: 'GET', path: '/api/ceramics/status', access: 'notTranslator' },
  { method: 'GET', path: '/api/ceramics/types', access: 'notTranslator' },
  { method: 'GET', path: '/api/ceramics/products', access: 'notTranslator' },
  { method: 'POST', path: '/api/ceramics/products', access: 'notTranslator' },
  { method: 'PATCH', path: `/api/ceramics/products/${X}`, access: 'notTranslator' },
  { method: 'DELETE', path: `/api/ceramics/products/${X}`, access: 'admin' },
  { method: 'POST', path: '/api/ceramics/products/import', access: 'admin' },
  { method: 'POST', path: '/api/ceramics/lookup', access: 'notTranslator' },
  { method: 'POST', path: '/api/ceramics/analyze', access: 'notTranslator', probe: false },
  { method: 'POST', path: '/api/ceramics/price', access: 'notTranslator', probe: false },

  { method: 'GET', path: '/api/translator/documents', access: 'translatorDesk' },
  { method: 'POST', path: '/api/translator/documents', access: 'translatorDesk' },
  { method: 'GET', path: `/api/translator/documents/${X}`, access: 'translatorDesk' },
  { method: 'DELETE', path: `/api/translator/documents/${X}`, access: 'translatorDesk' },
  { method: 'POST', path: `/api/translator/documents/${X}/translate`, access: 'translatorDesk', probe: false },
  { method: 'GET', path: `/api/translator/documents/${X}/export`, access: 'translatorDesk' },
  { method: 'PATCH', path: `/api/translator/segments/${X}`, access: 'translatorDesk' },
  { method: 'POST', path: `/api/translator/segments/${X}/retranslate`, access: 'translatorDesk', probe: false },
  { method: 'GET', path: `/api/translator/segments/${X}/history`, access: 'translatorDesk' },
]

const allowed: Record<Access, Array<'anon' | 'admin' | 'invoicer' | 'translator'>> = {
  public: ['anon', 'admin', 'invoicer', 'translator'],
  user: ['admin', 'invoicer', 'translator'],
  admin: ['admin'],
  notTranslator: ['admin', 'invoicer'],
  translatorDesk: ['admin', 'translator'],
}

for (const route of routes) {
  for (const who of ['anon', 'admin', 'invoicer', 'translator'] as const) {
    const ok = allowed[route.access].includes(who)
    if (ok && route.probe === false) continue
    const expected = ok ? 'пускает' : who === 'anon' ? '401' : '403'
    test(`${route.method} ${route.path} · ${who} → ${expected}`, async ({ apiAs }) => {
      const api = await apiAs(who)
      // Битый JSON: обработчик, если до него дойдёт, ответит 400 и ничего не запишет.
      const body = route.method === 'GET' || route.method === 'DELETE' ? {} : { data: '{', headers: { 'Content-Type': 'application/json' } }
      const response = await api.fetch(route.path, { method: route.method, ...body })
      const status = response.status()
      if (!ok) {
        expect(status, await response.text()).toBe(who === 'anon' ? 401 : 403)
        expect((await response.json()).error?.message, 'ошибка с понятным текстом').toBeTruthy()
      } else {
        expect([401, 403], `${route.method} ${route.path}: ${status} ${await response.text()}`).not.toContain(status)
        expect(status, 'без падения сервера').toBeLessThan(500)
      }
    })
  }
}

test('неизвестный адрес API → 404 JSON', async ({ apiAs }) => {
  const response = await (await apiAs('admin')).get('/api/no-such-route')
  expect(response.status()).toBe(404)
  expect((await response.json()).error.code).toBe('not_found')
})
