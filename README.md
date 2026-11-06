# TradeFlow — калькулятор предварительного ввоза

Внутреннее приложение East Trans Logistic для сотрудников: сметы товаров с предварительным
расчётом растаможки в Казахстане (импорт инвойса Excel, проверка строк, расчёт, подбор цен под
бюджет, выгрузка в Excel), справочник товаров, номенклатура ТН ВЭД ЕАЭС, подбор
кода ИИ-агентом и база решений, подбор наименования и цены керамики, сотрудники и роли.
Расчёт предварительный: это не официальное подтверждение ставок.

Текущее состояние и ограничения — [docs/STATUS.md](docs/STATUS.md), решения —
[docs/DECISIONS.md](docs/DECISIONS.md), развёртывание — [deploy/README.md](deploy/README.md).

## Стек

- **backend** — Go 1.26, Gin, GORM, PostgreSQL 17, миграции golang-migrate (`backend/migrations`).
- **frontend** — React 19, TypeScript, Vite, Vitest; псевдоним `@/` → `frontend/src`.
- **ИИ** — DeepSeek (OpenAI-совместимый API): подбор цен, керамика, перевод; агент подбора кода ТН ВЭД —
  DeepSeek или Claude (`TNVED_AI_PROVIDER`), фото — отдельная модель (`TNVED_AI_VISION_*`).
- **Деплой** — сайт на Vercel, API на Render, база в Supabase.

## Локальный запуск (Docker Compose)

Нужны Docker Desktop и свободные порты `5173`, `8080`, `5432`.

```powershell
Copy-Item .env.example .env          # задайте свой POSTGRES_PASSWORD и при желании DEEPSEEK_API_KEY
docker compose run --rm migrate      # миграции (безопасно запускать повторно)
docker compose -f docker-compose.yml -f docker-compose.dev.yml watch
```

Режим `watch` применяет правки `frontend/src` через Vite HMR и перезапускает backend при
изменении Go-файлов; при правке `package.json`, `go.mod` или Dockerfile образ пересобирается сам.
Новые миграции — снова `docker compose run --rm migrate`.

Первый сотрудник (пароль передаётся только окружением):

```powershell
$env:BOOTSTRAP_ADMIN_EMAIL='admin@example.com'
$env:BOOTSTRAP_ADMIN_PASSWORD='длинный-уникальный-пароль'
docker compose exec -e BOOTSTRAP_ADMIN_EMAIL -e BOOTSTRAP_ADMIN_PASSWORD backend go run ./cmd/bootstrap-admin
```

Откройте http://127.0.0.1:5173 (именно `127.0.0.1`: на Windows `localhost` иногда уходит в IPv6
и Vite не отвечает). Остановить — `docker compose down`, вместе с базой — `docker compose down -v`.

### Без Docker

Нужны Go 1.26+, Node.js 20.19+ и PostgreSQL 17; переменные — из `.env.example`.

```powershell
docker run --rm -v "${PWD}\backend\migrations:/migrations" migrate/migrate:v4.18.3 -path=/migrations -database="$env:DATABASE_URL" up
cd backend; go run ./cmd/server                 # API на :8080
cd frontend; npm ci; npm run dev               # сайт на :5173, /api проксируется на :8080
```

`VITE_API_PROXY_TARGET` меняет адрес API для Vite (по умолчанию `http://localhost:8080`).

## Переменные окружения

Полный список с пояснениями — в [.env.example](.env.example). Главное:

| Переменная | Назначение |
|---|---|
| `DATABASE_URL` | строка подключения PostgreSQL (вне Docker Compose и на Render) |
| `BACKEND_PORT` / `PORT` | порт API (Render задаёт `PORT`) |
| `SESSION_COOKIE_NAME`, `SESSION_TTL_HOURS`, `COOKIE_SECURE` | сессия; `COOKIE_SECURE=true` за HTTPS |
| `ALLOW_REGISTRATION` | самостоятельная регистрация (на сервере `false`) |
| `DEEPSEEK_API_KEY`, `DEEPSEEK_MODEL` | ИИ для подбора цен; по умолчанию и для керамики, и для агента |
| `CERAMICS_AI_*` | свой провайдер ИИ для керамики |
| `TNVED_AI_*` | агент подбора кода: ключ, модель, фото, дневной лимит, цены токенов, правило наименования |
| `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD` | первый сотрудник (`cmd/bootstrap-admin`) |
| `RUN_MIGRATIONS` | применять миграции при старте образа Render (по умолчанию `true`) |

## Проверки

```bash
cd backend && gofmt -l . && go vet ./... && go build ./... && go test ./...
cd backend && go run golang.org/x/tools/cmd/deadcode@latest ./...
cd frontend && npm ci && npm run check && npm test && npm run build
```

Точность агента на проверенных записях базы решений (расходует токены):
`cd backend && go run ./cmd/tnved-agent-eval -limit 20`.

## Миграции

SQL-миграции — `backend/migrations`, golang-migrate; GORM схему не создаёт. Уже применённые
миграции не меняются и не удаляются (боевая база хранит номер версии): изменения схемы — только
новыми файлами со следующим номером. Render применяет миграции сам при старте (`RUN_MIGRATIONS=true`).

## Структура

```
backend/
├── cmd/
│   ├── server/              API
│   ├── bootstrap-admin/     создание первого сотрудника
│   └── tnved-agent-eval/    проверка точности агента подбора кода
├── internal/
│   ├── config/              переменные окружения
│   ├── http/                маршруты Gin и обработчики
│   ├── repository/          GORM и SQL
│   ├── thirdparty/grate/    чтение старых .xls (вендорная копия)
│   └── service/             бизнес-логика (общее: ValidationError, округление, десятичные и текстовые помощники)
│       ├── ai/              клиент DeepSeek и ошибки провайдера ИИ
│       ├── antidumping/     антидемпинговые меры ЕАЭС (реестр ЕЭК)
│       ├── auth/            вход, регистрация, сессии, пароли
│       ├── calculation/     расчёт растаможки строки, распределение доставки и сбора
│       ├── ceramics/        подбор наименования и цены керамики
│       ├── decisions/       база решений «товар → код», хеш и описание фото, похожие
│       ├── directory/       справочник товаров, номенклатура, перечень РК, страницы TWS
│       ├── employees/       сотрудники и роли
│       ├── exportcheck/     сверка инвойса с экспортной декларацией КНР
│       ├── goods/           сметы товаров: разбор Excel, расчёт, выгрузка, подбор цен
│       ├── rates/           курс НБРК
│       ├── riskindicators/  стоимостные индикаторы риска КГД
│       ├── tnvedagent/      ИИ-агент подбора кода ТН ВЭД
│       ├── trademarks/      ТРОИС: реестр КГД, проверка наименований
│       ├── translator/      перевод Word-документов
│       └── utilfee/         калькулятор утильсбора
└── migrations/              000001…000053

frontend/src/
├── app/                     корень приложения, выбор страницы
├── features/                модули (страницы); друг у друга берут только API-клиенты
│   ├── agentLog/            журнал агента (админ)
│   ├── auth/                вход и регистрация
│   ├── ceramics/            керамика
│   ├── decisions/           база решений
│   ├── directories/         справочники: common/, components/, kzList/, nomenclature/, products/, riskIndicators/, trademarks/
│   ├── documentsBase/       база документов «Документы для ввоза» (админ)
│   ├── employees/           сотрудники
│   ├── exportCheck/         сверка с экспортной декларацией КНР
│   ├── goods/               сметы товаров: agent/ (чат подбора кода), calculator/, components/, hooks/, lib/
│   ├── home/                главная
│   ├── translator/          перевод Word-документов
│   └── utilFee/             калькулятор утильсбора
└── shared/                  общее для модулей
    ├── api/                 HTTP-клиент, goodsApi (разбор Excel, расчёт, выгрузка)
    ├── components/          AppHeader, Drawer, Modal, PhotoLightbox, CopyCode, иконки…
    ├── hooks/, lib/         общие хуки и функции (даты, фото, числа…)
    ├── router/              адреса страниц
    ├── styles/              base, app-shell, list-page, drawer, modal, sheet, verification-status
    ├── tnved/               формат кода, типы номенклатуры и решений
    ├── trademarks/          подсветка совпадений с ТРОИС
    └── types/               пользователь, данные смет
```

Правило: модуль в `features/` не импортирует код другого модуля, кроме его API-клиента; всё общее
лежит в `shared/`. В backend подпакеты `service/*` не зависят друг от друга по кругу.

## API

Все маршруты, кроме `/api/health`, `/api/auth/{login,register,logout}` и `/api/rates/today`, требуют сессию (HttpOnly-cookie).
«админ» — только администратор.

| Маршрут | Назначение |
|---|---|
| `GET /api/health` | проверка живости |
| `POST /api/auth/login`, `register`, `logout`; `GET /api/auth/me` | вход, регистрация, выход, текущий пользователь |
| `GET/POST /api/employees`, `PATCH/DELETE /api/employees/:id` | сотрудники (админ) |
| `POST /api/goods/parse-xlsx`, `calculate`, `export` | разбор Excel, расчёт и выгрузка сметы товаров |
| `GET/POST /api/goods-estimates`, `GET/PATCH/DELETE /api/goods-estimates/:id`, `PATCH …/:id/draft` | сметы товаров и автосохранение черновика (`?userId=` — сметы сотрудника, админ) |
| `GET /api/rates/nbk?date=&currency=USD\|EUR`, `GET /api/rates/today` | курс НБРК на дату и на сегодня |
| `POST /api/estimates/price-weights`, `price-history` | подбор цен под бюджет: ИИ и история цен |
| `/api/product-directory` (+ `export`, `import`, `:id/archive`, `restore`, `copy`) | справочник товаров (правка — админ) |
| `/api/risk-indicators` (+ `import`, `:id`) | стоимостные индикаторы риска КГД: профили риска (загрузка и удаление — админ) |
| `GET /api/tnved-nomenclature`, `POST lookup`, `POST import` | номенклатура ТН ВЭД ЕАЭС (загрузка — админ) |
| `POST /api/tnved-kz-rates/excel`, `import-pdf`, `import` | перечень РК (админ) |
| `GET /api/tnved-antidumping`, `POST …/refresh` | антидемпинговые меры ЕАЭС из реестра ЕЭК; обновление — админ |
| `/api/code-decisions` (+ `stats`, `import`, `:id/verify`, `:id/reject`) | база решений (проверка, правка, импорт — админ) |
| `/api/trademarks` (+ `logo`, `check`, `import`, `logo-name`) | ТРОИС (загрузка реестра — админ) |
| `/api/tnved-agent/status`, `usage`, `sessions` (+ `:id`, `begin`, `messages`, `rerun`, `cancel`, `apply`, `name`, `documents`, `shares`), `reference` (+ `:id`, `trademarks`), `share-users` | ИИ-агент подбора кода; `reference` — чат-справочник на главной |
| `/api/tnved-agent/log`, `documents-base` | журнал агента и база документов (админ) |
| `/api/ceramics/*` | керамика: база марок, разбор, подбор, цена |
| `GET/POST /api/util-fee`, `POST …/rows` | калькулятор утильсбора |
| `POST /api/export-check` | сверка инвойса с экспортной декларацией КНР |
| `/api/translator/*` | перевод Word-документов (переводчик и админ) |

Неизвестный адрес под `/api` отвечает JSON `404`.
