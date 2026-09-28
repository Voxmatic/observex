# F6.1 dedicated synthetic probe (G-1: one Deployment per vantage).
# Build from the repository root:
#   docker build -f services/synthetic-probe/Dockerfile -t observex/synthetic-probe .
# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o /bin/service ./services/synthetic-probe/

# ── Runtime stage ─────────────────────────────────────────────────────────────
# distroless/static: no shell, no package manager. The system roots verify
# probe targets; the processor's CA is mounted separately when it is private.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/service /service

USER nonroot:nonroot

# Health endpoint (/healthz, /readyz). The probe listens on nothing else.
EXPOSE 8081

ENTRYPOINT ["/service"]
CMD ["serve"]
