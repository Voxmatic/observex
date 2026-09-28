// services/oneagent/ebpf/span_exporter.go
//
// OTLP Span Exporter — sends auto-instrumented HTTP spans to the ingestor
// in OpenTelemetry format. Batches spans for efficiency and handles backpressure.

package ebpf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ── OTLP-compatible span format ───────────────────────────────────────────────

type OTLPSpan struct {
	TraceID           string            `json:"trace_id"`
	SpanID            string            `json:"span_id"`
	ParentSpanID      string            `json:"parent_span_id,omitempty"`
	Name              string            `json:"name"`
	Kind              int               `json:"kind"`    // 2=CLIENT, 3=SERVER
	StartTimeUnixNano int64             `json:"start_time_unix_nano"`
	EndTimeUnixNano   int64             `json:"end_time_unix_nano"`
	StatusCode        int               `json:"status_code"`
	Attributes        map[string]string `json:"attributes"`
	Service           string            `json:"service_name"`
}

type OTLPBatch struct {
	ResourceSpans []OTLPResource `json:"resourceSpans"`
}

type OTLPResource struct {
	Resource OTLPResourceAttrs `json:"resource"`
	ScopeSpans []OTLPScopeSpans `json:"scopeSpans"`
}

type OTLPResourceAttrs struct {
	Attributes map[string]string `json:"attributes"`
}

type OTLPScopeSpans struct {
	Scope OTLPScope   `json:"scope"`
	Spans []OTLPSpan  `json:"spans"`
}

type OTLPScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ── SpanExporter ──────────────────────────────────────────────────────────────

type SpanExporter struct {
	ingestorURL string
	client      *http.Client
	log         *zap.Logger

	mu      sync.Mutex
	buffer  []HTTPSpan
	maxBuf  int
	flushInterval time.Duration
}

func NewSpanExporter(ingestorURL string, log *zap.Logger) *SpanExporter {
	return &SpanExporter{
		ingestorURL:   ingestorURL,
		client:        &http.Client{Timeout: 5 * time.Second},
		log:           log,
		maxBuf:        500,
		flushInterval: 5 * time.Second,
	}
}

// SendSpan implements SpanSender interface.
func (e *SpanExporter) SendSpan(_ context.Context, span HTTPSpan) error {
	e.mu.Lock()
	e.buffer = append(e.buffer, span)
	shouldFlush := len(e.buffer) >= e.maxBuf
	e.mu.Unlock()

	if shouldFlush {
		go e.flush()
	}
	return nil
}

// Run starts the periodic flush loop.
func (e *SpanExporter) Run(ctx context.Context) {
	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			e.flush() // final flush
			return
		case <-ticker.C:
			e.flush()
		}
	}
}

// flush sends all buffered spans to the ingestor.
func (e *SpanExporter) flush() {
	e.mu.Lock()
	if len(e.buffer) == 0 { e.mu.Unlock(); return }
	spans := e.buffer
	e.buffer = make([]HTTPSpan, 0, 64)
	e.mu.Unlock()

	// Group spans by service
	byService := make(map[string][]OTLPSpan)
	for _, s := range spans {
		otlp := spanToOTLP(s)
		svc := s.Service
		if svc == "" { svc = "unknown" }
		byService[svc] = append(byService[svc], otlp)
	}

	// Build OTLP batch
	batch := OTLPBatch{}
	for svc, svcSpans := range byService {
		batch.ResourceSpans = append(batch.ResourceSpans, OTLPResource{
			Resource: OTLPResourceAttrs{
				Attributes: map[string]string{
					"service.name":    svc,
					"telemetry.sdk":   "observex-oneagent",
					"agent.instrumentation": "ebpf-auto",
				},
			},
			ScopeSpans: []OTLPScopeSpans{{
				Scope: OTLPScope{Name: "observex.ebpf", Version: "1.0.0"},
				Spans: svcSpans,
			}},
		})
	}

	body, err := json.Marshal(batch)
	if err != nil { e.log.Error("marshal spans", zap.Error(err)); return }

	url := fmt.Sprintf("%s/v1/traces", e.ingestorURL)
	resp, err := e.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		e.log.Debug("flush spans failed", zap.Error(err), zap.Int("count", len(spans)))
		return
	}
	resp.Body.Close()

	e.log.Debug("spans flushed",
		zap.Int("count", len(spans)),
		zap.Int("services", len(byService)),
		zap.Int("status", resp.StatusCode))
}

// ── Conversion ────────────────────────────────────────────────────────────────

func spanToOTLP(s HTTPSpan) OTLPSpan {
	// Determine span kind: if we're the origin (no parent), treat as SERVER
	kind := 3 // SERVER
	if s.ParentID != "" { kind = 2 } // CLIENT

	// Status code: OTLP 1=OK, 2=ERROR
	statusCode := 1
	if s.StatusCode >= 500 { statusCode = 2 }

	attrs := make(map[string]string, len(s.Attrs)+4)
	for k, v := range s.Attrs { attrs[k] = v }
	attrs["http.method"]      = s.Method
	attrs["http.target"]      = s.Path
	attrs["http.host"]        = s.Host
	attrs["http.status_code"] = fmt.Sprintf("%d", s.StatusCode)
	attrs["duration_ms"]      = fmt.Sprintf("%.2f", s.DurationMs)

	spanName := s.Method + " " + s.Path
	if spanName == " " { spanName = "HTTP " + s.Service }

	return OTLPSpan{
		TraceID:           s.TraceID,
		SpanID:            s.SpanID,
		ParentSpanID:      s.ParentID,
		Name:              spanName,
		Kind:              kind,
		StartTimeUnixNano: s.StartTime.UnixNano(),
		EndTimeUnixNano:   s.EndTime.UnixNano(),
		StatusCode:        statusCode,
		Attributes:        attrs,
		Service:           s.Service,
	}
}
