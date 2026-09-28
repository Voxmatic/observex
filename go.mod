module github.com/observex/platform

go 1.22.0

toolchain go1.22.2

require (
	github.com/cilium/ebpf v0.14.0
	github.com/gofiber/fiber/v2 v2.52.4
	github.com/gofiber/websocket/v2 v2.2.1
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.5.5
	github.com/redis/go-redis/v9 v9.5.1
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.22.0
	k8s.io/api v0.30.0
	k8s.io/apimachinery v0.30.0
	k8s.io/client-go v0.30.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/fasthttp/websocket v1.5.3 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/savsgio/gotils v0.0.0-20230208104028-c358bd845dee // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.52.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20230510235704-dd950f8aeaea // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/oauth2 v0.18.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/term v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/protobuf v1.34.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

// ── Route all blocked vanity domains → github.com ────────────────────────────
// proxy.golang.org and vanity domain redirects are not accessible here.
// All packages resolve through github.com directly.
replace (
	go.uber.org/atomic => github.com/uber-go/atomic v1.11.0
	go.uber.org/goleak => github.com/uber-go/goleak v1.3.0
	go.uber.org/multierr => github.com/uber-go/multierr v1.11.0
	// uber
	go.uber.org/zap => github.com/uber-go/zap v1.27.0

	// golang.org/x/*
	golang.org/x/crypto => github.com/golang/crypto v0.22.0
	golang.org/x/exp => github.com/golang/exp v0.0.0-20240404231335-c0f41cb1a7a0
	golang.org/x/mod => github.com/golang/mod v0.17.0
	golang.org/x/net => github.com/golang/net v0.24.0
	golang.org/x/oauth2 => github.com/golang/oauth2 v0.20.0
	golang.org/x/sync => github.com/golang/sync v0.7.0
	golang.org/x/sys => github.com/golang/sys v0.19.0
	golang.org/x/telemetry => github.com/golang/telemetry v0.0.0-20240228155512-f48c80bd79b2
	golang.org/x/term => github.com/golang/term v0.19.0
	golang.org/x/text => github.com/golang/text v0.14.0
	golang.org/x/time => github.com/golang/time v0.5.0
	golang.org/x/tools => github.com/golang/tools v0.20.0
	golang.org/x/xerrors => github.com/golang/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/api => github.com/googleapis/google-api-go-client v0.177.0
	google.golang.org/appengine => github.com/golang/appengine v1.6.8
	google.golang.org/genproto => github.com/googleapis/go-genproto v0.0.0-20240401170217-c3f982113cda
	google.golang.org/grpc => github.com/grpc/grpc-go v1.63.2

	// google.golang.org/*
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.34.0
	gopkg.in/check.v1 => github.com/go-check/check v0.0.0-20201130134442-10cb98267c6c
	gopkg.in/inf.v0 => github.com/go-inf/inf v0.9.1

	// gopkg.in/*

	// k8s.io/*
	k8s.io/api => github.com/kubernetes/api v0.30.0
	k8s.io/apimachinery => github.com/kubernetes/apimachinery v0.30.0
	k8s.io/client-go => github.com/kubernetes/client-go v0.30.0
	k8s.io/klog/v2 => github.com/kubernetes/klog/v2 v2.120.1
	k8s.io/kube-openapi => github.com/kubernetes/kube-openapi v0.0.0-20240322212309-b815d8309940
	k8s.io/metrics => github.com/kubernetes/metrics v0.30.0
	k8s.io/utils => github.com/kubernetes/utils v0.0.0-20240310230437-4693a0247e57

	// sigs.k8s.io/* (required by client-go)
	sigs.k8s.io/json => github.com/kubernetes-sigs/json v0.0.0-20221116044647-bc3834ca7abd
	sigs.k8s.io/structured-merge-diff/v4 => github.com/kubernetes-sigs/structured-merge-diff/v4 v4.4.1
	sigs.k8s.io/yaml => github.com/kubernetes-sigs/yaml v1.4.0
)
