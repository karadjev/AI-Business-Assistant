FROM golang:1.26.11-alpine3.23@sha256:23f198931d4bd83b4f5a613bd75656257ae8bf832dcbc2db80e1050b5219ca2a AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -o /out/healthcheck ./cmd/healthcheck

FROM alpine:3.23.3 AS base
RUN apk add --no-cache ca-certificates && addgroup -g 10001 app && adduser -D -u 10001 -G app app
USER app
WORKDIR /app

FROM base AS api
COPY --from=build /out/api /out/healthcheck /app/
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM base AS worker
COPY --from=build /out/worker /app/
ENTRYPOINT ["/app/worker"]

FROM base AS migrate
COPY --from=build /out/migrate /app/
ENTRYPOINT ["/app/migrate"]
CMD ["up"]
