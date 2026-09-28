FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev
WORKDIR /src

# db-monitor has its own go.mod
COPY services/db-monitor/go.mod services/db-monitor/go.sum ./
RUN go mod download 2>/dev/null || true

COPY services/db-monitor/ ./
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /bin/db-monitor ./main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/db-monitor /db-monitor
USER nobody
EXPOSE 8087
ENTRYPOINT ["/db-monitor"]
