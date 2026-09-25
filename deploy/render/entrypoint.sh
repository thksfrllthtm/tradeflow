#!/bin/sh
# Запуск на Render: миграции → первый сотрудник (если задан) → сервер.
set -eu

# Некоторые провайдеры (Neon) добавляют в адрес channel_binding=require, а используемые драйверы (lib/pq в migrate,
# pgx 5.6) этот параметр не понимают. Соединение остаётся зашифрованным: sslmode=require.
DATABASE_URL=$(printf '%s' "$DATABASE_URL" | sed -E 's/([?&])channel_binding=[^&]*&?/\1/; s/[?&]$//')
export DATABASE_URL

if [ "${RUN_MIGRATIONS:-true}" = "true" ]; then
  echo "Applying database migrations"
  migrate -path /app/migrations -database "$DATABASE_URL" up
fi

# Консоли на бесплатном Render нет, поэтому первого сотрудника создаём при запуске.
# Если он уже есть, bootstrap-admin сообщит об этом, и запуск продолжится.
if [ -n "${BOOTSTRAP_ADMIN_EMAIL:-}" ]; then
  su-exec app /app/bootstrap-admin || echo "Bootstrap employee not created (probably already exists)"
fi

exec su-exec app /app/server
