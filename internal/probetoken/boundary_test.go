package probetoken

import (
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allowedProductionImports is the complete set this package may import: the
// standard library pieces it uses and the JWT library the agent install token
// already depends on. No first-party package, no network, no database.
var allowedProductionImports = map[string]bool{
	"crypto/hmac": true, "crypto/sha256": true, "crypto/subtle": true,
	"encoding/binary": true, "encoding/hex": true,
	"errors": true, "fmt": true, "io": true, "io/fs": true, "os": true,
	"strings": true, "time": true, "unicode": true, "unicode/utf8": true,
	"github.com/golang-jwt/jwt/v5": true,
}

func TestImportBoundaryAllowlist(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
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
		if strings.HasPrefix(p, "github.com/observex/") {
			t.Errorf("production import %q is first-party", p)
		}
	}
	sort.Strings(got)
	t.Logf("production imports: %v", got)
}
