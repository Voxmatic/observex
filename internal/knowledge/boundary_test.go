package knowledge

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allowedProductionImports is the COMPLETE set of packages the production
// (non-test) files may import: standard library, no I/O beyond in-memory
// readers, no os, no network. Any other import fails TestImportBoundaryAllowlist.
var allowedProductionImports = map[string]bool{
	"bytes": true, "crypto/sha256": true, "embed": true, "encoding/hex": true,
	"encoding/json": true, "errors": true, "fmt": true, "io": true,
	"strconv": true, "strings": true, "sync": true, "unicode/utf8": true,
}

// forbidden names the dependency categories this package must never use.
// Matching is exact or by path prefix ("x" matches "x" and "x/...").
var forbidden = []struct{ path, category string }{
	{"github.com/observex/platform", "any first-party package: services/, gateway, agents, remediation/execution, internal/db"},
	{"net", "network clients (standard library)"},
	{"os/exec", "process execution"},
	{"plugin", "dynamic code loading"},
	{"crypto/tls", "network transport"},
	{"database/sql", "database packages"},
	{"github.com/lib/pq", "database packages"},
	{"github.com/jackc", "database packages"},
	{"github.com/ClickHouse", "database packages"},
	{"github.com/go-sql-driver", "database packages"},
	{"github.com/redis", "database packages"},
	{"go.mongodb.org", "database packages"},
	{"gorm.io", "database packages"},
	{"github.com/anthropics", "external LLM clients"},
	{"github.com/sashabaranov/go-openai", "external LLM clients"},
	{"github.com/openai", "external LLM clients"},
	{"github.com/ollama", "external LLM clients"},
	{"github.com/tmc/langchaingo", "external LLM clients"},
	{"github.com/google/generative-ai-go", "external LLM clients"},
	{"google.golang.org/genai", "external LLM clients"},
	{"google.golang.org/grpc", "network clients"},
	{"github.com/gofiber", "network clients"},
	{"github.com/valyala/fasthttp", "network clients"},
	{"github.com/gorilla/websocket", "network clients"},
	{"github.com/go-resty", "network clients"},
	{"github.com/nats-io", "network clients"},
	{"github.com/segmentio/kafka-go", "network clients"},
	{"github.com/IBM/sarama", "network clients"},
	{"k8s.io", "Kubernetes clients"},
	{"sigs.k8s.io", "Kubernetes clients"},
	{"github.com/aws", "cloud-provider clients"},
	{"cloud.google.com", "cloud-provider clients"},
	{"google.golang.org/api", "cloud-provider clients"},
	{"github.com/Azure", "cloud-provider clients"},
	{"github.com/digitalocean", "cloud-provider clients"},
	{"github.com/oracle/oci-go-sdk", "cloud-provider clients"},
	{"gopkg.in/yaml", "YAML dependency (KB-FMT-1)"},
	{"github.com/goccy/go-yaml", "YAML dependency (KB-FMT-1)"},
	{"github.com/ghodss/yaml", "YAML dependency (KB-FMT-1)"},
}

func forbiddenCategory(path string) (string, bool) {
	for _, f := range forbidden {
		if path == f.path || strings.HasPrefix(path, f.path+"/") {
			return f.category, true
		}
	}
	return "", false
}

func isStdPath(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

type goFile struct {
	name    string
	test    bool
	imports []string
	embeds  []string
}

// packageFiles parses every .go file in the package directory.
func packageFiles(t *testing.T) []goFile {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []goFile
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		gf := goFile{name: e.Name(), test: strings.HasSuffix(e.Name(), "_test.go")}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			gf.imports = append(gf.imports, p)
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if rest, ok := strings.CutPrefix(c.Text, "//go:embed"); ok {
					gf.embeds = append(gf.embeds, strings.Fields(rest)...)
				}
			}
		}
		files = append(files, gf)
	}
	if len(files) == 0 {
		t.Fatal("no Go files found")
	}
	return files
}

func productionImports(t *testing.T) []string {
	set := map[string]bool{}
	for _, f := range packageFiles(t) {
		if !f.test {
			for _, p := range f.imports {
				set[p] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// TestImportBoundaryAllowlist: production files import only the allowlist.
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

// TestImportBoundaryForbiddenCategories: no file, production or test, imports a
// forbidden category or any non-standard-library package.
func TestImportBoundaryForbiddenCategories(t *testing.T) {
	for _, f := range packageFiles(t) {
		for _, p := range f.imports {
			if cat, bad := forbiddenCategory(p); bad {
				t.Errorf("%s imports %q: forbidden category %q", f.name, p, cat)
			}
			if !isStdPath(p) {
				t.Errorf("%s imports non-standard-library package %q", f.name, p)
			}
			if p == "C" || p == "unsafe" {
				t.Errorf("%s imports %q", f.name, p)
			}
		}
	}
}

// TestImportBoundaryTransitive walks the full dependency closure of the
// production imports (go/build, host GOOS/GOARCH). Every package in it must be
// standard library and none may be in a forbidden category. Honest limit: the
// standard library's own transitive dependencies (for example fmt -> os) cannot
// be excluded; the allowlist above prevents this package from using them.
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
			t.Errorf("cgo (import \"C\") in the dependency closure")
			continue
		}
		pkg, err := build.Default.Import(it.path, it.srcDir, 0)
		if err != nil {
			t.Fatalf("resolve %s: %v", it.path, err)
		}
		if !pkg.Goroot {
			t.Errorf("non-standard-library package %q in the dependency closure", pkg.ImportPath)
		}
		if cat, bad := forbiddenCategory(pkg.ImportPath); bad {
			t.Errorf("forbidden package %q (%s) in the dependency closure", pkg.ImportPath, cat)
		}
		for _, imp := range pkg.Imports {
			queue = append(queue, item{imp, pkg.Dir})
		}
	}
	if len(seen) < len(allowedProductionImports) {
		t.Fatalf("dependency closure has only %d packages; walk did not run", len(seen))
	}
	t.Logf("dependency closure: %d standard-library packages, none forbidden", len(seen))
}

// TestEmbedDirective: the package embeds exactly the approved catalog file and
// nothing else.
func TestEmbedDirective(t *testing.T) {
	var embeds []string
	for _, f := range packageFiles(t) {
		if len(f.embeds) > 0 && f.test {
			t.Errorf("%s: test files must not embed data", f.name)
		}
		embeds = append(embeds, f.embeds...)
	}
	if len(embeds) != 1 || embeds[0] != "catalog/catalog.json" {
		t.Fatalf("go:embed directives %v; want exactly [catalog/catalog.json]", embeds)
	}
}
