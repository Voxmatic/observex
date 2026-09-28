package knowledge

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// parse validates data as a catalog and builds the read-only view. It does not
// check the byte hash (loadFrom does); tests call it directly with modified
// catalogs.
func parse(data []byte) (Catalog, error) {
	if !utf8.Valid(data) {
		// encoding/json would silently replace invalid UTF-8 with U+FFFD.
		return Catalog{}, invalid("not valid UTF-8")
	}
	if err := checkJSONSyntaxAndDuplicateKeys(data); err != nil {
		return Catalog{}, invalid("%v", err)
	}
	if err := checkRequiredKeys(data); err != nil {
		return Catalog{}, invalid("%v", err)
	}

	var doc struct {
		Metadata Metadata  `json:"metadata"`
		Patterns []Pattern `json:"patterns"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return Catalog{}, invalid("decode: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Catalog{}, invalid("trailing data after the catalog object")
	}

	if err := checkMetadata(doc.Metadata); err != nil {
		return Catalog{}, invalid("%v", err)
	}
	if len(doc.Patterns) != PatternCount {
		return Catalog{}, invalid("catalog has %d patterns; want exactly %d", len(doc.Patterns), PatternCount)
	}
	index := make(map[string]int, len(doc.Patterns))
	for i := range doc.Patterns {
		p := &doc.Patterns[i]
		if err := checkPattern(p); err != nil {
			return Catalog{}, invalid("pattern %d (%q): %v", i, p.ID, err)
		}
		if j, dup := index[p.ID]; dup {
			return Catalog{}, invalid("duplicate pattern ID %q at positions %d and %d", p.ID, j, i)
		}
		index[p.ID] = i
	}
	return Catalog{d: &catalogData{meta: doc.Metadata, patterns: doc.Patterns, index: index}}, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidCatalog, fmt.Sprintf(format, args...))
}

// checkJSONSyntaxAndDuplicateKeys walks every token. It rejects malformed JSON,
// more than one top-level value, and objects with a repeated key (which
// encoding/json would otherwise resolve silently by keeping the last value).
func checkJSONSyntaxAndDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := walkValue(dec, "$"); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the top-level JSON value")
	}
	return nil
}

func walkValue(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("malformed JSON at %s: %v", path, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil // scalar
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return fmt.Errorf("malformed JSON at %s: %v", path, err)
			}
			key, _ := kt.(string)
			if seen[key] {
				return fmt.Errorf("duplicate key %q in object at %s", key, path)
			}
			seen[key] = true
			if err := walkValue(dec, path+"."+key); err != nil {
				return err
			}
		}
	case '[':
		for i := 0; dec.More(); i++ {
			if err := walkValue(dec, path+"["+strconv.Itoa(i)+"]"); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("malformed JSON at %s: unexpected %q", path, delim)
	}
	if _, err := dec.Token(); err != nil { // closing delimiter
		return fmt.Errorf("malformed JSON at %s: %v", path, err)
	}
	return nil
}

// Keys that must be present and non-null. Lists mirror the schema's "required"
// arrays; human_authority_required is an ObserveX addition (see package doc).
var (
	reqTop            = []string{"metadata", "patterns"}
	reqMetadata       = []string{"version", "compiled", "record_count", "completeness", "known_gaps_ref", "source", "supersedes"}
	reqSource         = []string{"artifact", "version", "sha256", "schema", "note"}
	reqSupersedes     = []string{"version", "sha256"}
	reqPattern        = []string{"id", "version", "title", "family", "classification", "problem_and_mechanism", "affected_systems", "preconditions", "observable_symptoms", "leading_indicators", "required_telemetry", "root_causes", "diagnostic_methods", "incidents", "remediations", "verification", "rollback_and_escalation", "automation_safety_limits", "predictive_monitoring", "confidence", "evidence", "solution_maturity", "last_reviewed"}
	reqClassification = []string{"trigger_class", "propagation", "observability_class", "reversibility", "recovery_shape"}
	reqIndicator      = []string{"signal", "prediction_class", "evidence_tier"}
	reqTelemetry      = []string{"signal", "source", "availability_risk"}
	reqIncident       = []string{"organisation", "date", "url", "evidence_tier"}
	reqRemediation    = []string{"action", "reversibility", "automation_class", "failure_mode_if_wrong"}
	reqRollback       = []string{"rollback_available", "escalation_trigger", "human_authority_required"}
	reqPredictive     = []string{"feasible", "rationale"}
	reqEvidence       = []string{"tier", "url", "retrieval"}
)

type rawObject = map[string]json.RawMessage

func requireKeys(obj rawObject, keys []string, path string) error {
	for _, k := range keys {
		v, ok := obj[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("required field %s.%s is missing or null", path, k)
		}
	}
	return nil
}

func requireObject(parent rawObject, key string, keys []string, path string) error {
	var obj rawObject
	if err := json.Unmarshal(parent[key], &obj); err != nil {
		return fmt.Errorf("%s.%s: %v", path, key, err)
	}
	return requireKeys(obj, keys, path+"."+key)
}

func requireEach(parent rawObject, key string, keys []string, path string) error {
	var items []rawObject
	if err := json.Unmarshal(parent[key], &items); err != nil {
		return fmt.Errorf("%s.%s: %v", path, key, err)
	}
	for i, it := range items {
		if err := requireKeys(it, keys, fmt.Sprintf("%s.%s[%d]", path, key, i)); err != nil {
			return err
		}
	}
	return nil
}

// checkRequiredKeys verifies key presence independently of Go zero values, so
// that an absent field can never be mistaken for an empty or false one.
func checkRequiredKeys(data []byte) error {
	var top rawObject
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("top level: %v", err)
	}
	if err := requireKeys(top, reqTop, "$"); err != nil {
		return err
	}
	var meta rawObject
	if err := json.Unmarshal(top["metadata"], &meta); err != nil {
		return fmt.Errorf("$.metadata: %v", err)
	}
	if err := requireKeys(meta, reqMetadata, "$.metadata"); err != nil {
		return err
	}
	if err := requireObject(meta, "source", reqSource, "$.metadata"); err != nil {
		return err
	}
	if err := requireObject(meta, "supersedes", reqSupersedes, "$.metadata"); err != nil {
		return err
	}
	var patterns []rawObject
	if err := json.Unmarshal(top["patterns"], &patterns); err != nil {
		return fmt.Errorf("$.patterns: %v", err)
	}
	for i, p := range patterns {
		path := fmt.Sprintf("$.patterns[%d]", i)
		if err := requireKeys(p, reqPattern, path); err != nil {
			return err
		}
		checks := []error{
			requireObject(p, "classification", reqClassification, path),
			requireEach(p, "leading_indicators", reqIndicator, path),
			requireEach(p, "required_telemetry", reqTelemetry, path),
			requireEach(p, "incidents", reqIncident, path),
			requireEach(p, "remediations", reqRemediation, path),
			requireObject(p, "rollback_and_escalation", reqRollback, path),
			requireObject(p, "predictive_monitoring", reqPredictive, path),
			requireEach(p, "evidence", reqEvidence, path),
		}
		if err := errors.Join(checks...); err != nil {
			return err
		}
	}
	return nil
}

func checkMetadata(m Metadata) error {
	switch {
	case m.Version != SourceVersion:
		return fmt.Errorf("metadata.version %q; want %q", m.Version, SourceVersion)
	case m.RecordCount != PatternCount:
		return fmt.Errorf("metadata.record_count %d; want %d", m.RecordCount, PatternCount)
	case m.Source.Artifact != SourceArtifact:
		return fmt.Errorf("metadata.source.artifact %q; want %q", m.Source.Artifact, SourceArtifact)
	case m.Source.Version != SourceVersion:
		return fmt.Errorf("metadata.source.version %q; want %q", m.Source.Version, SourceVersion)
	case m.Source.SHA256 != SourceSHA256: // exact, case-sensitive
		return fmt.Errorf("metadata.source.sha256 %q; want %q", m.Source.SHA256, SourceSHA256)
	case m.Source.Schema != SchemaName:
		return fmt.Errorf("metadata.source.schema %q; want %q", m.Source.Schema, SchemaName)
	case m.Supersedes.Version != SupersedesVersion:
		return fmt.Errorf("metadata.supersedes.version %q; want %q", m.Supersedes.Version, SupersedesVersion)
	case !isLowerHex64(m.Supersedes.SHA256) || m.Supersedes.SHA256 == SourceSHA256:
		return fmt.Errorf("metadata.supersedes.sha256 %q is not a distinct lowercase SHA-256", m.Supersedes.SHA256)
	case m.Compiled == "" || m.Completeness == "" || m.KnownGapsRef == "" || m.Source.Note == "":
		return errors.New("metadata has an empty required field")
	}
	return nil
}

func checkPattern(p *Pattern) error {
	famNum, ok := parseID(p.ID)
	if !ok {
		return fmt.Errorf("id %q does not match ^F(1[0-3]|[1-9])\\.[0-9]{1,2}$", p.ID)
	}
	if !isSemver(p.Version) {
		return fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", p.Version)
	}
	if !families[p.Family] {
		return fmt.Errorf("family %q is not a schema family", p.Family)
	}
	if want := "F" + strconv.Itoa(famNum) + "-"; !strings.HasPrefix(string(p.Family), want) {
		return fmt.Errorf("family %q does not match id %q", p.Family, p.ID)
	}
	if utf8.RuneCountInString(p.Title) < 3 {
		return errors.New("title shorter than 3 characters")
	}
	if utf8.RuneCountInString(p.ProblemAndMechanism) < 40 {
		return errors.New("problem_and_mechanism shorter than 40 characters")
	}
	for _, l := range []struct {
		name string
		n    int
	}{
		{"affected_systems", len(p.AffectedSystems)}, {"preconditions", len(p.Preconditions)},
		{"observable_symptoms", len(p.ObservableSymptoms)}, {"root_causes", len(p.RootCauses)},
		{"diagnostic_methods", len(p.DiagnosticMethods)}, {"verification", len(p.Verification)},
		{"automation_safety_limits", len(p.AutomationSafetyLimits)},
		{"classification.trigger_class", len(p.Classification.TriggerClass)},
	} {
		if l.n == 0 {
			return fmt.Errorf("%s must have at least one item", l.name)
		}
	}
	if len(p.RequiredTelemetry) == 0 || len(p.Remediations) == 0 || len(p.Evidence) == 0 {
		return errors.New("required_telemetry, remediations and evidence must each have at least one item")
	}

	c := p.Classification
	for _, t := range c.TriggerClass {
		if err := enum("classification.trigger_class", t, triggerClasses); err != nil {
			return err
		}
	}
	if err := firstErr(
		enum("classification.propagation", c.Propagation, propagations),
		enum("classification.observability_class", c.ObservabilityClass, observabilityClasses),
		enum("classification.reversibility", c.Reversibility, reversibilities),
		enum("classification.recovery_shape", c.RecoveryShape, recoveryShapes),
		enum("confidence", p.Confidence, confidences),
		enum("solution_maturity", p.SolutionMaturity, maturities),
		enum("rollback_and_escalation.rollback_available", p.RollbackAndEscalation.RollbackAvailable, rollbackAvailability),
		enum("predictive_monitoring.feasible", p.PredictiveMonitoring.Feasible, feasibilities),
		nonEmpty("rollback_and_escalation.escalation_trigger", p.RollbackAndEscalation.EscalationTrigger),
		nonEmpty("predictive_monitoring.rationale", p.PredictiveMonitoring.Rationale),
		nonEmpty("last_reviewed", p.LastReviewed),
	); err != nil {
		return err
	}
	for i, li := range p.LeadingIndicators {
		if err := firstErr(
			nonEmpty(fmt.Sprintf("leading_indicators[%d].signal", i), li.Signal),
			enum(fmt.Sprintf("leading_indicators[%d].prediction_class", i), li.PredictionClass, predictionClasses),
			enum(fmt.Sprintf("leading_indicators[%d].evidence_tier", i), li.EvidenceTier, evidenceTiers),
			optionalEnum(fmt.Sprintf("leading_indicators[%d].method_fit", i), string(li.MethodFit), methodFits),
		); err != nil {
			return err
		}
	}
	for i, t := range p.RequiredTelemetry {
		if err := firstErr(
			nonEmpty(fmt.Sprintf("required_telemetry[%d].signal", i), t.Signal),
			nonEmpty(fmt.Sprintf("required_telemetry[%d].source", i), t.Source),
			enum(fmt.Sprintf("required_telemetry[%d].availability_risk", i), t.AvailabilityRisk, availabilityRisks),
		); err != nil {
			return err
		}
	}
	for i, in := range p.Incidents {
		if err := firstErr(
			nonEmpty(fmt.Sprintf("incidents[%d].organisation", i), in.Organisation),
			nonEmpty(fmt.Sprintf("incidents[%d].date", i), in.Date),
			nonEmpty(fmt.Sprintf("incidents[%d].url", i), in.URL),
			enum(fmt.Sprintf("incidents[%d].evidence_tier", i), in.EvidenceTier, evidenceTiers),
			optionalEnum(fmt.Sprintf("incidents[%d].retrieval", i), in.Retrieval, retrievals),
		); err != nil {
			return err
		}
	}
	for i, r := range p.Remediations {
		if err := firstErr(
			nonEmpty(fmt.Sprintf("remediations[%d].action", i), r.Action),
			enum(fmt.Sprintf("remediations[%d].reversibility", i), r.Reversibility, reversibilities),
			enum(fmt.Sprintf("remediations[%d].automation_class", i), string(r.AutomationClass), automationClasses),
			optionalEnum(fmt.Sprintf("remediations[%d].evidence_tier", i), r.EvidenceTier, evidenceTiers),
		); err != nil {
			return err
		}
		if utf8.RuneCountInString(r.FailureModeIfWrong) < 10 {
			return fmt.Errorf("remediations[%d].failure_mode_if_wrong shorter than 10 characters", i)
		}
	}
	for i, e := range p.Evidence {
		if err := firstErr(
			enum(fmt.Sprintf("evidence[%d].tier", i), e.Tier, evidenceTiers),
			nonEmpty(fmt.Sprintf("evidence[%d].url", i), e.URL),
			enum(fmt.Sprintf("evidence[%d].retrieval", i), e.Retrieval, retrievals),
		); err != nil {
			return err
		}
	}
	return nil
}

// parseID checks ^F(1[0-3]|[1-9])\.[0-9]{1,2}$ and returns the family number.
func parseID(id string) (int, bool) {
	fam, sub, ok := strings.Cut(id, ".")
	if !ok || len(fam) < 2 || fam[0] != 'F' || len(sub) < 1 || len(sub) > 2 || !allDigits(sub) {
		return 0, false
	}
	num := fam[1:]
	if !allDigits(num) || num[0] == '0' || len(num) > 2 {
		return 0, false
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 || n > 13 {
		return 0, false
	}
	return n, true
}

func isSemver(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || !allDigits(p) {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isLowerHex64(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func enum(field, v string, allowed map[string]bool) error {
	if !allowed[v] {
		return fmt.Errorf("%s %q is not an allowed value", field, v)
	}
	return nil
}

func optionalEnum(field, v string, allowed map[string]bool) error {
	if v == "" {
		return nil
	}
	return enum(field, v, allowed)
}

func nonEmpty(field, v string) error {
	if v == "" {
		return fmt.Errorf("%s is empty", field)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func set(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

// Schema enums (observex-sre-kb-schema-v0.1.0.json).
var (
	families = func() map[Family]bool {
		m := map[Family]bool{}
		for _, f := range Families() {
			m[f] = true
		}
		return m
	}()
	methodFits = func() map[string]bool {
		m := map[string]bool{}
		for _, v := range MethodFits() {
			m[string(v)] = true
		}
		return m
	}()
	automationClasses = func() map[string]bool {
		m := map[string]bool{}
		for _, v := range AutomationClasses() {
			m[string(v)] = true
		}
		return m
	}()
	triggerClasses       = set("change", "load", "fault", "expiry", "external", "latent-detonation", "spontaneous")
	propagations         = set("contained", "cascading", "metastable", "correlated-fleet")
	observabilityClasses = set("loud", "gray", "silent")
	reversibilities      = set("reversible", "reversible-with-cost", "irreversible")
	recoveryShapes       = set("instant", "drain-bound", "restore-bound", "logistics-bound")
	predictionClasses    = set("P1-deterministic", "P2-statistical-evidenced", "P3-early-detection-only", "P4-not-predictable")
	availabilityRisks    = set("independent", "shares-failure-domain", "inside-blast-radius", "unknown")
	rollbackAvailability = set("yes", "conditional", "no")
	feasibilities        = set("yes-deterministic", "yes-statistical", "detection-only", "no")
	confidences          = set("high", "medium", "low")
	maturities           = set("established", "emerging", "experimental", "unsolved")
	evidenceTiers        = set("E1-first-party", "E2-peer-reviewed", "E3-second-party-press", "E4-mechanism-only", "E5-inference")
	retrievals           = set("fetched", "snippet", "not_retrieved")
)
