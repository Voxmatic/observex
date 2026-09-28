FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY services/query-engine/go.mod services/query-engine/go.sum* ./
RUN go mod download 2>/dev/null || true
COPY services/query-engine/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o query-engine .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/query-engine .
EXPOSE 9090
HEALTHCHECK --interval=15s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:9090/health || exit 1
ENTRYPOINT ["./query-engine"]
