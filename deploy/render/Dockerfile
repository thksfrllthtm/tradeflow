# Образ для Render: сайт, API и миграции в одном контейнере.
# Собирается из корня репозитория (см. render.yaml). Локальный docker-compose его не использует.

# 1. Сборка сайта
FROM node:20-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 2. Сборка API и команды создания сотрудника
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && CGO_ENABLED=0 go build -o /out/bootstrap-admin ./cmd/bootstrap-admin

# 3. Та же версия migrate, что и в docker-compose.yml
FROM migrate/migrate:v4.18.3 AS migrate

# 4. Итоговый образ
FROM alpine:3.21
RUN apk add --no-cache su-exec ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=backend /out/server /out/bootstrap-admin /app/
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY backend/migrations /app/migrations
COPY --from=frontend /src/dist /app/public
COPY --chmod=755 deploy/render/entrypoint.sh /app/entrypoint.sh
ENV STATIC_DIR=/app/public \
    FILE_STORAGE_DIR=/app/data/uploads
EXPOSE 10000
ENTRYPOINT ["/app/entrypoint.sh"]
