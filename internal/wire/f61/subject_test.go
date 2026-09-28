package f61

import (
	"errors"
	"reflect"
	"testing"
)

// serverRow is a representative server-side synthetic check row.
func serverRow() CheckRow {
	return CheckRow{
		OrgID:     "org-7f3c",
		Namespace: "payments",
		ID:        "chk-91ab",
		Target:    "https://api.example.com:443",
	}
}

// 1. All five fields are populated, and every one of them comes from the
// server-side check row.
func TestAllFiveFieldsComeFromTheCheckRow(t *testing.T) {
	row := serverRow()
	got, err := SubjectFromCheck(row)
	if err != nil {
		t.Fatalf("SubjectFromCheck returned error: %v", err)
	}
	want := Subject{
		OrgID:     "org-7f3c",
		Namespace: "payments",
		ServiceID: "synthetic:chk-91ab",
		CheckID:   "chk-91ab",
		Endpoint:  "https://api.example.com:443",
	}
	if got != want {
		t.Errorf("subject = %+v, want %+v", got, want)
	}
	if got.OrgID != row.OrgID || got.Namespace != row.Namespace ||
		got.CheckID != row.ID || got.Endpoint != row.Target {
		t.Errorf("subject fields do not match the row they were built from: %+v vs %+v", got, row)
	}
}

// Values are copied byte for byte: no trimming, no case folding, no rewriting
// of a tenant identifier.
func TestRowValuesAreCopiedVerbatim(t *testing.T) {
	row := CheckRow{
		OrgID:     "Org-MiXeD",
		Namespace: "Team_A/sub",
		ID:        "CHK-Upper",
		Target:    "HTTPS://Example.COM:8443/Path",
	}
	got, err := SubjectFromCheck(row)
	if err != nil {
		t.Fatalf("SubjectFromCheck returned error: %v", err)
	}
	if got.OrgID != row.OrgID {
		t.Errorf("OrgID rewritten: %q -> %q", row.OrgID, got.OrgID)
	}
	if got.Namespace != row.Namespace {
		t.Errorf("Namespace rewritten: %q -> %q", row.Namespace, got.Namespace)
	}
	if got.CheckID != row.ID {
		t.Errorf("CheckID rewritten: %q -> %q", row.ID, got.CheckID)
	}
	if got.Endpoint != row.Target {
		t.Errorf("Endpoint rewritten: %q -> %q", row.Target, got.Endpoint)
	}
}

// 2. ServiceID is exactly "synthetic:" + CheckID, for every check ID.
func TestServiceIDIsPrefixPlusCheckID(t *testing.T) {
	if ServicePrefix != "synthetic:" {
		t.Fatalf("ServicePrefix = %q, want %q", ServicePrefix, "synthetic:")
	}
	ids := []string{"chk-91ab", "a", "0", "chk:with:colons", "chk with space", "синтетика", "chk-" + string(rune(0x1F600))}
	for _, id := range ids {
		row := serverRow()
		row.ID = id
		got, err := SubjectFromCheck(row)
		if err != nil {
			t.Fatalf("id %q: SubjectFromCheck returned error: %v", id, err)
		}
		if want := "synthetic:" + id; got.ServiceID != want {
			t.Errorf("id %q: ServiceID = %q, want %q", id, got.ServiceID, want)
		}
		if got.ServiceID != ServicePrefix+got.CheckID {
			t.Errorf("id %q: ServiceID %q is not ServicePrefix+CheckID %q", id, got.ServiceID, ServicePrefix+got.CheckID)
		}
	}
}

// 3. OrgID cannot come from any alternate source: the only parameter that can
// carry it is the check row, so changing the row is the only way to change the
// resulting OrgID.
func TestOrgIDHasExactlyOneSource(t *testing.T) {
	// There is one input, and it is a CheckRow.
	fn := reflect.TypeOf(SubjectFromCheck)
	if fn.NumIn() != 1 {
		t.Fatalf("SubjectFromCheck takes %d parameters; want exactly 1 (the check row)", fn.NumIn())
	}
	if fn.In(0) != reflect.TypeOf(CheckRow{}) {
		t.Fatalf("SubjectFromCheck parameter is %s; want f61.CheckRow", fn.In(0))
	}
	if fn.IsVariadic() {
		t.Fatal("SubjectFromCheck is variadic; identity must have a fixed single source")
	}

	// And the org in the subject tracks the org in the row, one to one.
	for _, org := range []string{"org-a", "org-b", "org-c"} {
		row := serverRow()
		row.OrgID = org
		got, err := SubjectFromCheck(row)
		if err != nil {
			t.Fatalf("org %q: %v", org, err)
		}
		if got.OrgID != org {
			t.Errorf("org %q: subject OrgID = %q", org, got.OrgID)
		}
	}
}

// 4-7. Each missing row field fails closed. 8. No partial subject is returned.
func TestMissingFieldsFailClosed(t *testing.T) {
	blanks := []string{"", " ", "\t", "\n", "   \t\n "}
	cases := []struct {
		name  string
		clear func(*CheckRow, string)
		want  error
	}{
		{"org_id", func(r *CheckRow, b string) { r.OrgID = b }, ErrMissingOrgID},
		{"namespace", func(r *CheckRow, b string) { r.Namespace = b }, ErrMissingNamespace},
		{"id", func(r *CheckRow, b string) { r.ID = b }, ErrMissingCheckID},
		{"target", func(r *CheckRow, b string) { r.Target = b }, ErrMissingEndpoint},
	}
	for _, tc := range cases {
		for _, b := range blanks {
			row := serverRow()
			tc.clear(&row, b)
			got, err := SubjectFromCheck(row)
			if err == nil {
				t.Errorf("%s=%q: expected an error, got subject %+v", tc.name, b, got)
				continue
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("%s=%q: error %v does not match %v", tc.name, b, err, tc.want)
			}
			// No partial subject: every field is the zero value.
			if got != (Subject{}) {
				t.Errorf("%s=%q: returned a partial subject %+v; want the zero Subject", tc.name, b, got)
			}
		}
	}
}

// 8 (continued). An entirely empty row yields no subject and reports every
// missing field rather than the first one.
func TestEmptyRowReportsEveryMissingField(t *testing.T) {
	got, err := SubjectFromCheck(CheckRow{})
	if err == nil {
		t.Fatalf("expected an error for an empty row, got subject %+v", got)
	}
	if got != (Subject{}) {
		t.Errorf("returned a partial subject %+v; want the zero Subject", got)
	}
	for _, want := range []error{ErrMissingOrgID, ErrMissingNamespace, ErrMissingCheckID, ErrMissingEndpoint} {
		if !errors.Is(err, want) {
			t.Errorf("error %v does not report %v", err, want)
		}
	}
}

// A missing org is never substituted by a default, a placeholder or a fallback
// scope.
func TestNoDefaultOrgOrNamespaceIsEverInvented(t *testing.T) {
	row := serverRow()
	row.OrgID = ""
	if got, err := SubjectFromCheck(row); err == nil {
		t.Fatalf("missing org_id produced a subject %+v", got)
	}
	row = serverRow()
	row.Namespace = ""
	got, err := SubjectFromCheck(row)
	if err == nil {
		t.Fatalf("missing namespace produced a subject %+v", got)
	}
	if got.Namespace == "default" || got.Namespace != "" {
		t.Errorf("namespace was defaulted to %q", got.Namespace)
	}
}

// 9. No caller, authentication, header, environment or process value can
// override the identity carried by the check row.
func TestEnvironmentAndProcessStateCannotOverrideRowIdentity(t *testing.T) {
	for _, key := range []string{
		"ORG_ID", "ORGANIZATION_ID", "OBSERVEX_ORG_ID", "OBSERVEX_ORGANIZATION",
		"NAMESPACE", "OBSERVEX_NAMESPACE", "POD_NAMESPACE", "TENANT", "TENANT_ID",
		"CHECK_ID", "SERVICE_ID", "ENDPOINT", "TARGET", "X_ORG_ID",
	} {
		t.Setenv(key, "attacker-org")
	}
	row := serverRow()
	got, err := SubjectFromCheck(row)
	if err != nil {
		t.Fatalf("SubjectFromCheck returned error: %v", err)
	}
	if got.OrgID != row.OrgID || got.Namespace != row.Namespace {
		t.Fatalf("environment influenced the subject: %+v", got)
	}
	for _, f := range []string{got.OrgID, got.Namespace, got.ServiceID, got.CheckID, got.Endpoint} {
		if f == "attacker-org" {
			t.Fatalf("an environment value reached the subject: %+v", got)
		}
	}

	// A blank row is not rescued by the environment either: it still fails closed.
	if s, err := SubjectFromCheck(CheckRow{}); err == nil {
		t.Fatalf("empty row produced a subject while the environment was populated: %+v", s)
	}
}

// 9 (continued). The row is the whole input: the function is pure, and calling
// it repeatedly with the same row always yields the same subject. It also does
// not mutate the row it was given.
func TestDeterministicAndNonMutating(t *testing.T) {
	row := serverRow()
	before := row
	first, err := SubjectFromCheck(row)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		got, err := SubjectFromCheck(row)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if got != first {
			t.Fatalf("call %d returned %+v, first call returned %+v", i, got, first)
		}
	}
	if row != before {
		t.Errorf("SubjectFromCheck mutated its input: %+v -> %+v", before, row)
	}
}

// 10. The envelope contains exactly the five intended fields, all strings, and
// nothing else.
func TestSubjectHasExactlyTheFiveIntendedFields(t *testing.T) {
	want := []string{"OrgID", "Namespace", "ServiceID", "CheckID", "Endpoint"}
	st := reflect.TypeOf(Subject{})
	if st.NumField() != len(want) {
		var got []string
		for i := 0; i < st.NumField(); i++ {
			got = append(got, st.Field(i).Name)
		}
		t.Fatalf("Subject has fields %v; want exactly %v", got, want)
	}
	for i, name := range want {
		f := st.Field(i)
		if f.Name != name {
			t.Errorf("field %d is %q, want %q", i, f.Name, name)
		}
		if f.Type.Kind() != reflect.String {
			t.Errorf("field %q has type %s, want string", f.Name, f.Type)
		}
	}
	// Nothing that belongs to another layer has been smuggled in.
	for _, banned := range []string{
		"Role", "Roles", "Permission", "Permissions", "Auth", "AuthContext", "Token", "Claims",
		"Vantage", "VantagePoint", "Certificate", "Cert", "NotAfter", "Timestamp", "ObservedAt",
		"Time", "Severity", "Confidence", "Remediation", "Policy", "State", "Status", "Execution",
	} {
		if _, ok := st.FieldByName(banned); ok {
			t.Errorf("Subject carries %q, which belongs to another layer", banned)
		}
	}
	// The row that feeds it carries exactly the four server-side columns.
	rt := reflect.TypeOf(CheckRow{})
	wantRow := []string{"OrgID", "Namespace", "ID", "Target"}
	if rt.NumField() != len(wantRow) {
		t.Fatalf("CheckRow has %d fields; want exactly %v", rt.NumField(), wantRow)
	}
	for i, name := range wantRow {
		if f := rt.Field(i); f.Name != name || f.Type.Kind() != reflect.String {
			t.Errorf("CheckRow field %d is %s %s, want %s string", i, f.Name, f.Type, name)
		}
	}
}
