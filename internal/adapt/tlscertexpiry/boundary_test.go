package tlscertexpiry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	detectorPkg = "github.com/observex/platform/internal/detect/certexpiry"
	observerPkg = "github.com/observex/platform/internal/observe/tlscert"
)

// allowedProductionImports is the complete set this adapter may import: the two
// packages it joins, and the standard library it needs to join them.
var allowedProductionImports = map[string]bool{
	"errors": true, "fmt": true, "time": true,
	detectorPkg: true, observerPkg: true,
}

// forbidden direct imports. The adapter is pure: it must not reach the runtime
// tiers, the network, the filesystem or the environment. (Its transitive
// closure does contain net and crypto/tls, because the observer's types come
// from a package that dials; the adapter itself never does.)
var forbidden = []struct{ path, category string }{
	{"github.com/observex/platform/services", "processor, gateway, agents and every other service"},
	{"github.com/observex/platform/internal/db", "database packages"},
	{"github.com/observex/platform/internal/servicetoken", "credentials and service transport"},
	{"github.com/observex/platform/internal/middleware", "HTTP middleware"},
	{"github.com/observex/platform/pkg", "first-party runtime models"},
	{"net", "network access"},
	{"crypto/tls", "TLS dialling belongs to the observer"},
	{"crypto/x509", "certificate parsing belongs to the observer"},
	{"os", "environment and filesystem"},
	{"database/sql", "database packages"},
	{"k8s.io", "Kubernetes clients"},
	{"sigs.k8s.io", "Kubernetes clients"},
	{"github.com/aws", "cloud-provider clients"},
	{"cloud.google.com", "cloud-provider clients"},
	{"github.com/Azure", "cloud-provider clients"},
	{"github.com/anthropics", "external LLM clients"},
	{"github.com/sashabaranov/go-openai", "external LLM clients"},
	{"github.com/ollama", "external LLM clients"},
	{"gopkg.in/yaml", "YAML dependency (KB-FMT-1)"},
}

func forbiddenCategory(path string) (string, bool) {
	for _, f := range forbidden {
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

func TestImportBoundaryAllowlist(t *testing.T) {
	imports := productionImports(t)
	for _, p := range imports {
		if !allowedProductionImports[p] {
			cat, _ := forbiddenCategory(p)
			t.Errorf("production import %q is not on the allowlist %s", p, cat)
		}
	}
	// The adapter exists to join exactly these two packages.
	var hasDetector, hasObserver bool
	for _, p := range imports {
		hasDetector = hasDetector || p == detectorPkg
		hasObserver = hasObserver || p == observerPkg
	}
	if !hasDetector || !hasObserver {
		t.Errorf("adapter must import both the detector and the observer; imports = %v", imports)
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

// No clock read, no environment, no network, no package-level mutable state.
func TestNoClockEnvironmentOrGlobalState(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"time.Now", "time.Since", "time.Until", "os.Getenv", "http.", "net.Dial", "InsecureSkipVerify"} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s contains %q", name, banned)
			}
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				for _, ident := range spec.(*ast.ValueSpec).Names {
					// Sentinel errors only.
					if !strings.HasPrefix(ident.Name, "Err") {
						t.Errorf("%s declares package-level variable %q", name, ident.Name)
					}
				}
			}
		}
	}
}

// The adapter never constructs a Finding itself: every finding must come from
// certexpiry.Evaluate.
func TestFindingsComeOnlyFromTheDetector(t *testing.T) {
	fset := token.NewFileSet()
	evaluateCalls := 0
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "certexpiry" && sel.Sel.Name == "Evaluate" {
						evaluateCalls++
					}
				}
			case *ast.CompositeLit:
				// certexpiry.Finding{} is allowed only as the empty zero value.
				sel, ok := node.Type.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "certexpiry" || sel.Sel.Name != "Finding" {
					return true
				}
				if len(node.Elts) != 0 {
					t.Errorf("%s builds a certexpiry.Finding with fields; findings must come from the detector", name)
				}
			}
			return true
		})
	}
	if evaluateCalls != 1 {
		t.Errorf("certexpiry.Evaluate is called %d times; want exactly 1 call site", evaluateCalls)
	}
}
