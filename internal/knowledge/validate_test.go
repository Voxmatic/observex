package knowledge

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Superseded provenance values. They appear here ONLY as inputs the package
// must reject; none of them is ever an accepted value.
const (
	supersededSeedV011      = "e0aaa08d340a07a388add8122260b9f22cb591377b7c370800dfaf0b16a61890"
	supersededCatalogV011   = "ecfd1a2e6e015f46d2cc96a4a0d93c11ee4c2a88c9100ba464f95510065bed56"
	supersededPreCorrection = "32b9900f0ad2aa64fe3d470e9fde2605d1445e383a17910234604e8b489b9b90"
	supersededSeedV010      = "f3246e21e0e0488ca508b0d06d2ce3c3445c417c32a6b04c87939d58ccc3a300"
)

// mutated decodes the approved catalog generically, applies fn and
// re-serializes it. The result is a test input only.
func mutated(t *testing.T, fn func(doc map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(embeddedCatalog, &doc); err != nil {
		t.Fatalf("decode approved catalog: %v", err)
	}
	fn(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	return out
}

func meta(doc map[string]any) map[string]any { return doc["metadata"].(map[string]any) }
func pat(doc map[string]any, i int) map[string]any {
	return doc["patterns"].([]any)[i].(map[string]any)
}
func obj(m map[string]any, key string) map[string]any { return m[key].(map[string]any) }
func item(m map[string]any, key string, i int) map[string]any {
	return m[key].([]any)[i].(map[string]any)
}

func expectReject(t *testing.T, data []byte, wantSubstr string) {
	t.Helper()
	_, err := parse(data)
	if err == nil {
		t.Fatalf("parse accepted an invalid catalog (want error containing %q)", wantSubstr)
	}
	if !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("error %v does not wrap ErrInvalidCatalog", err)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("error %q does not mention %q", err, wantSubstr)
	}
}

// TestParseControl shows the rejection tests below fail because of their
// mutation, not because of re-serialization.
func TestParseControl(t *testing.T) {
	if _, err := parse(embeddedCatalog); err != nil {
		t.Fatalf("approved bytes: %v", err)
	}
	c, err := parse(mutated(t, func(map[string]any) {}))
	if err != nil {
		t.Fatalf("re-serialized unmodified catalog: %v", err)
	}
	if c.Len() != PatternCount {
		t.Fatalf("control has %d patterns", c.Len())
	}
}

func TestRejectMalformedCatalog(t *testing.T) {
	badUTF8 := bytes.Clone(embeddedCatalog)
	badUTF8[bytes.Index(badUTF8, []byte("Latent code defect"))] = 0xff

	dupKey := bytes.Replace(embeddedCatalog, []byte(`"id": "F1.1",`), []byte(`"id": "F1.1", "id": "F1.1",`), 1)
	if bytes.Equal(dupKey, embeddedCatalog) {
		t.Fatal("duplicate-key fixture was not applied")
	}

	cases := []struct {
		name, want string
		data       []byte
	}{
		{"empty", "malformed JSON", nil},
		{"not JSON", "malformed JSON", []byte("{")},
		{"top-level array", "top level", []byte("[]")},
		{"top-level null", "required field $.metadata", []byte("null")},
		{"truncated", "malformed JSON", embeddedCatalog[:len(embeddedCatalog)/2]},
		{"trailing data", "trailing data", append(bytes.Clone(embeddedCatalog), []byte("{}")...)},
		{"invalid UTF-8", "not valid UTF-8", badUTF8},
		{"duplicate JSON key", "duplicate key \"id\"", dupKey},
		{"unknown top-level field", "unknown field", mutated(t, func(d map[string]any) { d["extra"] = 1 })},
		{"unknown pattern field", "unknown field", mutated(t, func(d map[string]any) { pat(d, 0)["owner"] = "x" })},
		{"wrong type", "cannot unmarshal", mutated(t, func(d map[string]any) { pat(d, 0)["title"] = 42 })},
		// Regressions for the three KB-SEED-2 schema-violation classes.
		{"class A: stray telemetry key", "unknown field", mutated(t, func(d map[string]any) {
			item(pat(d, 1), "required_telemetry", 0)["scope and diff"] = nil
		})},
		{"class B: list item is an object", "cannot unmarshal object", mutated(t, func(d map[string]any) {
			pat(d, 9)["diagnostic_methods"].([]any)[0] = map[string]any{"Remove load and observe": "if the system does not recover"}
		})},
		{"class C: numeric date", "cannot unmarshal number", mutated(t, func(d map[string]any) {
			item(pat(d, 7), "incidents", 1)["date"] = 2021
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectReject(t, tc.data, tc.want) })
	}
}

func TestRejectDuplicateID(t *testing.T) {
	data := mutated(t, func(d map[string]any) { pat(d, 1)["id"] = pat(d, 0)["id"] })
	expectReject(t, data, `duplicate pattern ID "F1.1"`)
}

func TestRejectWrongSourceSHA(t *testing.T) {
	for name, v := range map[string]any{
		"superseded v0.1.1 seed":        supersededSeedV011,
		"superseded v0.1.1 catalog":     supersededCatalogV011,
		"superseded pre-correction":     supersededPreCorrection,
		"superseded v0.1.0 seed":        supersededSeedV010,
		"approved catalog hash":         CatalogSHA256,
		"uppercase of the correct hash": strings.ToUpper(SourceSHA256),
		"one character changed":         SourceSHA256[:63] + "0",
		"trailing space":                SourceSHA256 + " ",
		"empty":                         "",
	} {
		t.Run(name, func(t *testing.T) {
			data := mutated(t, func(d map[string]any) { obj(meta(d), "source")["sha256"] = v })
			expectReject(t, data, "metadata.source.sha256")
		})
	}
}

func TestRejectWrongMetadata(t *testing.T) {
	cases := []struct {
		name, want string
		fn         func(d map[string]any)
	}{
		{"source version 0.1.1", "metadata.source.version", func(d map[string]any) { obj(meta(d), "source")["version"] = "0.1.1" }},
		{"catalog version 0.1.1", "metadata.version", func(d map[string]any) { meta(d)["version"] = "0.1.1" }},
		{"source artifact v0.1.1", "metadata.source.artifact", func(d map[string]any) {
			obj(meta(d), "source")["artifact"] = "observex-sre-kb-seed-v0.1.1.yaml"
		}},
		{"schema name", "metadata.source.schema", func(d map[string]any) { obj(meta(d), "source")["schema"] = "other.json" }},
		{"record_count 64", "metadata.record_count", func(d map[string]any) { meta(d)["record_count"] = 64 }},
		{"supersedes version", "metadata.supersedes.version", func(d map[string]any) { obj(meta(d), "supersedes")["version"] = "0.1.0" }},
		{"supersedes = source", "metadata.supersedes.sha256", func(d map[string]any) { obj(meta(d), "supersedes")["sha256"] = SourceSHA256 }},
		{"supersedes uppercase", "metadata.supersedes.sha256", func(d map[string]any) {
			obj(meta(d), "supersedes")["sha256"] = strings.ToUpper(supersededSeedV011)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectReject(t, mutated(t, tc.fn), tc.want) })
	}
}

func TestRejectWrongPatternCount(t *testing.T) {
	drop := mutated(t, func(d map[string]any) { d["patterns"] = d["patterns"].([]any)[:62] })
	expectReject(t, drop, "catalog has 62 patterns")

	add := mutated(t, func(d map[string]any) {
		ps := d["patterns"].([]any)
		extra := map[string]any{}
		for k, v := range ps[62].(map[string]any) {
			extra[k] = v
		}
		extra["id"] = "F13.3"
		d["patterns"] = append(ps, extra)
	})
	expectReject(t, add, "catalog has 64 patterns")
}

func TestRejectMissingRequiredField(t *testing.T) {
	cases := []struct {
		name, want string
		fn         func(d map[string]any)
	}{
		{"title", "$.patterns[0].title", func(d map[string]any) { delete(pat(d, 0), "title") }},
		{"title null", "$.patterns[0].title", func(d map[string]any) { pat(d, 0)["title"] = nil }},
		{"remediations", "$.patterns[0].remediations", func(d map[string]any) { delete(pat(d, 0), "remediations") }},
		{"incidents", "$.patterns[0].incidents", func(d map[string]any) { delete(pat(d, 0), "incidents") }},
		{"human_authority_required", "human_authority_required", func(d map[string]any) {
			delete(obj(pat(d, 0), "rollback_and_escalation"), "human_authority_required")
		}},
		{"classification.propagation", "classification.propagation", func(d map[string]any) {
			delete(obj(pat(d, 0), "classification"), "propagation")
		}},
		{"evidence url", "evidence[0].url", func(d map[string]any) { delete(item(pat(d, 0), "evidence", 0), "url") }},
		{"indicator signal", "leading_indicators[0].signal", func(d map[string]any) {
			delete(item(pat(d, 0), "leading_indicators", 0), "signal")
		}},
		{"remediation automation_class", "remediations[0].automation_class", func(d map[string]any) {
			delete(item(pat(d, 0), "remediations", 0), "automation_class")
		}},
		{"metadata.source", "$.metadata.source", func(d map[string]any) { delete(meta(d), "source") }},
		{"empty affected_systems", "affected_systems must have at least one item", func(d map[string]any) {
			pat(d, 0)["affected_systems"] = []any{}
		}},
		{"empty title", "title shorter", func(d map[string]any) { pat(d, 0)["title"] = "" }},
		{"empty telemetry source", "required_telemetry[0].source is empty", func(d map[string]any) {
			item(pat(d, 0), "required_telemetry", 0)["source"] = ""
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectReject(t, mutated(t, tc.fn), tc.want) })
	}
}

func TestRejectInvalidValues(t *testing.T) {
	cases := []struct {
		name, want string
		fn         func(d map[string]any)
	}{
		{"unknown family", "is not a schema family", func(d map[string]any) { pat(d, 0)["family"] = "F99-unknown" }},
		{"family does not match id", "does not match id", func(d map[string]any) {
			pat(d, 0)["family"] = string(FamilySaturationAndMetastability)
		}},
		{"id family 14", "does not match ^F", func(d map[string]any) { pat(d, 0)["id"] = "F14.1" }},
		{"id leading zero", "does not match ^F", func(d map[string]any) { pat(d, 0)["id"] = "F01.1" }},
		{"id three-digit sub", "does not match ^F", func(d map[string]any) { pat(d, 0)["id"] = "F1.123" }},
		{"pattern version", "is not MAJOR.MINOR.PATCH", func(d map[string]any) { pat(d, 0)["version"] = "1.0" }},
		{"automation class", "automation_class", func(d map[string]any) {
			item(pat(d, 0), "remediations", 0)["automation_class"] = "A5-fully-autonomous"
		}},
		{"method fit", "method_fit", func(d map[string]any) {
			item(pat(d, 0), "leading_indicators", 0)["method_fit"] = "llm"
		}},
		{"confidence", "confidence", func(d map[string]any) { pat(d, 0)["confidence"] = "certain" }},
		{"trigger class", "trigger_class", func(d map[string]any) {
			obj(pat(d, 0), "classification")["trigger_class"] = []any{"cosmic-rays"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectReject(t, mutated(t, tc.fn), tc.want) })
	}
}

// TestLoadRejectsNonApprovedBytes: Load's byte check refuses anything but the
// approved artifact, even a catalog that parse would accept.
func TestLoadRejectsNonApprovedBytes(t *testing.T) {
	if _, err := loadFrom(embeddedCatalog); err != nil {
		t.Fatalf("approved bytes rejected: %v", err)
	}
	for name, data := range map[string][]byte{
		"re-serialized but valid": mutated(t, func(map[string]any) {}),
		"CRLF line endings":       bytes.ReplaceAll(embeddedCatalog, []byte("\n"), []byte("\r\n")),
		"extra trailing newline":  append(bytes.Clone(embeddedCatalog), '\n'),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadFrom(data)
			if err == nil || !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "not the approved artifact") {
				t.Fatalf("loadFrom accepted non-approved bytes: %v", err)
			}
		})
	}
}
