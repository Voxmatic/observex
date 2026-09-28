// services/observex-agent/main.go
// ObserveX Agent — zero-configuration auto-discovery.
// Runs as a DaemonSet on every node. Discovers ALL services automatically
// using: (1) eBPF kernel probes, (2) K8s API watch, (3) /proc scanner.
// Sends all telemetry to the ingestor via gRPC streams.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/observex/platform/pkg/models"
	ebpftrace "github.com/observex/platform/services/oneagent/ebpf"
)

// ═══════════════════════════════════════════════════════
//  CONFIG
// ═══════════════════════════════════════════════════════

type Config struct {
	ConfigFile         string
	AgentID            string
	OrgID              string
	NodeName           string
	ClusterName        string
	Environment        string
	TenantURL          string
	IngestorURL        string // http://ingestor:4318
	AgentToken         string
	AgentVersion       string
	AgentIDFile        string
	StateDir           string
	RuntimeDir         string
	DownloadDir        string
	ScrapeIntervalS    int
	HeartbeatIntervalS int
	FlushIntervalS     int
	MaxBatchSize       int
	MaxQueueSize       int
	MonitoringMode     string
	CollectionMode     string
	HostGroup          string
	NetworkZone        string
	ProxyURL           string
	ProcRoot           string
	SysRoot            string
	LogPaths           []string
	LogMonitoring      bool
	AutoUpdate         bool
	UpdateChannel      string
	UpdateManifestURL  string
	UpdateIntervalS    int
	UpdatePublicKey    string
	UpdateInstallPath  string
	ActivateUpdates    bool
	EnableNativeMetrics bool
	EnableEBPF         bool
	EnableK8s          bool
	EnableProc         bool
	EnableProfiling    bool
	EnableRuntimeTracing bool
	EnableAutoInjection bool
	ProfileIntervalS  int
}

func loadConfig() Config {
	configFile := configPathFromArgs()
	fileCfg := loadAgentConfigFile(configFile)

	nodeName := cfgString(fileCfg, []string{"node_name", "host_name", "hostname"}, []string{"NODE_NAME", "HOSTNAME"}, hostname())
	agentIDFile := cfgString(fileCfg, []string{"agent_id_file"}, []string{"AGENT_ID_FILE"}, "/var/lib/observex/agent-id")
	agentID := cfgString(fileCfg, []string{"agent_id"}, []string{"AGENT_ID"}, "")
	if agentID == "" {
		agentID = loadOrCreateAgentID(agentIDFile, nodeName)
	}
	stateDir := cfgString(fileCfg, []string{"state_dir", "var_dir"}, []string{"OBSERVEX_STATE_DIR"}, "/var/lib/observex/oneagent")
	tenantURL := cfgString(fileCfg, []string{"tenant_url", "server", "server_url", "endpoint"}, []string{"OBSERVEX_TENANT_URL", "OBSERVEX_PUBLIC_URL"}, "")
	ingestorURL := cfgString(fileCfg, []string{"ingestor_url", "ingest_url", "activegate_url", "server", "server_url", "endpoint"}, []string{"OBSERVEX_INGESTOR", "INGESTOR_URL"}, tenantURL)
	if ingestorURL == "" {
		ingestorURL = "http://observex-ingestor:4318"
	}
	updateManifestURL := cfgString(fileCfg, []string{"update_manifest_url", "update_url"}, []string{"OBSERVEX_UPDATE_MANIFEST_URL", "UPDATE_MANIFEST_URL"}, "")
	if updateManifestURL == "" && ingestorURL != "" {
		updateManifestURL = strings.TrimRight(ingestorURL, "/") + "/v1/agent/updates/manifest"
	}
	monitoringMode := strings.ToLower(cfgString(fileCfg, []string{"monitoring_mode", "mode"}, []string{"OBSERVEX_MODE", "MONITORING_MODE"}, "fullstack"))
	collectionMode := strings.ToLower(cfgString(fileCfg, []string{"collection_mode", "telemetry_mode"}, []string{"OBSERVEX_COLLECTION_MODE"}, "native"))
	if collectionMode == "" {
		collectionMode = "native"
	}
	return Config{
		ConfigFile:         configFile,
		AgentID:            agentID,
		OrgID:              cfgString(fileCfg, []string{"org_id", "tenant", "tenant_id", "environment_id"}, []string{"OBSERVEX_ORG_ID", "ORG_ID"}, ""),
		NodeName:           nodeName,
		ClusterName:        cfgString(fileCfg, []string{"cluster_name", "cluster"}, []string{"CLUSTER_NAME"}, "default"),
		Environment:        cfgString(fileCfg, []string{"environment", "env"}, []string{"OBSERVEX_ENV", "ENVIRONMENT"}, "production"),
		TenantURL:          tenantURL,
		IngestorURL:        ingestorURL,
		AgentToken:         cfgString(fileCfg, []string{"token", "tenant_token", "paas_token", "deployment_token", "ingest_token"}, []string{"OBSERVEX_TOKEN", "AGENT_TOKEN"}, ""),
		AgentVersion:       cfgString(fileCfg, []string{"agent_version", "version"}, []string{"AGENT_VERSION"}, "2.0.0"),
		AgentIDFile:        agentIDFile,
		StateDir:           stateDir,
		RuntimeDir:         cfgString(fileCfg, []string{"runtime_dir"}, []string{"OBSERVEX_RUNTIME_DIR"}, filepath.Join(stateDir, "runtime")),
		DownloadDir:        cfgString(fileCfg, []string{"download_dir"}, []string{"OBSERVEX_DOWNLOAD_DIR"}, filepath.Join(stateDir, "downloads")),
		ScrapeIntervalS:    cfgInt(fileCfg, []string{"scrape_interval_s", "scrape_interval_seconds"}, []string{"SCRAPE_INTERVAL_S"}, 15),
		HeartbeatIntervalS: cfgInt(fileCfg, []string{"heartbeat_interval_s", "heartbeat_interval_seconds"}, []string{"HEARTBEAT_INTERVAL_S"}, 10),
		FlushIntervalS:     cfgInt(fileCfg, []string{"flush_interval_s", "flush_interval_seconds"}, []string{"FLUSH_INTERVAL_S"}, 5),
		MaxBatchSize:       cfgInt(fileCfg, []string{"max_batch_size"}, []string{"MAX_BATCH_SIZE"}, 1000),
		MaxQueueSize:       cfgInt(fileCfg, []string{"max_queue_size"}, []string{"MAX_QUEUE_SIZE"}, 50000),
		MonitoringMode:     monitoringMode,
		CollectionMode:     collectionMode,
		HostGroup:          cfgString(fileCfg, []string{"host_group", "hostgroup"}, []string{"OBSERVEX_HOST_GROUP", "HOST_GROUP"}, "default"),
		NetworkZone:        cfgString(fileCfg, []string{"network_zone", "networkzone"}, []string{"OBSERVEX_NETWORK_ZONE", "NETWORK_ZONE"}, "default"),
		ProxyURL:           cfgString(fileCfg, []string{"proxy", "proxy_url"}, []string{"OBSERVEX_PROXY", "HTTPS_PROXY", "HTTP_PROXY"}, ""),
		ProcRoot:           strings.TrimRight(cfgString(fileCfg, []string{"proc_root"}, []string{"HOST_PROC", "OBSERVEX_PROC_ROOT"}, "/proc"), "/"),
		SysRoot:            strings.TrimRight(cfgString(fileCfg, []string{"sys_root"}, []string{"HOST_SYS", "OBSERVEX_SYS_ROOT"}, "/sys"), "/"),
		LogPaths:           cfgList(fileCfg, []string{"log_paths", "logs"}, []string{"OBSERVEX_LOG_PATHS"}, defaultLogPaths()),
		LogMonitoring:      cfgBool(fileCfg, []string{"log_monitoring", "logs_enabled"}, []string{"ENABLE_LOG_MONITORING"}, true),
		AutoUpdate:         cfgBool(fileCfg, []string{"auto_update"}, []string{"AUTO_UPDATE"}, true),
		UpdateChannel:      cfgString(fileCfg, []string{"update_channel"}, []string{"UPDATE_CHANNEL"}, "stable"),
		UpdateManifestURL:  updateManifestURL,
		UpdateIntervalS:    cfgInt(fileCfg, []string{"update_interval_s", "update_check_interval_s"}, []string{"UPDATE_INTERVAL_S"}, 3600),
		UpdatePublicKey:    cfgString(fileCfg, []string{"update_public_key", "ed25519_public_key"}, []string{"OBSERVEX_UPDATE_PUBLIC_KEY"}, ""),
		UpdateInstallPath:  cfgString(fileCfg, []string{"update_install_path", "agent_binary"}, []string{"OBSERVEX_AGENT_BINARY", "UPDATE_INSTALL_PATH"}, ""),
		ActivateUpdates:    cfgBool(fileCfg, []string{"activate_updates", "auto_update_activate"}, []string{"AUTO_UPDATE_ACTIVATE"}, true),
		EnableNativeMetrics: cfgBool(fileCfg, []string{"native_metrics", "enable_native_metrics"}, []string{"ENABLE_NATIVE_METRICS"}, true),
		EnableEBPF:         cfgBool(fileCfg, []string{"ebpf_enabled", "enable_ebpf"}, []string{"ENABLE_EBPF"}, true),
		EnableK8s:          cfgBool(fileCfg, []string{"k8s_monitoring", "kubernetes_monitoring", "k8s_enabled", "enable_k8s"}, []string{"ENABLE_K8S"}, true),
		EnableProc:         cfgBool(fileCfg, []string{"process_discovery", "proc_enabled", "enable_proc"}, []string{"ENABLE_PROC"}, true),
		EnableProfiling:    cfgBool(fileCfg, []string{"profiling", "profiling_enabled", "enable_profiling"}, []string{"ENABLE_PROFILING"}, true),
		EnableRuntimeTracing: cfgBool(fileCfg, []string{"runtime_tracing", "apm_tracing", "auto_tracing"}, []string{"ENABLE_RUNTIME_TRACING"}, true),
		EnableAutoInjection: cfgBool(fileCfg, []string{"auto_injection", "injection_enabled", "enable_auto_injection"}, []string{"ENABLE_AUTO_INJECTION"}, true),
		ProfileIntervalS:  cfgInt(fileCfg, []string{"profile_interval_s", "profile_interval_seconds"}, []string{"PROFILE_INTERVAL_S"}, 60),
	}
}

// ═══════════════════════════════════════════════════════
//  AGENT
// ═══════════════════════════════════════════════════════

type ObserveXAgent struct {
	cfg      Config
	logger   *zap.Logger
	k8s      *kubernetes.Clientset
	sender   *TelemetrySender
	startedAt time.Time

	mu       sync.RWMutex
	services map[string]*models.Service // serviceID → service
	netConns map[string]*NetFlow        // "srcIP:dstIP:dstPort" → flow
	logOffsets map[string]int64
	moduleMu       sync.RWMutex
	moduleStatus   map[string]string
	moduleRestarts map[string]int
	updateState    string
	targetVersion  string
}

type OneAgent = ObserveXAgent

type NetFlow struct {
	SrcIP     string
	DstIP     string
	DstPort   int
	Protocol  string
	BytesSent uint64
	BytesRecv uint64
	Latency   time.Duration
	UpdatedAt time.Time
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := loadConfig()
	k8sClient := buildK8sClient(logger)
	if cfg.AgentToken == "" {
		logger.Warn("OBSERVEX_TOKEN is not set; ingestor must explicitly allow unauthenticated agent traffic")
	}

	agent := &ObserveXAgent{
		cfg:      cfg,
		logger:   logger,
		k8s:      k8sClient,
		sender:   NewTelemetrySender(cfg, logger),
		startedAt: time.Now(),
		services: make(map[string]*models.Service),
		netConns: make(map[string]*NetFlow),
		logOffsets: make(map[string]int64),
		moduleStatus: make(map[string]string),
		moduleRestarts: make(map[string]int),
		updateState: "idle",
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer agent.sender.Close()

	agent.prepareRuntimeDirs()
	agent.ensureInjectionArtifacts()
	agent.markModule("agent_core", "running")
	agent.saveRuntimeState()

	// Register this agent with the server
	agent.registerSelf(ctx)

	var wg sync.WaitGroup

	if cfg.EnableK8s && k8sClient == nil {
		agent.markModule("kubernetes_discovery", "unavailable")
	} else {
		agent.runModule(ctx, &wg, "kubernetes_discovery", cfg.EnableK8s, agent.watchK8s)
	}
	agent.runModule(ctx, &wg, "process_discovery", cfg.EnableProc, agent.scanProcesses)

	if cfg.EnableEBPF {
		agent.runModule(ctx, &wg, "network_topology", true, agent.runEBPFProbe)
		secLoader := NewSecurityLoader(agent, logger)
		agent.runModule(ctx, &wg, "runtime_security", true, secLoader.Run)
		agent.runModule(ctx, &wg, "runtime_tracing", cfg.EnableRuntimeTracing, agent.startRuntimeTracing)
	} else {
		agent.markModule("network_topology", "disabled")
		agent.markModule("runtime_security", "disabled")
		agent.markModule("runtime_tracing", "disabled")
	}

	agent.runModule(ctx, &wg, "native_metrics", cfg.EnableNativeMetrics, agent.collectMetrics)
	agent.runModule(ctx, &wg, "log_monitoring", cfg.LogMonitoring, agent.collectLogs)
	agent.runModule(ctx, &wg, "profiling", cfg.EnableProfiling, agent.collectProfiles)
	agent.runModule(ctx, &wg, "auto_update", cfg.AutoUpdate, agent.runUpdater)
	agent.runModule(ctx, &wg, "runtime_state", true, agent.runtimeStateLoop)
	agent.runModule(ctx, &wg, "heartbeat", true, agent.heartbeat)

	wg.Wait()
}

type agentRuntimeState struct {
	AgentID        string            `json:"agent_id"`
	OrgID          string            `json:"org_id,omitempty"`
	NodeName       string            `json:"node_name"`
	Version        string            `json:"version"`
	CollectionMode string            `json:"collection_mode"`
	UpdateState    string            `json:"update_state"`
	TargetVersion  string            `json:"target_version,omitempty"`
	StartedAt      time.Time         `json:"started_at"`
	WrittenAt      time.Time         `json:"written_at"`
	Services       int               `json:"services"`
	Flows          int               `json:"flows"`
	Modules        map[string]string `json:"modules"`
}

type agentUpdateManifest struct {
	Version    string   `json:"version"`
	Channel    string   `json:"channel"`
	URL        string   `json:"url"`
	SHA256     string   `json:"sha256"`
	Signature  string   `json:"signature,omitempty"`
	Platforms  []string `json:"platforms,omitempty"`
	ReleasedAt string   `json:"released_at,omitempty"`
}

type stagedAgentUpdate struct {
	Version    string    `json:"version"`
	Channel    string    `json:"channel"`
	SHA256     string    `json:"sha256"`
	Artifact   string    `json:"artifact"`
	StagedAt   time.Time `json:"staged_at"`
	ReleasedAt string    `json:"released_at,omitempty"`
}

func (a *ObserveXAgent) prepareRuntimeDirs() {
	for _, dir := range []string{a.cfg.StateDir, a.cfg.RuntimeDir, a.cfg.DownloadDir, filepath.Join(a.cfg.StateDir, "injection")} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			a.logger.Warn("unable to prepare agent runtime directory", zap.String("path", dir), zap.Error(err))
		}
	}
}

func (a *ObserveXAgent) markModule(name, state string) {
	a.moduleMu.Lock()
	a.moduleStatus[name] = state
	a.moduleMu.Unlock()
}

func (a *ObserveXAgent) moduleStatuses() map[string]string {
	a.moduleMu.RLock()
	defer a.moduleMu.RUnlock()
	out := make(map[string]string, len(a.moduleStatus))
	for name, state := range a.moduleStatus {
		out[name] = state
	}
	return out
}

func (a *ObserveXAgent) updateSnapshot() (string, string) {
	a.moduleMu.RLock()
	defer a.moduleMu.RUnlock()
	return a.updateState, a.targetVersion
}

func (a *ObserveXAgent) setUpdateState(state, targetVersion string) {
	a.moduleMu.Lock()
	a.updateState = state
	a.targetVersion = targetVersion
	a.moduleMu.Unlock()
}

// runModule gives long-lived collectors a OneAgent-style watchdog: a panic or
// unexpected return is contained and retried with bounded exponential backoff.
func (a *ObserveXAgent) runModule(ctx context.Context, wg *sync.WaitGroup, name string, enabled bool, run func(context.Context)) {
	if !enabled {
		a.markModule(name, "disabled")
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := 2 * time.Second
		for {
			a.markModule(name, "starting")
			panicked := false
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						panicked = true
						a.logger.Error("agent module panicked", zap.String("module", name), zap.Any("panic", recovered))
					}
				}()
				a.markModule(name, "running")
				run(ctx)
			}()

			if ctx.Err() != nil {
				a.markModule(name, "stopped")
				return
			}
			a.moduleMu.Lock()
			a.moduleRestarts[name]++
			restarts := a.moduleRestarts[name]
			a.moduleMu.Unlock()
			state := "restarting"
			if panicked {
				state = "crashed"
			}
			a.markModule(name, state)
			a.logger.Warn("agent module stopped unexpectedly; scheduling restart", zap.String("module", name), zap.Int("restart", restarts), zap.Duration("backoff", backoff))
			select {
			case <-ctx.Done():
				a.markModule(name, "stopped")
				return
			case <-time.After(backoff):
			}
			if backoff < time.Minute {
				backoff *= 2
				if backoff > time.Minute {
					backoff = time.Minute
				}
			}
		}
	}()
}

func (a *ObserveXAgent) runtimeStateLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.saveRuntimeState()
			return
		case <-ticker.C:
			a.saveRuntimeState()
		}
	}
}

func (a *ObserveXAgent) saveRuntimeState() {
	a.mu.RLock()
	services, flows := len(a.services), len(a.netConns)
	a.mu.RUnlock()
	updateState, targetVersion := a.updateSnapshot()
	state := agentRuntimeState{
		AgentID:        a.cfg.AgentID,
		OrgID:          a.cfg.OrgID,
		NodeName:       a.cfg.NodeName,
		Version:        a.cfg.AgentVersion,
		CollectionMode: a.cfg.CollectionMode,
		UpdateState:    updateState,
		TargetVersion:  targetVersion,
		StartedAt:      a.startedAt,
		WrittenAt:      time.Now().UTC(),
		Services:       services,
		Flows:          flows,
		Modules:        a.moduleStatuses(),
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		a.logger.Warn("unable to encode agent runtime state", zap.Error(err))
		return
	}
	path := filepath.Join(a.cfg.RuntimeDir, "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		a.logger.Warn("unable to create runtime-state directory", zap.Error(err))
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0640); err != nil {
		a.logger.Warn("unable to write agent runtime state", zap.Error(err))
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		a.logger.Warn("unable to publish agent runtime state", zap.Error(err))
	}
}

func (a *ObserveXAgent) ensureInjectionArtifacts() {
	if !a.cfg.EnableAutoInjection {
		a.markModule("auto_injection", "disabled")
		return
	}
	dir := filepath.Join(a.cfg.StateDir, "injection")
	if err := os.MkdirAll(dir, 0750); err != nil {
		a.markModule("auto_injection", "unavailable")
		a.logger.Warn("unable to create injection artifacts", zap.Error(err))
		return
	}
	artifacts := map[string]string{
		"java.env":   "JAVA_TOOL_OPTIONS=-javaagent:/opt/observex/oneagent/instrumentation/java/observex-javaagent.jar\n",
		"node.env":   "NODE_OPTIONS=--require /opt/observex/oneagent/instrumentation/node/observex-loader.js\n",
		"python.env": "PYTHONPATH=/opt/observex/oneagent/instrumentation/python\n",
		"dotnet.env": "CORECLR_PROFILER_PATH=/opt/observex/oneagent/instrumentation/dotnet/ObservexProfiler.so\n",
		"php.env":    "PHP_INI_SCAN_DIR=/opt/observex/oneagent/instrumentation/php\n",
	}
	for name, content := range artifacts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0640); err != nil {
			a.markModule("auto_injection", "degraded")
			a.logger.Warn("unable to write injection artifact", zap.String("artifact", name), zap.Error(err))
			return
		}
	}
	a.markModule("auto_injection", "ready")
}

func (a *ObserveXAgent) runUpdater(ctx context.Context) {
	if strings.TrimSpace(a.cfg.UpdateManifestURL) == "" {
		a.setUpdateState("idle", "")
		a.markModule("auto_update", "waiting_for_manifest")
		<-ctx.Done()
		return
	}
	if strings.TrimSpace(a.cfg.UpdatePublicKey) == "" {
		a.setUpdateState("waiting_for_signing_key", "")
		a.markModule("auto_update", "waiting_for_signing_key")
		<-ctx.Done()
		return
	}
	interval := time.Duration(a.cfg.UpdateIntervalS) * time.Second
	if interval < time.Minute {
		interval = time.Hour
	}
	a.checkForUpdate(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkForUpdate(ctx)
		}
	}
}

func (a *ObserveXAgent) checkForUpdate(ctx context.Context) {
	manifestURL, err := url.Parse(a.cfg.UpdateManifestURL)
	if err != nil || manifestURL.Scheme != "https" || manifestURL.Host == "" {
		a.setUpdateState("rejected_manifest_url", "")
		a.markModule("auto_update", "degraded")
		a.logger.Warn("agent update manifest URL must use HTTPS", zap.String("url", a.cfg.UpdateManifestURL))
		return
	}
	q := manifestURL.Query()
	q.Set("channel", a.cfg.UpdateChannel)
	q.Set("platform", runtime.GOOS+"/"+runtime.GOARCH)
	manifestURL.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL.String(), nil)
	if err != nil {
		a.setUpdateState("manifest_request_error", "")
		return
	}
	a.sender.addIdentityHeaders(req)
	resp, err := a.sender.client.Do(req)
	if err != nil {
		a.setUpdateState("manifest_unreachable", "")
		a.markModule("auto_update", "degraded")
		a.logger.Warn("agent update manifest request failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		a.setUpdateState("current", a.cfg.AgentVersion)
		a.markModule("auto_update", "running")
		return
	}
	if resp.StatusCode != http.StatusOK {
		a.setUpdateState("manifest_unavailable", "")
		a.markModule("auto_update", "degraded")
		a.logger.Warn("agent update manifest rejected", zap.Int("status", resp.StatusCode))
		return
	}
	var manifest agentUpdateManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&manifest); err != nil {
		a.setUpdateState("invalid_manifest", "")
		a.markModule("auto_update", "degraded")
		a.logger.Warn("invalid agent update manifest", zap.Error(err))
		return
	}
	if manifest.Version == "" || manifest.URL == "" || manifest.SHA256 == "" {
		a.setUpdateState("invalid_manifest", manifest.Version)
		a.markModule("auto_update", "degraded")
		return
	}
	if manifest.Channel != "" && manifest.Channel != a.cfg.UpdateChannel {
		a.setUpdateState("channel_mismatch", manifest.Version)
		return
	}
	if !updatePlatformAllowed(manifest.Platforms) {
		a.setUpdateState("platform_not_supported", manifest.Version)
		return
	}
	if !versionNewer(manifest.Version, a.cfg.AgentVersion) {
		a.setUpdateState("current", a.cfg.AgentVersion)
		a.markModule("auto_update", "running")
		return
	}
	if err := a.stageVerifiedUpdate(ctx, manifest); err != nil {
		a.setUpdateState("stage_failed", manifest.Version)
		a.markModule("auto_update", "degraded")
		a.logger.Warn("agent update staging failed", zap.String("version", manifest.Version), zap.Error(err))
		return
	}
	a.setUpdateState("staged", manifest.Version)
	a.markModule("auto_update", "staged")
	a.logger.Info("verified agent update staged for supervisor activation", zap.String("version", manifest.Version))
	if !a.cfg.ActivateUpdates || strings.TrimSpace(a.cfg.UpdateInstallPath) == "" {
		return
	}
	if err := a.activateStagedUpdate(manifest.Version); err != nil {
		a.setUpdateState("activation_failed", manifest.Version)
		a.markModule("auto_update", "degraded")
		a.logger.Warn("verified agent update could not be activated", zap.String("version", manifest.Version), zap.Error(err))
	}
}

func (a *ObserveXAgent) stageVerifiedUpdate(ctx context.Context, manifest agentUpdateManifest) error {
	artifactURL, err := url.Parse(manifest.URL)
	if err != nil || artifactURL.Scheme != "https" || artifactURL.Host == "" {
		return fmt.Errorf("update artifact URL must use HTTPS")
	}
	if a.cfg.UpdatePublicKey == "" || manifest.Signature == "" {
		return fmt.Errorf("signed update manifest is required")
	}
	publicKey, err := base64.StdEncoding.DecodeString(a.cfg.UpdatePublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid update public key")
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(manifest.Version+"\n"+manifest.SHA256), signature) {
		return fmt.Errorf("update manifest signature verification failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL.String(), nil)
	if err != nil {
		return err
	}
	a.sender.addIdentityHeaders(req)
	resp, err := a.sender.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update artifact returned HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(a.cfg.DownloadDir, 0750); err != nil {
		return err
	}
	name := "observex-agent-" + sanitizeIDPart(manifest.Version) + "-" + runtime.GOOS + "-" + runtime.GOARCH + ".stage"
	path := filepath.Join(a.cfg.DownloadDir, name)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hasher), io.LimitReader(resp.Body, 512<<20))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, strings.TrimSpace(manifest.SHA256)) {
		_ = os.Remove(tmp)
		return fmt.Errorf("update artifact checksum mismatch")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	metadata, err := json.MarshalIndent(stagedAgentUpdate{Version: manifest.Version, Channel: a.cfg.UpdateChannel, SHA256: actualHash, Artifact: path, StagedAt: time.Now().UTC(), ReleasedAt: manifest.ReleasedAt}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.cfg.RuntimeDir, "staged-update.json"), metadata, 0640)
}

// activateStagedUpdate atomically replaces the managed executable and asks the
// service supervisor to restart it. It is deliberately limited to the binary
// path emitted by the installer; container images continue to use deployment
// rollouts and keep the verified artifact staged for inspection.
func (a *ObserveXAgent) activateStagedUpdate(version string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic binary activation is managed by the platform installer on %s", runtime.GOOS)
	}
	metadataPath := filepath.Join(a.cfg.RuntimeDir, "staged-update.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return err
	}
	var staged stagedAgentUpdate
	if err := json.Unmarshal(data, &staged); err != nil {
		return err
	}
	if staged.Version != version || staged.Artifact == "" || staged.SHA256 == "" {
		return fmt.Errorf("staged update metadata does not match requested version")
	}
	artifact, err := os.Open(staged.Artifact)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, artifact); err != nil {
		artifact.Close()
		return err
	}
	if err := artifact.Close(); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), staged.SHA256) {
		return fmt.Errorf("staged update checksum no longer matches")
	}
	installPath := filepath.Clean(a.cfg.UpdateInstallPath)
	if !filepath.IsAbs(installPath) {
		return fmt.Errorf("configured update install path must be absolute")
	}
	currentPath, err := os.Executable()
	if err != nil {
		return err
	}
	if filepath.Clean(currentPath) != installPath {
		return fmt.Errorf("configured update install path does not match the running agent")
	}
	info, err := os.Stat(installPath)
	if err != nil {
		return err
	}
	nextPath := installPath + ".next"
	backupPath := installPath + ".previous"
	if err := copyExecutable(staged.Artifact, nextPath, info.Mode()); err != nil {
		return err
	}
	_ = os.Remove(backupPath)
	if err := os.Rename(installPath, backupPath); err != nil {
		_ = os.Remove(nextPath)
		return err
	}
	if err := os.Rename(nextPath, installPath); err != nil {
		_ = os.Rename(backupPath, installPath)
		return err
	}
	if err := os.Remove(metadataPath); err != nil {
		a.logger.Warn("activated update but could not clear staging metadata", zap.Error(err))
	}
	a.setUpdateState("activated", staged.Version)
	a.saveRuntimeState()
	return syscall.Kill(os.Getpid(), syscall.SIGTERM)
}

func copyExecutable(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	return os.Chmod(destination, mode.Perm())
}

func updatePlatformAllowed(platforms []string) bool {
	if len(platforms) == 0 {
		return true
	}
	current := runtime.GOOS + "/" + runtime.GOARCH
	for _, platform := range platforms {
		if platform == current || platform == runtime.GOOS || platform == "*" {
			return true
		}
	}
	return false
}

func versionNewer(candidate, current string) bool {
	parse := func(version string) []int {
		version = strings.TrimPrefix(strings.TrimSpace(version), "v")
		version = strings.SplitN(version, "-", 2)[0]
		parts := strings.Split(version, ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			out[i], _ = strconv.Atoi(part)
		}
		return out
	}
	a, b := parse(candidate), parse(current)
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		var av, bv int
		if i < len(a) { av = a[i] }
		if i < len(b) { bv = b[i] }
		if av != bv { return av > bv }
	}
	return false
}

// ═══════════════════════════════════════════════════════
//  SELF REGISTRATION
// ═══════════════════════════════════════════════════════

func (a *ObserveXAgent) registerSelf(ctx context.Context) {
	agent := models.Agent{
		ID:          a.cfg.AgentID,
		OrgID:       a.cfg.OrgID,
		NodeName:    a.cfg.NodeName,
		ClusterName: a.cfg.ClusterName,
		Environment: a.cfg.Environment,
		HostGroup:   a.cfg.HostGroup,
		NetworkZone: a.cfg.NetworkZone,
		MonitoringMode: a.cfg.MonitoringMode,
		CollectionMode: a.cfg.CollectionMode,
		LogMonitoring: a.cfg.LogMonitoring,
		AutoUpdate:  a.cfg.AutoUpdate,
		UpdateChannel: a.cfg.UpdateChannel,
		Version:     a.cfg.AgentVersion,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		EBPFEnabled: a.cfg.EnableEBPF,
		Capabilities: agentCapabilities(a.cfg),
		Status:      "active",
		RegisteredAt: time.Now(),
	}

	// Read kernel version
	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) > 2 {
			agent.KernelVer = parts[2]
		}
	}

	// Read IP address
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if addrs, err := iface.Addrs(); err == nil && len(addrs) > 0 {
				agent.IPAddress = strings.Split(addrs[0].String(), "/")[0]
				break
			}
		}
	}

	a.sender.SendAgent(ctx, agent)
	a.logger.Info("agent registered",
		zap.String("id", agent.ID),
		zap.String("node", agent.NodeName),
	)
}

// ═══════════════════════════════════════════════════════
//  K8S WATCH — auto-discovers every pod, service, node
// ═══════════════════════════════════════════════════════

func (a *ObserveXAgent) watchK8s(ctx context.Context) {
	a.logger.Info("starting K8s watcher")

	go a.watchPods(ctx)
	go a.watchNodes(ctx)
	go a.watchServices(ctx)
	go a.watchDeployments(ctx)
	go a.watchStatefulSets(ctx)

	<-ctx.Done()
}

func (a *ObserveXAgent) watchPods(ctx context.Context) {
	for {
		watcher, err := a.k8s.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			a.logger.Warn("pod watch error, retrying", zap.Error(err))
			time.Sleep(5 * time.Second)
			continue
		}

		for evt := range watcher.ResultChan() {
			pod, ok := evt.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			svc := a.podToService(pod)
			a.upsertService(ctx, svc, evt.Type)
		}

		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(2 * time.Second) // brief pause before re-watching
		}
	}
}

func (a *ObserveXAgent) podToService(pod *corev1.Pod) *models.Service {
	svc := &models.Service{
		ID:          fmt.Sprintf("k8s:%s:%s:%s", a.cfg.ClusterName, pod.Namespace, pod.Name),
		Name:        pod.Name,
		DisplayName: podDisplayName(pod),
		Kind:        detectPodKind(pod),
		Namespace:   pod.Namespace,
		ClusterName: a.cfg.ClusterName,
		NodeName:    pod.Spec.NodeName,
		PodName:     pod.Name,
		Deployment:  podDeployment(pod),
		Labels:      pod.Labels,
		Annotations: pod.Annotations,
		AgentID:     a.cfg.AgentID,
		LastSeenAt:  time.Now(),
		TechStack:   detectTechStack(pod),
	}

	// Endpoints
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			svc.Endpoints = append(svc.Endpoints, models.Endpoint{
				Address:  pod.Status.PodIP,
				Port:     int(p.ContainerPort),
				Protocol: string(p.Protocol),
			})
		}
	}

	// Health
	svc.Health = podHealth(pod)
	return svc
}

func (a *ObserveXAgent) watchNodes(ctx context.Context) {
	for {
		watcher, err := a.k8s.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		for evt := range watcher.ResultChan() {
			node, ok := evt.Object.(*corev1.Node)
			if !ok {
				continue
			}
			svc := &models.Service{
				ID:          fmt.Sprintf("k8s-node:%s:%s", a.cfg.ClusterName, node.Name),
				Name:        node.Name,
				DisplayName: node.Name,
				Kind:        models.SvcK8sNode,
				ClusterName: a.cfg.ClusterName,
				NodeName:    node.Name,
				Labels:      node.Labels,
				AgentID:     a.cfg.AgentID,
				LastSeenAt:  time.Now(),
				Health:      nodeHealth(node),
			}
			a.upsertService(ctx, svc, evt.Type)
		}
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func (a *ObserveXAgent) watchServices(ctx context.Context) {
	for {
		watcher, err := a.k8s.CoreV1().Services("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		for evt := range watcher.ResultChan() {
			ksvc, ok := evt.Object.(*corev1.Service)
			if !ok {
				continue
			}
			svc := &models.Service{
				ID:          fmt.Sprintf("k8s-svc:%s:%s:%s", a.cfg.ClusterName, ksvc.Namespace, ksvc.Name),
				Name:        ksvc.Name,
				DisplayName: ksvc.Name,
				Kind:        models.SvcK8sService,
				Namespace:   ksvc.Namespace,
				ClusterName: a.cfg.ClusterName,
				Labels:      ksvc.Labels,
				AgentID:     a.cfg.AgentID,
				LastSeenAt:  time.Now(),
				Health:      models.Health{State: models.HealthGood, Score: 100},
			}
			for _, p := range ksvc.Spec.Ports {
				svc.Endpoints = append(svc.Endpoints, models.Endpoint{
					Address:  ksvc.Spec.ClusterIP,
					Port:     int(p.Port),
					Protocol: string(p.Protocol),
				})
			}
			a.upsertService(ctx, svc, evt.Type)
		}
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func (a *ObserveXAgent) watchDeployments(ctx context.Context) {
	for {
		watcher, err := a.k8s.AppsV1().Deployments("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		for range watcher.ResultChan() {
			// Deployment changes are tracked via pod health changes
		}
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func (a *ObserveXAgent) watchStatefulSets(ctx context.Context) {
	for {
		watcher, err := a.k8s.AppsV1().StatefulSets("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		for range watcher.ResultChan() {
			// tracked via pod health
		}
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

// ═══════════════════════════════════════════════════════
//  PROCESS SCANNER — discovers non-K8s services
// ═══════════════════════════════════════════════════════

// Known services by listening port
var wellKnownPorts = map[int]struct {
	Kind models.ServiceKind
	Name string
	Tech []string
}{
	5432:  {models.SvcDatabase, "postgresql", []string{"postgresql"}},
	3306:  {models.SvcDatabase, "mysql", []string{"mysql"}},
	27017: {models.SvcDatabase, "mongodb", []string{"mongodb"}},
	9042:  {models.SvcDatabase, "cassandra", []string{"cassandra"}},
	5984:  {models.SvcDatabase, "couchdb", []string{"couchdb"}},
	6379:  {models.SvcCache, "redis", []string{"redis"}},
	11211: {models.SvcCache, "memcached", []string{"memcached"}},
	9092:  {models.SvcQueue, "kafka", []string{"kafka", "java"}},
	5672:  {models.SvcQueue, "rabbitmq", []string{"rabbitmq", "erlang"}},
	4222:  {models.SvcQueue, "nats", []string{"nats", "go"}},
	9200:  {models.SvcDatabase, "elasticsearch", []string{"elasticsearch", "java"}},
	2181:  {models.SvcQueue, "zookeeper", []string{"zookeeper", "java"}},
	6443:  {models.SvcHTTP, "kubernetes-api", []string{"kubernetes", "go"}},
	2379:  {models.SvcDatabase, "etcd", []string{"etcd", "go"}},
	8080:  {models.SvcHTTP, "http-service", []string{}},
	8443:  {models.SvcHTTP, "https-service", []string{}},
	3000:  {models.SvcHTTP, "http-service", []string{}},
	3100:  {models.SvcHTTP, "loki", []string{"loki", "go"}},
	9411:  {models.SvcHTTP, "zipkin", []string{"zipkin", "java"}},
}

func (a *ObserveXAgent) scanProcesses(ctx context.Context) {
	a.logger.Info("starting process scanner")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	a.doScanProcesses(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.doScanProcesses(ctx)
		}
	}
}

func (a *ObserveXAgent) doScanProcesses(ctx context.Context) {
	// Read /proc/net/tcp and /proc/net/tcp6 to find listening ports
	listeningPorts := a.getListeningPorts()

	for port, pid := range listeningPorts {
		procName := a.getProcName(pid)
		cmdline := a.getProcCmdline(pid)
		runtimeName := inferRuntime(procName, cmdline)
		workDir := a.getProcWorkDir(pid)
		exePath := a.getProcExePath(pid)
		parentPID := a.getProcParentPID(pid)
		processGroup := inferProcessGroup(procName, cmdline, runtimeName)
		injection := a.injectionMode(runtimeName)
		info, ok := wellKnownPorts[port]
		if !ok {
			info = inferServiceFromProcess(port, procName, runtimeName)
		}
		tech := append([]string{}, info.Tech...)
		if runtimeName != "" && runtimeName != "unknown" && !containsString(tech, runtimeName) {
			tech = append(tech, runtimeName)
		}
		displayName := procName
		if displayName == "" || displayName == "unknown" {
			displayName = info.Name
		}

		svcID := fmt.Sprintf("proc:%s:%d:%d", a.cfg.NodeName, pid, port)
		if pid <= 0 {
			svcID = fmt.Sprintf("proc:%s:%s:%d", a.cfg.NodeName, info.Name, port)
		}

		svc := &models.Service{
			ID:          svcID,
			Name:        fmt.Sprintf("%s-%d", info.Name, port),
			DisplayName: displayName,
			Kind:        info.Kind,
			NodeName:    a.cfg.NodeName,
			ClusterName: a.cfg.ClusterName,
			AgentID:     a.cfg.AgentID,
			Labels: map[string]string{
				"pid":            strconv.Itoa(pid),
				"parent_pid":     strconv.Itoa(parentPID),
				"process":        procName,
				"process_group":  processGroup,
				"runtime":        runtimeName,
				"auto_injection": injection,
				"cmdline":        truncateLabel(cmdline, 512),
				"exe_path":       truncateLabel(exePath, 512),
				"work_dir":       truncateLabel(workDir, 512),
				"port":           strconv.Itoa(port),
			},
			TechStack:   tech,
			LastSeenAt:  time.Now(),
			Endpoints: []models.Endpoint{{
				Address:  a.cfg.NodeName,
				Port:     port,
				Protocol: "TCP",
			}},
			Health: models.Health{State: models.HealthGood, Score: 100},
		}

		a.upsertService(ctx, svc, watch.Added)
	}
}

// getListeningPorts reads /proc/net/tcp to find all listening sockets
func (a *ObserveXAgent) getListeningPorts() map[int]int {
	ports := map[int]int{} // port → pid

	inodeToPID := a.mapSocketInodesToPIDs()

	for _, f := range []string{a.procPath("net/tcp"), a.procPath("net/tcp6")} {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Scan() // skip header

		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}

			// State 0A = LISTEN
			if fields[3] != "0A" {
				continue
			}

			// Parse local address "0100007F:1F90" → port
			localAddr := fields[1]
			parts := strings.Split(localAddr, ":")
			if len(parts) != 2 {
				continue
			}

			portHex := parts[1]
			portNum, err := strconv.ParseInt(portHex, 16, 32)
			if err != nil {
				continue
			}

			// Try to find PID from inode (simplified — real impl uses /proc/<pid>/fd)
			pid := 0
			if len(fields) > 9 {
				pid = inodeToPID[fields[9]]
			}
			ports[int(portNum)] = pid
		}
	}

	return ports
}

func (a *ObserveXAgent) mapSocketInodesToPIDs() map[string]int {
	out := map[string]int{}
	entries, err := os.ReadDir(a.cfg.ProcRoot)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join(a.cfg.ProcRoot, entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			if inode != "" {
				out[inode] = pid
			}
		}
	}
	return out
}

func (a *ObserveXAgent) getProcName(pid int) string {
	if pid <= 0 {
		return "unknown"
	}
	comm, err := os.ReadFile(a.procPath(strconv.Itoa(pid), "comm"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(comm))
}

func (a *ObserveXAgent) getProcCmdline(pid int) string {
	if pid <= 0 {
		return ""
	}
	data, err := os.ReadFile(a.procPath(strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
}

func (a *ObserveXAgent) getProcWorkDir(pid int) string {
	if pid <= 0 {
		return ""
	}
	path, err := os.Readlink(a.procPath(strconv.Itoa(pid), "cwd"))
	if err != nil {
		return ""
	}
	return path
}

func (a *ObserveXAgent) getProcExePath(pid int) string {
	if pid <= 0 {
		return ""
	}
	path, err := os.Readlink(a.procPath(strconv.Itoa(pid), "exe"))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(path, " (deleted)")
}

func (a *ObserveXAgent) getProcParentPID(pid int) int {
	if pid <= 0 {
		return 0
	}
	data, err := os.ReadFile(a.procPath(strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	// The command may contain spaces and parentheses, so find the end of comm
	// before reading the state and PPID fields that follow it.
	end := strings.LastIndex(string(data), ")")
	if end < 0 {
		return 0
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) < 2 {
		return 0
	}
	parentPID, _ := strconv.Atoi(fields[1])
	return parentPID
}

func (a *ObserveXAgent) injectionMode(runtimeName string) string {
	if !a.cfg.EnableAutoInjection {
		return "disabled"
	}
	switch runtimeName {
	case "java", "node", "python", "dotnet", "php":
		return "available"
	default:
		return "not_supported"
	}
}

// runNetflowFallback reads /proc/net/tcp to build topology when eBPF unavailable
func (a *ObserveXAgent) runNetflowFallback(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.buildTopologyFromProcNet(ctx)
		}
	}
}

func (a *ObserveXAgent) buildTopologyFromProcNet(ctx context.Context) {
	// Read established TCP connections from /proc/net/tcp
	// Map IP:port → service, then emit topology edges

	data, err := os.ReadFile(a.procPath("net/tcp"))
	if err != nil {
		return
	}

	type connEntry struct {
		LocalIP   string
		LocalPort int
		RemoteIP  string
		RemotePort int
	}

	var conns []connEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Scan() // skip header

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		// State 01 = ESTABLISHED
		if fields[3] != "01" {
			continue
		}

		localParts := strings.Split(fields[1], ":")
		remoteParts := strings.Split(fields[2], ":")
		if len(localParts) != 2 || len(remoteParts) != 2 {
			continue
		}

		localPort, _ := strconv.ParseInt(localParts[1], 16, 32)
		remotePort, _ := strconv.ParseInt(remoteParts[1], 16, 32)
		localIP := hexToIP(localParts[0])
		remoteIP := hexToIP(remoteParts[0])

		conns = append(conns, connEntry{
			LocalIP:    localIP,
			LocalPort:  int(localPort),
			RemoteIP:   remoteIP,
			RemotePort: int(remotePort),
		})
	}

	// Build edges and send topology data
	for _, conn := range conns {
		edge := models.TopoEdge{
			ID:          fmt.Sprintf("%s:%d->%s:%d", conn.LocalIP, conn.LocalPort, conn.RemoteIP, conn.RemotePort),
			Protocol:    "TCP",
			CallsPerMin: 0,
			UpdatedAt:   time.Now(),
		}
		a.sender.SendTopoEdge(ctx, edge)
	}
}

func hexToIP(hexStr string) string {
	// Little-endian hex IP used in /proc/net/tcp
	if len(hexStr) != 8 {
		return "0.0.0.0"
	}
	b := make([]byte, 4)
	v, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		return "0.0.0.0"
	}
	binary.LittleEndian.PutUint32(b, uint32(v))
	return net.IP(b).String()
}

// ═══════════════════════════════════════════════════════
//  METRICS COLLECTION
// ═══════════════════════════════════════════════════════

func (a *ObserveXAgent) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(a.cfg.ScrapeIntervalS) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.mu.RLock()
			svcs := make([]*models.Service, 0, len(a.services))
			for _, s := range a.services {
				svcs = append(svcs, s)
			}
			a.mu.RUnlock()

			for _, svc := range svcs {
				pts := a.collectServiceMetrics(svc)
				if len(pts) > 0 {
					a.sender.SendMetrics(ctx, pts)
				}
			}

			// Collect node-level metrics from /proc
			nodePts := a.collectNodeMetrics()
			if len(nodePts) > 0 {
				a.sender.SendMetrics(ctx, nodePts)
			}
		}
	}
}

func (a *ObserveXAgent) collectServiceMetrics(svc *models.Service) []models.MetricPoint {
	labels := map[string]string{
		"service_id":   svc.ID,
		"service_name": svc.Name,
		"namespace":    svc.Namespace,
		"cluster":      svc.ClusterName,
		"kind":         string(svc.Kind),
	}

	var pts []models.MetricPoint
	now := time.Now()

	// For K8s pods: read cgroup v2 metrics
	if svc.Kind == models.SvcK8sPod && svc.PodName != "" {
		cgroupPts := a.readCgroupMetrics(svc.PodName, labels, now)
		pts = append(pts, cgroupPts...)
	}

	// For process services: read from /proc/<pid>/stat
	if svc.Kind == models.SvcProcess {
		procPts := a.readProcMetrics(svc.Name, labels, now)
		pts = append(pts, procPts...)
	}

	return pts
}

// readCgroupMetrics reads cpu/memory from cgroup v2
func (a *ObserveXAgent) readCgroupMetrics(podName string, labels map[string]string, now time.Time) []models.MetricPoint {
	var pts []models.MetricPoint

	// cgroup v2 paths
	cgroupBase := filepath.Join(a.cfg.SysRoot, "fs/cgroup", fmt.Sprintf("kubepods.slice/kubepods-pod%s.slice", podName))

	// Memory usage
	if data, err := os.ReadFile(filepath.Join(cgroupBase, "memory.current")); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
			pts = append(pts, models.MetricPoint{
				Name: "container_memory_working_set_bytes", Value: v,
				Timestamp: now, Labels: labels,
			})
		}
	}

	// Memory limit
	if data, err := os.ReadFile(filepath.Join(cgroupBase, "memory.max")); err == nil {
		val := strings.TrimSpace(string(data))
		if val != "max" {
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				pts = append(pts, models.MetricPoint{
					Name: "container_memory_limit_bytes", Value: v,
					Timestamp: now, Labels: labels,
				})
			}
		}
	}

	// CPU usage (from cpu.stat)
	if data, err := os.ReadFile(filepath.Join(cgroupBase, "cpu.stat")); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			parts := strings.Fields(scanner.Text())
			if len(parts) == 2 && parts[0] == "usage_usec" {
				if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
					pts = append(pts, models.MetricPoint{
						Name:      "container_cpu_usage_seconds_total",
						Value:     v / 1e6, // microseconds → seconds
						Timestamp: now, Labels: labels,
					})
				}
			}
		}
	}

	// Restart count from K8s API
	if a.k8s != nil {
		pod, err := a.k8s.CoreV1().Pods(labels["namespace"]).Get(context.Background(), podName, metav1.GetOptions{})
		if err == nil {
			var restarts int32
			for _, cs := range pod.Status.ContainerStatuses {
				restarts += cs.RestartCount
			}
			pts = append(pts, models.MetricPoint{
				Name: "kube_pod_container_status_restarts_total", Value: float64(restarts),
				Timestamp: now, Labels: labels,
			})
		}
	}

	return pts
}

// readProcMetrics reads CPU/memory for a process from /proc
func (a *ObserveXAgent) readProcMetrics(procName string, labels map[string]string, now time.Time) []models.MetricPoint {
	var pts []models.MetricPoint
	if pidText := labels["pid"]; pidText != "" && pidText != "0" {
		if pid, err := strconv.Atoi(pidText); err == nil {
			return a.readProcMetricsByPID(pid, labels, now)
		}
	}

	// Find PID by process name
	procs, _ := filepath.Glob(filepath.Join(a.cfg.ProcRoot, "*/comm"))
	for _, commFile := range procs {
		data, err := os.ReadFile(commFile)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) != procName {
			continue
		}

		pidStr := filepath.Base(filepath.Dir(commFile))
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Read /proc/<pid>/stat for CPU and memory
		statData, err := os.ReadFile(a.procPath(strconv.Itoa(pid), "stat"))
		if err != nil {
			continue
		}

		fields := strings.Fields(string(statData))
		if len(fields) < 24 {
			continue
		}

		// Field 13: utime (user), Field 14: stime (sys) in clock ticks
		utime, _ := strconv.ParseFloat(fields[13], 64)
		stime, _ := strconv.ParseFloat(fields[14], 64)
		clkTck := float64(100) // typical HZ

		pts = append(pts, models.MetricPoint{
			Name: "process_cpu_seconds_total", Value: (utime + stime) / clkTck,
			Timestamp: now, Labels: labels,
		})

		// Field 23: RSS memory in pages
		rss, _ := strconv.ParseFloat(fields[23], 64)
		pageSize := float64(os.Getpagesize())
		pts = append(pts, models.MetricPoint{
			Name: "process_resident_memory_bytes", Value: rss * pageSize,
			Timestamp: now, Labels: labels,
		})

		break
	}

	return pts
}

func (a *ObserveXAgent) readProcMetricsByPID(pid int, labels map[string]string, now time.Time) []models.MetricPoint {
	statData, err := os.ReadFile(a.procPath(strconv.Itoa(pid), "stat"))
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(statData))
	if len(fields) < 24 {
		return nil
	}
	utime, _ := strconv.ParseFloat(fields[13], 64)
	stime, _ := strconv.ParseFloat(fields[14], 64)
	rss, _ := strconv.ParseFloat(fields[23], 64)
	pageSize := float64(os.Getpagesize())
	clkTck := float64(100)
	return []models.MetricPoint{
		{Name: "process_cpu_seconds_total", Value: (utime + stime) / clkTck, Timestamp: now, Labels: labels},
		{Name: "process_resident_memory_bytes", Value: rss * pageSize, Timestamp: now, Labels: labels},
	}
}

// collectNodeMetrics reads /proc/stat, /proc/meminfo, /proc/diskstats
func (a *ObserveXAgent) collectNodeMetrics() []models.MetricPoint {
	labels := map[string]string{
		"node":    a.cfg.NodeName,
		"cluster": a.cfg.ClusterName,
	}
	now := time.Now()
	var pts []models.MetricPoint

	// CPU from /proc/stat
	if data, err := os.ReadFile(a.procPath("stat")); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "cpu ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 8 {
				break
			}
			parse := func(i int) float64 {
				v, _ := strconv.ParseFloat(fields[i], 64)
				return v
			}
			total := parse(1) + parse(2) + parse(3) + parse(4) + parse(5) + parse(6) + parse(7)
			idle := parse(4)
			pts = append(pts,
				models.MetricPoint{Name: "node_cpu_total", Value: total, Timestamp: now, Labels: labels},
				models.MetricPoint{Name: "node_cpu_idle", Value: idle, Timestamp: now, Labels: labels},
			)
			break
		}
	}

	// Memory from /proc/meminfo
	if data, err := os.ReadFile(a.procPath("meminfo")); err == nil {
		memInfo := parseMemInfo(string(data))
		if total, ok := memInfo["MemTotal"]; ok {
			pts = append(pts, models.MetricPoint{Name: "node_memory_MemTotal_bytes", Value: total * 1024, Timestamp: now, Labels: labels})
		}
		if free, ok := memInfo["MemAvailable"]; ok {
			pts = append(pts, models.MetricPoint{Name: "node_memory_MemAvailable_bytes", Value: free * 1024, Timestamp: now, Labels: labels})
		}
	}

	// Disk from /proc/diskstats
	if data, err := os.ReadFile(a.procPath("diskstats")); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 14 {
				continue
			}
			dev := fields[2]
			if !strings.HasPrefix(dev, "sd") && !strings.HasPrefix(dev, "nvme") {
				continue
			}
			lbls := copyMap(labels)
			lbls["device"] = dev
			readsCompleted, _ := strconv.ParseFloat(fields[3], 64)
			writesCompleted, _ := strconv.ParseFloat(fields[7], 64)
			pts = append(pts,
				models.MetricPoint{Name: "node_disk_reads_completed_total", Value: readsCompleted, Timestamp: now, Labels: lbls},
				models.MetricPoint{Name: "node_disk_writes_completed_total", Value: writesCompleted, Timestamp: now, Labels: lbls},
			)
		}
	}

	// Network from /proc/net/dev
	if data, err := os.ReadFile(a.procPath("net/dev")); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Scan()
		scanner.Scan() // skip 2 header lines
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			iface := strings.TrimSpace(parts[0])
			if iface == "lo" {
				continue
			}
			fields := strings.Fields(parts[1])
			if len(fields) < 9 {
				continue
			}
			lbls := copyMap(labels)
			lbls["device"] = iface
			rxBytes, _ := strconv.ParseFloat(fields[0], 64)
			txBytes, _ := strconv.ParseFloat(fields[8], 64)
			pts = append(pts,
				models.MetricPoint{Name: "node_network_receive_bytes_total", Value: rxBytes, Timestamp: now, Labels: lbls},
				models.MetricPoint{Name: "node_network_transmit_bytes_total", Value: txBytes, Timestamp: now, Labels: lbls},
			)
		}
	}

	// Filesystem usage from /proc/mounts + statfs syscall
	for _, mount := range []string{"/", "/data", "/var/lib"} {
		var stat syscall_Statfs
		if err := syscallStatfs(mount, &stat); err == nil {
			total := float64(stat.Blocks) * float64(stat.Bsize)
			avail := float64(stat.Bavail) * float64(stat.Bsize)
			lbls := copyMap(labels)
			lbls["mountpoint"] = mount
			pts = append(pts,
				models.MetricPoint{Name: "node_filesystem_size_bytes", Value: total, Timestamp: now, Labels: lbls},
				models.MetricPoint{Name: "node_filesystem_avail_bytes", Value: avail, Timestamp: now, Labels: lbls},
			)
		}
	}

	return pts
}

// ═══════════════════════════════════════════════════════
//  SERVICE REGISTRY
// ═══════════════════════════════════════════════════════

func (a *ObserveXAgent) collectLogs(ctx context.Context) {
	a.logger.Info("starting native log monitor", zap.Strings("paths", a.cfg.LogPaths))
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	a.scanLogFiles(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.scanLogFiles(ctx)
		}
	}
}

func (a *ObserveXAgent) scanLogFiles(ctx context.Context) {
	var batch []models.LogEntry
	for _, path := range a.expandLogPaths() {
		batch = append(batch, a.readNewLogEntries(path)...)
		if len(batch) >= a.cfg.MaxBatchSize {
			a.sender.SendLogs(ctx, batch)
			batch = nil
		}
	}
	if len(batch) > 0 {
		a.sender.SendLogs(ctx, batch)
	}
}

func (a *ObserveXAgent) expandLogPaths() []string {
	seen := map[string]bool{}
	var out []string
	for _, pattern := range a.cfg.LogPaths {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			matches = []string{pattern}
		}
		for _, path := range matches {
			if seen[path] {
				continue
			}
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

func (a *ObserveXAgent) readNewLogEntries(path string) []models.LogEntry {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	a.mu.Lock()
	offset, known := a.logOffsets[path]
	if !known {
		offset = info.Size() - 64*1024
		if offset < 0 {
			offset = 0
		}
	}
	if info.Size() < offset {
		offset = 0
	}
	a.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil
	}

	var entries []models.LogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var current string
	flushCurrent := func() {
		if strings.TrimSpace(current) == "" {
			return
		}
		entries = append(entries, a.logLineToEntry(path, current))
		current = ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && current != "" {
			current += "\n" + line
			continue
		}
		flushCurrent()
		current = line
	}
	flushCurrent()

	a.mu.Lock()
	a.logOffsets[path] = info.Size()
	a.mu.Unlock()
	return entries
}

func (a *ObserveXAgent) logLineToEntry(path, line string) models.LogEntry {
	return models.LogEntry{
		Timestamp: time.Now(),
		Level:     inferLogLevel(line),
		Message:   line,
		ServiceID: inferLogServiceID(path, a.cfg.NodeName),
		Labels: map[string]string{
			"file":    path,
			"source":  "observex-agent",
			"node":    a.cfg.NodeName,
			"cluster": a.cfg.ClusterName,
		},
	}
}

func (a *ObserveXAgent) collectProfiles(ctx context.Context) {
	interval := time.Duration(a.cfg.ProfileIntervalS) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendProcessSnapshot(ctx, int(interval.Seconds()))
		}
	}
}

type processSnapshot struct {
	PID           int     `json:"pid"`
	ParentPID     int     `json:"parent_pid"`
	Process       string  `json:"process"`
	ProcessGroup  string  `json:"process_group"`
	Runtime       string  `json:"runtime"`
	AutoInjection string  `json:"auto_injection"`
	Cmdline       string  `json:"cmdline"`
	ExePath       string  `json:"exe_path"`
	WorkDir       string  `json:"work_dir"`
	CPUSeconds    float64 `json:"cpu_seconds"`
	RSSBytes      uint64  `json:"rss_bytes"`
}

func (a *ObserveXAgent) sendProcessSnapshot(ctx context.Context, durationSec int) {
	data, err := json.Marshal(a.collectProcessSnapshots())
	if err != nil {
		return
	}
	a.sender.SendProfile(ctx, models.Profile{
		ServiceID:   "host:" + a.cfg.NodeName,
		ProfileType: "process_snapshot",
		StartTime:   time.Now(),
		DurationSec: durationSec,
		Data:        data,
	})
}

func (a *ObserveXAgent) collectProcessSnapshots() []processSnapshot {
	entries, err := os.ReadDir(a.cfg.ProcRoot)
	if err != nil {
		return nil
	}
	var out []processSnapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		name := a.getProcName(pid)
		cmdline := a.getProcCmdline(pid)
		statData, err := os.ReadFile(a.procPath(entry.Name(), "stat"))
		if err != nil {
			continue
		}
		fields := strings.Fields(string(statData))
		if len(fields) < 24 {
			continue
		}
		utime, _ := strconv.ParseFloat(fields[13], 64)
		stime, _ := strconv.ParseFloat(fields[14], 64)
		rss, _ := strconv.ParseUint(fields[23], 10, 64)
		out = append(out, processSnapshot{
			PID:           pid,
			ParentPID:     a.getProcParentPID(pid),
			Process:       name,
			ProcessGroup:  inferProcessGroup(name, cmdline, inferRuntime(name, cmdline)),
			Runtime:       inferRuntime(name, cmdline),
			AutoInjection: a.injectionMode(inferRuntime(name, cmdline)),
			Cmdline:       truncateLabel(cmdline, 1024),
			ExePath:       truncateLabel(a.getProcExePath(pid), 1024),
			WorkDir:       truncateLabel(a.getProcWorkDir(pid), 1024),
			CPUSeconds:    (utime + stime) / 100,
			RSSBytes:      rss * uint64(os.Getpagesize()),
		})
	}
	return out
}

func (a *ObserveXAgent) startRuntimeTracing(ctx context.Context) {
	a.logger.Info("starting runtime auto-instrumentation watcher")
	tracer := ebpftrace.NewHTTPTracer(a.logger, a.sender)
	manager := ebpftrace.NewUprobeManager(tracer, a.logger)
	manager.Run(ctx)
}

func (a *ObserveXAgent) upsertService(ctx context.Context, svc *models.Service, evtType watch.EventType) {
	a.mu.Lock()
	existing, exists := a.services[svc.ID]
	if exists {
		svc.DiscoveredAt = existing.DiscoveredAt
	} else {
		svc.DiscoveredAt = time.Now()
	}
	a.services[svc.ID] = svc
	a.mu.Unlock()

	if !exists || evtType == watch.Added {
		a.logger.Info("service discovered",
			zap.String("id", svc.ID),
			zap.String("name", svc.Name),
			zap.String("kind", string(svc.Kind)),
		)
	}

	a.sender.SendService(ctx, svc)
}

// ═══════════════════════════════════════════════════════
//  HEARTBEAT
// ═══════════════════════════════════════════════════════

func (a *ObserveXAgent) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(a.cfg.HeartbeatIntervalS) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.mu.RLock()
			serviceCount := len(a.services)
			flowCount := len(a.netConns)
			a.mu.RUnlock()
			updateState, targetVersion := a.updateSnapshot()
			a.sender.SendHeartbeat(ctx, AgentHeartbeat{
				AgentID:      a.cfg.AgentID,
				OrgID:        a.cfg.OrgID,
				NodeName:     a.cfg.NodeName,
				ClusterName:  a.cfg.ClusterName,
				Environment:  a.cfg.Environment,
				HostGroup:    a.cfg.HostGroup,
				NetworkZone:  a.cfg.NetworkZone,
				MonitoringMode: a.cfg.MonitoringMode,
				CollectionMode:  a.cfg.CollectionMode,
				LogMonitoring: a.cfg.LogMonitoring,
				AutoUpdate:   a.cfg.AutoUpdate,
				UpdateChannel: a.cfg.UpdateChannel,
				UpdateState:  updateState,
				TargetVersion: targetVersion,
				ModuleStatus: a.moduleStatuses(),
				Version:      a.cfg.AgentVersion,
				Status:       "active",
				UptimeSec:    int64(time.Since(a.startedAt).Seconds()),
				ServiceCount: serviceCount,
				FlowCount:    flowCount,
				Features:     agentCapabilities(a.cfg),
				Capabilities: agentCapabilities(a.cfg),
				Timestamp: time.Now(),
			})
		}
	}
}

// ═══════════════════════════════════════════════════════
//  TELEMETRY SENDER
// ═══════════════════════════════════════════════════════

type AgentHeartbeat struct {
	AgentID      string          `json:"agent_id"`
	OrgID        string          `json:"org_id,omitempty"`
	NodeName     string          `json:"node_name"`
	ClusterName  string          `json:"cluster_name"`
	Environment  string          `json:"environment,omitempty"`
	HostGroup    string          `json:"host_group,omitempty"`
	NetworkZone  string          `json:"network_zone,omitempty"`
	MonitoringMode string        `json:"monitoring_mode,omitempty"`
	CollectionMode  string       `json:"collection_mode,omitempty"`
	LogMonitoring bool           `json:"log_monitoring,omitempty"`
	AutoUpdate   bool            `json:"auto_update,omitempty"`
	UpdateChannel string         `json:"update_channel,omitempty"`
	UpdateState  string          `json:"update_state,omitempty"`
	TargetVersion string         `json:"target_version,omitempty"`
	ModuleStatus map[string]string `json:"module_status,omitempty"`
	Version      string          `json:"version"`
	Status       string          `json:"status"`
	UptimeSec    int64           `json:"uptime_sec"`
	ServiceCount int             `json:"service_count"`
	FlowCount    int             `json:"flow_count"`
	Features     map[string]bool `json:"features"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
}

type TelemetrySender struct {
	baseURL     string
	client      *http.Client
	logger      *zap.Logger
	buffer      []models.MetricPoint
	mu          sync.Mutex
	flushTimer  *time.Ticker
	// Identity headers sent to ActiveGate for agent tracking
	agentID     string
	orgID       string
	nodeName    string
	clusterName string
	environment string
	hostGroup   string
	networkZone string
	monitoringMode string
	collectionMode string
	version     string
	token       string
	maxBatchSize int
	maxQueueSize int
}

func NewTelemetrySender(cfg Config, logger *zap.Logger) *TelemetrySender {
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 1000
	}
	if cfg.MaxQueueSize <= cfg.MaxBatchSize {
		cfg.MaxQueueSize = cfg.MaxBatchSize * 10
	}
	client := &http.Client{Timeout: 10 * time.Second}
	if cfg.ProxyURL != "" {
		if proxyURL, err := url.Parse(cfg.ProxyURL); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		} else {
			logger.Warn("invalid agent proxy URL ignored", zap.String("proxy_url", cfg.ProxyURL), zap.Error(err))
		}
	}
	s := &TelemetrySender{
		baseURL:     strings.TrimRight(cfg.IngestorURL, "/"),
		client:      client,
		logger:      logger,
		flushTimer:  time.NewTicker(time.Duration(cfg.FlushIntervalS) * time.Second),
		agentID:     cfg.AgentID,
		orgID:       cfg.OrgID,
		nodeName:    cfg.NodeName,
		clusterName: cfg.ClusterName,
		environment: cfg.Environment,
		hostGroup:   cfg.HostGroup,
		networkZone: cfg.NetworkZone,
		monitoringMode: cfg.MonitoringMode,
		collectionMode: cfg.CollectionMode,
		version:     cfg.AgentVersion,
		token:       cfg.AgentToken,
		maxBatchSize: cfg.MaxBatchSize,
		maxQueueSize: cfg.MaxQueueSize,
	}
	go s.flushLoop()
	return s
}

func (s *TelemetrySender) Close() {
	if s.flushTimer != nil {
		s.flushTimer.Stop()
	}
}

func (s *TelemetrySender) flushLoop() {
	for range s.flushTimer.C {
		s.mu.Lock()
		if len(s.buffer) == 0 {
			s.mu.Unlock()
			continue
		}
		toSend := s.buffer
		s.buffer = nil
		s.mu.Unlock()
		if !s.postJSON("/v1/metrics/batch", toSend) {
			s.requeueMetrics(toSend)
		}
	}
}

func (s *TelemetrySender) SendService(ctx context.Context, svc *models.Service) {
	s.postJSONCtx(ctx, "/v1/services", svc)
}

func (s *TelemetrySender) SendAgent(ctx context.Context, agent models.Agent) {
	s.postJSONCtx(ctx, "/v1/agents/register", agent)
}

func (s *TelemetrySender) SendMetrics(ctx context.Context, pts []models.MetricPoint) {
	pts = s.enrichMetrics(pts)
	s.mu.Lock()
	s.buffer = append(s.buffer, pts...)
	if s.maxQueueSize > 0 && len(s.buffer) > s.maxQueueSize {
		dropped := len(s.buffer) - s.maxQueueSize
		s.buffer = s.buffer[dropped:]
		s.logger.Warn("metric buffer full; dropped oldest points", zap.Int("dropped", dropped))
	}
	shouldFlush := len(s.buffer) >= s.maxBatchSize
	s.mu.Unlock()

	if shouldFlush {
		s.mu.Lock()
		toSend := s.buffer
		s.buffer = nil
		s.mu.Unlock()
		go func() {
			if !s.postJSON("/v1/metrics/batch", toSend) {
				s.requeueMetrics(toSend)
			}
		}()
	}
}

func (s *TelemetrySender) SendLogs(ctx context.Context, logs []models.LogEntry) {
	if len(logs) == 0 {
		return
	}
	for i := range logs {
		if logs[i].Labels == nil {
			logs[i].Labels = map[string]string{}
		}
		if s.orgID != "" {
			logs[i].Labels["org"] = s.orgID
		}
		logs[i].Labels["agent_id"] = s.agentID
		logs[i].Labels["node"] = s.nodeName
		logs[i].Labels["cluster"] = s.clusterName
		logs[i].Labels["environment"] = s.environment
		logs[i].Labels["host_group"] = s.hostGroup
		logs[i].Labels["network_zone"] = s.networkZone
		logs[i].Labels["collection_mode"] = s.collectionMode
	}
	s.postJSONCtx(ctx, "/v1/logs/batch", logs)
}

func (s *TelemetrySender) SendSpans(ctx context.Context, spans []models.Span) {
	if len(spans) == 0 {
		return
	}
	for i := range spans {
		if spans[i].Attrs == nil {
			spans[i].Attrs = map[string]string{}
		}
		if s.orgID != "" {
			spans[i].Attrs["org"] = s.orgID
		}
		spans[i].Attrs["agent_id"] = s.agentID
		spans[i].Attrs["node"] = s.nodeName
		spans[i].Attrs["cluster"] = s.clusterName
		spans[i].Attrs["environment"] = s.environment
		spans[i].Attrs["collection_mode"] = s.collectionMode
	}
	s.postJSONCtx(ctx, "/v1/traces/batch", spans)
}

func (s *TelemetrySender) SendProfile(ctx context.Context, profile models.Profile) {
	s.postJSONCtx(ctx, "/v1/profiles", profile)
}

func (s *TelemetrySender) SendSpan(ctx context.Context, span ebpftrace.HTTPSpan) error {
	status := "ok"
	if span.StatusCode >= 500 {
		status = "error"
	}
	attrs := map[string]string{}
	for k, v := range span.Attrs {
		attrs[k] = v
	}
	attrs["process.pid"] = fmt.Sprintf("%d", span.PID)
	attrs["telemetry.agent"] = "observex-agent"
	s.SendSpans(ctx, []models.Span{{
		TraceID:       span.TraceID,
		SpanID:        span.SpanID,
		ParentSpanID:  span.ParentID,
		OperationName: strings.TrimSpace(span.Method + " " + span.Path),
		ServiceID:     "proc:" + span.Service,
		ServiceName:   span.Service,
		StartTime:     span.StartTime,
		EndTime:       span.EndTime,
		DurationMs:    span.DurationMs,
		Status:        status,
		Attrs:         attrs,
		Kind:          "server",
	}})
	return nil
}

func (s *TelemetrySender) SendTopoEdge(ctx context.Context, edge models.TopoEdge) {
	s.postJSONCtx(ctx, "/v1/topology/edges", edge)
}

func (s *TelemetrySender) SendHeartbeat(ctx context.Context, hb AgentHeartbeat) {
	s.postJSONCtx(ctx, fmt.Sprintf("/v1/agents/%s/heartbeat", hb.AgentID), hb)
}

func (s *TelemetrySender) SendSecurityEvent(ctx context.Context, evt models.SecurityEvent) {
	s.postJSONCtx(ctx, "/v1/security/events", evt)
}

func (s *TelemetrySender) postJSONCtx(ctx context.Context, path string, payload any) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		s.logger.Warn("marshal telemetry payload failed", zap.String("path", path), zap.Error(err))
		return false
	}

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+path, bytes.NewReader(data))
		if err != nil {
			s.logger.Warn("create telemetry request failed", zap.String("path", path), zap.Error(err))
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		s.addIdentityHeaders(req)

		resp, err := s.client.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return true
			}
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				s.logger.Warn("telemetry rejected", zap.String("path", path), zap.Int("status", resp.StatusCode))
				return false
			}
		} else {
			s.logger.Debug("send failed", zap.String("path", path), zap.Int("attempt", attempt+1), zap.Error(err))
		}

		delay := time.Duration(250*(1<<attempt)) * time.Millisecond
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
	}
	return false
}

func (s *TelemetrySender) postJSON(path string, payload any) bool {
	return s.postJSONCtx(context.Background(), path, payload)
}

// ═══════════════════════════════════════════════════════
//  HELPERS & DETECTION LOGIC
// ═══════════════════════════════════════════════════════

func (s *TelemetrySender) addIdentityHeaders(req *http.Request) {
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
		req.Header.Set("X-ObserveX-Token", s.token)
	}
	if s.agentID != "" {
		req.Header.Set("X-Agent-ID", s.agentID)
	}
	if s.orgID != "" {
		req.Header.Set("X-ObserveX-Org", s.orgID)
	}
	req.Header.Set("X-Node-Name", s.nodeName)
	req.Header.Set("X-Cluster", s.clusterName)
	req.Header.Set("X-ObserveX-Env", s.environment)
	req.Header.Set("X-Host-Group", s.hostGroup)
	req.Header.Set("X-Network-Zone", s.networkZone)
	req.Header.Set("X-Monitoring-Mode", s.monitoringMode)
	req.Header.Set("X-Collection-Mode", s.collectionMode)
	req.Header.Set("X-Agent-Version", s.version)
}

func (s *TelemetrySender) enrichMetrics(pts []models.MetricPoint) []models.MetricPoint {
	out := make([]models.MetricPoint, len(pts))
	copy(out, pts)
	for i := range out {
		if out[i].Labels == nil {
			out[i].Labels = map[string]string{}
		}
		if s.orgID != "" {
			out[i].Labels["org"] = s.orgID
		}
		out[i].Labels["agent_id"] = s.agentID
		out[i].Labels["node"] = s.nodeName
		out[i].Labels["cluster"] = s.clusterName
		out[i].Labels["environment"] = s.environment
		out[i].Labels["host_group"] = s.hostGroup
		out[i].Labels["network_zone"] = s.networkZone
		out[i].Labels["monitoring_mode"] = s.monitoringMode
		out[i].Labels["collection_mode"] = s.collectionMode
	}
	return out
}

func (s *TelemetrySender) requeueMetrics(pts []models.MetricPoint) {
	if len(pts) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer = append(pts, s.buffer...)
	if s.maxQueueSize > 0 && len(s.buffer) > s.maxQueueSize {
		dropped := len(s.buffer) - s.maxQueueSize
		s.buffer = s.buffer[:s.maxQueueSize]
		s.logger.Warn("metric retry buffer full; dropped newest points", zap.Int("dropped", dropped))
	}
}

func detectPodKind(pod *corev1.Pod) models.ServiceKind {
	labels := pod.Labels
	for _, key := range []string{"app", "app.kubernetes.io/name", "k8s-app"} {
		if app, ok := labels[key]; ok {
			switch strings.ToLower(app) {
			case "postgres", "postgresql":
				return models.SvcDatabase
			case "mysql", "mariadb":
				return models.SvcDatabase
			case "mongodb", "mongo":
				return models.SvcDatabase
			case "redis":
				return models.SvcCache
			case "memcached":
				return models.SvcCache
			case "kafka":
				return models.SvcQueue
			case "rabbitmq":
				return models.SvcQueue
			case "nats":
				return models.SvcQueue
			case "elasticsearch", "opensearch":
				return models.SvcDatabase
			}
		}
	}

	// Check container images
	for _, c := range pod.Spec.Containers {
		img := strings.ToLower(c.Image)
		for _, db := range []string{"postgres", "mysql", "mongo", "mariadb", "cassandra", "elasticsearch"} {
			if strings.Contains(img, db) {
				return models.SvcDatabase
			}
		}
		for _, q := range []string{"kafka", "rabbitmq", "nats", "activemq", "pulsar"} {
			if strings.Contains(img, q) {
				return models.SvcQueue
			}
		}
		for _, cache := range []string{"redis", "memcached"} {
			if strings.Contains(img, cache) {
				return models.SvcCache
			}
		}
	}

	// Check ports
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			switch p.ContainerPort {
			case 5432, 3306, 27017, 9042:
				return models.SvcDatabase
			case 6379, 11211:
				return models.SvcCache
			case 9092, 5672, 4222:
				return models.SvcQueue
			case 50051:
				return models.SvcGRPC
			}
		}
	}

	return models.SvcHTTP
}

func detectTechStack(pod *corev1.Pod) []string {
	var stack []string
	seen := map[string]bool{}

	for _, c := range pod.Spec.Containers {
		img := strings.ToLower(c.Image)
		for _, tech := range []string{"go", "python", "java", "node", "ruby", "php", "rust", "dotnet", "postgres", "mysql", "redis", "kafka", "nginx"} {
			if strings.Contains(img, tech) && !seen[tech] {
				stack = append(stack, tech)
				seen[tech] = true
			}
		}
	}

	return stack
}

func podDisplayName(pod *corev1.Pod) string {
	for _, key := range []string{"app", "app.kubernetes.io/name"} {
		if v := pod.Labels[key]; v != "" {
			return v
		}
	}
	// Strip hash suffix from pod name
	parts := strings.Split(pod.Name, "-")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return pod.Name
}

func podDeployment(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" {
			parts := strings.Split(ref.Name, "-")
			if len(parts) > 1 {
				return strings.Join(parts[:len(parts)-1], "-")
			}
		}
		if ref.Kind == "StatefulSet" || ref.Kind == "DaemonSet" {
			return ref.Name
		}
	}
	return ""
}

func podHealth(pod *corev1.Pod) models.Health {
	if pod.Status.Phase == corev1.PodRunning {
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				return models.Health{State: models.HealthDegraded, Score: 50, Since: time.Now()}
			}
		}
		return models.Health{State: models.HealthGood, Score: 100, Since: time.Now()}
	}
	if pod.Status.Phase == corev1.PodFailed {
		return models.Health{State: models.HealthBad, Score: 0, Since: time.Now()}
	}
	return models.Health{State: models.HealthUnknown, Score: 50, Since: time.Now()}
}

func nodeHealth(node *corev1.Node) models.Health {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionTrue {
				return models.Health{State: models.HealthGood, Score: 100}
			}
			return models.Health{State: models.HealthBad, Score: 0, Message: cond.Message}
		}
	}
	return models.Health{State: models.HealthUnknown, Score: 50}
}

func parseMemInfo(data string) map[string]float64 {
	result := map[string]float64{}
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 {
			key := strings.TrimSuffix(parts[0], ":")
			val, _ := strconv.ParseFloat(parts[1], 64)
			result[key] = val
		}
	}
	return result
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func buildK8sClient(logger *zap.Logger) *kubernetes.Clientset {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		home, _ := os.UserHomeDir()
		cfg, err = clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
		if err != nil {
			logger.Warn("K8s client unavailable — running in process-only mode")
			return nil
		}
	}
	client, _ := kubernetes.NewForConfig(cfg)
	return client
}

func configPathFromArgs() string {
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--config" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
		}
	}
	if v := os.Getenv("OBSERVEX_CONFIG"); v != "" {
		return v
	}
	if runtime.GOOS != "windows" {
		if _, err := os.Stat("/etc/observex/agent.yaml"); err == nil {
			return "/etc/observex/agent.yaml"
		}
	}
	return ""
}

func loadAgentConfigFile(path string) map[string]string {
	cfg := map[string]string{}
	if path == "" {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		key, value, ok := splitConfigLine(line)
		if !ok {
			continue
		}
		cfg[normalizeConfigKey(key)] = cleanConfigValue(value)
	}
	return cfg
}

func splitConfigLine(line string) (string, string, bool) {
	if idx := strings.Index(line, ":"); idx >= 0 {
		return line[:idx], line[idx+1:], true
	}
	if idx := strings.Index(line, "="); idx >= 0 {
		return line[:idx], line[idx+1:], true
	}
	return "", "", false
}

func normalizeConfigKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, ".", "_")
	return key
}

func cleanConfigValue(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}

func cfgString(fileCfg map[string]string, keys []string, envs []string, fallback string) string {
	for _, env := range envs {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	for _, key := range keys {
		if v := strings.TrimSpace(fileCfg[normalizeConfigKey(key)]); v != "" {
			return v
		}
	}
	return fallback
}

func cfgInt(fileCfg map[string]string, keys []string, envs []string, fallback int) int {
	if v := cfgString(fileCfg, keys, envs, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func cfgBool(fileCfg map[string]string, keys []string, envs []string, fallback bool) bool {
	if v := cfgString(fileCfg, keys, envs, ""); v != "" {
		if parsed, ok := parseBool(v); ok {
			return parsed
		}
	}
	return fallback
}

func cfgList(fileCfg map[string]string, keys []string, envs []string, fallback []string) []string {
	raw := cfgString(fileCfg, keys, envs, "")
	if raw == "" {
		return fallback
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "enabled", "on":
		return true, true
	case "0", "false", "no", "n", "disabled", "off":
		return false, true
	default:
		return false, false
	}
}

func defaultLogPaths() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return []string{
		"/var/log/syslog",
		"/var/log/messages",
		"/var/log/*.log",
		"/var/log/containers/*.log",
		"/var/log/pods/*/*/*.log",
	}
}

func (a *ObserveXAgent) procPath(parts ...string) string {
	all := append([]string{a.cfg.ProcRoot}, parts...)
	return filepath.Join(all...)
}

func inferServiceFromProcess(port int, procName, runtimeName string) struct {
	Kind models.ServiceKind
	Name string
	Tech []string
} {
	name := strings.ToLower(procName)
	if name == "" || name == "unknown" {
		name = fmt.Sprintf("tcp-service-%d", port)
	}
	kind := models.SvcProcess
	if port == 80 || port == 443 || port == 8080 || port == 8443 || port == 3000 || port == 5000 || port == 8000 || port == 9000 {
		kind = models.SvcHTTP
	}
	tech := []string{}
	if runtimeName != "" && runtimeName != "unknown" {
		tech = append(tech, runtimeName)
	}
	return struct {
		Kind models.ServiceKind
		Name string
		Tech []string
	}{Kind: kind, Name: name, Tech: tech}
}

func inferRuntime(procName, cmdline string) string {
	text := strings.ToLower(procName + " " + cmdline)
	switch {
	case strings.Contains(text, "java") || strings.Contains(text, ".jar"):
		return "java"
	case strings.Contains(text, "node") || strings.Contains(text, "nodejs") || strings.Contains(text, ".js"):
		return "node"
	case strings.Contains(text, "python") || strings.Contains(text, "gunicorn") || strings.Contains(text, "uvicorn"):
		return "python"
	case strings.Contains(text, "dotnet"):
		return "dotnet"
	case strings.Contains(text, "nginx"):
		return "nginx"
	case strings.Contains(text, "ruby"):
		return "ruby"
	case strings.Contains(text, "php"):
		return "php"
	default:
		return "unknown"
	}
}

func inferProcessGroup(procName, cmdline, runtimeName string) string {
	text := strings.ToLower(procName + " " + cmdline)
	if runtimeName != "" && runtimeName != "unknown" {
		if strings.Contains(text, "spring") || strings.Contains(text, "quarkus") || strings.Contains(text, "micronaut") {
			return runtimeName + ":application"
		}
		return runtimeName + ":runtime"
	}
	switch {
	case strings.Contains(text, "postgres"):
		return "postgresql:database"
	case strings.Contains(text, "mysql") || strings.Contains(text, "mariadb"):
		return "mysql:database"
	case strings.Contains(text, "redis"):
		return "redis:cache"
	case strings.Contains(text, "kafka"):
		return "kafka:messaging"
	case strings.Contains(text, "nginx"):
		return "nginx:web"
	case procName != "" && procName != "unknown":
		return procName + ":process"
	default:
		return "unknown"
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func truncateLabel(v string, limit int) string {
	if limit <= 0 || len(v) <= limit {
		return v
	}
	return v[:limit]
}

func inferLogLevel(line string) string {
	text := strings.ToLower(line)
	switch {
	case strings.Contains(text, "fatal") || strings.Contains(text, "panic"):
		return "fatal"
	case strings.Contains(text, "error") || strings.Contains(text, " err "):
		return "error"
	case strings.Contains(text, "warn"):
		return "warn"
	case strings.Contains(text, "debug"):
		return "debug"
	default:
		return "info"
	}
}

func inferLogServiceID(path, nodeName string) string {
	base := filepath.Base(path)
	if strings.Contains(path, "/containers/") || strings.Contains(path, `\containers\`) {
		parts := strings.Split(base, "_")
		if len(parts) >= 2 {
			return "k8s:" + parts[0] + ":" + parts[1]
		}
	}
	if strings.Contains(path, "/pods/") || strings.Contains(path, `\pods\`) {
		return "k8s-log:" + strings.TrimSuffix(base, filepath.Ext(base))
	}
	return "host:" + nodeName
}

func agentCapabilities(cfg Config) map[string]bool {
	return map[string]bool{
		"fullstack":               cfg.MonitoringMode == "fullstack",
		"infrastructure":          cfg.MonitoringMode == "infrastructure",
		"ebpf":                    cfg.EnableEBPF,
		"kubernetes":              cfg.EnableK8s,
		"process_discovery":       cfg.EnableProc,
		"profiling":               cfg.EnableProfiling,
		"native_collection":       cfg.CollectionMode == "native",
		"native_host_metrics":     cfg.EnableNativeMetrics,
		"native_process_metrics":  cfg.EnableProc && cfg.EnableNativeMetrics,
		"runtime_tracing":         cfg.EnableRuntimeTracing,
		"log_monitoring":          cfg.LogMonitoring,
		"activegate_routing":      strings.Contains(strings.ToLower(cfg.IngestorURL), "activegate") || cfg.NetworkZone != "",
		"auto_update":             cfg.AutoUpdate,
		"signed_update_staging":   cfg.AutoUpdate && cfg.UpdatePublicKey != "",
		"module_watchdog":         true,
		"runtime_state":           true,
		"auto_injection":          cfg.EnableAutoInjection,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || strings.ToLower(v) == "true"
	}
	return fallback
}

func loadOrCreateAgentID(path, nodeName string) string {
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	id := "agent-" + sanitizeIDPart(nodeName) + "-" + randomHex(8)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err == nil {
		_ = os.WriteFile(path, []byte(id+"\n"), 0600)
	}
	return id
}

func randomHex(n int) string {
	if n <= 0 {
		n = 8
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func sanitizeIDPart(v string) string {
	v = strings.ToLower(v)
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "node"
	}
	return b.String()
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

// syscall_Statfs is a minimal Statfs_t for disk usage
type syscall_Statfs struct {
	Bsize  int64
	Blocks uint64
	Bfree  uint64
	Bavail uint64
}

func syscallStatfs(path string, stat *syscall_Statfs) error {
	// Uses syscall.Statfs internally — implemented in observex-agent_linux.go
	return statfs(path, stat)
}
