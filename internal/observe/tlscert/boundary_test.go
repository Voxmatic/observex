package tlscert

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allowedProductionImports is the complete set this package may import. It is
// standard library only: no first-party package, and in particular not
// internal/detect/certexpiry — the observer must not depend on the detector.
var allowedProductionImports = map[string]bool{
	"context": true, "crypto/tls": true, "crypto/x509": true, "errors": true,
	"fmt": true, "io": true, "net": true, "strings": true, "time": true,
	// The observation wire codec (wire.go), authorised for the G-1 probe
	// boundary: still standard library only.
	"bytes": true, "encoding/json": true,
}

// deep entries are forbidden anywhere in the dependency closure; the rest are
// forbidden as direct imports only, because the standard library reaches them
// internally (crypto/tls reaches os, for example).
var forbidden = []struct {
	path, category string
	deep           bool
}{
	{"database/sql", "database packages", true},
	{"os/exec", "process execution", true},
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
	{"github.com/observex/platform", "any first-party package, including the detector", true},
	{"os", "environment and filesystem", false},
	{"net/http", "application traffic", false},
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

func TestImportBoundaryTransitive(t *testing.T) {
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
		if it.path == "C" {
			// Expected: the standard library's net package uses cgo for the
			// platform resolver. Nothing in this package uses cgo directly.
			continue
		}
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

// ── security properties enforced against the source itself ──────────────────

// InsecureSkipVerify may appear exactly once, in the observation handshake, and
// only as a literal true — never wired to a parameter, field or variable.
func TestInsecureSkipVerifyAppearsOnlyOnceAsALiteral(t *testing.T) {
	fset := token.NewFileSet()
	occurrences := 0
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "InsecureSkipVerify" {
				return true
			}
			occurrences++
			val, ok := kv.Value.(*ast.Ident)
			if !ok || val.Name != "true" {
				t.Errorf("%s: InsecureSkipVerify is set from %T, not a literal", name, kv.Value)
			}
			return true
		})
	}
	if occurrences != 1 {
		t.Fatalf("InsecureSkipVerify appears %d times in production files; want exactly 1", occurrences)
	}
}

// No environment variable, flag or global can influence this package.
func TestNoEnvironmentOrGlobalSwitches(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbiddenText := range []string{"os.Getenv", "os.LookupEnv", "flag.Parse", "http.DefaultTransport", "SetDefault"} {
			if strings.Contains(string(src), forbiddenText) {
				t.Errorf("%s contains %q", name, forbiddenText)
			}
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		// No package-level mutable state.
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for _, ident := range vs.Names {
					// The only package-level vars are the sentinel errors.
					if !strings.HasPrefix(ident.Name, "Err") {
						t.Errorf("%s declares package-level variable %q", name, ident.Name)
					}
				}
			}
		}
	}
}

// The public API exposes no verification switch and no key material.
func TestPublicAPIHasNoUnsafeSurface(t *testing.T) {
	cfg := reflect.TypeOf(Config{})
	wantCfg := []string{"Dial", "Now", "Roots", "Vantage"}
	gotCfg := fieldNames(cfg)
	if !reflect.DeepEqual(gotCfg, wantCfg) {
		t.Fatalf("Config fields = %v, want exactly %v", gotCfg, wantCfg)
	}
	for _, f := range gotCfg {
		lower := strings.ToLower(f)
		if strings.Contains(lower, "insecure") || strings.Contains(lower, "skip") || strings.Contains(lower, "verify") {
			t.Errorf("Config exposes a verification switch: %q", f)
		}
	}

	wantCert := []string{"Position", "IsLeaf", "IsCA", "Subject", "Issuer", "SerialNumber", "NotBefore", "NotAfter"}
	if got := exportedFieldNames(reflect.TypeOf(Certificate{})); !reflect.DeepEqual(got, wantCert) {
		t.Fatalf("Certificate exported fields = %v, want exactly %v", got, wantCert)
	}
	wantObs := []string{"CheckID", "Endpoint", "ObservedAt", "Duration", "Outcome", "Trust", "Vantage", "Detail"}
	if got := exportedFieldNames(reflect.TypeOf(Observation{})); !reflect.DeepEqual(got, wantObs) {
		t.Fatalf("Observation exported fields = %v, want exactly %v", got, wantObs)
	}
	// No severity, confidence, action, policy or key material anywhere.
	for _, tp := range []reflect.Type{reflect.TypeOf(Observation{}), reflect.TypeOf(Certificate{}), reflect.TypeOf(TrustStatus{})} {
		for _, f := range fieldNames(tp) {
			lower := strings.ToLower(f)
			for _, banned := range []string{"key", "secret", "severity", "confidence", "action", "policy", "remediation", "score"} {
				if strings.Contains(lower, banned) {
					t.Errorf("%s exposes field %q", tp.Name(), f)
				}
			}
		}
	}
}

func fieldNames(tp reflect.Type) []string {
	out := make([]string, 0, tp.NumField())
	for i := 0; i < tp.NumField(); i++ {
		out = append(out, tp.Field(i).Name)
	}
	return out
}

func exportedFieldNames(tp reflect.Type) []string {
	out := make([]string, 0, tp.NumField())
	for i := 0; i < tp.NumField(); i++ {
		if tp.Field(i).IsExported() {
			out = append(out, tp.Field(i).Name)
		}
	}
	return out
}
