# Развёртывание: Vercel + Render + Supabase (бесплатно)

| Что | Где | Файлы в репозитории |
|---|---|---|
| Сайт (frontend) | Vercel | `frontend/vercel.json` |
| API (backend) | Render | `render.yaml`, `deploy/render/Dockerfile`, `deploy/render/entrypoint.sh` |
| База PostgreSQL | Supabase Database | `backend/migrations` (применяет Render при запуске) |
| Код | GitHub, репозиторий `tradeflow` | — |

Браузер работает только с адресом Vercel. Запросы `/api/*` Vercel пересылает на Render
(`rewrites` в `frontend/vercel.json`), поэтому cookie сессии остаётся на домене сайта и CORS не нужен.
Render при каждом запуске применяет миграции и создаёт первого сотрудника.

## 1. Supabase

1. Зарегистрируйтесь на <https://supabase.com> (удобно через GitHub), **New project**:
   имя `tradeflow`, регион **Central EU (Frankfurt)**, придумайте пароль базы и сохраните его.
   Если форма спрашивает про Data API — выберите вариант **только строка подключения** (Data API не нужен).
2. Строка подключения: кнопка **Connect** вверху проекта → **Session pooler** (не Direct и не Transaction:
   у Render нет IPv6, а Transaction pooler не поддерживает нужные приложению запросы). Скопируйте строку вида
   `postgresql://postgres.<ref>:[YOUR-PASSWORD]@aws-….pooler.supabase.com:5432/postgres`,
   подставьте пароль вместо `[YOUR-PASSWORD]` и допишите в конец `?sslmode=require`.

## 2. Render (API)

Если сервис `tradeflow` уже есть: **Render → tradeflow → Environment**, задайте переменные и сохраните:

| Переменная | Значение |
|---|---|
| `DATABASE_URL` | строка из шага 1.2 |
| `GIN_MODE` | `release` |
| `DEEPSEEK_API_KEY` | ключ DeepSeek (platform.deepseek.com) — для «Подобрать с ИИ»; без него цены подбираются только математикой |
| `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD` | как были (пароль не короче 12 символов) |

Переменные `SUPABASE_URL`, `SUPABASE_SECRET_KEY`, `SUPABASE_STORAGE_BUCKET`, `AI_PROVIDER`,
`ANTHROPIC_*` и `GEMINI_*` больше не используются — если они остались в Render, их можно удалить.

Новый сервис: **New → Blueprint** → репозиторий `tradeflow`, ветка `main` — Render прочитает `render.yaml`
и спросит те же значения.

В **Logs** успешный запуск выглядит так:

```text
Applying database migrations
…
22/u drop_legacy_module …
employee created: you@example.com (id=1)
backend listening on :10000
```

Адрес сервиса (вверху страницы, вида `https://tradeflow.onrender.com`) должен совпадать с адресом
в `frontend/vercel.json`. Если отличается — исправьте его там и сделайте `git push`.

## 3. Vercel (сайт)

1. Зарегистрируйтесь на <https://vercel.com> через GitHub.
2. **Add New → Project** → импортируйте репозиторий `tradeflow`.
3. **Root Directory** → `frontend`. Остальное Vercel возьмёт из `frontend/vercel.json`. **Deploy**.
4. Откройте адрес проекта (вида `https://tradeflow.vercel.app`) и войдите.

## 4. Проверить после развёртывания

- Вход, открытие сметы, фото товаров в строках сметы.
- **Загрузка большого XLSX (10+ МБ) и сохранение сметы перчаток с фото.** Запросы к API идут через Vercel;
  если на больших запросах появится ошибка `413`, пришлите её — тогда загрузку нужно будет направить в обход Vercel.

## Как пользоваться дальше

- **Обновление** — `git push` в `main`: Vercel и Render пересоберут каждый свою часть.
- **Сотрудники и роли.** Регистрация закрыта (`ALLOW_REGISTRATION=false`). Администратор заводит сотрудников,
  меняет им роль и пароль и блокирует на странице **Сотрудники** (есть в меню только у администратора):

  | Роль | Доступ |
  |---|---|
  | Администратор | всё: сметы всех сотрудников, изменение справочников, страница «Сотрудники» |
  | Инвойсер | калькуляторы (свои сметы); справочники товаров и ТН ВЭД — только просмотр |
  | Переводчик | пока то же, что инвойсер |

  Сотрудник из `BOOTSTRAP_ADMIN_EMAIL`/`BOOTSTRAP_ADMIN_PASSWORD` создаётся администратором — это запасной вход,
  если доступ ко всем администраторам потерян. `Bootstrap employee not created (probably already exists)` в логах — нормально.

## Ограничения бесплатных тарифов

- Render **засыпает через 15 минут** без запросов; первое открытие после этого — около минуты
  (Vercel ждёт ответа до 2 минут).
- Supabase приостанавливает бесплатный проект после **недели без запросов** — его включают кнопкой
  **Restore** в панели Supabase. Лимит базы — 500 МБ.

## Если что-то не работает

- `password authentication failed` / `no such host` / `network is unreachable` в логах Render — неверный
  `DATABASE_URL`: нужна строка **Session pooler** с подставленным паролем.
- Сайт открывается, но вход не работает / `404` на `/api/...` — адрес Render в `frontend/vercel.json`
  не совпадает с настоящим.
- Сборка упала — откройте **Logs → Build** (Render) или **Deployments** (Vercel), пришлите последние строки.
