FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && CGO_ENABLED=0 go build -o /out/bootstrap-admin ./cmd/bootstrap-admin

# Development target. It is used only by docker-compose.dev.yml so that
# source changes do not require rebuilding the production image.
FROM golang:1.26-alpine AS dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
EXPOSE 8080
CMD ["go", "run", "./cmd/server"]

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/bootstrap-admin /app/bootstrap-admin
EXPOSE 8080
USER app
CMD ["/app/server"]
