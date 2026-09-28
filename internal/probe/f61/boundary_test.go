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

// allowedProductionImports: the probe side depends on the observer and the
// credential type only. In particular it does not import the processor's
// intake, the database, middleware, the service token or any service.
var allowedProductionImports = map[string]bool{
	"bytes": true, "context": true, "encoding/json": true, "errors": true, "fmt": true,
	"io": true, "io/fs": true, "net/http": true, "net/url": true, "os": true,
	"strings": true, "time": true, "crypto/tls": true, "crypto/x509": true, "sync": true,
	"net": true, "net/netip": true,
	"github.com/observex/platform/internal/observe/tlscert": true,
	"github.com/observex/platform/internal/probetoken":      true,
}

func TestImportBoundaryAllowlist(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			seen[p] = true
		}
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		// The probe sends no identity header of its own.
		for _, h := range []string{"X-ObserveX-Org", "X-ObserveX-Vantage", "X-ObserveX-Token"} {
			if strings.Contains(string(src), h) {
				t.Errorf("%s mentions %s", n, h)
			}
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
