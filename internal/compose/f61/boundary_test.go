package f61

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The five packages this one composes, and nothing else first-party.
const (
	observerPkg  = "github.com/observex/platform/internal/observe/tlscert"
	adapterPkg   = "github.com/observex/platform/internal/adapt/tlscertexpiry"
	detectorPkg  = "github.com/observex/platform/internal/detect/certexpiry"
	subjectPkg   = "github.com/observex/platform/internal/wire/f61"
	correlatePkg = "github.com/observex/platform/internal/correlate/f61"
)

// allowedProductionImports pins the dependency direction: the five F6.1
// packages, plus the three standard-library packages needed to carry values
// between them.
var allowedProductionImports = map[string]bool{
	"context": true, "errors": true, "time": true,
	observerPkg: true, adapterPkg: true, detectorPkg: true,
	subjectPkg: true, correlatePkg: true,
}

// deep entries are forbidden anywhere in the dependency closure; the rest are
// forbidden as direct imports only, because the observer legitimately reaches
// the network and the certificate packages on this package's behalf.
var forbidden = []struct {
	path, category string
	deep           bool
}{
	{"github.com/observex/platform/services", "processor, gateway, ingestor and every other service", true},
	{"github.com/observex/platform/agents", "agents", true},
	{"github.com/observex/platform/sdk", "the client SDK", true},
	{"github.com/observex/platform/pkg", "first-party runtime models, including models.Problem", true},
	{"github.com/observex/platform/internal/db", "database, store and migration packages", true},
	{"github.com/observex/platform/internal/middleware", "HTTP middleware and auth context", true},
	{"github.com/observex/platform/internal/servicetoken", "credentials and service transport", true},
	{"github.com/anthropics", "external LLM clients", true},
	{"github.com/sashabaranov/go-openai", "external LLM clients", true},
	{"github.com/ollama", "external LLM clients", true},
	{"k8s.io", "Kubernetes clients", true},
	{"sigs.k8s.io", "Kubernetes clients", true},
	{"github.com/aws", "cloud-provider clients", true},
	{"cloud.google.com", "cloud-provider clients", true},
	{"github.com/Azure", "cloud-provider clients", true},
	{"gopkg.in/yaml", "YAML dependency (KB-FMT-1)", true},
	{"database/sql", "database access", true},
	{"os/exec", "process execution", true},
	{"plugin", "dynamic code loading", true},
	{"net", "network access; probing belongs to the observer", false},
	{"net/http", "network access", false},
	{"crypto/tls", "TLS dialling belongs to the observer", false},
	{"crypto/x509", "certificate parsing belongs to the observer", false},
	{"os", "environment and filesystem", false},
}

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

func productionFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("no production files found")
	}
	sort.Strings(out)
	return out
}

func productionImports(t *testing.T) []string {
	t.Helper()
	set := map[string]bool{}
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// The dependency direction is exactly: compose → {observer, adapter, detector,
// subject, correlation}. Nothing else first-party, and no third party.
func TestDependencyDirectionIsPinned(t *testing.T) {
	imports := productionImports(t)
	for _, p := range imports {
		if !allowedProductionImports[p] {
			cat, _ := categoryFor(p, false)
			t.Errorf("production import %q is not on the allowlist %s", p, cat)
		}
	}
	for _, required := range []string{observerPkg, adapterPkg, detectorPkg, subjectPkg, correlatePkg} {
		found := false
		for _, p := range imports {
			found = found || p == required
		}
		if !found {
			t.Errorf("missing required import %q; imports = %v", required, imports)
		}
	}
	t.Logf("production imports: %v", imports)
}

func TestForbiddenCategoriesAreNotImported(t *testing.T) {
	for _, p := range productionImports(t) {
		if cat, bad := categoryFor(p, false); bad {
			t.Errorf("production import %q is in forbidden category %q", p, cat)
		}
	}
}

// No dependency is added: everything reachable is standard library or one of
// the first-party F6.1 packages, so go.mod and go.sum cannot need to change.
func TestImportClosureAddsNoDependency(t *testing.T) {
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
			// Expected: the standard library's net package uses cgo for the
			// platform resolver. Nothing here uses cgo directly.
			continue
		}
		pkg, err := build.Default.Import(it.path, it.srcDir, 0)
		if err != nil {
			t.Fatalf("resolve %s: %v", it.path, err)
		}
		if !pkg.Goroot {
			if !strings.HasPrefix(it.path, "github.com/observex/platform/internal/") {
				t.Errorf("third-party package %q in the dependency closure", it.path)
			}
			firstParty++
		}
		if cat, bad := categoryFor(it.path, true); bad {
			t.Errorf("forbidden package %q (%s) in the dependency closure", it.path, cat)
		}
	}
	t.Logf("dependency closure: %d packages, %d first-party, none forbidden", len(seen), firstParty)
}

// No clock, no environment, no filesystem, no network, no package-level
// mutable state, nothing running at import time.
func TestNoClockEnvironmentNetworkOrGlobalState(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			"time.Now", "time.Since", "time.Until", "time.Tick", "time.After",
			"os.Getenv", "os.LookupEnv", "os.Environ", "os.Open", "os.ReadFile",
			"flag.Parse", "net.Dial", "http.", "exec.Command",
			"InsecureSkipVerify", "sync.", "go func",
		} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s contains %q", name, banned)
			}
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					for _, ident := range spec.(*ast.ValueSpec).Names {
						if !strings.HasPrefix(ident.Name, "Err") {
							t.Errorf("%s declares package-level variable %q", name, ident.Name)
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" {
					t.Errorf("%s declares an init function", name)
				}
			}
		}
	}
}

// This package composes; it does not re-implement. It never builds a prober,
// never calls the detector directly, and never constructs a finding or a
// result of its own.
func TestNoLogicIsDuplicatedFromTheComposedPackages(t *testing.T) {
	fset := token.NewFileSet()
	adaptCalls := 0
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				switch {
				case pkg.Name == "tlscert" && sel.Sel.Name == "New":
					t.Errorf("%s builds a prober; the caller supplies one", name)
				case pkg.Name == "certexpiry":
					t.Errorf("%s calls certexpiry.%s; findings come from the adapter", name, sel.Sel.Name)
				case pkg.Name == "tlscertexpiry" && sel.Sel.Name == "Adapt":
					adaptCalls++
				}
			case *ast.CompositeLit:
				sel, ok := node.Type.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				// A Finding is never built here, and a Result only as the
				// empty zero value for an observation that has none.
				if pkg.Name == "certexpiry" && sel.Sel.Name == "Finding" {
					t.Errorf("%s constructs a certexpiry.Finding", name)
				}
				if pkg.Name == "tlscertexpiry" && sel.Sel.Name == "Result" && len(node.Elts) != 0 {
					t.Errorf("%s constructs a populated tlscertexpiry.Result", name)
				}
			}
			return true
		})
	}
	if adaptCalls != 1 {
		t.Errorf("tlscertexpiry.Adapt is called %d times; want exactly 1 call site", adaptCalls)
	}
}

// The exported surface is one function, its input, its report and the errors
// that refuse — nothing that carries policy, a destination, a location or a
// claim about where a probe ran.
func TestExportedSurface(t *testing.T) {
	want := []string{
		"ErrNoEvaluationTime", "ErrNoProber", "ErrNoSet", "ErrNoTimeout",
		"Input", "Observe", "Step",
	}
	sort.Strings(want)

	fset := token.NewFileSet()
	var got []string
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv == nil {
					got = append(got, d.Name.Name)
				} else {
					got = append(got, "method:"+d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							got = append(got, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if id.IsExported() {
								got = append(got, id.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("exported surface =\n  %v\nwant exactly\n  %v", got, want)
	}
}

// Nothing here assumes a destination, a deployment, a location or a schedule.
func TestNoDestinationDeploymentOrLocationConcepts(t *testing.T) {
	banned := []string{
		"Severity", "Confidence", "Priority", "Remediation", "Action", "Policy",
		"Persist", "Store", "Save", "Publish", "Emit", "Alert", "Incident",
		"Problem", "Migration", "Region", "Location", "Locations", "Schedule",
		"Scheduler", "External", "Cluster", "Namespace_", "Deployment",
	}
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			for _, b := range banned {
				if id.Name == b {
					t.Errorf("%s declares or references the identifier %q", name, b)
				}
			}
			return true
		})
	}
}
