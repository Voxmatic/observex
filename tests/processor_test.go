package tests

import (
	"math"
	"testing"
)

// ══ Anomaly Detection ════════════════════════════════════════════════════════

// Z-score based anomaly detection (mirrors processor's runAnomalyDetection)
func zScore(value, mean, stddev float64) float64 {
	if stddev == 0 { return 0 }
	return math.Abs(value-mean) / stddev
}

func mean(data []float64) float64 {
	if len(data) == 0 { return 0 }
	sum := 0.0
	for _, v := range data { sum += v }
	return sum / float64(len(data))
}

func stddev(data []float64) float64 {
	if len(data) < 2 { return 0 }
	m := mean(data)
	sum := 0.0
	for _, v := range data { sum += (v - m) * (v - m) }
	return math.Sqrt(sum / float64(len(data)-1))
}

func TestAnomalyDetection_NormalData(t *testing.T) {
	// 59 normal values + 1 spike
	data := make([]float64, 60)
	for i := range data { data[i] = 50.0 + float64(i%5) } // 50-54 range
	baseline := data[:55]

	m  := mean(baseline)
	sd := stddev(baseline)

	normal := data[56] // ~52 — normal
	z := zScore(normal, m, sd)
	if z > 3.0 { t.Errorf("normal value flagged as anomaly: z=%.2f", z) }
	t.Logf("✅ Normal value z-score: %.2f (threshold: 3.0)", z)
}

func TestAnomalyDetection_Spike(t *testing.T) {
	data := make([]float64, 60)
	for i := range data { data[i] = 50.0 }
	data[59] = 500.0 // 10x spike

	baseline := data[:55]
	m  := mean(baseline)
	sd := stddev(baseline)
	if sd == 0 { sd = 1 }

	spike := data[59]
	z := zScore(spike, m, sd)
	if z <= 3.0 { t.Errorf("spike not detected: z=%.2f (should be >3.0)", z) }
	t.Logf("✅ Spike detected: value=%.0f z-score=%.2f (threshold: 3.0)", spike, z)
}

func TestAnomalyDetection_ThresholdBoundary(t *testing.T) {
	// Test exactly at the boundary
	data := []float64{100,100,100,100,100,100,100,100,100,100}
	m   := mean(data)
	sd  := stddev(data)
	if sd == 0 { sd = 1 } // prevent division by zero

	// Value 3× stddev above mean
	borderline := m + 3.0*sd
	z := zScore(borderline+0.01, m, sd) // just over boundary
	if z <= 3.0 { t.Errorf("borderline anomaly not detected: z=%.4f", z) }
	t.Logf("✅ Boundary detection: z=%.4f for borderline value", z)
}

// ══ SLO Burn Rate (multi-window Google SRE model) ════════════════════════════

type Window struct {
	Name      string
	Duration  float64 // hours
	BurnFactor float64 // multiplier
}

var Windows = []Window{
	{"1h",  1,   14.4},
	{"5m",  0.083, 14.4},
	{"6h",  6,   6.0},
	{"30m", 0.5,  6.0},
	{"3d",  72,  1.0},
	{"6h_slow", 6, 1.0},
}

func burnRate(errorPct, targetPct float64) float64 {
	budget := 100.0 - targetPct
	if budget <= 0 { return 0 }
	return errorPct / budget
}

func TestSLOBurnRate_Haiku(t *testing.T) {
	// SLO: TTFT p99 < 500ms for 99.9% of requests
	// Current: 1.2% of requests exceed threshold
	errPct   := 1.2
	targetPct := 99.9
	burn     := burnRate(errPct, targetPct)

	if burn <= 1.0 { t.Errorf("burn rate %.1f should be >1 (SLO at risk)", burn) }
	t.Logf("✅ Haiku TTFT SLO burn rate: %.1fx (error=%.1f%% budget=%.1f%%)", burn, errPct, 100-targetPct)
}

func TestSLOBurnRate_FastBurn_Alert(t *testing.T) {
	// Fast burn alert: burn > 14.4x over 1h window means 100% budget in 30 days
	cases := []struct {
		name    string
		burn    float64
		alert   bool
	}{
		{"safe",       0.5,  false},
		{"watch",      2.0,  false},
		{"warn",       6.0,  false},
		{"fast-burn",  14.4, true},  // 14.4x = alert
		{"critical",   28.0, true},
	}
	for _, c := range cases {
		shouldAlert := c.burn >= 14.4
		if shouldAlert != c.alert {
			t.Errorf("%s: burn=%.1f shouldAlert=%v got=%v", c.name, c.burn, c.alert, shouldAlert)
		}
	}
	t.Log("✅ Multi-window burn rate alerting: all thresholds correct")
}

func TestSLOBurnRate_ErrorBudget(t *testing.T) {
	// 30-day error budget for 99.9% SLO = 0.1% = 43.2 min
	target := 99.9
	budget := (100.0 - target) / 100.0 // 0.001
	monthMinutes := 30.0 * 24 * 60     // 43,200 minutes
	budgetMinutes := budget * monthMinutes
	
	if math.Abs(budgetMinutes - 43.2) > 0.1 {
		t.Errorf("error budget: got %.1f min want 43.2 min", budgetMinutes)
	}
	t.Logf("✅ 30-day error budget for %.1f%% SLO: %.1f minutes", target, budgetMinutes)
}

// ══ Regression / Forecast ════════════════════════════════════════════════════

func TestLinearRegression_Upward(t *testing.T) {
	// Simulate CPU usage trending up
	xs := []float64{1,2,3,4,5,6,7,8,9,10}
	ys := []float64{20,22,24,26,28,30,32,34,36,38}

	n := float64(len(xs))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	for i := range xs {
		sumX  += xs[i]
		sumY  += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
	}
	slope     := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	intercept := (sumY - slope*sumX) / n

	// Predict next value
	predicted := slope*11 + intercept
	expected  := 40.0

	if math.Abs(predicted - expected) > 0.5 {
		t.Errorf("regression prediction: got %.1f want %.1f", predicted, expected)
	}
	if slope <= 0 { t.Error("slope should be positive for upward trend") }
	t.Logf("✅ Linear regression: slope=%.2f intercept=%.2f predicted[11]=%.1f", slope, intercept, predicted)
}

func TestLinearRegression_Flat(t *testing.T) {
	xs := []float64{1,2,3,4,5,6,7,8,9,10}
	ys := []float64{50,50,50,50,50,50,50,50,50,50}

	n := float64(len(xs))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	for i := range xs { sumX += xs[i]; sumY += ys[i]; sumXY += xs[i]*ys[i]; sumX2 += xs[i]*xs[i] }
	slope     := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	intercept := (sumY - slope*sumX) / n
	predicted  := slope*11 + intercept

	if math.Abs(predicted - 50.0) > 0.01 { t.Errorf("flat line: predicted %.2f want 50.0", predicted) }
	if math.Abs(slope) > 0.01             { t.Errorf("flat slope: %.4f want ~0", slope) }
	t.Logf("✅ Flat line regression: slope=%.4f predicted=%.1f", slope, predicted)
}

// ══ Synthetic Check Types ════════════════════════════════════════════════════

func TestSyntheticCheck_HTTPStatusValidation(t *testing.T) {
	type CheckResult struct {
		URL          string
		ExpectedCode int
		ActualCode   int
		DurationMs   float64
		Success      bool
	}

	results := []CheckResult{
		{"https://api.corp.io/health", 200, 200, 42.4, true},
		{"https://api.corp.io/checkout", 200, 503, 0, false},
		{"https://api.corp.io/admin", 401, 401, 12.4, true},
	}

	for _, r := range results {
		success := r.ExpectedCode == r.ActualCode
		if success != r.Success {
			t.Errorf("URL %s: success=%v want %v", r.URL, success, r.Success)
		}
	}
	t.Log("✅ Synthetic HTTP status validation: all cases correct")
}

func TestSyntheticCheck_SSLExpiry(t *testing.T) {
	// SSL check: alert if cert expires within 14 days
	cases := []struct{ daysLeft int; shouldAlert bool }{
		{180, false}, {30, false}, {14, true}, {7, true}, {0, true},
	}
	threshold := 14
	for _, c := range cases {
		alert := c.daysLeft <= threshold
		if alert != c.shouldAlert {
			t.Errorf("daysLeft=%d: alert=%v want %v", c.daysLeft, alert, c.shouldAlert)
		}
	}
	t.Log("✅ SSL expiry alert threshold (14 days): all cases correct")
}

func TestSyntheticCheck_DNSResolution(t *testing.T) {
	// DNS check validates response time < threshold
	cases := []struct{ latencyMs float64; threshold float64; ok bool }{
		{8.4,   100, true},
		{124.0, 100, false},
		{99.9,  100, true},
		{100.1, 100, false},
	}
	for _, c := range cases {
		ok := c.latencyMs <= c.threshold
		if ok != c.ok {
			t.Errorf("latency=%.1f threshold=%.0f: ok=%v want %v", c.latencyMs, c.threshold, ok, c.ok)
		}
	}
	t.Log("✅ DNS latency threshold checks: all cases correct")
}

// ══ Alert Evaluation ═════════════════════════════════════════════════════════

func TestAlertEval_ThresholdCrossing(t *testing.T) {
	type AlertRule struct {
		Name      string
		Threshold float64
		Operator  string // "gt" or "lt"
		Severity  string
	}

	rules := []AlertRule{
		{"CPU > 80%",           80.0, "gt", "HIGH"},
		{"Error rate > 1%",     1.0,  "gt", "CRITICAL"},
		{"Availability < 99.9%",99.9, "lt", "CRITICAL"},
	}

	testValues := []struct {
		value   float64
		rule    AlertRule
		fires   bool
	}{
		{84.2,  rules[0], true},   // CPU 84 > 80 → fires
		{72.4,  rules[0], false},  // CPU 72 < 80 → no
		{1.8,   rules[1], true},   // error 1.8 > 1 → fires
		{0.08,  rules[1], false},  // error 0.08 < 1 → no
		{99.84, rules[2], true},   // avail 99.84 < 99.9 → fires
		{99.94, rules[2], false},  // avail 99.94 > 99.9 → no
	}

	for _, tv := range testValues {
		var fires bool
		switch tv.rule.Operator {
		case "gt": fires = tv.value > tv.rule.Threshold
		case "lt": fires = tv.value < tv.rule.Threshold
		}
		if fires != tv.fires {
			t.Errorf("rule=%q value=%.2f: fires=%v want %v", tv.rule.Name, tv.value, fires, tv.fires)
		}
	}
	t.Log("✅ Alert threshold evaluation: all 6 cases correct (gt/lt operators)")
}

func TestAlertEval_Inhibition(t *testing.T) {
	// If a parent alert fires, child alerts should be inhibited
	activeAlerts := map[string]bool{
		"NodeDown": true,
	}
	inhibitRules := map[string]string{
		"PodCrashLoopBackOff": "NodeDown",
		"ServiceUnavailable":  "NodeDown",
		"HighLatency":         "",         // no inhibitor
	}

	shouldFire := func(alert string) bool {
		if inhibitor, ok := inhibitRules[alert]; ok && inhibitor != "" {
			if activeAlerts[inhibitor] { return false } // inhibited
		}
		return true
	}

	cases := []struct{ alert string; expected bool }{
		{"PodCrashLoopBackOff", false}, // inhibited by NodeDown
		{"ServiceUnavailable",  false}, // inhibited by NodeDown
		{"HighLatency",         true},  // not inhibited
		{"CPUHigh",             true},  // not in inhibit rules
	}
	for _, c := range cases {
		if shouldFire(c.alert) != c.expected {
			t.Errorf("alert=%q: shouldFire=%v want %v", c.alert, shouldFire(c.alert), c.expected)
		}
	}
	t.Log("✅ Alert inhibition rules: NodeDown correctly silences child alerts")
}
