// Package knowledge provides read-only access to the ObserveX SRE failure-pattern
// knowledge catalog.
//
// # What this package is
//
// The catalog is a data artifact: 63 documented production-failure patterns
// (mechanism, symptoms, leading indicators, telemetry, remediations and their
// automation classes, evidence). It is embedded into the binary with go:embed
// (D-INT-2, D-INT-3), parsed with the standard library's encoding/json only
// (KB-FMT-1), validated before use, and exposed through a small typed API that
// never hands out references to its internal slices or maps.
//
// # What this package is not
//
// It performs no detection, remediation, policy evaluation, network access,
// credential handling, database access, LLM calls or runtime configuration.
// Filtering functions such as ByAutomationClass are lookups over documented
// knowledge. A pattern listing a remediation as A3-guardrailed-autonomous is a
// statement in the catalog, NOT an authorization to automate anything. The
// package is not integrated into any caller.
//
// # Provenance (KB-SEED-2, approved 2026-09-22)
//
//	catalog/catalog.json  SHA-256 95aa0bd8aa24d000ee09c4b97cac925f2ba470cad4fead39896d5ffb6ba95767  (457,121 bytes)
//	  metadata.source     observex-sre-kb-seed-v0.1.2.yaml
//	                      SHA-256 e0cc03d3b4a00cfeef8eb34c33c48926c854a0f9d8e884f5b7e5c5312076c8fb
//
// The catalog bytes are a byte-for-byte copy of the approved artifact. They are
// never regenerated or re-serialized here. Load refuses to return a catalog
// unless the embedded bytes hash to CatalogSHA256 and the catalog records
// SourceSHA256 as its source. No artifact records its own hash; the expected
// values live in this package's source.
//
// # Validation performed before the catalog is made available
//
//   - embedded bytes hash to CatalogSHA256 (Load only);
//   - valid UTF-8, syntactically valid JSON, no duplicate object keys, no
//     trailing data;
//   - every key required by observex-sre-kb-schema-v0.1.0.json is present and
//     non-null, plus rollback_and_escalation.human_authority_required (see below);
//   - strict typed decoding: unknown fields and wrong JSON types are rejected;
//   - metadata: version, record_count, source artifact/version/SHA-256/schema,
//     supersedes version;
//   - exactly PatternCount patterns, unique IDs in the schema's ID format, family
//     consistent with the ID, schema enums, schema minimum lengths and minimum
//     item counts, and non-empty required strings.
//
// Deliberate differences from the schema: human_authority_required is optional
// in the schema but required here, so that its absence can never be read as
// "false"; required strings must be non-empty. The schema's conditional (allOf)
// rules and its "format" annotations are enforced by the catalog generator's
// schema gate, not re-implemented here.
package knowledge
