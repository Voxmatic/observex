// services/processor/ml_cost_forecast.go
//
// LLM Cost & Token Forecasting Engine.
//
// Extends the existing ForecastEngine for LLM-specific predictions:
//   - Token consumption rate (input + output per model)
//   - Cost per hour / day / month with 95% confidence bands
//   - Budget burn-rate: how fast error budget depletes at current cost trend
//   - Anomaly detection on cost spikes (z-score on 1h rolling window)
//   - Automated suggestions: model downgrade, caching, batching
//
// Data source: native ObserveX metric rows for llm_input_tokens_total and
//              llm_output_tokens_total written by the ingestor.
//
// Algorithm: Holt-Winters Double Exponential Smoothing (same as forecast.go)
// with α=0.4 (level) β=0.15 (trend) for token series. Cost is derived by
// multiplying token forecasts by model pricing.

package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type TokenSeries struct {
	Model        string
	InputRates   []float64 // tokens/min sampled every 5 min
	OutputRates  []float64
	CostRates    []float64 // USD/hour
	SampledAt    []time.Time
}

type CostForecastPoint struct {
	OffsetMin   int     `json:"offset_min"`
	CostUSD     float64 `json:"cost_usd"`
	Lower95     float64 `json:"lower_95"`
	Upper95     float64 `json:"upper_95"`
	InputTokens int64   `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

type ModelCostForecast struct {
	ModelID          string              `json:"model_id"`
	OrgID            string              `json:"org_id,omitempty"`
	HorizonHours     int                 `json:"horizon_hours"`
	Method           string              `json:"method"`
	SampleCount      int                 `json:"sample_count"`
	CurrentRatePerHr float64             `json:"current_rate_per_hr_usd"`
	ForecastTotal    float64             `json:"forecast_total_usd"`
	Lower95Total     float64             `json:"lower_95_total_usd"`
	Upper95Total     float64             `json:"upper_95_total_usd"`
	Trend            string              `json:"trend"`
	TrendPctPerDay   float64             `json:"trend_pct_per_day"`
	Points           []CostForecastPoint `json:"points"`
	Anomalies        []CostAnomaly       `json:"anomalies,omitempty"`
	Suggestions      []string            `json:"suggestions,omitempty"`
	BudgetBurnRateX  float64             `json:"budget_burn_rate_x,omitempty"`
	GeneratedAt      time.Time           `json:"generated_at"`
}

type CostAnomaly struct {
	DetectedAt time.Time `json:"detected_at"`
	ModelID    string    `json:"model_id"`
	ZScore     float64   `json:"z_score"`
	ActualCost float64   `json:"actual_cost_usd"`
	Expected   float64   `json:"expected_cost_usd"`
	Severity   string    `json:"severity"` // LOW / MEDIUM / HIGH
}

// Model pricing table (USD per 1M tokens)
var modelPricing = map[string][2]float64{
	"claude-haiku-4-5":  {0.80, 4.00},
	"claude-sonnet-4-6": {3.00, 15.00},
	"claude-opus-4-6":   {15.00, 75.00},
}

// ── Engine ────────────────────────────────────────────────────────────────────

type MLCostForecaster struct {
	p       *Processor
	mu      sync.RWMutex
	series  map[string]*TokenSeries // keyed by model
	latest  map[string]*ModelCostForecast
	metricStore nativeMetricStore
	ticker  *time.Ticker
	quit    chan struct{}
}

func newMLCostForecaster(p *Processor) *MLCostForecaster {
	return &MLCostForecaster{
		p:      p,
		series: make(map[string]*TokenSeries),
		latest: make(map[string]*ModelCostForecast),
		metricStore: newNativeMetricStore(p),
		quit:   make(chan struct{}),
	}
}

func (f *MLCostForecaster) Run() {
	f.ticker = time.NewTicker(5 * time.Minute)
	defer f.ticker.Stop()
	// First run immediately
	f.collect()
	f.recompute()
	for {
		select {
		case <-f.ticker.C:
			f.collect()
			f.recompute()
		case <-f.quit:
			return
		}
	}
}

func (f *MLCostForecaster) Stop() { close(f.quit) }

// collect samples the last five minutes of native token counters per model.
func (f *MLCostForecaster) collect() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	models := []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-6"}

	for _, modelID := range models {
		inputRate, err := f.metricStore.eventRatePerMinute(ctx, "llm_input_tokens_total", "model", modelID, 5*time.Minute)
		if err != nil {
			f.p.logger.Debug("native input-token query failed", zap.String("model", modelID), zap.Error(err))
			inputRate = 0
		}
		outputRate, err := f.metricStore.eventRatePerMinute(ctx, "llm_output_tokens_total", "model", modelID, 5*time.Minute)
		if err != nil {
			f.p.logger.Debug("native output-token query failed", zap.String("model", modelID), zap.Error(err))
			outputRate = 0
		}
		// Compute cost rate from token rates
		pricing, ok := modelPricing[modelID]
		costRate := 0.0
		if ok {
			costRate = (inputRate/1_000_000*pricing[0] + outputRate/1_000_000*pricing[1]) * 60 // USD/hr
		}

		f.mu.Lock()
		s, exists := f.series[modelID]
		if !exists {
			s = &TokenSeries{Model: modelID}
			f.series[modelID] = s
		}
		// Keep 288 samples = 24h at 5-min intervals
		if len(s.InputRates) >= 288 {
			s.InputRates  = s.InputRates[1:]
			s.OutputRates = s.OutputRates[1:]
			s.CostRates   = s.CostRates[1:]
			s.SampledAt   = s.SampledAt[1:]
		}
		s.InputRates  = append(s.InputRates, inputRate)
		s.OutputRates = append(s.OutputRates, outputRate)
		s.CostRates   = append(s.CostRates, costRate)
		s.SampledAt   = append(s.SampledAt, time.Now())
		f.mu.Unlock()
	}
}

// recompute regenerates all forecasts and anomaly detections.
func (f *MLCostForecaster) recompute() {
	f.mu.RLock()
	models := make([]string, 0, len(f.series))
	for k := range f.series { models = append(models, k) }
	f.mu.RUnlock()

	for _, modelID := range models {
		fc := f.forecastModel(modelID, 24)
		f.mu.Lock()
		f.latest[modelID] = fc
		f.mu.Unlock()
	}
}

// forecastModel runs Holt-Winters on the cost series and returns a forecast.
func (f *MLCostForecaster) forecastModel(modelID string, horizonHours int) *ModelCostForecast {
	f.mu.RLock()
	s, ok := f.series[modelID]
	if !ok || len(s.CostRates) < 3 {
		f.mu.RUnlock()
		return &ModelCostForecast{
			ModelID: modelID, HorizonHours: horizonHours,
			Method: "insufficient_data", GeneratedAt: time.Now(),
		}
	}
	rates := make([]float64, len(s.CostRates))
	copy(rates, s.CostRates)
	f.mu.RUnlock()

	n := len(rates)
	method := "linear"

	// ── Holt-Winters Double Exponential Smoothing ──────────────────────────────
	alpha, beta := 0.4, 0.15
	level := rates[0]
	trend := 0.0
	if n > 1 { trend = rates[1] - rates[0] }

	smoothed := make([]float64, n)
	for i, v := range rates {
		prevLevel := level
		level = alpha*v + (1-alpha)*(level+trend)
		trend = beta*(level-prevLevel) + (1-beta)*trend
		smoothed[i] = level + trend
	}
	if n >= 30 { method = "holt_winters" }

	// ── Generate forecast points (one per hour) ────────────────────────────────
	// Estimate residual std dev for confidence intervals
	sumSqErr := 0.0
	for i, v := range rates {
		diff := v - smoothed[i]
		sumSqErr += diff * diff
	}
	stddev := math.Sqrt(sumSqErr / float64(n))
	z95 := 1.96 // z-score for 95% CI

	stepsPerHour := 12 // 5-min intervals
	points := make([]CostForecastPoint, 0, horizonHours)
	totalCost := 0.0
	totalLower := 0.0
	totalUpper := 0.0

	for h := 1; h <= horizonHours; h++ {
		steps := float64(h * stepsPerHour)
		predicted := level + trend*steps
		if predicted < 0 { predicted = 0 }
		ci := z95 * stddev * math.Sqrt(steps)
		lower := math.Max(0, predicted-ci)
		upper := predicted + ci

		// Convert USD/hr to USD for this hour
		pricing := modelPricing[modelID]
		inputRate := (level + trend*float64((h-1)*stepsPerHour))
		if inputRate < 0 { inputRate = 0 }
		// Rough token split: assume 70% of cost is output
		inputTokens := int64(inputRate * 0.3 / (pricing[0] / 1_000_000) / 60)
		outputTokens := int64(inputRate * 0.7 / (pricing[1] / 1_000_000) / 60)
		if pricing[0] == 0 || pricing[1] == 0 { inputTokens, outputTokens = 0, 0 }

		points = append(points, CostForecastPoint{
			OffsetMin:    h * 60,
			CostUSD:      math.Round(predicted*10000) / 10000,
			Lower95:      math.Round(lower*10000) / 10000,
			Upper95:      math.Round(upper*10000) / 10000,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		})
		totalCost  += predicted
		totalLower += lower
		totalUpper += upper
	}

	// ── Trend classification ───────────────────────────────────────────────────
	trendStr := "stable"
	trendPct := 0.0
	if trend > 0.001 {
		trendStr = "rising"
		trendPct = (trend * float64(stepsPerHour*24)) / level * 100
	} else if trend < -0.001 {
		trendStr = "falling"
		trendPct = (trend * float64(stepsPerHour*24)) / level * 100
	}

	// ── Anomaly detection via z-score ─────────────────────────────────────────
	anomalies := f.detectAnomalies(modelID, rates, smoothed)

	// ── Budget suggestions ─────────────────────────────────────────────────────
	suggestions := f.generateSuggestions(modelID, level, trend, trendPct, anomalies)

	return &ModelCostForecast{
		ModelID:          modelID,
		HorizonHours:     horizonHours,
		Method:           method,
		SampleCount:      n,
		CurrentRatePerHr: math.Round(level*10000) / 10000,
		ForecastTotal:    math.Round(totalCost*100) / 100,
		Lower95Total:     math.Round(totalLower*100) / 100,
		Upper95Total:     math.Round(totalUpper*100) / 100,
		Trend:            trendStr,
		TrendPctPerDay:   math.Round(trendPct*100) / 100,
		Points:           points,
		Anomalies:        anomalies,
		Suggestions:      suggestions,
		GeneratedAt:      time.Now(),
	}
}

func (f *MLCostForecaster) detectAnomalies(modelID string, rates, smoothed []float64) []CostAnomaly {
	if len(rates) < 12 { return nil }
	// Compute residuals and their stats over last 60 samples (5h)
	window := rates
	if len(window) > 60 { window = window[len(window)-60:] }
	mean, variance := 0.0, 0.0
	for _, v := range window { mean += v }
	mean /= float64(len(window))
	for _, v := range window { d := v - mean; variance += d * d }
	stddev := math.Sqrt(variance / float64(len(window)))
	if stddev < 0.0001 { return nil }

	var anomalies []CostAnomaly
	for i := len(rates) - 12; i < len(rates); i++ {
		if i < 0 { continue }
		z := math.Abs(rates[i]-smoothed[i]) / stddev
		if z < 2.5 { continue }
		sev := "LOW"
		if z > 4.0 { sev = "HIGH" } else if z > 3.0 { sev = "MEDIUM" }
		f.mu.RLock()
		ts := time.Now()
		if i < len(f.series[modelID].SampledAt) { ts = f.series[modelID].SampledAt[i] }
		f.mu.RUnlock()
		anomalies = append(anomalies, CostAnomaly{
			DetectedAt: ts, ModelID: modelID,
			ZScore:     math.Round(z*100) / 100,
			ActualCost: rates[i], Expected: smoothed[i], Severity: sev,
		})
	}
	return anomalies
}

func (f *MLCostForecaster) generateSuggestions(modelID string, rate, trend, trendPct float64, anomalies []CostAnomaly) []string {
	var s []string
	if trendPct > 20 {
		s = append(s, fmt.Sprintf("Cost rising %.1f%%/day — review request volume or enable caching", trendPct))
	}
	if rate > 50 { // > $50/hr
		s = append(s, "High spend rate — consider routing low-complexity tasks to claude-haiku-4-5")
	}
	if modelID == "claude-opus-4-6" && rate > 10 {
		s = append(s, "Opus traffic at scale — verify tasks require Opus-level capability vs Sonnet")
	}
	for _, a := range anomalies {
		if a.Severity == "HIGH" {
			s = append(s, fmt.Sprintf("Cost spike detected (%.1fx normal) — check for runaway loops or batch jobs", a.ZScore/2.5))
		}
	}
	if len(s) == 0 && trendPct > 5 {
		s = append(s, "Gradual cost increase — normal growth, monitor weekly")
	}
	return s
}

// ── HTTP Handlers ─────────────────────────────────────────────────────────────

func (p *Processor) handleMLCostForecast(c *fiber.Ctx) error {
	model  := c.Query("model", "")
	hours  := c.QueryInt("hours", 24)
	if hours < 1 { hours = 1 }
	if hours > 720 { hours = 720 }

	p.mlCostForecaster.mu.RLock()
	defer p.mlCostForecaster.mu.RUnlock()

	if model != "" {
		fc, ok := p.mlCostForecaster.latest[model]
		if !ok {
			return c.Status(404).JSON(fiber.Map{"error": "no forecast available for model", "model": model})
		}
		return c.JSON(fc)
	}

	// Return all models sorted by cost
	all := make([]*ModelCostForecast, 0, len(p.mlCostForecaster.latest))
	for _, fc := range p.mlCostForecaster.latest { all = append(all, fc) }
	sort.Slice(all, func(i, j int) bool {
		return all[i].ForecastTotal > all[j].ForecastTotal
	})
	total := 0.0
	for _, fc := range all { total += fc.ForecastTotal }

	return c.JSON(fiber.Map{
		"horizon_hours":   hours,
		"models":          all,
		"total_forecast":  math.Round(total*100) / 100,
		"generated_at":    time.Now(),
	})
}

func (p *Processor) handleMLCostAnomalies(c *fiber.Ctx) error {
	p.mlCostForecaster.mu.RLock()
	defer p.mlCostForecaster.mu.RUnlock()

	var all []CostAnomaly
	for _, fc := range p.mlCostForecaster.latest {
		all = append(all, fc.Anomalies...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ZScore > all[j].ZScore
	})

	return c.JSON(fiber.Map{
		"anomalies": all,
		"total":     len(all),
		"scanned_at": time.Now(),
	})
}

func (p *Processor) handleMLTokenForecast(c *fiber.Ctx) error {
	model := c.Query("model", "claude-sonnet-4-6")
	hours := c.QueryInt("hours", 24)

	p.mlCostForecaster.mu.RLock()
	s, ok := p.mlCostForecaster.series[model]
	if !ok || len(s.InputRates) < 3 {
		p.mlCostForecaster.mu.RUnlock()
		return c.JSON(fiber.Map{
			"model":   model,
			"message": "collecting data — check back in 15 minutes",
			"samples": 0,
		})
	}
	n := len(s.InputRates)
	inputRates  := make([]float64, n)
	outputRates := make([]float64, n)
	copy(inputRates, s.InputRates)
	copy(outputRates, s.OutputRates)
	p.mlCostForecaster.mu.RUnlock()

	// Holt-Winters on token rates
	hwForecast := func(series []float64) (level, trend float64, points []float64) {
		alpha, beta := 0.4, 0.15
		level = series[0]
		if len(series) > 1 { trend = series[1] - series[0] }
		for _, v := range series {
			prev := level
			level = alpha*v + (1-alpha)*(level+trend)
			trend = beta*(level-prev) + (1-beta)*trend
		}
		for h := 1; h <= hours*12; h++ { // 5-min steps
			v := level + trend*float64(h)
			if v < 0 { v = 0 }
			points = append(points, v)
		}
		return
	}

	inLevel, inTrend, inPoints := hwForecast(inputRates)
	outLevel, outTrend, outPoints := hwForecast(outputRates)

	type TokenPoint struct {
		OffsetMin    int     `json:"offset_min"`
		InputPerMin  float64 `json:"input_tokens_per_min"`
		OutputPerMin float64 `json:"output_tokens_per_min"`
	}
	pts := make([]TokenPoint, 0, hours)
	step := 12 // sample hourly
	for i := 0; i < len(inPoints) && i < len(outPoints); i += step {
		pts = append(pts, TokenPoint{
			OffsetMin:    (i/12 + 1) * 60,
			InputPerMin:  math.Round(inPoints[i]*10) / 10,
			OutputPerMin: math.Round(outPoints[i]*10) / 10,
		})
	}

	return c.JSON(fiber.Map{
		"model":              model,
		"horizon_hours":      hours,
		"sample_count":       n,
		"method":             "holt_winters",
		"current_input_rpm":  math.Round(inLevel*10) / 10,
		"current_output_rpm": math.Round(outLevel*10) / 10,
		"input_trend":        math.Round(inTrend*1000) / 1000,
		"output_trend":       math.Round(outTrend*1000) / 1000,
		"points":             pts,
		"generated_at":       time.Now(),
	})
}

// Helper used by processor main
func (p *Processor) handleCostForecastSummary(c *fiber.Ctx) error {
	return p.handleMLCostForecast(c)
}

// logForecaster logs the current forecast state
func (f *MLCostForecaster) logSummary(log *zap.Logger) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for model, fc := range f.latest {
		if fc.SampleCount < 3 { continue }
		log.Info("cost forecast updated",
			zap.String("model", model),
			zap.Float64("rate_per_hr", fc.CurrentRatePerHr),
			zap.Float64("forecast_24h", fc.ForecastTotal),
			zap.String("trend", fc.Trend),
			zap.Int("anomalies", len(fc.Anomalies)),
		)
	}
}
