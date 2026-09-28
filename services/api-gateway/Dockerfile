# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Copy go.mod/go.sum for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the whole package so handlers split across files are included.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64     go build -ldflags="-s -w -X main.Version=${VERSION:-dev} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"     -o /bin/service ./services/api-gateway/

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /bin/service /service

# Run as non-root
USER nonroot:nonroot

EXPOSE 3001

ENTRYPOINT ["/service"]
