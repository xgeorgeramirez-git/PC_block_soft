# Образ сервера связи pcblock-relay (M7, T2). Собирается из корня репозитория:
#   docker build -f deploy/relay/Dockerfile .
# На VPS его собирает docker compose (deploy/relay/compose.yaml, контекст ./src).
# Итог: scratch, один статический файл, пользователь 65532, данные в /data.

FROM golang:1.24-alpine AS build
WORKDIR /src
# Модуль сервера отдельный и без зависимостей: сеть для сборки не нужна.
COPY cmd/pcblock-relay/ ./
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=mod
RUN go build -trimpath -ldflags "-s -w" -o /out/pcblock-relay . \
 && mkdir -m 0700 /out/data

FROM scratch
COPY --from=build /out/pcblock-relay /pcblock-relay
# Папка данных с владельцем 65532: именованный том при первом запуске получает эти права.
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --start-interval=2s --retries=3 \
  CMD ["/pcblock-relay", "-health", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["/pcblock-relay", "-addr", ":8080", "-data", "/data"]
