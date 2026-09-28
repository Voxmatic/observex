// services/oneagent/ebpf/http_tracer.go
//
// eBPF HTTP Auto-Instrumentation — zero-code distributed tracing.
//
// Captures HTTP/1.1 and HTTP/2 requests at the kernel level by attaching
// uprobes to the TLS write/read functions of common runtimes:
//   - Go:     crypto/tls.(*Conn).Write / Read
//   - Node.js: SSL_write / SSL_read  (via OpenSSL)
//   - Python:  SSL_write / SSL_read  (via OpenSSL)
//   - Java:    sun.security.ssl.SSLSocketImpl (via JVMTI in JVM agent — separate)
//
// For plain HTTP (non-TLS), we hook at the socket layer:
//   - kprobe/tcp_sendmsg — captures outgoing request bytes
//   - kprobe/tcp_recvmsg — captures incoming response bytes
//
// HTTP parsing happens in userspace after capturing raw bytes:
//   1. Extract method, path, status, host from first 512 bytes of each stream
//   2. Match request/response using connection (srcIP:port, dstIP:port)
//   3. Compute latency = response timestamp - request timestamp
//   4. Emit OTLP-compatible span to ingestor
//
// This gives us distributed traces with ZERO changes to application code —
// matching what Datadog APM and Dynatrace OneAgent do.
//
// Architecture:
//   kernel (eBPF ring buffer)
//     → userspace HTTP parser
//       → span builder (adds trace context propagation)
//         → ingestor /v1/traces (OTLP format)
//
// Build constraints: linux only, requires kernel ≥ 4.18 + BTF

package ebpf

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// HTTPEvent is produced when we capture HTTP request/response bytes.
type HTTPEvent struct {
	Timestamp  time.Time
	ConnID     string // "srcIP:srcPort->dstIP:dstPort"
	Direction  string // "request" | "response"
	RawBytes   []byte
	PID        uint32
	ProcessName string
}

// HTTPSpan is a completed request+response pair with timing.
type HTTPSpan struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Service    string
	Method     string
	Path       string
	Host       string
	StatusCode int
	StartTime  time.Time
	EndTime    time.Time
	DurationMs float64
	Attrs      map[string]string
	PID        uint32
}

// PendingRequest holds an in-flight request waiting for its response.
type PendingRequest struct {
	TraceID   string
	SpanID    string
	Method    string
	Path      string
	Host      string
	StartTime time.Time
}

// ── HTTP Tracer ────────────────────────────────────────────────────────────────

type HTTPTracer struct {
	mu       sync.Mutex
	pending  map[string]*PendingRequest // connID → pending request
	spans    chan HTTPSpan
	log      *zap.Logger
	sender   SpanSender
}

// SpanSender is implemented by the agent's TelemetrySender.
type SpanSender interface {
	SendSpan(ctx context.Context, span HTTPSpan) error
}

func NewHTTPTracer(log *zap.Logger, sender SpanSender) *HTTPTracer {
	return &HTTPTracer{
		pending: make(map[string]*PendingRequest),
		spans:   make(chan HTTPSpan, 4096),
		log:     log,
		sender:  sender,
	}
}

// ProcessEvent handles a raw HTTP event from the eBPF ring buffer.
// Called from the ring buffer read loop.
func (t *HTTPTracer) ProcessEvent(ctx context.Context, evt HTTPEvent) {
	switch evt.Direction {
	case "request":
		t.handleRequest(evt)
	case "response":
		t.handleResponse(ctx, evt)
	}
}

func (t *HTTPTracer) handleRequest(evt HTTPEvent) {
	req, err := parseHTTPRequest(evt.RawBytes)
	if err != nil {
		return // Not HTTP or malformed — ignore
	}

	pending := &PendingRequest{
		TraceID:   generateTraceID(),
		SpanID:    generateSpanID(),
		Method:    req.Method,
		Path:      req.URL.Path,
		Host:      req.Host,
		StartTime: evt.Timestamp,
	}

	t.mu.Lock()
	t.pending[evt.ConnID] = pending
	t.mu.Unlock()
}

func (t *HTTPTracer) handleResponse(ctx context.Context, evt HTTPEvent) {
	t.mu.Lock()
	req, ok := t.pending[evt.ConnID]
	if ok {
		delete(t.pending, evt.ConnID)
	}
	t.mu.Unlock()

	if !ok {
		return // No matching request — orphan response
	}

	statusCode := parseHTTPStatus(evt.RawBytes)
	endTime    := evt.Timestamp
	if endTime.IsZero() { endTime = time.Now() }

	span := HTTPSpan{
		TraceID:    req.TraceID,
		SpanID:     req.SpanID,
		Service:    evt.ProcessName,
		Method:     req.Method,
		Path:       req.Path,
		Host:       req.Host,
		StatusCode: statusCode,
		StartTime:  req.StartTime,
		EndTime:    endTime,
		DurationMs: float64(endTime.Sub(req.StartTime).Milliseconds()),
		PID:        evt.PID,
		Attrs: map[string]string{
			"http.method":     req.Method,
			"http.url":        req.Host + req.Path,
			"http.status":     fmt.Sprintf("%d", statusCode),
			"process.pid":     fmt.Sprintf("%d", evt.PID),
			"telemetry.agent": "observex-oneagent",
		},
	}

	if err := t.sender.SendSpan(ctx, span); err != nil {
		t.log.Debug("send span failed", zap.Error(err))
	}

	// Publish to internal span channel for local aggregation
	select {
	case t.spans <- span:
	default: // drop if backed up
	}
}

// CleanStalePending removes requests with no response after 30 seconds.
func (t *HTTPTracer) CleanStalePending() {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-30 * time.Second)
	for connID, req := range t.pending {
		if req.StartTime.Before(cutoff) {
			delete(t.pending, connID)
		}
	}
}

// Stats returns tracer metrics for health checks.
func (t *HTTPTracer) Stats() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]int{
		"pending_requests": len(t.pending),
		"span_queue":       len(t.spans),
	}
}

// ── HTTP parsing ───────────────────────────────────────────────────────────────

// parseHTTPRequest attempts to parse the first bytes of a TCP stream as HTTP/1.1.
// Returns nil if the bytes don't look like HTTP.
func parseHTTPRequest(raw []byte) (*http.Request, error) {
	if len(raw) == 0 { return nil, fmt.Errorf("empty") }

	// Quick HTTP method check — avoid parsing non-HTTP traffic
	methods := []string{"GET ", "POST ", "PUT ", "DELETE ", "PATCH ", "HEAD ", "OPTIONS "}
	isHTTP := false
	for _, m := range methods {
		if bytes.HasPrefix(raw, []byte(m)) { isHTTP = true; break }
	}
	if !isHTTP { return nil, fmt.Errorf("not HTTP") }

	// Pad to ensure bufio doesn't hang on partial reads
	if len(raw) < 4 || raw[len(raw)-1] != '\n' {
		raw = append(raw, '\n')
	}

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		// Fallback: extract method+path from first line manually
		line := string(raw)
		if idx := strings.Index(line, "\r\n"); idx > 0 { line = line[:idx] }
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return &http.Request{Method: parts[0]}, nil
		}
		return nil, err
	}
	return req, nil
}

// parseHTTPStatus extracts the status code from an HTTP response.
func parseHTTPStatus(raw []byte) int {
	// "HTTP/1.1 200 OK\r\n..." or "HTTP/2 200 \r\n..."
	if !bytes.HasPrefix(raw, []byte("HTTP/")) { return 0 }
	parts := strings.Fields(string(raw[:min2(len(raw), 32)]))
	if len(parts) < 2 { return 0 }
	code := 0
	fmt.Sscanf(parts[1], "%d", &code)
	return code
}

func min2(a, b int) int { if a < b { return a }; return b }

// ── Trace ID generation ────────────────────────────────────────────────────────

func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ── Process → service name mapper ─────────────────────────────────────────────

// InferServiceName maps common process names to human-readable service names.
func InferServiceName(comm, cmdline string) string {
	// Use cmdline path if comm is generic
	c := strings.ToLower(comm)
	switch {
	case strings.Contains(c, "nginx"):   return "nginx"
	case strings.Contains(c, "node"):    return inferNodeService(cmdline)
	case strings.Contains(c, "python"):  return inferPythonService(cmdline)
	case strings.Contains(c, "java"):    return inferJavaService(cmdline)
	case strings.Contains(c, "ruby"):    return "ruby-app"
	case strings.Contains(c, "php"):     return "php-app"
	case strings.Contains(c, "gunicorn"):return "gunicorn"
	case strings.Contains(c, "uvicorn"): return "uvicorn"
	case strings.Contains(c, "puma"):    return "rails"
	default:
		// Use binary name if not a generic runtime
		if comm != "" && !isGenericRuntime(comm) { return comm }
		return "unknown-service"
	}
}

func isGenericRuntime(comm string) bool {
	generics := []string{"sh", "bash", "dash", "python", "python3", "node", "ruby", "java"}
	c := strings.ToLower(comm)
	for _, g := range generics { if c == g { return true } }
	return false
}

func inferNodeService(cmdline string) string {
	// "node /app/server.js" → "server"
	parts := strings.Fields(cmdline)
	for _, p := range parts {
		if strings.HasSuffix(p, ".js") {
			name := strings.TrimSuffix(p, ".js")
			if idx := strings.LastIndex(name, "/"); idx >= 0 { name = name[idx+1:] }
			return name
		}
	}
	return "node-app"
}

func inferPythonService(cmdline string) string {
	parts := strings.Fields(cmdline)
	for _, p := range parts {
		if strings.HasSuffix(p, ".py") {
			name := strings.TrimSuffix(p, ".py")
			if idx := strings.LastIndex(name, "/"); idx >= 0 { name = name[idx+1:] }
			return name
		}
		if p == "-m" { continue }
		if !strings.HasPrefix(p, "-") && p != "python" && p != "python3" {
			// Strip .py if present
			return strings.TrimSuffix(p, ".py")
		}
	}
	return "python-app"
}

func inferJavaService(cmdline string) string {
	// Look for -Dservice.name=X or jar filename
	for _, part := range strings.Fields(cmdline) {
		if strings.HasPrefix(part, "-Dservice.name=") { return strings.TrimPrefix(part, "-Dservice.name=") }
		if strings.HasPrefix(part, "-Dspring.application.name=") { return strings.TrimPrefix(part, "-Dspring.application.name=") }
		if strings.HasSuffix(part, ".jar") {
			name := strings.TrimSuffix(part, ".jar")
			if idx := strings.LastIndex(name, "/"); idx >= 0 { name = name[idx+1:] }
			return name
		}
	}
	return "java-app"
}

// ── Connection-level flow tracker ─────────────────────────────────────────────

// ConnKey uniquely identifies a TCP connection.
type ConnKey struct {
	SrcIP   net.IP
	SrcPort uint16
	DstIP   net.IP
	DstPort uint16
}

func (k ConnKey) String() string {
	return fmt.Sprintf("%s:%d->%s:%d", k.SrcIP, k.SrcPort, k.DstIP, k.DstPort)
}

func (k ConnKey) Reverse() ConnKey {
	return ConnKey{SrcIP: k.DstIP, SrcPort: k.DstPort, DstIP: k.SrcIP, DstPort: k.SrcPort}
}
