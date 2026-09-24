# Образ API для Render: backend, миграции и команда создания сотрудника.
# Сайт собирает и отдаёт Vercel (frontend/vercel.json), база и файлы — в Supabase.
# Собирается из корня репозитория (см. render.yaml). Локальный docker-compose его не использует.

# 1. Сборка API и команды создания сотрудника
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && CGO_ENABLED=0 go build -o /out/bootstrap-admin ./cmd/bootstrap-admin

# 2. Та же версия migrate, что и в docker-compose.yml
FROM migrate/migrate:v4.18.3 AS migrate

# 3. Итоговый образ
FROM alpine:3.21
RUN apk add --no-cache su-exec ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=backend /out/server /out/bootstrap-admin /app/
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY backend/migrations /app/migrations
COPY --chmod=755 deploy/render/entrypoint.sh /app/entrypoint.sh
# Используется, только если SUPABASE_URL не задан: тогда файлы лежат на временном диске.
ENV FILE_STORAGE_DIR=/app/data/uploads
# Бесплатный Render даёт 512 МБ: разбор XLSX с фото держит в памяти сотни МБ, поэтому
# сборщик мусора Go должен освобождать память раньше, чем контейнер упрётся в лимит.
ENV GOMEMLIMIT=400MiB
EXPOSE 10000
ENTRYPOINT ["/app/entrypoint.sh"]
