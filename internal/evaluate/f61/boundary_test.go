package f61

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The evaluation is pure: no network, database, clock source, environment or
// service package. It may use the frozen F6.1 packages and the standard
// library pieces below.
var allowedProductionImports = map[string]bool{
	"context": true, "crypto/sha256": true, "encoding/hex": true, "errors": true, "sort": true, "time": true,
	"github.com/observex/platform/internal/adapt/tlscertexpiry": true,
	"github.com/observex/platform/internal/compose/f61":         true,
	"github.com/observex/platform/internal/correlate/f61":       true,
	"github.com/observex/platform/internal/detect/certexpiry":   true,
	"github.com/observex/platform/internal/observe/tlscert":     true,
	"github.com/observex/platform/internal/wire/f61":            true,
}

func TestImportBoundaryAllowlist(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
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
			if !allowedProductionImports[p] {
				t.Errorf("%s imports %q, which is not allowed in a pure evaluation package", n, p)
			}
		}
		src, _ := os.ReadFile(n)
		if strings.Contains(string(src), "time.Now(") {
			t.Errorf("%s reads the clock", n)
		}
	}
}
