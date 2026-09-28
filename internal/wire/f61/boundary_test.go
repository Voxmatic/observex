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

// allowedProductionImports is the complete set this package may import. It is
// standard library only, and only the two packages needed to report missing
// fields and to recognise a blank one. No first-party package appears here: in
// particular not the observer, the adapter or the detector, which this package
// must be able to exist without.
var allowedProductionImports = map[string]bool{
	"errors": true, "strings": true,
}

// deep entries are forbidden anywhere in the dependency closure; the rest are
// forbidden as direct imports only, because the standard library reaches some
// of them internally. This package's closure is small enough that both lists
// hold, but the distinction is kept so the test stays honest if it grows.
var forbidden = []struct {
	path, category string
	deep           bool
}{
	{"github.com/observex/platform/services", "processor, api-gateway, ingestor and every other service", true},
	{"github.com/observex/platform/agents", "agents", true},
	{"github.com/observex/platform/internal/observe", "the observation layer", true},
	{"github.com/observex/platform/internal/adapt", "the adapter layer", true},
	{"github.com/observex/platform/internal/detect", "the detector layer", true},
	{"github.com/observex/platform/internal/knowledge", "the knowledge package", true},
	{"github.com/observex/platform/internal/db", "database packages", true},
	{"github.com/observex/platform/internal/servicetoken", "credentials and service transport", true},
	{"github.com/observex/platform/internal/middleware", "HTTP middleware", true},
	{"github.com/observex/platform/pkg", "first-party runtime models", true},
	{"github.com/observex/platform", "any first-party package", true},
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
	{"net", "network access", true},
	{"net/http", "network access", true},
	{"crypto/tls", "TLS dialling belongs to the observer", true},
	{"crypto/x509", "certificate parsing belongs to the observer", true},
	{"time", "clocks and time-based behaviour", true},
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

func TestImportBoundaryAllowlist(t *testing.T) {
	imports := productionImports(t)
	for _, p := range imports {
		if !allowedProductionImports[p] {
			cat, _ := categoryFor(p, false)
			t.Errorf("production import %q is not on the allowlist %s", p, cat)
		}
	}
	t.Logf("production imports: %v", imports)
}

func TestImportBoundaryForbiddenCategories(t *testing.T) {
	for _, p := range productionImports(t) {
		if cat, bad := categoryFor(p, false); bad {
			t.Errorf("production import %q is in forbidden category %q", p, cat)
		}
	}
}

// No new dependency: every package reachable from this one is in the standard
// library, so nothing here can require a go.mod or go.sum change.
func TestImportBoundaryTransitiveIsStandardLibraryOnly(t *testing.T) {
	type item struct{ path, srcDir string }
	var queue []item
	for _, p := range productionImports(t) {
		queue = append(queue, item{p, ""})
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if seen[it.path] {
			continue
		}
		seen[it.path] = true
		pkg, err := build.Default.Import(it.path, it.srcDir, 0)
		if err != nil {
			t.Fatalf("resolve %s: %v", it.path, err)
		}
		if !pkg.Goroot {
			t.Errorf("non-standard-library package %q in the dependency closure", it.path)
		}
		if cat, bad := categoryFor(it.path, true); bad {
			t.Errorf("forbidden package %q (%s) in the dependency closure", it.path, cat)
		}
		for _, imp := range pkg.Imports {
			queue = append(queue, item{imp, pkg.Dir})
		}
	}
	t.Logf("dependency closure: %d standard-library packages, none forbidden", len(seen))
}

// 11. No observer, adapter or detector call occurs: neither package is
// imported, and no identifier from one is named anywhere in the source.
//
// Limit worth stating: this is a structural proof over this package's own
// files. It shows this package cannot reach those layers; it says nothing about
// what a future caller may do with the Subject it returns.
func TestNoObserverAdapterOrDetectorIsReferenced(t *testing.T) {
	for _, p := range productionImports(t) {
		for _, layer := range []string{
			"github.com/observex/platform/internal/observe",
			"github.com/observex/platform/internal/adapt",
			"github.com/observex/platform/internal/detect",
		} {
			if p == layer || strings.HasPrefix(p, layer+"/") {
				t.Errorf("production import %q reaches %s", p, layer)
			}
		}
	}
	for _, name := range productionFiles(t) {
		code := codeTokens(t, name)
		for _, ident := range []string{"tlscert", "certexpiry", "tlscertexpiry", "Probe", "Adapt", "Evaluate", "Finding", "Observation"} {
			if strings.Contains(code, ident) {
				t.Errorf("%s references %q in code", name, ident)
			}
		}
	}
}

// codeTokens returns the file's identifiers and literals, one per line, with
// comments excluded — so documentation may discuss the other layers by name
// while the code may not reference them.
func codeTokens(t *testing.T, name string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var b strings.Builder
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			b.WriteString(id.Name)
			b.WriteByte('\n')
		}
		if lit, ok := n.(*ast.BasicLit); ok {
			b.WriteString(lit.Value)
			b.WriteByte('\n')
		}
		return true
	})
	return b.String()
}

// No clock, no environment, no filesystem, no network, no package-level mutable
// state.
func TestNoClockEnvironmentNetworkOrGlobalState(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			"time.Now", "time.Since", "time.Until", "time.Time", "time.Duration",
			"os.Getenv", "os.LookupEnv", "os.Environ", "os.Open", "os.ReadFile",
			"flag.Parse", "net.Dial", "http.", "exec.Command", "sync.",
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
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				for _, ident := range spec.(*ast.ValueSpec).Names {
					// The only package-level vars are the sentinel errors.
					if !strings.HasPrefix(ident.Name, "Err") {
						t.Errorf("%s declares package-level variable %q", name, ident.Name)
					}
				}
			}
		}
		// No init function: nothing runs at import time.
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "init" {
				t.Errorf("%s declares an init function", name)
			}
		}
	}
}

// The exported surface is exactly the envelope, its single source, its single
// constructor, the prefix and the four fail-closed sentinels — nothing that
// could carry policy, state or an alternate identity source.
func TestExportedSurfaceIsExactlyTheEnvelope(t *testing.T) {
	want := []string{
		"CheckRow",
		"ErrMissingCheckID",
		"ErrMissingEndpoint",
		"ErrMissingNamespace",
		"ErrMissingOrgID",
		"ServicePrefix",
		"Subject",
		"SubjectFromCheck",
	}
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
				if d.Recv == nil && d.Name.IsExported() {
					got = append(got, d.Name.Name)
				}
				if d.Recv != nil && d.Name.IsExported() {
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
		t.Errorf("exported surface = %v, want exactly %v", got, want)
	}
}
