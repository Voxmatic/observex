// pkg/models/models.go
package models

import "time"

// ═══════════════════════════════════════════════════════
//  CORE ENTITIES
// ═══════════════════════════════════════════════════════

type ServiceKind string

const (
	SvcHTTP        ServiceKind = "http"
	SvcGRPC        ServiceKind = "grpc"
	SvcDatabase    ServiceKind = "database"
	SvcCache       ServiceKind = "cache"
	SvcQueue       ServiceKind = "queue"
	SvcK8sPod      ServiceKind = "k8s_pod"
	SvcK8sNode     ServiceKind = "k8s_node"
	SvcK8sService  ServiceKind = "k8s_service"
	SvcK8sDeploy   ServiceKind = "k8s_deployment"
	SvcVM          ServiceKind = "vm"
	SvcProcess     ServiceKind = "process"
	SvcLambda      ServiceKind = "lambda"
	SvcUnknown     ServiceKind = "unknown"
)

type HealthState string

const (
	HealthGood     HealthState = "GOOD"
	HealthDegraded HealthState = "DEGRADED"
	HealthBad      HealthState = "BAD"
	HealthUnknown  HealthState = "UNKNOWN"
)

// Service — a monitored entity (pod, process, DB, queue, VM, etc.)
type Service struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	DisplayName  string            `json:"display_name"`
	Kind         ServiceKind       `json:"kind"`
	Namespace    string            `json:"namespace,omitempty"`
	ClusterName  string            `json:"cluster_name,omitempty"`
	NodeName     string            `json:"node_name,omitempty"`
	PodName      string            `json:"pod_name,omitempty"`
	Deployment   string            `json:"deployment,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Endpoints    []Endpoint        `json:"endpoints,omitempty"`
	Health       Health            `json:"health"`
	AgentID      string            `json:"agent_id"`
	DiscoveredAt time.Time         `json:"discovered_at"`
	LastSeenAt   time.Time         `json:"last_seen_at"`
	Tags         []string          `json:"tags,omitempty"`
	Version      string            `json:"version,omitempty"`
	TechStack    []string          `json:"tech_stack,omitempty"` // ["go", "postgres", "redis"]
}

type Endpoint struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type Health struct {
	State   HealthState `json:"state"`
	Score   float64     `json:"score"` // 0-100
	Message string      `json:"message,omitempty"`
	Since   time.Time   `json:"since"`
}

// TopoEdge — a dependency between two services
type TopoEdge struct {
	ID           string    `json:"id"`
	SourceID     string    `json:"source_id"`
	TargetID     string    `json:"target_id"`
	Protocol     string    `json:"protocol"`
	CallsPerMin  float64   `json:"calls_per_min"`
	AvgLatencyMs float64   `json:"avg_latency_ms"`
	ErrorRate    float64   `json:"error_rate"`
	BytesPerSec  float64   `json:"bytes_per_sec"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ═══════════════════════════════════════════════════════
//  TELEMETRY
// ═══════════════════════════════════════════════════════

type MetricPoint struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	ServiceID string            `json:"service_id"`
}

type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	ServiceID string            `json:"service_id"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
	Labels    map[string]string `json:"labels"`
	Body      map[string]any    `json:"body,omitempty"`
}

type Span struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	OperationName string           `json:"operation_name"`
	ServiceID    string            `json:"service_id"`
	ServiceName  string            `json:"service_name"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	DurationMs   float64           `json:"duration_ms"`
	Status       string            `json:"status"` // ok, error, unset
	StatusMsg    string            `json:"status_message,omitempty"`
	Attrs        map[string]string `json:"attrs,omitempty"`
	Events       []SpanEvent       `json:"events,omitempty"`
	Kind         string            `json:"kind"` // server, client, producer, consumer
}

type SpanEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	Name      string            `json:"name"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

type Profile struct {
	ServiceID   string    `json:"service_id"`
	ProfileType string    `json:"profile_type"` // cpu, memory, goroutine, block, mutex
	StartTime   time.Time `json:"start_time"`
	DurationSec int       `json:"duration_sec"`
	Data        []byte    `json:"data"` // pprof format
}

// ═══════════════════════════════════════════════════════
//  INCIDENTS & PROBLEMS
// ═══════════════════════════════════════════════════════

type Severity string

const (
	SevCritical Severity = "CRITICAL"
	SevHigh     Severity = "HIGH"
	SevMedium   Severity = "MEDIUM"
	SevLow      Severity = "LOW"
	SevInfo     Severity = "INFO"
)

type ProblemClass string

const (
	ProbCrashLoop       ProblemClass = "CRASH_LOOP"
	ProbHighRestarts    ProblemClass = "HIGH_RESTARTS"
	ProbOOMKilled       ProblemClass = "OOM_KILLED"
	ProbPodPending      ProblemClass = "POD_PENDING"
	ProbNodeNotReady    ProblemClass = "NODE_NOT_READY"
	ProbImagePullFail   ProblemClass = "IMAGE_PULL_FAILURE"
	ProbDeployStuck     ProblemClass = "DEPLOY_STUCK"
	ProbServiceDown     ProblemClass = "SERVICE_DOWN"
	ProbHighCPU         ProblemClass = "HIGH_CPU"
	ProbHighMemory      ProblemClass = "HIGH_MEMORY"
	ProbHighLatency     ProblemClass = "HIGH_LATENCY"
	ProbHighErrorRate   ProblemClass = "HIGH_ERROR_RATE"
	ProbDiskFull        ProblemClass = "DISK_FULL"
	ProbQueueLag        ProblemClass = "QUEUE_LAG"
	ProbDBSlow          ProblemClass = "DB_SLOW_QUERIES"
	ProbMemoryLeak      ProblemClass = "MEMORY_LEAK"
	ProbCertExpiry      ProblemClass = "CERT_EXPIRY"
	ProbNetworkLoss     ProblemClass = "NETWORK_LOSS"
	ProbDependencyFail  ProblemClass = "DEPENDENCY_FAILURE"
	ProbUnknown         ProblemClass = "UNKNOWN"
	ProbRegression      ProblemClass = "REGRESSION"  // post-deploy metric regression
)

type Problem struct {
	ID          string            `json:"id"`
	Class       ProblemClass      `json:"class"`
	Severity    Severity          `json:"severity"`
	Title       string            `json:"title"`
	Detail      string            `json:"detail"`
	RootCause   string            `json:"root_cause"`
	ServiceID   string            `json:"service_id"`
	ServiceName string            `json:"service_name"`
	Namespace   string            `json:"namespace"`
	PodName     string            `json:"pod_name,omitempty"`
	NodeName    string            `json:"node_name,omitempty"`
	Deployment  string            `json:"deployment,omitempty"`
	Metrics     map[string]float64 `json:"metrics"`
	Evidence    []string          `json:"evidence"`
	Confidence  float64           `json:"confidence"`
	Status      string            `json:"status"` // open, remediating, resolved, escalated
	DetectedAt  time.Time         `json:"detected_at"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ═══════════════════════════════════════════════════════
//  REMEDIATIONS
// ═══════════════════════════════════════════════════════

type RemediationStatus string

const (
	RemSuccess   RemediationStatus = "SUCCESS"
	RemFailed    RemediationStatus = "FAILED"
	RemSkipped   RemediationStatus = "SKIPPED"
	RemEscalated RemediationStatus = "ESCALATED"
	RemDryRun    RemediationStatus = "DRY_RUN"
)

type Remediation struct {
	ID          string            `json:"id"`
	ProblemID   string            `json:"problem_id"`
	Action      string            `json:"action"`
	Params      map[string]string `json:"params"`
	Status      RemediationStatus `json:"status"`
	Detail      string            `json:"detail"`
	DryRun      bool              `json:"dry_run"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	VerifyResult string           `json:"verify_result,omitempty"`
}

// ═══════════════════════════════════════════════════════
//  SLOs
// ═══════════════════════════════════════════════════════

type SLOKind string

const (
	SLOAvailability SLOKind = "availability"
	SLOLatency      SLOKind = "latency"
	SLOErrorRate    SLOKind = "error_rate"
	SLOThroughput   SLOKind = "throughput"
)

type SLO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ServiceID    string    `json:"service_id"`
	ServiceName  string    `json:"service_name"`
	Kind         SLOKind   `json:"kind"`
	Target       float64   `json:"target"`
	Window       string    `json:"window"` // 7d, 30d
	SLI          float64   `json:"sli"`    // current value
	BudgetTotal  float64   `json:"budget_total"`
	BudgetLeft   float64   `json:"budget_left"`
	BurnRate1h   float64   `json:"burn_rate_1h"`
	BurnRate6h   float64   `json:"burn_rate_6h"`
	BurnRate24h  float64   `json:"burn_rate_24h"`
	Status       string    `json:"status"` // ok, warning, breached
	UpdatedAt    time.Time `json:"updated_at"`
}

// ═══════════════════════════════════════════════════════
//  ALERTS
// ═══════════════════════════════════════════════════════

type AlertRule struct {
	ID                   string            `json:"id"`
	OrgID                string            `json:"org_id,omitempty"`
	Name                 string            `json:"name"`
	ServiceID            string            `json:"service_id,omitempty"`
	Namespace            string            `json:"namespace,omitempty"`
	Expr                 string            `json:"expr"`
	Threshold            float64           `json:"threshold"`
	Operator             string            `json:"operator"`
	Duration             string            `json:"duration"`
	Severity             Severity          `json:"severity"`
	Message              string            `json:"message"`
	Labels               map[string]string `json:"labels"`
	Silenced             bool              `json:"silenced"`
	NotificationChannels string            `json:"notification_channels,omitempty"`
	RunbookURL           string            `json:"runbook_url,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
}

type FiredAlert struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	ServiceID   string            `json:"service_id,omitempty"`
	Severity    Severity          `json:"severity"`
	Value       float64           `json:"value"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels"`
	FiredAt     time.Time         `json:"fired_at"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	State       string            `json:"state"`
}

// ═══════════════════════════════════════════════════════
//  DASHBOARDS
// ═══════════════════════════════════════════════════════

type Dashboard struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	OwnerID     string          `json:"owner_id"`
	Tags        []string        `json:"tags"`
	Widgets     []Widget        `json:"widgets"`
	Variables   []DashVariable  `json:"variables"`
	TimeRange   string          `json:"time_range"`
	AutoRefresh int             `json:"auto_refresh_sec"`
	IsTemplate  bool            `json:"is_template"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Widget struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"` // timeseries|stat|gauge|bar|pie|heatmap|table|logs|topology|flamegraph|text|alertlist|slo
	Title      string         `json:"title"`
	GridPos    GridPos        `json:"grid_pos"`
	DataSource string         `json:"data_source"` // metrics|logs|traces|topology|slos|alerts
	Query      string         `json:"query"`
	Legend     string         `json:"legend,omitempty"`
	Options    map[string]any `json:"options"`
	Thresholds []Threshold    `json:"thresholds"`
}

type GridPos struct { X, Y, W, H int }

type Threshold struct {
	Value float64 `json:"value"`
	Color string  `json:"color"`
	Label string  `json:"label"`
}

type DashVariable struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // query|custom|textbox|interval
	Query   string   `json:"query,omitempty"`
	Options []string `json:"options,omitempty"`
	Current string   `json:"current"`
	Multi   bool     `json:"multi"`
}

// ═══════════════════════════════════════════════════════
//  AGENTS
// ═══════════════════════════════════════════════════════

type Agent struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id,omitempty"`
	NodeName    string    `json:"node_name"`
	ClusterName string    `json:"cluster_name"`
	Environment string    `json:"environment,omitempty"`
	HostGroup   string    `json:"host_group,omitempty"`
	NetworkZone string    `json:"network_zone,omitempty"`
	MonitoringMode string  `json:"monitoring_mode,omitempty"`
	CollectionMode string  `json:"collection_mode,omitempty"`
	LogMonitoring bool     `json:"log_monitoring,omitempty"`
	AutoUpdate  bool      `json:"auto_update,omitempty"`
	UpdateChannel string   `json:"update_channel,omitempty"`
	UpdateState string     `json:"update_state,omitempty"`
	TargetVersion string   `json:"target_version,omitempty"`
	Version     string    `json:"version"`
	IPAddress   string    `json:"ip_address"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	KernelVer   string    `json:"kernel_version"`
	EBPFEnabled bool      `json:"ebpf_enabled"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	ModuleStatus map[string]string `json:"module_status,omitempty"`
	Status      string    `json:"status"` // active, inactive, error
	LastSeen    time.Time `json:"last_seen"`
	RegisteredAt time.Time `json:"registered_at"`
}

// ═══════════════════════════════════════════════════════
//  NOTIFICATIONS
// ═══════════════════════════════════════════════════════

type Notification struct {
	ID        string    `json:"id"`
	ProblemID string    `json:"problem_id"`
	Channel   string    `json:"channel"` // slack|pagerduty|email|webhook
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	AutoFixed bool      `json:"auto_fixed"`
	FixDetail string    `json:"fix_detail,omitempty"`
	SentAt    time.Time `json:"sent_at"`
	Status    string    `json:"status"` // sent|failed
}

// ═══════════════════════════════════════════════════════
//  SECURITY MONITORING
// ═══════════════════════════════════════════════════════

// SecurityEvent is a runtime threat detection event from the eBPF security
// monitor. Created when a suspicious syscall is detected on any cluster node.
type SecurityEvent struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	EventType      string    `json:"event_type"`    // exec | connect | file_open | ptrace
	Severity       string    `json:"severity"`      // CRITICAL | HIGH | MEDIUM | LOW | INFO
	PID            int       `json:"pid"`
	PPID           int       `json:"ppid"`
	UID            int       `json:"uid"`
	ProcessName    string    `json:"process_name"`
	Path           string    `json:"path,omitempty"`  // execve path or file path
	Args           string    `json:"args,omitempty"`
	DstIP          string    `json:"dst_ip,omitempty"`
	DstPort        int       `json:"dst_port,omitempty"`
	NodeName       string    `json:"node_name"`
	ClusterName    string    `json:"cluster_name"`
	Namespace      string    `json:"namespace,omitempty"`
	PodName        string    `json:"pod_name,omitempty"`
	ContainerImage string    `json:"container_image,omitempty"`
	Description    string    `json:"description"`
	MitreTactic    string    `json:"mitre_tactic,omitempty"`
	MitreTechnique string    `json:"mitre_technique,omitempty"`
	MitreTechID    string    `json:"mitre_tech_id,omitempty"`
	Suppressed     bool      `json:"suppressed,omitempty"` // true if auto-suppressed as noise
}

// ─── Vulnerability (from Trivy image scans) ───────────────────────────────

// VulnSeverity matches Trivy's severity levels.
type VulnSeverity string

const (
	VulnCritical VulnSeverity = "CRITICAL"
	VulnHigh     VulnSeverity = "HIGH"
	VulnMedium   VulnSeverity = "MEDIUM"
	VulnLow      VulnSeverity = "LOW"
	VulnUnknown  VulnSeverity = "UNKNOWN"
)

// Vulnerability is a single CVE finding from a Trivy scan.
type Vulnerability struct {
	VulnID          string       `json:"vuln_id"`          // CVE-2024-XXXX
	PkgName         string       `json:"pkg_name"`
	InstalledVersion string      `json:"installed_version"`
	FixedVersion    string       `json:"fixed_version,omitempty"`
	Severity        VulnSeverity `json:"severity"`
	Title           string       `json:"title"`
	Description     string       `json:"description,omitempty"`
	CVSS            float64      `json:"cvss,omitempty"`    // CVSS v3 score
	PublishedDate   *time.Time   `json:"published_date,omitempty"`
	LastModified    *time.Time   `json:"last_modified,omitempty"`
	References      []string     `json:"references,omitempty"`
}

// ImageScanResult is the output of a Trivy scan of a container image.
type ImageScanResult struct {
	ID              string          `json:"id"`
	Image           string          `json:"image"`           // repo/name:tag
	Digest          string          `json:"digest,omitempty"` // sha256:...
	ScannedAt       time.Time       `json:"scanned_at"`
	ServiceName     string          `json:"service_name,omitempty"`
	Namespace       string          `json:"namespace,omitempty"`
	ClusterName     string          `json:"cluster_name"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	// Counts by severity
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	// Overall risk score: 0–100
	RiskScore float64 `json:"risk_score"`
}

// ═══════════════════════════════════════════════════════
//  DEPLOYMENT MARKERS
// ═══════════════════════════════════════════════════════

// DeployStatus tracks the regression-analysis result for a deployment.
type DeployStatus string

const (
	DeployPending    DeployStatus = "pending"    // analysis not yet run (< 30 min ago)
	DeployOK         DeployStatus = "ok"         // no regressions detected
	DeployRegression DeployStatus = "regression" // latency or error rate worsened ≥20%
	DeployImproved   DeployStatus = "improved"   // both metrics improved ≥10%
)

// DeploymentMarker records a point-in-time service deployment event.
// Created via POST /v1/deployments. The processor picks it up and
// schedules a regression analysis 30 minutes after DeployedAt.
type DeploymentMarker struct {
	ID          string       `json:"id"`
	ServiceName string       `json:"service_name"`
	ServiceID   string       `json:"service_id,omitempty"`
	Namespace   string       `json:"namespace"`
	ClusterName string       `json:"cluster_name"`
	Version     string       `json:"version"`     // image tag / git SHA / semver
	PrevVersion string       `json:"prev_version,omitempty"`
	DeployedAt  time.Time    `json:"deployed_at"`
	DeployedBy  string       `json:"deployed_by,omitempty"` // user or CI system
	Environment string       `json:"environment"`           // production | staging | etc.
	Notes       string       `json:"notes,omitempty"`

	// Regression analysis results (populated ~30 min after DeployedAt)
	Status          DeployStatus `json:"status"`
	AnalysedAt      *time.Time   `json:"analysed_at,omitempty"`

	// Before/after metrics (30-minute windows)
	P99Before       float64 `json:"p99_latency_before_ms,omitempty"`
	P99After        float64 `json:"p99_latency_after_ms,omitempty"`
	P99DeltaPct     float64 `json:"p99_latency_delta_pct,omitempty"` // positive = worse
	ErrRateBefore   float64 `json:"error_rate_before_pct,omitempty"`
	ErrRateAfter    float64 `json:"error_rate_after_pct,omitempty"`
	ErrRateDeltaPct float64 `json:"error_rate_delta_pct,omitempty"`

	// Link to the regression Problem if one was created
	ProblemID string `json:"problem_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
