# Развёртывание на Render + Neon (бесплатно, для тестов)

Схема: **один веб-сервис Render** (сайт + API в одном Docker-образе, `deploy/render/Dockerfile`)
и **база PostgreSQL в Neon**. При каждом запуске сервис сам применяет миграции и создаёт
первого сотрудника. Настройки сервиса описаны в `render.yaml` в корне репозитория.

## 1. База в Neon

1. Зарегистрируйтесь на <https://neon.tech> (удобно через GitHub).
2. **Create project**: имя `tradeflow`, версия PostgreSQL 17, регион **AWS Europe Central 1 (Frankfurt)** —
   рядом с сервером Render.
3. На главной странице проекта нажмите **Connect**:
   - выключите **Connection pooling** — нужен прямой адрес (без `-pooler` в имени хоста);
   - скопируйте строку подключения вида
     `postgresql://neondb_owner:…@ep-….eu-central-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require`.

   Параметр `channel_binding` можно оставить — при запуске он убирается автоматически.

## 2. Сервис в Render

1. Зарегистрируйтесь на <https://render.com> через GitHub и разрешите доступ к репозиторию `tradeflow`.
2. **New → Blueprint**, выберите репозиторий `tradeflow`, ветку `main`. Render прочитает `render.yaml`.
3. Заполните значения, которые он спросит:

   | Переменная | Что указать |
   |---|---|
   | `DATABASE_URL` | строка подключения из Neon (шаг 1.3) |
   | `BOOTSTRAP_ADMIN_EMAIL` | email, которым будете входить |
   | `BOOTSTRAP_ADMIN_PASSWORD` | пароль **не короче 12 символов** |

4. **Apply**. Первая сборка идёт 5–10 минут; ход видно во вкладке **Logs**. Успешный запуск выглядит так:

   ```text
   Applying database migrations
   1/u initial …
   …
   employee created: you@example.com (id=1)
   backend listening on :10000
   ```

5. Откройте адрес сервиса (вверху страницы сервиса, вида `https://tradeflow.onrender.com`)
   и войдите с email и паролем из шага 3.

## Как пользоваться дальше

- **Обновление сайта** — просто `git push` в `main`: Render сам пересоберёт и перезапустит сервис.
- **Добавить сотрудника.** Самостоятельная регистрация закрыта (`ALLOW_REGISTRATION=false`).
  В Render → **Environment** замените `BOOTSTRAP_ADMIN_EMAIL` и `BOOTSTRAP_ADMIN_PASSWORD` на данные
  нового сотрудника и сохраните — сервис перезапустится и создаст аккаунт. Прежние аккаунты остаются.
- Сообщение `Bootstrap employee not created (probably already exists)` в логах при повторных
  запусках — нормально: такой сотрудник уже есть.

## Ограничения бесплатных тарифов

- Сервис Render **засыпает через 15 минут** без запросов; первое открытие после этого — около минуты.
- **Диск Render временный**: загруженные исходные XLSX и изображения на сервере (`FILE_STORAGE_DIR`)
  пропадают при перезапуске и новой сборке. Сметы, справочники и расчёты хранятся в базе Neon
  и не теряются; фото строк сметы хранятся в браузере пользователя.
- Бесплатная база Neon — около 0.5 ГБ; после простоя она тоже «просыпается» за секунду-две.

## Если что-то не работает

- `password authentication failed` / `no such host` в логах — неверный `DATABASE_URL`: скопируйте заново из Neon.
- `bootstrap password must be at least 12 characters` — пароль короче 12 символов; исправьте в Environment.
- Сборка упала — откройте **Logs → Build**, пришлите последние строки.

## Проверить образ локально (необязательно)

```powershell
docker build -f deploy/render/Dockerfile -t tradeflow-render .
```
