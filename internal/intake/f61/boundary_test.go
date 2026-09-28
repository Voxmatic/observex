package f61

import (
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allowedProductionImports is the complete set this package may import. The
// three first-party packages are the credential verifier, the frozen subject
// contract and the frozen observer (for the Vantage type only). No service,
// database, HTTP, middleware or service-token package appears here.
var allowedProductionImports = map[string]bool{
	"context": true, "errors": true, "fmt": true, "strings": true, "time": true, "unicode": true,
	"crypto/sha256": true, "encoding/binary": true, "sort": true,
	"github.com/observex/platform/internal/observe/tlscert": true,
	"github.com/observex/platform/internal/probetoken":      true,
	"github.com/observex/platform/internal/wire/f61":        true,
}

func productionSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		out[n] = string(b)
	}
	if len(out) == 0 {
		t.Fatal("no production files")
	}
	return out
}

func TestImportBoundaryAllowlist(t *testing.T) {
	fset := token.NewFileSet()
	seen := map[string]bool{}
	for name, src := range productionSources(t) {
		f, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			seen[p] = true
		}
	}
	var got []string
	for p := range seen {
		got = append(got, p)
		if !allowedProductionImports[p] {
			t.Errorf("production import %q is not on the allowlist", p)
		}
	}
	sort.Strings(got)
	t.Logf("production imports: %v", got)
}

// The intake path never reads tenant identity or a credential from any header
// other than Authorization. The header names below must not appear in
// production code at all, so no future edit can add a fallback silently.
func TestNoHeaderFallbackInProductionCode(t *testing.T) {
	for name, src := range productionSources(t) {
		code := stripComments(t, name, src)
		for _, h := range []string{"X-ObserveX-Org", "X-ObserveX-Token", "X-API-Key", "X-ObserveX-Vantage"} {
			if strings.Contains(strings.ToLower(code), strings.ToLower(h)) {
				t.Errorf("%s references header %s", name, h)
			}
		}
	}
}

func stripComments(t *testing.T, name, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	b := []byte(src)
	for _, cg := range f.Comments {
		for i := fset.Position(cg.Pos()).Offset; i < fset.Position(cg.End()).Offset; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}
	return string(b)
}
