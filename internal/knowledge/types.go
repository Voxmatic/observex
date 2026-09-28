package knowledge

// Family is a failure-pattern family as named by the schema.
type Family string

// Families defined by observex-sre-kb-schema-v0.1.0.json.
const (
	FamilyChangeAndRelease              Family = "F1-change-and-release"
	FamilySaturationAndMetastability    Family = "F2-saturation-and-metastability"
	FamilyDependencyAndCoupling         Family = "F3-dependency-and-coupling"
	FamilyStateDataAndStorage           Family = "F4-state-data-and-storage"
	FamilyNetworkAndNaming              Family = "F5-network-and-naming"
	FamilyExpiryQuotaAndCounters        Family = "F6-expiry-quota-and-counters"
	FamilyTime                          Family = "F7-time"
	FamilyPlatformAndOrchestration      Family = "F8-platform-and-orchestration"
	FamilyObservabilityItself           Family = "F9-observability-itself"
	FamilyHumanAndProcess               Family = "F10-human-and-process"
	FamilyAutomationInduced             Family = "F11-automation-induced"
	FamilyHardwarePhysicalEnvironmental Family = "F12-hardware-physical-environmental"
	FamilyAIMLServing                   Family = "F13-ai-ml-serving"
)

// Families returns every schema family in taxonomy order. The slice is new on
// every call.
func Families() []Family {
	return []Family{
		FamilyChangeAndRelease, FamilySaturationAndMetastability, FamilyDependencyAndCoupling,
		FamilyStateDataAndStorage, FamilyNetworkAndNaming, FamilyExpiryQuotaAndCounters,
		FamilyTime, FamilyPlatformAndOrchestration, FamilyObservabilityItself,
		FamilyHumanAndProcess, FamilyAutomationInduced, FamilyHardwarePhysicalEnvironmental,
		FamilyAIMLServing,
	}
}

// MethodFit is the kind of method a leading indicator is suited to.
type MethodFit string

// Method fits defined by the schema.
const (
	MethodDeterministicRule    MethodFit = "deterministic-rule"
	MethodThresholdWithTrend   MethodFit = "threshold-with-trend"
	MethodExtrapolation        MethodFit = "extrapolation-forecast"
	MethodQueueingModel        MethodFit = "queueing-model"
	MethodBaselineComparison   MethodFit = "baseline-comparison"
	MethodChangeCorrelation    MethodFit = "change-correlation"
	MethodPeerOutlierDetection MethodFit = "peer-outlier-detection"
	MethodChangePointDetection MethodFit = "change-point-detection"
	MethodSupervisedModel      MethodFit = "supervised-model"
	MethodLanguageModel        MethodFit = "language-model"
	MethodNoneKnown            MethodFit = "none-known"
)

// MethodFits returns every schema method fit. The slice is new on every call.
func MethodFits() []MethodFit {
	return []MethodFit{
		MethodDeterministicRule, MethodThresholdWithTrend, MethodExtrapolation,
		MethodQueueingModel, MethodBaselineComparison, MethodChangeCorrelation,
		MethodPeerOutlierDetection, MethodChangePointDetection, MethodSupervisedModel,
		MethodLanguageModel, MethodNoneKnown,
	}
}

// AutomationClass is the documented automation ceiling of a remediation.
// It is catalog knowledge, never an authorization.
type AutomationClass string

// Automation classes defined by the schema.
const (
	AutomationNever                 AutomationClass = "A0-never-automate"
	AutomationDetectAndDiagnoseOnly AutomationClass = "A1-detect-and-diagnose-only"
	AutomationHumanApprovalGate     AutomationClass = "A2-human-approval-gate"
	AutomationGuardrailedAutonomous AutomationClass = "A3-guardrailed-autonomous"
	AutomationAdaptiveClosedLoop    AutomationClass = "A4-adaptive-closed-loop"
)

// AutomationClasses returns every schema automation class. The slice is new on
// every call.
func AutomationClasses() []AutomationClass {
	return []AutomationClass{
		AutomationNever, AutomationDetectAndDiagnoseOnly, AutomationHumanApprovalGate,
		AutomationGuardrailedAutonomous, AutomationAdaptiveClosedLoop,
	}
}

// Metadata describes the catalog and its provenance. It contains no slices or
// maps, so a returned value is an independent copy.
type Metadata struct {
	Version      string     `json:"version"`
	Compiled     string     `json:"compiled"`
	RecordCount  int        `json:"record_count"`
	Completeness string     `json:"completeness"`
	KnownGapsRef string     `json:"known_gaps_ref"`
	Source       Source     `json:"source"`
	Supersedes   Supersedes `json:"supersedes"`
}

// Source identifies the seed the catalog was generated from.
type Source struct {
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Schema   string `json:"schema"`
	Note     string `json:"note"`
}

// Supersedes identifies the seed version this catalog's seed replaced.
type Supersedes struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Pattern is one failure-pattern record. Values returned by Catalog methods are
// deep copies; modifying them does not affect the catalog.
type Pattern struct {
	ID                     string                `json:"id"`
	Version                string                `json:"version"`
	Supersedes             string                `json:"supersedes"`
	Title                  string                `json:"title"`
	Family                 Family                `json:"family"`
	Classification         Classification        `json:"classification"`
	ProblemAndMechanism    string                `json:"problem_and_mechanism"`
	AffectedSystems        []string              `json:"affected_systems"`
	Preconditions          []string              `json:"preconditions"`
	ObservableSymptoms     []string              `json:"observable_symptoms"`
	LeadingIndicators      []LeadingIndicator    `json:"leading_indicators"`
	RequiredTelemetry      []TelemetrySignal     `json:"required_telemetry"`
	RootCauses             []string              `json:"root_causes"`
	DiagnosticMethods      []string              `json:"diagnostic_methods"`
	Incidents              []Incident            `json:"incidents"`
	Remediations           []Remediation         `json:"remediations"`
	Verification           []string              `json:"verification"`
	RollbackAndEscalation  RollbackAndEscalation `json:"rollback_and_escalation"`
	AutomationSafetyLimits []string              `json:"automation_safety_limits"`
	PredictiveMonitoring   PredictiveMonitoring  `json:"predictive_monitoring"`
	Confidence             string                `json:"confidence"`
	Evidence               []Evidence            `json:"evidence"`
	SolutionMaturity       string                `json:"solution_maturity"`
	Notes                  string                `json:"notes"`
	LastReviewed           string                `json:"last_reviewed"`
	ReviewDue              string                `json:"review_due"`
}

// Classification is the pattern's failure classification.
type Classification struct {
	TriggerClass       []string `json:"trigger_class"`
	Propagation        string   `json:"propagation"`
	ObservabilityClass string   `json:"observability_class"`
	Reversibility      string   `json:"reversibility"`
	RecoveryShape      string   `json:"recovery_shape"`
}

// LeadingIndicator is a signal that may precede the failure.
type LeadingIndicator struct {
	Signal          string    `json:"signal"`
	LeadTime        string    `json:"lead_time"`
	PredictionClass string    `json:"prediction_class"`
	EvidenceTier    string    `json:"evidence_tier"`
	MethodFit       MethodFit `json:"method_fit"`
}

// TelemetrySignal is telemetry needed to detect or diagnose the pattern.
type TelemetrySignal struct {
	Signal           string `json:"signal"`
	Source           string `json:"source"`
	AvailabilityRisk string `json:"availability_risk"`
}

// Incident is a documented public incident of the pattern.
type Incident struct {
	Organisation string `json:"organisation"`
	Date         string `json:"date"`
	Summary      string `json:"summary"`
	URL          string `json:"url"`
	EvidenceTier string `json:"evidence_tier"`
	Retrieval    string `json:"retrieval"`
}

// Remediation is a documented remediation and its automation ceiling.
type Remediation struct {
	Action             string          `json:"action"`
	Reversibility      string          `json:"reversibility"`
	AutomationClass    AutomationClass `json:"automation_class"`
	Preconditions      []string        `json:"preconditions"`
	BlastRadius        string          `json:"blast_radius"`
	Verification       string          `json:"verification"`
	FailureModeIfWrong string          `json:"failure_mode_if_wrong"`
	EvidenceTier       string          `json:"evidence_tier"`
}

// RollbackAndEscalation describes rollback availability and escalation.
type RollbackAndEscalation struct {
	RollbackAvailable      string `json:"rollback_available"`
	RollbackNotes          string `json:"rollback_notes"`
	EscalationTrigger      string `json:"escalation_trigger"`
	HumanAuthorityRequired bool   `json:"human_authority_required"`
}

// PredictiveMonitoring describes whether the pattern can be predicted.
type PredictiveMonitoring struct {
	Feasible    string   `json:"feasible"`
	Rationale   string   `json:"rationale"`
	KnownLimits []string `json:"known_limits"`
}

// Evidence is a source supporting the pattern.
type Evidence struct {
	Tier      string `json:"tier"`
	Citation  string `json:"citation"`
	URL       string `json:"url"`
	Retrieval string `json:"retrieval"`
	Publisher string `json:"publisher"`
}

// clone returns a deep copy: no slice in the result shares a backing array
// with p.
func (p Pattern) clone() Pattern {
	c := p
	c.Classification.TriggerClass = cloneStrings(p.Classification.TriggerClass)
	c.AffectedSystems = cloneStrings(p.AffectedSystems)
	c.Preconditions = cloneStrings(p.Preconditions)
	c.ObservableSymptoms = cloneStrings(p.ObservableSymptoms)
	c.LeadingIndicators = cloneSlice(p.LeadingIndicators)
	c.RequiredTelemetry = cloneSlice(p.RequiredTelemetry)
	c.RootCauses = cloneStrings(p.RootCauses)
	c.DiagnosticMethods = cloneStrings(p.DiagnosticMethods)
	c.Incidents = cloneSlice(p.Incidents)
	c.Remediations = cloneSlice(p.Remediations)
	for i := range c.Remediations {
		c.Remediations[i].Preconditions = cloneStrings(p.Remediations[i].Preconditions)
	}
	c.Verification = cloneStrings(p.Verification)
	c.AutomationSafetyLimits = cloneStrings(p.AutomationSafetyLimits)
	c.PredictiveMonitoring.KnownLimits = cloneStrings(p.PredictiveMonitoring.KnownLimits)
	c.Evidence = cloneSlice(p.Evidence)
	return c
}

func cloneStrings(s []string) []string { return cloneSlice(s) }

// cloneSlice returns a new slice holding shallow copies of the elements. Nested
// slices inside elements must be copied by the caller (see clone). A nil input
// stays nil; an empty input stays empty.
func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	copy(out, s)
	return out
}
