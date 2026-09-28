package f61

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
	wire "github.com/observex/platform/internal/wire/f61"
)

type recordingReceiver struct {
	calls []struct {
		adm Admission
		obs tlscert.Observation
	}
	err error
}

func (r *recordingReceiver) Receive(_ context.Context, adm Admission, obs tlscert.Observation, _ time.Time) (Receipt, error) {
	r.calls = append(r.calls, struct {
		adm Admission
		obs tlscert.Observation
	}{adm, obs})
	return Receipt{Stored: r.err == nil}, r.err
}

// timeoutPayload is a valid v1 document for chk-a (target api.example.com:443).
func timeoutPayload(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"v": 1, "check_id": "chk-a", "endpoint": "api.example.com:443",
		"observed_at": "2026-09-26T12:00:30Z", "duration_ns": 5000000, "outcome": "timeout",
		"trust":  map[string]any{"chain_verified": false, "hostname_verified": false, "reason": ""},
		"detail": "timeout during dial",
	}
	if edit != nil {
		edit(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func setupIntake(t *testing.T) (probetoken.Key, *fakeLookup, *recordingReceiver, *Intake) {
	t.Helper()
	k, l, a := setup(t)
	l.rows["chk-golden-1"] = CheckRecord{
		Row:  wire.CheckRow{ID: "chk-golden-1", OrgID: "org-a", Namespace: "payments", Target: "observex.test:443"},
		Type: CheckTypeSSL, Enabled: true, Locations: []string{"edge-eu"}, IntervalSec: 300, TimeoutSec: 10,
	}
	r := &recordingReceiver{}
	in, err := NewIntake(a, r)
	if err != nil {
		t.Fatal(err)
	}
	return k, l, r, in
}

func TestAcceptHandsOffServerIdentityOnly(t *testing.T) {
	k, l, r, _ := setupIntake(t)
	// The golden document was observed at 2026-09-23T12:00:00Z; the intake's
	// clock and the credential are placed around that moment.
	goldenAt := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	out, err := k.Issue(probetoken.IssueRequest{OrgID: "org-a", Declared: probetoken.Declaration{NetworkZone: "edge-eu"}}, goldenAt.Add(-time.Hour), bytes.NewReader(bytes.Repeat([]byte{4}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	cred, vid := out.Credential.Reveal(), out.VantageID
	ga, err := New(k, l, &fakeRevocations{}, func() time.Time { return goldenAt.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	in, err := NewIntake(ga, r)
	if err != nil {
		t.Fatal(err)
	}

	golden, err := os.ReadFile("../../observe/tlscert/testdata/observation-v1-observed-untrusted.json")
	if err != nil {
		t.Fatal(err)
	}
	adm, _, err := in.Accept(context.Background(), "Bearer "+cred, "chk-golden-1", golden)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("receiver called %d times", len(r.calls))
	}
	got := r.calls[0]
	wantVantage := tlscert.Vantage{Kind: VantageKind, ID: vid}
	if got.adm != adm || adm.Subject.OrgID != "org-a" || adm.Subject.Namespace != "payments" || adm.Vantage != wantVantage {
		t.Fatalf("unexpected admission %+v", adm)
	}
	if got.obs.Vantage != wantVantage || got.obs.CheckID != "chk-golden-1" || got.obs.Endpoint != "observex.test:443" {
		t.Fatalf("observation identity not taken from the server: %+v", got.obs)
	}
	if leaf, ok := got.obs.Leaf(); !ok || !leaf.NotAfter.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("certificate chain not rebuilt")
	}

	cred2, _ := credential(t, k, "org-a", probetoken.Declaration{})
	_, _, in2 := func() (probetoken.Key, *fakeLookup, *Intake) { k, l, _, in := setupIntake(t); return k, l, in }()
	if _, rc, err := in2.Accept(context.Background(), "Bearer "+cred2, "chk-a", timeoutPayload(t, nil)); err != nil || !rc.Stored {
		t.Fatalf("timeout payload: %v", err)
	}
}

func TestAcceptRefusals(t *testing.T) {
	k, _, _, _ := setupIntake(t)
	credA, _ := credential(t, k, "org-a", probetoken.Declaration{})
	credB, _ := credential(t, k, "org-b", probetoken.Declaration{})
	good := timeoutPayload(t, nil)

	cases := []struct {
		name        string
		header      string
		check       string
		body        []byte
		class       error
		status      int
		reason      string
		lookupCalls int
	}{
		{"oversized, before authentication", "", "chk-a", make([]byte, tlscert.MaxWireBytes+1), ErrPayloadTooLarge, 413, "payload_too_large", 0},
		{"no credential", "", "chk-a", good, ErrUnauthenticated, 401, "unauthenticated", 0},
		{"malformed payload without credential: authentication first", "", "chk-a", []byte("{"), ErrUnauthenticated, 401, "unauthenticated", 0},
		{"malformed payload for another org's check: authorization first", "Bearer " + credB, "chk-a", []byte("{"), ErrForbidden, 403, "forbidden", 1},
		{"other org's check", "Bearer " + credB, "chk-a", good, ErrForbidden, 403, "forbidden", 1},
		{"missing check", "Bearer " + credA, "chk-missing", good, ErrForbidden, 403, "forbidden", 1},
		{"bad check id", "Bearer " + credA, "chk a", good, ErrInvalidRequest, 400, "invalid_check_id", 0},
		{"payload names another check", "Bearer " + credA, "chk-a",
			timeoutPayload(t, func(m map[string]any) { m["check_id"] = "chk-b" }), ErrConflict, 409, "observation_check_mismatch", 1},
		{"payload names a stale endpoint", "Bearer " + credA, "chk-a",
			timeoutPayload(t, func(m map[string]any) { m["endpoint"] = "old.example.com:443" }), ErrConflict, 409, "observation_check_mismatch", 1},
		{"payload carries a vantage", "Bearer " + credA, "chk-a",
			timeoutPayload(t, func(m map[string]any) { m["vantage"] = "external-eu" }), ErrInvalidRequest, 400, "malformed_observation", 1},
		{"payload carries an org", "Bearer " + credA, "chk-a",
			timeoutPayload(t, func(m map[string]any) { m["org_id"] = "org-b" }), ErrInvalidRequest, 400, "malformed_observation", 1},
		{"future wire version", "Bearer " + credA, "chk-a",
			timeoutPayload(t, func(m map[string]any) { m["v"] = 2 }), ErrInvalidRequest, 400, "unsupported_wire_version", 1},
		{"inconsistent observation", "Bearer " + credA, "chk-a",
			timeoutPayload(t, func(m map[string]any) { m["detail"] = "free text" }), ErrInvalidRequest, 400, "inconsistent_observation", 1},
		{"not json", "Bearer " + credA, "chk-a", []byte("hello"), ErrInvalidRequest, 400, "malformed_observation", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, l, r, in := setupIntake(t)
			adm, _, err := in.Accept(context.Background(), tc.header, tc.check, tc.body)
			if !errors.Is(err, tc.class) {
				t.Fatalf("got %v, want class %v", err, tc.class)
			}
			if StatusCode(err) != tc.status || Reason(err) != tc.reason {
				t.Fatalf("status %d reason %q, want %d %q", StatusCode(err), Reason(err), tc.status, tc.reason)
			}
			if adm != (Admission{}) || len(r.calls) != 0 {
				t.Fatal("a refusal must admit nothing and must not reach the receiver")
			}
			if l.calls != tc.lookupCalls {
				t.Fatalf("lookup called %d times, want %d", l.calls, tc.lookupCalls)
			}
			if strings.Contains(err.Error(), strings.TrimPrefix(credA, probetoken.TokenPrefix)[:16]) {
				t.Fatal("error text contains part of the credential")
			}
		})
	}
}

func TestAcceptReceiverFailureIsUnavailable(t *testing.T) {
	k, _, r, in := setupIntake(t)
	r.err = errors.New("downstream full")
	cred, _ := credential(t, k, "org-a", probetoken.Declaration{})
	_, _, err := in.Accept(context.Background(), "Bearer "+cred, "chk-a", timeoutPayload(t, nil))
	if !errors.Is(err, ErrUnavailable) || StatusCode(err) != 503 || Reason(err) != "unavailable" {
		t.Fatalf("got %v", err)
	}
}

func TestNewIntakeRequiresBoth(t *testing.T) {
	_, _, a := setup(t)
	if _, err := NewIntake(nil, &recordingReceiver{}); err == nil {
		t.Fatal("nil authorizer accepted")
	}
	if _, err := NewIntake(a, nil); err == nil {
		t.Fatal("nil receiver accepted")
	}
}

func TestReasonCodes(t *testing.T) {
	if Reason(nil) != "" || Reason(errors.New("x")) != "internal_error" {
		t.Fatal("unexpected reason for nil or unclassified errors")
	}
	// The two forbidden causes produce the same code.
	if Reason(refuse(ErrForbidden, ErrOrgMismatch)) != Reason(refuse(ErrForbidden, ErrCheckNotFound)) {
		t.Fatal("forbidden causes must be indistinguishable")
	}
}
