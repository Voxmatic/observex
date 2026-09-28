package knowledge

import (
	"crypto/sha256"
	_ "embed" // go:embed of the approved catalog (D-INT-2)
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// Provenance of the embedded catalog (KB-SEED-2, approved 2026-09-22).
const (
	// CatalogSHA256 is the SHA-256 of the approved catalog.json bytes.
	CatalogSHA256 = "95aa0bd8aa24d000ee09c4b97cac925f2ba470cad4fead39896d5ffb6ba95767"
	// CatalogSize is the size in bytes of the approved catalog.json.
	CatalogSize = 457121

	// SourceArtifact, SourceVersion and SourceSHA256 identify the seed the
	// catalog must record as its source (metadata.source).
	SourceArtifact = "observex-sre-kb-seed-v0.1.2.yaml"
	SourceVersion  = "0.1.2"
	SourceSHA256   = "e0cc03d3b4a00cfeef8eb34c33c48926c854a0f9d8e884f5b7e5c5312076c8fb"

	// SchemaName is the schema the catalog must declare (metadata.source.schema).
	SchemaName = "observex-sre-kb-schema-v0.1.0.json"

	// SupersedesVersion is the seed version the catalog must record as
	// superseded (metadata.supersedes.version).
	SupersedesVersion = "0.1.1"

	// PatternCount is the exact number of patterns the catalog must contain.
	PatternCount = 63
)

// ErrInvalidCatalog is wrapped by every error returned for a catalog that
// fails validation.
var ErrInvalidCatalog = errors.New("knowledge: invalid catalog")

//go:embed catalog/catalog.json
var embeddedCatalog []byte

var loadOnce = sync.OnceValues(func() (Catalog, error) { return loadFrom(embeddedCatalog) })

// Load returns the embedded catalog after validating it. Validation runs once
// per process; every call returns the same result. A Catalog is never returned
// together with a non-nil error: on failure the Catalog is the empty zero value.
func Load() (Catalog, error) { return loadOnce() }

// loadFrom checks that data is exactly the approved catalog, then parses and
// validates it.
func loadFrom(data []byte) (Catalog, error) {
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != CatalogSHA256 || len(data) != CatalogSize {
		return Catalog{}, fmt.Errorf("%w: embedded catalog is not the approved artifact (SHA-256 %s, %d bytes; want %s, %d bytes)",
			ErrInvalidCatalog, got, len(data), CatalogSHA256, CatalogSize)
	}
	return parse(data)
}

// Catalog is a validated, read-only view of the knowledge catalog. It is safe
// for concurrent use. Its zero value is an empty catalog.
type Catalog struct {
	d *catalogData
}

type catalogData struct {
	meta     Metadata
	patterns []Pattern      // catalog order
	index    map[string]int // pattern ID -> position in patterns
}

// Metadata returns the catalog metadata, including its provenance.
func (c Catalog) Metadata() Metadata {
	if c.d == nil {
		return Metadata{}
	}
	return c.d.meta
}

// Len returns the number of patterns.
func (c Catalog) Len() int {
	if c.d == nil {
		return 0
	}
	return len(c.d.patterns)
}

// PatternIDs returns every pattern ID in catalog order, in a new slice.
func (c Catalog) PatternIDs() []string {
	if c.d == nil {
		return nil
	}
	ids := make([]string, len(c.d.patterns))
	for i, p := range c.d.patterns {
		ids[i] = p.ID
	}
	return ids
}

// Pattern returns a deep copy of the pattern with the given ID.
func (c Catalog) Pattern(id string) (Pattern, bool) {
	if c.d == nil {
		return Pattern{}, false
	}
	i, ok := c.d.index[id]
	if !ok {
		return Pattern{}, false
	}
	return c.d.patterns[i].clone(), true
}

// ByFamily returns deep copies of the patterns in family f, in catalog order.
// It returns nil if there are none.
func (c Catalog) ByFamily(f Family) []Pattern {
	return c.filter(func(p *Pattern) bool { return p.Family == f })
}

// ByMethodFit returns deep copies of the patterns that have at least one
// leading indicator with method fit m, in catalog order. It returns nil if
// there are none.
func (c Catalog) ByMethodFit(m MethodFit) []Pattern {
	return c.filter(func(p *Pattern) bool {
		for _, li := range p.LeadingIndicators {
			if li.MethodFit == m {
				return true
			}
		}
		return false
	})
}

// ByAutomationClass returns deep copies of the patterns that document at least
// one remediation with automation class a, in catalog order. It returns nil if
// there are none. This is a knowledge lookup only; it authorizes nothing.
func (c Catalog) ByAutomationClass(a AutomationClass) []Pattern {
	return c.filter(func(p *Pattern) bool {
		for _, r := range p.Remediations {
			if r.AutomationClass == a {
				return true
			}
		}
		return false
	})
}

func (c Catalog) filter(match func(*Pattern) bool) []Pattern {
	if c.d == nil {
		return nil
	}
	var out []Pattern
	for i := range c.d.patterns {
		if match(&c.d.patterns[i]) {
			out = append(out, c.d.patterns[i].clone())
		}
	}
	return out
}
