module github.com/observex/query-engine

go 1.22

require (
	github.com/gofiber/fiber/v2 v2.52.4
	go.uber.org/zap v1.27.0
)

require (
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
	go.uber.org/atomic => github.com/uber-go/atomic v1.11.0
	go.uber.org/goleak => github.com/uber-go/goleak v1.3.0
	go.uber.org/multierr => github.com/uber-go/multierr v1.11.0
	go.uber.org/zap => github.com/uber-go/zap v1.27.0
	golang.org/x/net => github.com/golang/net v0.24.0
	golang.org/x/sys => github.com/golang/sys v0.19.0
	golang.org/x/text => github.com/golang/text v0.14.0
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.34.0
	gopkg.in/yaml.v3 => github.com/go-yaml/yaml v3.0.1+incompatible
)
