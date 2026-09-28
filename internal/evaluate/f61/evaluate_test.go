package f61

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

var (
	t0      = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	subject = wire.Subject{OrgID: "org-a", Namespace: "payments", ServiceID: "synthetic:chk-1", CheckID: "chk-1", Endpoint: "svc.example:443"}
	policy  = Policy{Horizon: DefaultHorizon, Span: SpanFor(5 * time.Minute)}
)

// cert mints a certificate for dnsName valid until notAfter.
func cert(t *testing.T, cn, dnsName string, notAfter time.Time, isCA bool) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: t0.Add(-400 * 24 * time.Hour), NotAfter: notAfter,
		DNSNames: []string{dnsName}, IsCA: isCA, BasicConstraintsValid: isCA,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// observe builds a validated observation the way the processor does: a v1
// wire document decoded against the server's binding.
func observe(t *testing.T, vantage string, at time.Time, chain [][]byte, hostOK bool) tlscert.Observation {
	t.Helper()
	m := map[string]any{"v": 1, "check_id": subject.CheckID, "endpoint": subject.Endpoint, "observed_at": at.Format(time.RFC3339Nano), "duration_ns": 1000}
	if chain == nil {
		m["outcome"], m["detail"] = "timeout", "timeout during dial"
		m["trust"] = map[string]any{"chain_verified": false, "hostname_verified": false, "reason": ""}
	} else {
		reason := tlscert.ReasonUnknownAuthority
		if !hostOK {
			reason += " and " + tlscert.ReasonHostnameMismatch
		}
		m["outcome"] = "observed_untrusted"
		m["trust"] = map[string]any{"chain_verified": false, "hostname_verified": hostOK, "reason": reason}
		m["detail"] = fmt.Sprintf("observed %d certificate(s); verification failed: %s", len(chain), reason)
		m["certificates"] = chain
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	o, err := tlscert.UnmarshalObservation(data, tlscert.Binding{CheckID: subject.CheckID, Endpoint: subject.Endpoint,
		Vantage: tlscert.Vantage{Kind: "synthetic-probe", ID: vantage}})
	if err != nil {
		t.Fatalf("fixture observation: %v", err)
	}
	return o
}

func leafValidUntil(t *testing.T, notAfter time.Time) [][]byte {
	return [][]byte{cert(t, "svc", "svc.example", notAfter, false)}
}

func TestStatusFromHorizon(t *testing.T) {
	cases := []struct {
		name     string
		notAfter time.Time
		want     Status
	}{
		{"well beyond horizon", t0.Add(90 * 24 * time.Hour), StatusOK},
		{"one second beyond horizon", t0.Add(DefaultHorizon + time.Second), StatusOK},
		{"exactly at horizon is inside (inclusive)", t0.Add(DefaultHorizon), StatusExpiring},
		{"inside horizon", t0.Add(10 * 24 * time.Hour), StatusExpiring},
		{"expires this instant", t0, StatusExpired},
		{"already expired", t0.Add(-time.Hour), StatusExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := Evaluate(subject, []tlscert.Observation{observe(t, "vtg_a", t0, leafValidUntil(t, tc.notAfter), true)}, t0, policy)
			if err != nil {
				t.Fatal(err)
			}
			if v.Status != tc.want {
				t.Fatalf("status %q, want %q", v.Status, tc.want)
			}
			if !v.EarliestNotAfter.Equal(tc.notAfter.Truncate(time.Second)) || v.Remaining != v.EarliestNotAfter.Sub(t0) {
				t.Fatalf("earliest %v remaining %v", v.EarliestNotAfter, v.Remaining)
			}
		})
	}
}

func TestIntermediateExpiryCounts(t *testing.T) {
	chain := [][]byte{
		cert(t, "svc", "svc.example", t0.Add(200*24*time.Hour), false),
		cert(t, "intermediate", "ca.example", t0.Add(5*24*time.Hour), true),
	}
	v, err := Evaluate(subject, []tlscert.Observation{observe(t, "vtg_a", t0, chain, true)}, t0, policy)
	if err != nil || v.Status != StatusExpiring || len(v.Vantages[0].Certificates) != 2 {
		t.Fatalf("status %q err %v", v.Status, err)
	}
}

func TestHostnameMismatchAndUnreachable(t *testing.T) {
	mismatch := observe(t, "vtg_a", t0, [][]byte{cert(t, "other", "other.example", t0.Add(5*24*time.Hour), false)}, false)
	v, _ := Evaluate(subject, []tlscert.Observation{mismatch}, t0, policy)
	if v.Status != StatusHostnameMismatch || v.VantagesEvaluated != 0 {
		t.Fatalf("mismatch: %q evaluated %d", v.Status, v.VantagesEvaluated)
	}
	v, _ = Evaluate(subject, []tlscert.Observation{observe(t, "vtg_a", t0, nil, false)}, t0, policy)
	if v.Status != StatusUnreachable || !v.EarliestNotAfter.IsZero() {
		t.Fatalf("unreachable: %q", v.Status)
	}
	v, _ = Evaluate(subject, nil, t0, policy)
	if v.Status != StatusUnknown || v.VantagesInSpan != 0 {
		t.Fatalf("no observations: %q", v.Status)
	}
}

func TestWorstCaseAndDisagreement(t *testing.T) {
	renewed := observe(t, "vtg_a", t0, leafValidUntil(t, t0.Add(90*24*time.Hour)), true)
	oldEdge := observe(t, "vtg_b", t0.Add(-time.Minute), leafValidUntil(t, t0.Add(3*24*time.Hour)), true)
	down := observe(t, "vtg_c", t0, nil, false)
	v, err := Evaluate(subject, []tlscert.Observation{renewed, oldEdge, down}, t0, policy)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusExpiring || !v.Disagreement || v.VantagesInSpan != 3 || v.VantagesEvaluated != 2 {
		t.Fatalf("got %q disagreement %v inspan %d evaluated %d", v.Status, v.Disagreement, v.VantagesInSpan, v.VantagesEvaluated)
	}
	// The same leaf everywhere is agreement.
	chain := leafValidUntil(t, t0.Add(90*24*time.Hour))
	v, _ = Evaluate(subject, []tlscert.Observation{observe(t, "vtg_a", t0, chain, true), observe(t, "vtg_b", t0, chain, true)}, t0, policy)
	if v.Disagreement || v.Status != StatusOK {
		t.Fatalf("agreement: %q %v", v.Status, v.Disagreement)
	}
}

func TestStaleVantageIsExcluded(t *testing.T) {
	fresh := observe(t, "vtg_a", t0, leafValidUntil(t, t0.Add(90*24*time.Hour)), true)
	old := observe(t, "vtg_b", t0.Add(-policy.Span-time.Second), leafValidUntil(t, t0.Add(-time.Hour)), true)
	v, err := Evaluate(subject, []tlscert.Observation{fresh, old}, t0, policy)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusOK || v.VantagesStale != 1 || v.VantagesInSpan != 1 {
		t.Fatalf("got %q stale %d inspan %d", v.Status, v.VantagesStale, v.VantagesInSpan)
	}
	if v.Vantages[1].VantageID != "vtg_b" || v.Vantages[1].Status != StatusStale || v.Vantages[1].InSpan {
		t.Fatalf("stale vantage not reported: %+v", v.Vantages[1])
	}
	// Exactly at the span boundary is still in span.
	edge := observe(t, "vtg_b", t0.Add(-policy.Span), leafValidUntil(t, t0.Add(-time.Hour)), true)
	v, _ = Evaluate(subject, []tlscert.Observation{fresh, edge}, t0, policy)
	if v.Status != StatusExpired || v.VantagesStale != 0 {
		t.Fatalf("boundary: %q stale %d", v.Status, v.VantagesStale)
	}
}

func TestDuplicatesUseNewestAndOrderDoesNotMatter(t *testing.T) {
	older := observe(t, "vtg_a", t0.Add(-2*time.Minute), leafValidUntil(t, t0.Add(-time.Hour)), true)
	newer := observe(t, "vtg_a", t0, leafValidUntil(t, t0.Add(90*24*time.Hour)), true)
	b := observe(t, "vtg_b", t0.Add(-time.Minute), leafValidUntil(t, t0.Add(80*24*time.Hour)), true)
	v1, err := Evaluate(subject, []tlscert.Observation{older, newer, b}, t0, policy)
	if err != nil {
		t.Fatal(err)
	}
	v2, _ := Evaluate(subject, []tlscert.Observation{b, newer, older}, t0, policy)
	if v1.Status != StatusOK || v1.VantagesInSpan != 2 {
		t.Fatalf("got %q with %d vantages", v1.Status, v1.VantagesInSpan)
	}
	if !reflect.DeepEqual(v1, v2) {
		t.Fatal("verdict depends on input order")
	}
}

func TestClockSkew(t *testing.T) {
	ahead := observe(t, "vtg_a", t0.Add(10*time.Second), leafValidUntil(t, t0.Add(90*24*time.Hour)), true)
	v, err := Evaluate(subject, []tlscert.Observation{ahead}, t0, policy)
	if err != nil || v.Status != StatusOK || !v.EvaluatedAt.Equal(t0.Add(10*time.Second)) {
		t.Fatalf("within skew: %q %v %v", v.Status, v.EvaluatedAt, err)
	}
	tooFar := observe(t, "vtg_a", t0.Add(ClockSkew+time.Second), leafValidUntil(t, t0.Add(90*24*time.Hour)), true)
	if _, err := Evaluate(subject, []tlscert.Observation{tooFar}, t0, policy); !errors.Is(err, ErrFutureObserved) {
		t.Fatalf("beyond skew: %v", err)
	}
}

func TestRefusals(t *testing.T) {
	good := observe(t, "vtg_a", t0, leafValidUntil(t, t0.Add(90*24*time.Hour)), true)
	foreign := good
	foreign.CheckID = "chk-2"
	unidentified := good
	unidentified.Vantage = tlscert.Vantage{Kind: "synthetic-probe"}
	cases := []struct {
		name string
		subj wire.Subject
		obs  []tlscert.Observation
		now  time.Time
		p    Policy
		want error
	}{
		{"foreign observation", subject, []tlscert.Observation{foreign}, t0, policy, ErrForeign},
		{"unidentified vantage", subject, []tlscert.Observation{unidentified}, t0, policy, ErrUnidentified},
		{"no time", subject, nil, time.Time{}, policy, ErrNoTime},
		{"incomplete subject", wire.Subject{CheckID: "chk-1"}, nil, t0, policy, ErrNoSubject},
		{"horizon too short", subject, nil, t0, Policy{Horizon: time.Hour, Span: policy.Span}, ErrPolicy},
		{"horizon too long", subject, nil, t0, Policy{Horizon: 2 * MaxHorizon, Span: policy.Span}, ErrPolicy},
		{"no span", subject, nil, t0, Policy{Horizon: DefaultHorizon}, ErrPolicy},
	}
	for _, tc := range cases {
		if _, err := Evaluate(tc.subj, tc.obs, tc.now, tc.p); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestUntrustedChainDoesNotChangeExpiryStatus(t *testing.T) {
	v, _ := Evaluate(subject, []tlscert.Observation{observe(t, "vtg_a", t0, leafValidUntil(t, t0.Add(90*24*time.Hour)), true)}, t0, policy)
	if v.Status != StatusOK || v.Vantages[0].Trusted || v.Vantages[0].TrustReason == "" {
		t.Fatalf("got %+v", v.Vantages[0])
	}
}

func TestSpanFor(t *testing.T) {
	if SpanFor(60*time.Second) != 3*time.Minute || SpanFor(time.Hour) != 2*time.Hour+time.Minute {
		t.Fatal("span formula changed")
	}
	if !StatusExpired.Alerting() || !StatusExpiring.Alerting() || StatusHostnameMismatch.Alerting() || StatusUnreachable.Definitive() {
		t.Fatal("status classes changed")
	}
}
