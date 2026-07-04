# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /control-plane ./cmd/control-plane && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /sheeld-server ./cmd/sheeld-server

# Runtime base
FROM alpine:3.20 AS base

RUN apk --no-cache add ca-certificates && \
    adduser -D -H sheeld

USER sheeld

# Control plane: config API + dashboard backend + user auth
FROM base AS control-plane

COPY --from=builder /control-plane /control-plane
COPY openapi.yaml /openapi.yaml

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/control-plane"]

# Data plane: proxy + guard engine
FROM base AS sheeld-server

COPY --from=builder /sheeld-server /sheeld-server

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8081/healthz || exit 1

ENTRYPOINT ["/sheeld-server"]
