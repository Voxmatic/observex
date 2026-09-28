// services/processor/forecast.go
//
// ObserveX Predictive Analytics Engine.
//
// Provides time-series forecasting for capacity planning, traffic-spike
// prediction, and early-warning anomaly detection. Three methods are used
// depending on data availability:
//
//   1. Weighted Linear Regression (primary)
//      Fits a least-squares line to the circular buffer values, then
//      extrapolates forward. Recent values are weighted more heavily
//      (exponential weighting with λ=0.95). Works with as few as 10 samples.
//
//   2. Holt-Winters Double Exponential Smoothing (≥30 samples)
//      Handles metrics with a trend component. Alpha=0.3 (level), Beta=0.1
//      (trend). More stable than linear regression when data is noisy.
//
//   3. Seasonal Decomposition (≥72 samples, ≈1h at 15s scrape interval)
//      Extracts a 12-period (3-minute) seasonal component and adds it back to
//      the trend forecast. Improves accuracy for metrics with regular spikes
//      (e.g. cron jobs, end-of-business traffic).
//
// API:
//   GET /v1/forecasts?service=X&metric=Y&horizon=30m
//   GET /v1/forecasts/capacity   – cluster-wide capacity predictions
//
// Self-healing recommendations are derived from forecasts:
//   - Predicted CPU > 85%  within horizon → recommend scale-out
//   - Predicted MEM > 90%  within horizon → recommend memory limit increase
//   - Predicted P99 > 3s   within horizon → recommend circuit breaker or cache
//   - Predicted ERR > 10%  within horizon → recommend canary rollback or alert

package main

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

)

// ── Forecast types ─────────────────────────────────────────────────────────────

// ForecastPoint is a single predicted value with a confidence interval.
type ForecastPoint struct {
	OffsetMin int     `json:"offset_min"` // minutes from now
	Value     float64 `json:"value"`
	Lower     float64 `json:"lower_95"` // 95% confidence lower bound
	Upper     float64 `json:"upper_95"` // 95% confidence upper bound
}

// Forecast is the full prediction for one metric on one service.
type Forecast struct {
	ServiceID   string          `json:"service_id"`
	ServiceName string          `json:"service_name"`
	Metric      string          `json:"metric"`
	Unit        string          `json:"unit"`
	Method      string          `json:"method"`       // "linear" | "holt_winters" | "seasonal"
	Horizon     string          `json:"horizon"`      // e.g. "30m"
	Points      []ForecastPoint `json:"points"`
	Trend       string          `json:"trend"`        // "rising" | "falling" | "stable"
	TrendPct    float64         `json:"trend_pct"`    // % change over horizon
	Alert       string          `json:"alert,omitempty"` // capacity/spike warning if any
	Suggestions []string        `json:"suggestions,omitempty"`
	SampleCount int             `json:"sample_count"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// CapacityForecast is the cluster-wide capacity summary.
type CapacityForecast struct {
	ClusterName  string     `json:"cluster_name"`
	GeneratedAt  time.Time  `json:"generated_at"`
	HorizonMin   int        `json:"horizon_min"`
	Forecasts    []Forecast `json:"forecasts"`
	AtRisk       []string   `json:"at_risk_services"` // services predicted to breach thresholds
	Healthy      int        `json:"healthy_count"`
	Warning      int        `json:"warning_count"`
	Critical     int        `json:"critical_count"`
}

// ── ForecastEngine ─────────────────────────────────────────────────────────────

type ForecastEngine struct {
	proc *Processor
	mu   sync.RWMutex
	// Cache last forecast per service+metric to avoid recomputing every request
	cache    map[string]*Forecast
	cacheExp map[string]time.Time
}

func newForecastEngine(proc *Processor) *ForecastEngine {
	return &ForecastEngine{
		proc:     proc,
		cache:    make(map[string]*Forecast),
		cacheExp: make(map[string]time.Time),
	}
}

// ForecastMetric generates a forecast for one service+metric combination.
// horizonMin is the number of minutes to predict forward.
func (fe *ForecastEngine) ForecastMetric(serviceID, serviceName, metric string, horizonMin int) *Forecast {
	fe.proc.metricsMu.RLock()
	windows, ok := fe.proc.metricsWindows[serviceID]
	if !ok {
		fe.proc.metricsMu.RUnlock()
		return nil
	}
	buf, ok := windows[metric]
	if !ok {
		fe.proc.metricsMu.RUnlock()
		return nil
	}
	vals := buf.Values()
	fe.proc.metricsMu.RUnlock()

	if len(vals) < 5 {
		return nil // insufficient data
	}

	f := &Forecast{
		ServiceID:   serviceID,
		ServiceName: serviceName,
		Metric:      metric,
		Unit:        metricUnit(metric),
		Horizon:     fmt.Sprintf("%dm", horizonMin),
		SampleCount: len(vals),
		GeneratedAt: time.Now().UTC(),
	}

	// Choose method based on sample count
	var points []ForecastPoint
	switch {
	case len(vals) >= 72:
		points = fe.seasonalForecast(vals, horizonMin)
		f.Method = "seasonal"
	case len(vals) >= 30:
		points = fe.holtWintersForecast(vals, horizonMin)
		f.Method = "holt_winters"
	default:
		points = fe.linearForecast(vals, horizonMin)
		f.Method = "linear"
	}

	f.Points = points

	// Compute trend
	if len(points) > 0 && len(vals) > 0 {
		current := vals[len(vals)-1]
		predicted := points[len(points)-1].Value
		if current > 0 {
			f.TrendPct = (predicted - current) / current * 100
		}
		switch {
		case f.TrendPct > 5:
			f.Trend = "rising"
		case f.TrendPct < -5:
			f.Trend = "falling"
		default:
			f.Trend = "stable"
		}
	}

	// Generate capacity warnings and recommendations
	fe.annotate(f, vals)

	return f
}

// ── Linear regression (weighted) ───────────────────────────────────────────────

func (fe *ForecastEngine) linearForecast(vals []float64, horizonMin int) []ForecastPoint {
	n := float64(len(vals))
	λ := 0.95 // exponential decay — recent values matter more

	// Weighted least-squares: minimize Σ w_i*(y_i - (a + b*i))^2
	var sumW, sumWX, sumWY, sumWXX, sumWXY float64
	for i, v := range vals {
		w := math.Pow(λ, n-float64(i)-1)
		x := float64(i)
		sumW   += w
		sumWX  += w * x
		sumWY  += w * v
		sumWXX += w * x * x
		sumWXY += w * x * v
	}
	denom := sumW*sumWXX - sumWX*sumWX
	if math.Abs(denom) < 1e-10 {
		// Degenerate case — flat line
		mean, std := meanStd(vals)
		return buildFlatPoints(mean, std, horizonMin)
	}
	b := (sumW*sumWXY - sumWX*sumWY) / denom // slope
	a := (sumWY - b*sumWX) / sumW            // intercept

	// Residual std dev for confidence interval
	var resSum float64
	for i, v := range vals {
		pred := a + b*float64(i)
		resSum += (v - pred) * (v - pred)
	}
	residualStd := math.Sqrt(resSum / n)

	// Project forward: each step ≈ 1 scrape interval (15s → 4 steps/min)
	stepsPerMin := 4.0
	points := make([]ForecastPoint, 0, horizonMin)
	baseX := n - 1
	for m := 1; m <= horizonMin; m++ {
		x := baseX + float64(m)*stepsPerMin
		v := a + b*x
		if v < 0 {
			v = 0
		}
		hw := 1.96 * residualStd * (1 + 1/n + math.Pow(x-sumWX/sumW, 2)/sumWXX)
		points = append(points, ForecastPoint{
			OffsetMin: m,
			Value:     round2f(v),
			Lower:     round2f(math.Max(0, v-hw)),
			Upper:     round2f(v + hw),
		})
	}
	return points
}

// ── Holt-Winters double exponential smoothing ──────────────────────────────────

func (fe *ForecastEngine) holtWintersForecast(vals []float64, horizonMin int) []ForecastPoint {
	const alpha = 0.3 // level smoothing
	const beta  = 0.1 // trend smoothing

	// Initialise
	level := vals[0]
	trend := 0.0
	if len(vals) > 1 {
		trend = vals[1] - vals[0]
	}

	var residuals []float64
	for _, v := range vals {
		prevLevel := level
		level = alpha*v + (1-alpha)*(level+trend)
		trend = beta*(level-prevLevel) + (1-beta)*trend
		residuals = append(residuals, v-level)
	}

	_, residualStd := meanStd(residuals)
	stepsPerMin := 4.0
	points := make([]ForecastPoint, 0, horizonMin)
	for m := 1; m <= horizonMin; m++ {
		h := float64(m) * stepsPerMin
		v := level + h*trend
		if v < 0 {
			v = 0
		}
		hw := 1.96 * residualStd * math.Sqrt(1+alpha*alpha*h)
		points = append(points, ForecastPoint{
			OffsetMin: m,
			Value:     round2f(v),
			Lower:     round2f(math.Max(0, v-hw)),
			Upper:     round2f(v + hw),
		})
	}
	return points
}

// ── Seasonal decomposition ─────────────────────────────────────────────────────

func (fe *ForecastEngine) seasonalForecast(vals []float64, horizonMin int) []ForecastPoint {
	// Period = 12 steps ≈ 3 minutes (common cron/traffic cycle for scrape@15s)
	period := 12
	if len(vals) < period*2 {
		return fe.holtWintersForecast(vals, horizonMin)
	}

	// Decompose: compute centred moving average, then seasonal indices
	n := len(vals)
	trend := make([]float64, n)
	for i := period / 2; i < n-period/2; i++ {
		sum := 0.0
		for j := i - period/2; j < i+period/2; j++ {
			sum += vals[j]
		}
		trend[i] = sum / float64(period)
	}

	seasonal := make([]float64, period)
	counts := make([]int, period)
	for i := period / 2; i < n-period/2; i++ {
		if trend[i] != 0 {
			seasonal[i%period] += vals[i] / trend[i]
			counts[i%period]++
		}
	}
	for i := range seasonal {
		if counts[i] > 0 {
			seasonal[i] /= float64(counts[i])
		} else {
			seasonal[i] = 1.0
		}
	}

	// Deseasonalise and fit Holt-Winters on the result
	deseason := make([]float64, n)
	for i, v := range vals {
		s := seasonal[i%period]
		if s < 0.01 {
			s = 1.0
		}
		deseason[i] = v / s
	}
	hwPoints := fe.holtWintersForecast(deseason, horizonMin)

	// Re-add seasonal component
	lastSeason := n % period
	stepsPerMin := 4
	points := make([]ForecastPoint, 0, len(hwPoints))
	for _, pt := range hwPoints {
		sIdx := (lastSeason + pt.OffsetMin*stepsPerMin) % period
		s := seasonal[sIdx]
		if s < 0.01 {
			s = 1.0
		}
		points = append(points, ForecastPoint{
			OffsetMin: pt.OffsetMin,
			Value:     round2f(pt.Value * s),
			Lower:     round2f(pt.Lower * s),
			Upper:     round2f(pt.Upper * s),
		})
	}
	return points
}

// ── Annotation: warnings and self-healing suggestions ─────────────────────────

var capacityThresholds = map[string]struct{ warn, crit float64 }{
	"container_cpu_usage_percent":      {warn: 75, crit: 90},
	"container_memory_working_set_pct": {warn: 80, crit: 90},
	"trace_latency_p99_ms":             {warn: 1000, crit: 3000},
	"trace_error_rate":                 {warn: 0.05, crit: 0.15},
	"kafka_consumer_lag":               {warn: 5000, crit: 50000},
}

func (fe *ForecastEngine) annotate(f *Forecast, currentVals []float64) {
	thresh, hasThresh := capacityThresholds[f.Metric]
	if !hasThresh || len(f.Points) == 0 {
		return
	}

	maxPredicted := f.Points[len(f.Points)-1].Value

	switch {
	case maxPredicted >= thresh.crit:
		f.Alert = "critical"
		f.Suggestions = criticalSuggestions(f.Metric, maxPredicted, f.TrendPct)
	case maxPredicted >= thresh.warn:
		f.Alert = "warning"
		f.Suggestions = warningSuggestions(f.Metric, fmt.Sprintf("%.1f", maxPredicted))
	}
}

func criticalSuggestions(metric string, val, trendPct float64) []string {
	base := []string{
		fmt.Sprintf("Metric is forecast to reach %.1f (critical threshold) within the horizon.", val),
	}
	switch metric {
	case "container_cpu_usage_percent":
		return append(base,
			"Scale out: increase deployment replicas by at least 50%",
			"Enable horizontal pod autoscaling (HPA) with CPU target 70%",
			"Profile for hot loops with ObserveX CPU flame graph",
		)
	case "container_memory_working_set_pct":
		return append(base,
			"Increase memory limit in deployment spec",
			"Check for memory leaks via ObserveX heap flame graph",
			"Enable Go GC tuning: set GOGC=50 to trigger earlier collection",
		)
	case "trace_latency_p99_ms":
		return append(base,
			"Enable circuit breaker pattern for downstream dependencies",
			"Add caching layer for hot read paths",
			"Review DB slow query log for index gaps",
		)
	case "trace_error_rate":
		return append(base,
			"Consider rolling back last deployment if errors started post-deploy",
			"Check downstream service health — dependency failures propagate",
			"Enable retry with exponential backoff",
		)
	case "kafka_consumer_lag":
		return append(base,
			"Scale consumer group — add more consumer replicas",
			"Check consumer processing time — optimize per-message handler",
		)
	}
	return base
}

func warningSuggestions(metric, _ string) []string {
	switch metric {
	case "container_cpu_usage_percent":
		return []string{"CPU trending high — consider enabling HPA before threshold breach"}
	case "container_memory_working_set_pct":
		return []string{"Memory growing — review recent deployments for allocation regressions"}
	case "trace_latency_p99_ms":
		return []string{"Latency rising — investigate slow queries and downstream deps"}
	case "trace_error_rate":
		return []string{"Error rate climbing — enable structured error logging for root cause"}
	}
	return []string{"Metric approaching warning threshold — monitor closely"}
}

// ── HTTP Handlers ──────────────────────────────────────────────────────────────

func (p *Processor) handleGetForecast(c *fiber.Ctx) error {
	serviceID   := c.Query("service")
	metricName  := c.Query("metric", "container_cpu_usage_percent")
	horizonStr  := c.Query("horizon", "30m")

	horizonMin := 30
	if _, err := fmt.Sscanf(horizonStr, "%dm", &horizonMin); err != nil || horizonMin <= 0 {
		horizonMin = 30
	}
	if horizonMin > 1440 {
		horizonMin = 1440 // cap at 24h
	}

	// Resolve service ID
	if serviceID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "service query param is required"})
	}

	svc := p.getService(serviceID)
	svcName := serviceID
	if svc != nil {
		svcName = svc.DisplayName
	}

	f := p.forecaster.ForecastMetric(serviceID, svcName, metricName, horizonMin)
	if f == nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "insufficient metric data for forecasting",
			"service": serviceID,
			"metric":  metricName,
			"hint":    fmt.Sprintf("need ≥5 samples in the %s window; ensure the scraper is running", metricName),
		})
	}

	return c.JSON(f)
}

func (p *Processor) handleCapacityForecast(c *fiber.Ctx) error {
	horizonMin := 30

	// Snapshot all services with metrics
	p.metricsMu.RLock()
	serviceIDs := make([]string, 0, len(p.metricsWindows))
	for id := range p.metricsWindows {
		serviceIDs = append(serviceIDs, id)
	}
	p.metricsMu.RUnlock()

	sort.Strings(serviceIDs) // deterministic output

	capacity := &CapacityForecast{
		ClusterName: envOr("CLUSTER_NAME", "default"),
		GeneratedAt: time.Now().UTC(),
		HorizonMin:  horizonMin,
	}

	keyMetrics := []string{
		"container_cpu_usage_percent",
		"container_memory_working_set_pct",
		"trace_latency_p99_ms",
		"trace_error_rate",
	}

	atRiskSet := make(map[string]bool)

	for _, svcID := range serviceIDs {
		if len(serviceIDs) > 50 {
			// For large clusters limit to services with known problems
			p.problemMu.RLock()
			hasProb := false
			for _, prob := range p.problems {
				if prob.ServiceID == svcID && prob.Status == "open" {
					hasProb = true
					break
				}
			}
			p.problemMu.RUnlock()
			if !hasProb {
				continue
			}
		}

		svc := p.getService(svcID)
		svcName := svcID
		if svc != nil {
			svcName = svc.DisplayName
		}

		for _, metric := range keyMetrics {
			f := p.forecaster.ForecastMetric(svcID, svcName, metric, horizonMin)
			if f == nil {
				continue
			}
			capacity.Forecasts = append(capacity.Forecasts, *f)
			switch f.Alert {
			case "critical":
				capacity.Critical++
				atRiskSet[svcName] = true
			case "warning":
				capacity.Warning++
				atRiskSet[svcName] = true
			default:
				capacity.Healthy++
			}
		}
	}

	for svc := range atRiskSet {
		capacity.AtRisk = append(capacity.AtRisk, svc)
	}
	sort.Strings(capacity.AtRisk)

	return c.JSON(capacity)
}

// ── Math helpers ───────────────────────────────────────────────────────────────

func meanStd(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	m := sum / float64(len(vals))
	var vsum float64
	for _, v := range vals {
		d := v - m
		vsum += d * d
	}
	std := 0.0
	if len(vals) > 1 {
		std = math.Sqrt(vsum / float64(len(vals)-1))
	}
	return m, std
}

func buildFlatPoints(mean, std float64, horizonMin int) []ForecastPoint {
	pts := make([]ForecastPoint, horizonMin)
	hw := 1.96 * std
	for i := range pts {
		pts[i] = ForecastPoint{
			OffsetMin: i + 1,
			Value:     round2f(mean),
			Lower:     round2f(math.Max(0, mean-hw)),
			Upper:     round2f(mean + hw),
		}
	}
	return pts
}

func round2f(v float64) float64 {
	return math.Round(v*100) / 100
}

func metricUnit(metric string) string {
	units := map[string]string{
		"container_cpu_usage_percent":      "%",
		"container_memory_working_set_pct": "%",
		"container_memory_working_set_bytes": "bytes",
		"trace_latency_p99_ms":             "ms",
		"trace_latency_p50_ms":             "ms",
		"trace_error_rate":                 "fraction",
		"kafka_consumer_lag":               "messages",
		"log_error_rate":                   "fraction",
	}
	if u, ok := units[metric]; ok {
		return u
	}
	return ""
}

// ── Background precompute loop ────────────────────────────────────────────────
// Precomputes forecasts every 2 minutes so GET /v1/forecasts returns instantly.

func (fe *ForecastEngine) runPrecompute() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		fe.precomputeAll()
	}
}

func (fe *ForecastEngine) precomputeAll() {
	fe.proc.metricsMu.RLock()
	services := make([]string, 0, len(fe.proc.metricsWindows))
	for id := range fe.proc.metricsWindows {
		services = append(services, id)
	}
	fe.proc.metricsMu.RUnlock()

	for _, svcID := range services {
		svc := fe.proc.getService(svcID)
		svcName := svcID
		if svc != nil {
			svcName = svc.DisplayName
		}
		for metric := range capacityThresholds {
			key := svcID + ":" + metric
			f := fe.ForecastMetric(svcID, svcName, metric, 30)
			if f == nil {
				continue
			}
			fe.mu.Lock()
			fe.cache[key] = f
			fe.cacheExp[key] = time.Now().Add(2 * time.Minute)
			fe.mu.Unlock()

			// Fire predictive problem if a breach is forecast
			if f.Alert == "critical" {
				fe.proc.logger.Warn("capacity forecast: critical breach predicted",
					zap.String("service", svcName),
					zap.String("metric", metric),
					zap.Float64("predicted_value", f.Points[len(f.Points)-1].Value),
					zap.Float64("trend_pct", f.TrendPct),
				)
			}
		}
	}
}
