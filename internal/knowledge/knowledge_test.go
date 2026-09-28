package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Expected values below were computed independently from the approved
// catalog.json (SHA-256 95aa0bd8…) with a separate tool, not derived from this
// package.
var expectedIDs = strings.Fields(`
	F1.1 F1.2 F1.3 F1.4 F1.5 F1.6 F1.7 F1.8 F1.9
	F2.1 F2.2 F2.3 F2.4 F2.5 F2.6 F2.7 F2.8
	F3.1 F3.2 F3.3 F3.4 F3.5 F3.6
	F4.1 F4.2 F4.3 F4.4 F4.5 F4.6 F4.7 F4.8 F4.9
	F5.1 F5.2 F5.3 F5.4 F5.5 F5.6 F5.7
	F6.1 F6.2 F6.3 F6.4 F6.5 F6.6
	F7.1 F7.2 F7.3
	F8.1 F8.2 F8.3 F8.4
	F9.1 F9.2 F9.3
	F10.1
	F11.1 F11.2 F11.3 F11.4
	F12.1
	F13.1 F13.2`)

func mustLoad(t *testing.T) Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestLoadSucceeds(t *testing.T) {
	c := mustLoad(t)
	if c.Len() == 0 {
		t.Fatal("loaded catalog is empty")
	}
}

func TestLoadIsStableAndConcurrencySafe(t *testing.T) {
	first := mustLoad(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := Load()
			if err != nil || c.d != first.d {
				t.Errorf("Load returned a different result: err=%v", err)
			}
			_ = c.ByFamily(FamilyTime)
		}()
	}
	wg.Wait()
}

func TestSixtyThreeRecords(t *testing.T) {
	c := mustLoad(t)
	if c.Len() != 63 || len(c.PatternIDs()) != 63 || c.Metadata().RecordCount != 63 {
		t.Fatalf("Len=%d IDs=%d record_count=%d; want 63", c.Len(), len(c.PatternIDs()), c.Metadata().RecordCount)
	}
}

func TestUniqueIDsInCatalogOrder(t *testing.T) {
	ids := mustLoad(t).PatternIDs()
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
	if !reflect.DeepEqual(ids, expectedIDs) {
		t.Fatalf("PatternIDs order differs from the approved catalog:\n got %v\nwant %v", ids, expectedIDs)
	}
}

func TestKnownPatternLookup(t *testing.T) {
	c := mustLoad(t)

	p, ok := c.Pattern("F1.2")
	if !ok {
		t.Fatal("F1.2 not found")
	}
	if p.Title != "Configuration change that is syntactically valid and semantically wrong" || p.Family != FamilyChangeAndRelease {
		t.Errorf("F1.2: title %q family %q", p.Title, p.Family)
	}
	// KB-SEED-2 class-A repair: the signal is whole, not cut at the comma.
	if got := p.RequiredTelemetry[0].Signal; got != "Config commit stream with author, scope and diff" {
		t.Errorf("F1.2 telemetry signal %q", got)
	}

	// KB-SEED-2 structural decision: F11.3 affected_systems is exactly ONE item.
	p, ok = c.Pattern("F11.3")
	if !ok {
		t.Fatal("F11.3 not found")
	}
	want := []string{"any predict-and-act loop — failure prediction, capacity prediction, anomaly-driven remediation"}
	if !reflect.DeepEqual(p.AffectedSystems, want) {
		t.Errorf("F11.3 affected_systems = %q; want exactly %q", p.AffectedSystems, want)
	}
	if p.Title != "Predictor self-poisoning — remediation destroys the ground truth" {
		t.Errorf("F11.3 title %q", p.Title)
	}

	// KB-SEED-2 class-C repair: a bare year is text.
	p, _ = c.Pattern("F1.8")
	if got := p.Incidents[1].Date; got != "2021" {
		t.Errorf("F1.8 incidents[1].date = %q; want \"2021\"", got)
	}
}

func TestUnknownPatternLookup(t *testing.T) {
	c := mustLoad(t)
	for _, id := range []string{"", "F0.1", "F14.1", "f1.1", "F1.10", "F1.1 ", "F11.5"} {
		if _, ok := c.Pattern(id); ok {
			t.Errorf("Pattern(%q) found; want not found", id)
		}
	}
}

func TestByFamily(t *testing.T) {
	c := mustLoad(t)
	want := map[Family]int{
		FamilyChangeAndRelease: 9, FamilySaturationAndMetastability: 8, FamilyDependencyAndCoupling: 6,
		FamilyStateDataAndStorage: 9, FamilyNetworkAndNaming: 7, FamilyExpiryQuotaAndCounters: 6,
		FamilyTime: 3, FamilyPlatformAndOrchestration: 4, FamilyObservabilityItself: 3,
		FamilyHumanAndProcess: 1, FamilyAutomationInduced: 4, FamilyHardwarePhysicalEnvironmental: 1,
		FamilyAIMLServing: 2,
	}
	total := 0
	for _, f := range Families() {
		got := c.ByFamily(f)
		if len(got) != want[f] {
			t.Errorf("ByFamily(%s) = %d patterns; want %d", f, len(got), want[f])
		}
		for _, p := range got {
			if p.Family != f {
				t.Errorf("ByFamily(%s) returned %s of family %s", f, p.ID, p.Family)
			}
		}
		total += len(got)
	}
	if total != 63 {
		t.Errorf("families cover %d patterns; want 63", total)
	}
	if got := c.ByFamily("F14-unknown"); got != nil {
		t.Errorf("unknown family returned %d patterns", len(got))
	}
	// Catalog order is preserved.
	if got := idsOf(c.ByFamily(FamilyTime)); !reflect.DeepEqual(got, []string{"F7.1", "F7.2", "F7.3"}) {
		t.Errorf("ByFamily(F7) order %v", got)
	}
}

func TestByMethodFit(t *testing.T) {
	c := mustLoad(t)
	want := map[MethodFit]int{
		MethodDeterministicRule: 49, MethodThresholdWithTrend: 20, MethodExtrapolation: 7,
		MethodQueueingModel: 3, MethodBaselineComparison: 7, MethodChangeCorrelation: 2,
		MethodPeerOutlierDetection: 4, MethodNoneKnown: 2,
		MethodChangePointDetection: 0, MethodSupervisedModel: 0, MethodLanguageModel: 0,
	}
	for _, m := range MethodFits() {
		got := c.ByMethodFit(m)
		if len(got) != want[m] {
			t.Errorf("ByMethodFit(%s) = %d patterns; want %d", m, len(got), want[m])
		}
		for _, p := range got {
			if !hasMethodFit(p, m) {
				t.Errorf("ByMethodFit(%s) returned %s without such an indicator", m, p.ID)
			}
		}
	}
	if got := c.ByMethodFit("no-such-method"); got != nil {
		t.Errorf("unknown method fit returned %d patterns", len(got))
	}
}

func TestByAutomationClass(t *testing.T) {
	c := mustLoad(t)
	want := map[AutomationClass]int{
		AutomationNever: 19, AutomationDetectAndDiagnoseOnly: 17, AutomationHumanApprovalGate: 53,
		AutomationGuardrailedAutonomous: 37, AutomationAdaptiveClosedLoop: 0,
	}
	for _, a := range AutomationClasses() {
		got := c.ByAutomationClass(a)
		if len(got) != want[a] {
			t.Errorf("ByAutomationClass(%s) = %d patterns; want %d", a, len(got), want[a])
		}
		for _, p := range got {
			found := false
			for _, r := range p.Remediations {
				found = found || r.AutomationClass == a
			}
			if !found {
				t.Errorf("ByAutomationClass(%s) returned %s without such a remediation", a, p.ID)
			}
		}
	}
	if got := c.ByAutomationClass("A9-unknown"); got != nil {
		t.Errorf("unknown automation class returned %d patterns", len(got))
	}
}

// TestReturnedValuesAreCopies proves no caller can reach internal state.
func TestReturnedValuesAreCopies(t *testing.T) {
	c := mustLoad(t)
	before, _ := c.Pattern("F2.6")

	ids := c.PatternIDs()
	ids[0] = "MUTATED"

	p, _ := c.Pattern("F2.6")
	p.Title = "MUTATED"
	p.AffectedSystems[0] = "MUTATED"
	p.Classification.TriggerClass[0] = "MUTATED"
	p.LeadingIndicators[0].Signal = "MUTATED"
	p.RequiredTelemetry[0].Signal = "MUTATED"
	p.Remediations[0].Preconditions[0] = "MUTATED"
	p.Remediations[0].AutomationClass = "MUTATED"
	p.Evidence[0].URL = "MUTATED"
	p.PredictiveMonitoring.KnownLimits = append(p.PredictiveMonitoring.KnownLimits, "MUTATED")

	for _, q := range c.ByFamily(FamilySaturationAndMetastability) {
		q.Title = "MUTATED"
		if len(q.Incidents) > 0 {
			q.Incidents[0].Summary = "MUTATED"
		}
	}
	for _, q := range c.ByMethodFit(MethodDeterministicRule) {
		q.Verification[0] = "MUTATED"
	}
	for _, q := range c.ByAutomationClass(AutomationNever) {
		q.RootCauses[0] = "MUTATED"
	}
	m := c.Metadata()
	m.Source.SHA256 = "MUTATED"

	after, _ := c.Pattern("F2.6")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("mutating returned values changed the catalog")
	}
	if c.PatternIDs()[0] != "F1.1" || c.Metadata().Source.SHA256 != SourceSHA256 {
		t.Fatal("mutating returned IDs or metadata changed the catalog")
	}
}

func TestZeroCatalogIsEmpty(t *testing.T) {
	var c Catalog
	if c.Len() != 0 || c.PatternIDs() != nil || c.ByFamily(FamilyTime) != nil ||
		c.ByMethodFit(MethodDeterministicRule) != nil || c.ByAutomationClass(AutomationNever) != nil ||
		c.Metadata() != (Metadata{}) {
		t.Fatal("zero Catalog is not empty")
	}
	if _, ok := c.Pattern("F1.1"); ok {
		t.Fatal("zero Catalog found a pattern")
	}
}

// TestProvenance pins the embedded bytes and the recorded source to the
// KB-SEED-2 approval (2026-09-22).
func TestProvenance(t *testing.T) {
	const (
		approvedCatalog = "95aa0bd8aa24d000ee09c4b97cac925f2ba470cad4fead39896d5ffb6ba95767"
		approvedSource  = "e0cc03d3b4a00cfeef8eb34c33c48926c854a0f9d8e884f5b7e5c5312076c8fb"
	)
	sum := sha256.Sum256(embeddedCatalog)
	if got := hex.EncodeToString(sum[:]); got != approvedCatalog || len(embeddedCatalog) != 457121 {
		t.Fatalf("embedded catalog SHA-256 %s (%d bytes); want %s (457121 bytes)", got, len(embeddedCatalog), approvedCatalog)
	}
	if CatalogSHA256 != approvedCatalog || SourceSHA256 != approvedSource {
		t.Fatal("package provenance constants differ from the approved values")
	}

	src := mustLoad(t).Metadata().Source
	if src.SHA256 != approvedSource {
		t.Fatalf("metadata.source.sha256 = %q; want exactly %q", src.SHA256, approvedSource)
	}
	if src.Version != "0.1.2" || src.Artifact != "observex-sre-kb-seed-v0.1.2.yaml" ||
		src.Schema != "observex-sre-kb-schema-v0.1.0.json" {
		t.Fatalf("metadata.source = %+v", src)
	}
	if v := mustLoad(t).Metadata().Supersedes.Version; v != "0.1.1" {
		t.Fatalf("metadata.supersedes.version = %q; want 0.1.1", v)
	}

	// Non-circularity: the catalog never records its own hash.
	if bytes.Contains(embeddedCatalog, []byte(approvedCatalog)) {
		t.Fatal("catalog contains its own hash")
	}
	// The source hash appears exactly once (metadata.source.sha256).
	if n := bytes.Count(embeddedCatalog, []byte(approvedSource)); n != 1 {
		t.Fatalf("source hash appears %d times; want 1", n)
	}
	// The superseded pre-correction hash must not appear anywhere.
	if bytes.Contains(embeddedCatalog, []byte(supersededPreCorrection)) {
		t.Fatal("catalog references the superseded pre-correction seed")
	}
}

func idsOf(ps []Pattern) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func hasMethodFit(p Pattern, m MethodFit) bool {
	for _, li := range p.LeadingIndicators {
		if li.MethodFit == m {
			return true
		}
	}
	return false
}
