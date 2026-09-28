# ObserveX API Gateway — Production Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /observex-api ./services/api-gateway/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /observex-api /usr/local/bin/
COPY internal/db/migrations/ /app/migrations/
EXPOSE 3001
HEALTHCHECK --interval=10s --timeout=3s CMD wget -qO- http://localhost:3001/health || exit 1
CMD ["observex-api"]
