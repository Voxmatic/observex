FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/trivy-scanner ./services/trivy-scanner/main.go

FROM aquasec/trivy:0.50.0 AS trivy

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/trivy-scanner /trivy-scanner
COPY --from=trivy /usr/local/bin/trivy /usr/local/bin/trivy
USER nobody
ENTRYPOINT ["/trivy-scanner"]
