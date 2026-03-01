# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /sheeld ./cmd/sheeld

# Runtime stage
FROM alpine:3.20

RUN apk --no-cache add ca-certificates && \
    adduser -D -H sheeld

COPY --from=builder /sheeld /sheeld
COPY openapi.yaml /openapi.yaml

USER sheeld

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/sheeld"]
