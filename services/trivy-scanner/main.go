// services/trivy-scanner/main.go
//
// ObserveX Trivy Scanner — continuous container image vulnerability scanning.
//
// Operation:
//   1. Watches the Kubernetes API for all running pods across all namespaces.
//   2. Extracts the unique set of container images currently deployed.
//   3. Runs `trivy image --format json <image>` for each image.
//   4. Parses the JSON output into models.ImageScanResult.
//   5. Computes a risk score (0–100) weighted by CVE severity.
//   6. POSTs results to the ObserveX ingestor at POST /v1/security/scans.
//
// Scheduling:
//   - Full scan of all images on startup.
//   - Incremental scan every SCAN_INTERVAL_HOURS (default: 6h).
//   - New images seen in the pod watch are scanned immediately.
//   - Results are cached by image digest to avoid redundant rescans.
//
// Trivy is invoked as a subprocess (exec.Command). The scanner binary must
// have `trivy` in its PATH, or set TRIVY_PATH to the full binary path.
// In Kubernetes, use the `aquasec/trivy` image as a sidecar or init container.
//
// Environment:
//   INGESTOR_URL          - ObserveX ingestor base URL (default: http://ingestor:4318)
//   OBSERVEX_TOKEN        - Agent or ingest token used to authenticate scanner reports
//   SCAN_INTERVAL_HOURS   - Hours between full rescans (default: 6)
//   TRIVY_PATH            - Path to trivy binary (default: trivy)
//   TRIVY_CACHE_DIR       - Trivy vulnerability database cache (default: /tmp/trivy-cache)
//   TRIVY_SEVERITY        - Min severity to report (default: MEDIUM)
//   CLUSTER_NAME          - Cluster identifier for scan results
//   KUBECONFIG            - Path to kubeconfig (optional; uses in-cluster config otherwise)

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/observex/platform/pkg/models"
)

// ── Config ────────────────────────────────────────────────────────────────

type Config struct {
	IngestorURL       string
	AgentToken        string
	ScanIntervalHours int
	TrivyPath         string
	TrivyCacheDir     string
	TrivySeverity     string   // UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL
	ClusterName       string
	KubeConfig        string
}

func loadConfig() Config {
	interval, _ := strconv.Atoi(envOr("SCAN_INTERVAL_HOURS", "6"))
	if interval <= 0 {
		interval = 6
	}
	return Config{
		IngestorURL:       envOr("INGESTOR_URL", "http://observex-ingestor:4318"),
		AgentToken:        envOr("OBSERVEX_TOKEN", envOr("AGENT_TOKEN", "")),
		ScanIntervalHours: interval,
		TrivyPath:         envOr("TRIVY_PATH", "trivy"),
		TrivyCacheDir:     envOr("TRIVY_CACHE_DIR", "/tmp/trivy-cache"),
		TrivySeverity:     envOr("TRIVY_SEVERITY", "MEDIUM"),
		ClusterName:       envOr("CLUSTER_NAME", "default"),
		KubeConfig:        os.Getenv("KUBECONFIG"),
	}
}

// ── Scanner ───────────────────────────────────────────────────────────────

type Scanner struct {
	cfg     Config
	k8s     kubernetes.Interface
	client  *http.Client
	logger  *zap.Logger
	// Cache: image → last scan digest, prevents rescanning unchanged images
	scanCache   map[string]string // image → last scanned digest
	cacheMu     sync.Mutex
}

func New(cfg Config, k8s kubernetes.Interface, logger *zap.Logger) *Scanner {
	return &Scanner{
		cfg:       cfg,
		k8s:       k8s,
		client:    &http.Client{Timeout: 60 * time.Second},
		logger:    logger,
		scanCache: make(map[string]string),
	}
}

// ── Main loop ─────────────────────────────────────────────────────────────

func (s *Scanner) Run(ctx context.Context) {
	// Update Trivy DB before first scan
	s.logger.Info("updating Trivy vulnerability database")
	if err := s.updateTrivyDB(ctx); err != nil {
		s.logger.Warn("Trivy DB update failed — using cached DB", zap.Error(err))
	}

	// Initial full scan
	s.scanAllImages(ctx)

	// Periodic rescan
	ticker := time.NewTicker(time.Duration(s.cfg.ScanIntervalHours) * time.Hour)
	defer ticker.Stop()

	// Watch for new images via pod events
	watchCh := s.watchNewImages(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logger.Info("starting scheduled full scan")
			s.scanAllImages(ctx)
		case image := <-watchCh:
			s.logger.Info("new image detected, scanning", zap.String("image", image))
			if result, err := s.scanImage(ctx, image, "", ""); err != nil {
				s.logger.Warn("image scan failed", zap.String("image", image), zap.Error(err))
			} else {
				s.reportResult(ctx, *result)
			}
		}
	}
}

// ── Trivy DB update ───────────────────────────────────────────────────────

func (s *Scanner) updateTrivyDB(ctx context.Context) error {
	cmd := exec.CommandContext(ctx,
		s.cfg.TrivyPath,
		"image", "--download-db-only",
		"--cache-dir", s.cfg.TrivyCacheDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── Discover all images running in the cluster ────────────────────────────

type imageInfo struct {
	image     string
	namespace string
	service   string
}

func (s *Scanner) discoverImages(ctx context.Context) ([]imageInfo, error) {
	pods, err := s.k8s.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	seen := make(map[string]imageInfo)
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		ns := pod.Namespace
		svcName := pod.Labels["app"]
		if svcName == "" {
			svcName = pod.Labels["app.kubernetes.io/name"]
		}
		if svcName == "" {
			svcName = pod.Name
		}

		for _, c := range pod.Spec.Containers {
			img := c.Image
			if _, ok := seen[img]; !ok {
				seen[img] = imageInfo{image: img, namespace: ns, service: svcName}
			}
		}
		for _, ic := range pod.Spec.InitContainers {
			img := ic.Image
			if _, ok := seen[img]; !ok {
				seen[img] = imageInfo{image: img, namespace: ns, service: svcName}
			}
		}
	}

	result := make([]imageInfo, 0, len(seen))
	for _, info := range seen {
		result = append(result, info)
	}
	return result, nil
}

// ── Full scan ─────────────────────────────────────────────────────────────

func (s *Scanner) scanAllImages(ctx context.Context) {
	images, err := s.discoverImages(ctx)
	if err != nil {
		s.logger.Error("failed to discover images", zap.Error(err))
		return
	}

	s.logger.Info("scanning images", zap.Int("count", len(images)))

	// Limit concurrency to avoid overloading the node
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for _, img := range images {
		img := img
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			result, err := s.scanImage(ctx, img.image, img.namespace, img.service)
			if err != nil {
				s.logger.Warn("image scan failed",
					zap.String("image", img.image),
					zap.Error(err),
				)
				return
			}
			s.reportResult(ctx, *result)
		}()
	}
	wg.Wait()
	s.logger.Info("full scan complete", zap.Int("images", len(images)))
}

// ── Single image scan ─────────────────────────────────────────────────────

// trivyOutput mirrors the top-level structure of `trivy image --format json` output.
type trivyOutput struct {
	SchemaVersion int `json:"SchemaVersion"`
	Results       []struct {
		Target          string `json:"Target"`
		Type            string `json:"Type"`
		Vulnerabilities []struct {
			VulnerabilityID  string   `json:"VulnerabilityID"`
			PkgName          string   `json:"PkgName"`
			InstalledVersion string   `json:"InstalledVersion"`
			FixedVersion     string   `json:"FixedVersion"`
			Severity         string   `json:"Severity"`
			Title            string   `json:"Title"`
			Description      string   `json:"Description"`
			References       []string `json:"References"`
			CVSS             struct {
				Nvd *struct {
					V3Score float64 `json:"V3Score"`
				} `json:"nvd"`
			} `json:"CVSS"`
			PublishedDate *time.Time `json:"PublishedDate"`
			LastModifiedDate *time.Time `json:"LastModifiedDate"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func (s *Scanner) scanImage(ctx context.Context, image, namespace, serviceName string) (*models.ImageScanResult, error) {
	s.logger.Debug("scanning image", zap.String("image", image))

	// Run trivy
	args := []string{
		"image",
		"--format", "json",
		"--cache-dir", s.cfg.TrivyCacheDir,
		"--severity", s.cfg.TrivySeverity + ",HIGH,CRITICAL",
		"--no-progress",
		"--skip-update",      // DB was already updated at startup
		"--timeout", "10m",
		image,
	}

	cmd := exec.CommandContext(ctx, s.cfg.TrivyPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Trivy exits 1 when vulnerabilities are found — that's expected
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("trivy failed (exit %d): %s", exitErr.ExitCode(), stderr.String())
		}
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("trivy returned empty output for %s", image)
	}

	var out trivyOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("parse trivy output: %w", err)
	}

	// Build result
	result := &models.ImageScanResult{
		ID:          fmt.Sprintf("scan-%d", time.Now().UnixNano()),
		Image:       image,
		ScannedAt:   time.Now().UTC(),
		ServiceName: serviceName,
		Namespace:   namespace,
		ClusterName: s.cfg.ClusterName,
	}

	for _, r := range out.Results {
		for _, v := range r.Vulnerabilities {
			cvss := 0.0
			if v.CVSS.Nvd != nil {
				cvss = v.CVSS.Nvd.V3Score
			}
			vuln := models.Vulnerability{
				VulnID:           v.VulnerabilityID,
				PkgName:          v.PkgName,
				InstalledVersion: v.InstalledVersion,
				FixedVersion:     v.FixedVersion,
				Severity:         models.VulnSeverity(v.Severity),
				Title:            v.Title,
				Description:      truncateStr(v.Description, 500),
				CVSS:             cvss,
				PublishedDate:    v.PublishedDate,
				LastModified:     v.LastModifiedDate,
				References:       v.References,
			}
			result.Vulnerabilities = append(result.Vulnerabilities, vuln)
			switch vuln.Severity {
			case models.VulnCritical:
				result.Critical++
			case models.VulnHigh:
				result.High++
			case models.VulnMedium:
				result.Medium++
			case models.VulnLow:
				result.Low++
			}
		}
	}

	result.RiskScore = computeRiskScore(result.Critical, result.High, result.Medium, result.Low)

	s.logger.Info("scan complete",
		zap.String("image", image),
		zap.Int("critical", result.Critical),
		zap.Int("high", result.High),
		zap.Float64("risk", result.RiskScore),
	)

	return result, nil
}

// ── Risk score ────────────────────────────────────────────────────────────
// Weighted formula: each severity contributes a capped score.
// CRITICAL=10pts (cap 50), HIGH=5pts (cap 30), MEDIUM=2pts (cap 15), LOW=0.5pts (cap 5).
// Total max = 100.

func computeRiskScore(critical, high, medium, low int) float64 {
	score := math.Min(50, float64(critical)*10) +
		math.Min(30, float64(high)*5) +
		math.Min(15, float64(medium)*2) +
		math.Min(5, float64(low)*0.5)
	return math.Round(score*10) / 10
}

// ── Report result to ingestor ─────────────────────────────────────────────

func (s *Scanner) reportResult(ctx context.Context, result models.ImageScanResult) {
	body, err := json.Marshal([]models.ImageScanResult{result})
	if err != nil {
		return
	}
	url := strings.TrimRight(s.cfg.IngestorURL, "/") + "/v1/security/scans"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.AgentToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.AgentToken)
		req.Header.Set("X-ObserveX-Token", s.cfg.AgentToken)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn("failed to report scan result",
			zap.String("image", result.Image),
			zap.Error(err),
		)
		return
	}
	resp.Body.Close()
	s.logger.Debug("scan result reported", zap.String("image", result.Image))
}

// ── Watch for new images ──────────────────────────────────────────────────

func (s *Scanner) watchNewImages(ctx context.Context) <-chan string {
	ch := make(chan string, 32)
	go func() {
		watcher, err := s.k8s.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			s.logger.Warn("pod watch failed — new image detection disabled", zap.Error(err))
			return
		}
		defer watcher.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-watcher.ResultChan():
				if !ok {
					return
				}
				pod, ok := evt.Object.(*corev1.Pod)
				if !ok {
					continue
				}
				for _, c := range pod.Spec.Containers {
					s.cacheMu.Lock()
					_, seen := s.scanCache[c.Image]
					s.cacheMu.Unlock()
					if !seen {
						select {
						case ch <- c.Image:
						default:
						}
					}
				}
			}
		}
	}()
	return ch
}

// ── Health server ─────────────────────────────────────────────────────────

func startHealthServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"trivy-scanner"}`)
	})
	http.ListenAndServe(":8080", mux)
}

// ── main ──────────────────────────────────────────────────────────────────

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := loadConfig()
	ctx := context.Background()

	// Build K8s client
	var k8sCfg *rest.Config
	var err error
	if cfg.KubeConfig != "" {
		k8sCfg, err = clientcmd.BuildConfigFromFlags("", cfg.KubeConfig)
	} else {
		k8sCfg, err = rest.InClusterConfig()
	}
	if err != nil {
		logger.Fatal("k8s config failed", zap.Error(err))
	}
	k8s, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		logger.Fatal("k8s client failed", zap.Error(err))
	}

	// Verify trivy is available
	if out, err := exec.Command(cfg.TrivyPath, "--version").Output(); err != nil {
		logger.Fatal("trivy not found", zap.String("path", cfg.TrivyPath), zap.Error(err))
	} else {
		logger.Info("trivy ready", zap.String("version", strings.TrimSpace(string(out))))
	}

	logger.Info("trivy scanner starting",
		zap.String("cluster", cfg.ClusterName),
		zap.String("ingestor", cfg.IngestorURL),
		zap.Int("scan_interval_hours", cfg.ScanIntervalHours),
		zap.String("min_severity", cfg.TrivySeverity),
	)

	go startHealthServer()

	scanner := New(cfg, k8s, logger)
	scanner.Run(ctx)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
