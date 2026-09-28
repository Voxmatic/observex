package certexpiry

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/observex/platform/internal/knowledge"
)

const knowledgePkg = "github.com/observex/platform/internal/knowledge"

func loadPattern(t *testing.T) knowledge.Pattern {
	t.Helper()
	c, err := knowledge.Load()
	if err != nil {
		t.Fatalf("knowledge.Load: %v", err)
	}
	// 21: the catalog this provenance came from is the approved one.
	if knowledge.CatalogSHA256 != "95aa0bd8aa24d000ee09c4b97cac925f2ba470cad4fead39896d5ffb6ba95767" {
		t.Fatalf("catalog SHA-256 = %s; want the approved KB-SEED-2 catalog", knowledge.CatalogSHA256)
	}
	if CatalogSHA256 != knowledge.CatalogSHA256 {
		t.Fatalf("package CatalogSHA256 %s does not match the embedded catalog %s", CatalogSHA256, knowledge.CatalogSHA256)
	}
	p, ok := c.Pattern(PatternID)
	if !ok {
		t.Fatalf("pattern %s is not in the catalog", PatternID)
	}
	return p
}

// 22: the record exists and is the one this detector claims.
func TestCatalogRecordIdentity(t *testing.T) {
	p := loadPattern(t)
	if p.ID != "F6.1" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Family != knowledge.FamilyExpiryQuotaAndCounters {
		t.Errorf("Family = %q, want %q", p.Family, knowledge.FamilyExpiryQuotaAndCounters)
	}
	if p.Title != "TLS certificate expiry" {
		t.Errorf("Title = %q", p.Title)
	}
}

// 23: the record still classifies this pattern as deterministically predictable.
func TestCatalogRecordIsDeterministic(t *testing.T) {
	p := loadPattern(t)
	if got := p.PredictiveMonitoring.Feasible; got != "yes-deterministic" {
		t.Fatalf("predictive_monitoring.feasible = %q, want %q", got, "yes-deterministic")
	}
}

// 24: the indicator this detector implements is still present, still a
// deterministic rule, and still says what the detector computes.
func TestCatalogIndicatorBinding(t *testing.T) {
	p := loadPattern(t)
	if len(p.LeadingIndicators) == 0 {
		t.Fatal("F6.1 has no leading indicators")
	}
	li := p.LeadingIndicators[0]
	if li.Signal != IndicatorSignal {
		t.Errorf("leading_indicators[0].signal = %q, want %q", li.Signal, IndicatorSignal)
	}
	if li.MethodFit != knowledge.MethodDeterministicRule {
		t.Errorf("leading_indicators[0].method_fit = %q, want %q", li.MethodFit, knowledge.MethodDeterministicRule)
	}
	if li.PredictionClass != PredictionClass {
		t.Errorf("leading_indicators[0].prediction_class = %q, want %q", li.PredictionClass, PredictionClass)
	}
	deterministic := false
	for _, ind := range p.LeadingIndicators {
		if ind.MethodFit == knowledge.MethodDeterministicRule && ind.PredictionClass == PredictionClass {
			deterministic = true
		}
	}
	if !deterministic {
		t.Error("no deterministic-rule / P1-deterministic indicator remains on F6.1")
	}
}

// 25: a finding's provenance fields equal the catalog's values.
func TestFindingProvenanceMatchesCatalog(t *testing.T) {
	p := loadPattern(t)
	if p.Version != PatternVersion {
		t.Fatalf("record version = %q, package PatternVersion = %q", p.Version, PatternVersion)
	}
	base := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	f, found, err := Evaluate(base, Observation{
		CheckID: "chk-001", Endpoint: "api.example.com:443",
		NotAfter: base.Add(24 * time.Hour), ObservedAt: base.Add(-time.Minute),
	}, Params{Horizon: 720 * time.Hour, MaxObservationAge: 15 * time.Minute})
	if err != nil || !found {
		t.Fatalf("setup: found = %v, err = %v", found, err)
	}
	if f.PatternID != p.ID || f.PatternVersion != p.Version ||
		f.PredictionClass != p.LeadingIndicators[0].PredictionClass ||
		f.CatalogSHA256 != knowledge.CatalogSHA256 {
		t.Errorf("finding provenance %+v does not match the catalog record", f)
	}
}

// The catalog states an automation class for F6.1's remediations. It must not
// appear anywhere in this package's output: detection authorizes nothing.
func TestAutomationClassIsNotCarriedIntoFindings(t *testing.T) {
	p := loadPattern(t)
	if len(p.Remediations) == 0 || p.Remediations[0].AutomationClass == "" {
		t.Fatal("expected F6.1 to document remediations with automation classes")
	}
	base := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	f, _, _ := Evaluate(base, Observation{
		CheckID: "c", Endpoint: "e:443", NotAfter: base, ObservedAt: base,
	}, Params{Horizon: time.Hour, MaxObservationAge: time.Minute})
	for _, s := range []string{string(p.Remediations[0].AutomationClass), p.Remediations[0].Action} {
		for _, v := range []string{f.PatternID, f.PatternVersion, f.CatalogSHA256, f.PredictionClass, f.CheckID, f.Endpoint} {
			if strings.Contains(v, s) {
				t.Errorf("finding field %q carries remediation text %q", v, s)
			}
		}
	}
}

// ── Import boundary ──────────────────────────────────────────────────────────

var allowedProductionImports = map[string]bool{
	"errors": true, "time": true, knowledgePkg: true,
}

// deep entries are forbidden anywhere in the dependency closure; the others are
// forbidden as direct imports only, because the standard library reaches them
// internally (fmt reaches os, for example) and no package can prevent that.
var forbidden = []struct {
	path, category string
	deep           bool
}{
	{"net", "network clients", true},
	{"os/exec", "process execution", true},
	{"crypto/tls", "network transport", true},
	{"database/sql", "database packages", true},
	{"plugin", "dynamic code loading", true},
	{"k8s.io", "Kubernetes clients", true},
	{"sigs.k8s.io", "Kubernetes clients", true},
	{"github.com/aws", "cloud-provider clients", true},
	{"cloud.google.com", "cloud-provider clients", true},
	{"github.com/Azure", "cloud-provider clients", true},
	{"github.com/anthropics", "external LLM clients", true},
	{"github.com/sashabaranov/go-openai", "external LLM clients", true},
	{"github.com/ollama", "external LLM clients", true},
	{"gopkg.in/yaml", "YAML dependency (KB-FMT-1)", true},
	{"github.com/observex/platform/services", "first-party services: processor, gateway, agents", true},
	{"github.com/observex/platform/internal/db", "database packages", true},
	{"github.com/observex/platform/internal/servicetoken", "credentials and service transport", true},
	{"github.com/observex/platform/pkg", "first-party runtime models", true},
	{"os", "environment and filesystem", false},
	{"crypto/x509", "certificate parsing belongs to the observation layer", false},
	{"time/tzdata", "embedded zone database", false},
}

func forbiddenCategory(path string) (string, bool) { return categoryFor(path, false) }

func deepForbiddenCategory(path string) (string, bool) { return categoryFor(path, true) }

func categoryFor(path string, deepOnly bool) (string, bool) {
	for _, f := range forbidden {
		if deepOnly && !f.deep {
			continue
		}
		if path == f.path || strings.HasPrefix(path, f.path+"/") {
			return f.category, true
		}
	}
	return "", false
}

func productionImports(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			set[p] = true
		}
	}
	if len(set) == 0 {
		t.Fatal("no production imports found; the walk did not run")
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestImportBoundaryAllowlist(t *testing.T) {
	imports := productionImports(t)
	for _, p := range imports {
		if !allowedProductionImports[p] {
			cat, _ := forbiddenCategory(p)
			t.Errorf("production import %q is not on the allowlist %s", p, cat)
		}
	}
	t.Logf("production imports: %v", imports)
}

func TestImportBoundaryForbiddenCategories(t *testing.T) {
	for _, p := range productionImports(t) {
		if cat, bad := forbiddenCategory(p); bad {
			t.Errorf("production import %q is in forbidden category %q", p, cat)
		}
	}
}

// The full dependency closure must contain no networking, process execution,
// database or TLS package. Honest limit: the standard library's own internals
// (for example fmt -> os inside knowledge) cannot be excluded; the allowlist
// above is what stops this package from using them directly.
func TestImportBoundaryTransitive(t *testing.T) {
	type item struct{ path, srcDir string }
	var queue []item
	for _, p := range productionImports(t) {
		queue = append(queue, item{p, ""})
	}
	seen := map[string]bool{}
	firstParty := 0
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if seen[it.path] {
			continue
		}
		seen[it.path] = true
		if it.path == "C" {
			t.Error("cgo (import \"C\") in the dependency closure")
			continue
		}

		var pkg *build.Package
		var err error
		if it.path == knowledgePkg {
			firstParty++
			pkg, err = build.Default.ImportDir("../../knowledge", 0)
		} else {
			pkg, err = build.Default.Import(it.path, it.srcDir, 0)
		}
		if err != nil {
			t.Fatalf("resolve %s: %v", it.path, err)
		}
		if cat, bad := deepForbiddenCategory(it.path); bad {
			t.Errorf("forbidden package %q (%s) in the dependency closure", it.path, cat)
		}
		if !pkg.Goroot && it.path != knowledgePkg {
			t.Errorf("non-standard-library package %q in the dependency closure", it.path)
		}
		for _, imp := range pkg.Imports {
			queue = append(queue, item{imp, pkg.Dir})
		}
	}
	if firstParty != 1 {
		t.Errorf("first-party dependencies = %d; want exactly 1 (internal/knowledge)", firstParty)
	}
	t.Logf("dependency closure: %d packages, none forbidden", len(seen))
}
