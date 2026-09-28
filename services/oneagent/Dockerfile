# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# eBPF build tools
RUN apk add --no-cache git ca-certificates tzdata \
    clang llvm linux-headers libbpf-dev make gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build eBPF objects (graceful no-op if headers unavailable)
RUN make -C ebpf generate 2>/dev/null || echo "eBPF generation skipped (no kernel headers)"

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
    -o /bin/oneagent ./services/oneagent/

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata libbpf

COPY --from=builder /bin/oneagent /oneagent

# OneAgent must run as root for eBPF and /proc access
USER root

ENTRYPOINT ["/oneagent"]
