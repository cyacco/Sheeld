# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sheeld ./cmd/sheeld

# Runtime stage
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=builder /sheeld /sheeld

EXPOSE 8080

ENTRYPOINT ["/sheeld"]
