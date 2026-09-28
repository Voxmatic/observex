module github.com/observex/db-monitor

go 1.22

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/gofiber/fiber/v2 v2.52.4
	github.com/lib/pq v1.10.9
	go.uber.org/zap v1.27.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/klauspost/compress v1.17.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
)

replace (
	filippo.io/edwards25519 => github.com/FiloSottile/edwards25519 v1.1.0
	go.uber.org/atomic => github.com/uber-go/atomic v1.11.0
	go.uber.org/goleak => github.com/uber-go/goleak v1.3.0
	go.uber.org/multierr => github.com/uber-go/multierr v1.11.0
	go.uber.org/zap => github.com/uber-go/zap v1.27.0
	golang.org/x/crypto => github.com/golang/crypto v0.22.0
	golang.org/x/net => github.com/golang/net v0.24.0
	golang.org/x/sys => github.com/golang/sys v0.19.0
	golang.org/x/telemetry => github.com/golang/telemetry v0.0.0-20240228155512-f48c80bd79b2
	golang.org/x/term => github.com/golang/term v0.19.0
	golang.org/x/text => github.com/golang/text v0.14.0
	golang.org/x/xerrors => github.com/golang/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/grpc => github.com/grpc/grpc-go v1.63.2
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.34.0
	gopkg.in/yaml.v2 => github.com/go-yaml/yaml v2.4.0+incompatible
	gopkg.in/yaml.v3 => github.com/go-yaml/yaml v3.0.1+incompatible
)
