package f61

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
	wire "github.com/observex/platform/internal/wire/f61"
)

const testKey = "intake-test-key-0123456789abcdef0123456789"

var t0 = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

type fakeLookup struct {
	rows  map[string]CheckRecord
	err   error
	calls int
}

func (f *fakeLookup) LookupCheck(_ context.Context, id string) (CheckRecord, error) {
	f.calls++
	if f.err != nil {
		return CheckRecord{}, f.err
	}
	r, ok := f.rows[id]
	if !ok {
		return CheckRecord{}, fmt.Errorf("lookup %q: %w", id, ErrCheckNotFound)
	}
	return r, nil
}

func sslRow(id, org, ns string) CheckRecord {
	return CheckRecord{
		Row:     wire.CheckRow{ID: id, OrgID: org, Namespace: ns, Target: "api.example.com:443"},
		Type:    CheckTypeSSL,
		Enabled: true, Locations: []string{LocationAll}, IntervalSec: 300, TimeoutSec: 10,
	}
}

// fakeRevocations holds revocation bounds keyed by org + "/" + vantage.
type fakeRevocations struct {
	bounds map[string]time.Time
	err    error
	calls  int
}

func (f *fakeRevocations) RevokedBefore(_ context.Context, org, vantage string) (time.Time, bool, error) {
	f.calls++
	if f.err != nil {
		return time.Time{}, false, f.err
	}
	b, ok := f.bounds[org+"/"+vantage]
	return b, ok, nil
}

func setup(t *testing.T) (probetoken.Key, *fakeLookup, *Authorizer) {
	t.Helper()
	k := probetoken.NewKey(testKey)
	l := &fakeLookup{rows: map[string]CheckRecord{
		"chk-a":          sslRow("chk-a", "org-a", "payments"),
		"chk-b":          sslRow("chk-b", "org-b", "default"),
		"chk-http":       {Row: wire.CheckRow{ID: "chk-http", OrgID: "org-a", Namespace: "default", Target: "https://x"}, Type: "http", Enabled: true, Locations: []string{LocationAll}, IntervalSec: 300, TimeoutSec: 10},
		"chk-off":        {Row: wire.CheckRow{ID: "chk-off", OrgID: "org-a", Namespace: "default", Target: "x:443"}, Type: CheckTypeSSL, Enabled: false, Locations: []string{LocationAll}, IntervalSec: 300, TimeoutSec: 10},
		"chk-no-org":     sslRow("chk-no-org", "", "default"),
		"chk-no-ns":      sslRow("chk-no-ns", "org-a", "  "),
		"chk-mismatched": sslRow("chk-other", "org-a", "default"),
	}}
	a, err := New(k, l, &fakeRevocations{}, func() time.Time { return t0.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	return k, l, a
}

func credential(t *testing.T, k probetoken.Key, org string, decl probetoken.Declaration) (string, string) {
	t.Helper()
	out, err := k.Issue(probetoken.IssueRequest{OrgID: org, Declared: decl}, t0, bytes.NewReader(bytes.Repeat([]byte{4}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return out.Credential.Reveal(), out.VantageID
}

func TestAuthorizeAdmitsSubjectFromRowAndVantageFromCredential(t *testing.T) {
	k, _, a := setup(t)
	decl := probetoken.Declaration{NetworkZone: "edge-eu", ClusterName: "probe-eu-1", Environment: "production", HostGroup: "synthetic"}
	cred, vid := credential(t, k, "org-a", decl)

	got, err := a.Authorize(context.Background(), "Bearer "+cred, "chk-a")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	want := Admission{
		Subject:             wire.Subject{OrgID: "org-a", Namespace: "payments", ServiceID: "synthetic:chk-a", CheckID: "chk-a", Endpoint: "api.example.com:443"},
		Vantage:             tlscert.Vantage{Kind: VantageKind, ID: vid},
		Declared:            decl,
		CredentialExpiresAt: t0.Add(probetoken.DefaultTTL),
		IntervalSec:         300,
		TimeoutSec:          10,
	}
	if got != want {
		t.Fatalf("admission mismatch:\n got %+v\nwant %+v", got, want)
	}
	// The scheme is case-insensitive.
	if _, err := a.Authorize(context.Background(), "bearer "+cred, "chk-a"); err != nil {
		t.Fatalf("lower-case scheme: %v", err)
	}
}

func TestAuthorizeRefusals(t *testing.T) {
	k, _, _ := setup(t)
	credA, _ := credential(t, k, "org-a", probetoken.Declaration{})
	otherKey := probetoken.NewKey(strings.Repeat("z", 40))
	credOther, _ := credential(t, otherKey, "org-a", probetoken.Declaration{})

	cases := []struct {
		name        string
		header      string
		check       string
		class       error
		detail      error
		status      int
		lookupCalls int
	}{
		{"no header", "", "chk-a", ErrUnauthenticated, ErrNoCredential, 401, 0},
		{"basic scheme", "Basic " + credA, "chk-a", ErrUnauthenticated, ErrNoCredential, 401, 0},
		{"scheme only", "Bearer ", "chk-a", ErrUnauthenticated, ErrNoCredential, 401, 0},
		{"two spaces", "Bearer  " + credA, "chk-a", ErrUnauthenticated, ErrNoCredential, 401, 0},
		{"trailing data", "Bearer " + credA + " extra", "chk-a", ErrUnauthenticated, ErrNoCredential, 401, 0},
		{"bare credential", credA, "chk-a", ErrUnauthenticated, ErrNoCredential, 401, 0},
		{"wrong key", "Bearer " + credOther, "chk-a", ErrUnauthenticated, probetoken.ErrSignature, 401, 0},
		{"garbage", "Bearer oxpt_x.y.z", "chk-a", ErrUnauthenticated, probetoken.ErrMalformed, 401, 0},
		{"blank check id", "Bearer " + credA, "", ErrInvalidRequest, ErrBadCheckID, 400, 0},
		{"space in check id", "Bearer " + credA, "chk a", ErrInvalidRequest, ErrBadCheckID, 400, 0},
		{"long check id", "Bearer " + credA, strings.Repeat("c", MaxCheckIDLength+1), ErrInvalidRequest, ErrBadCheckID, 400, 0},
		{"unknown check", "Bearer " + credA, "chk-missing", ErrForbidden, ErrCheckNotFound, 403, 1},
		{"other org's check", "Bearer " + credA, "chk-b", ErrForbidden, ErrOrgMismatch, 403, 1},
		{"row without org", "Bearer " + credA, "chk-no-org", ErrForbidden, ErrTenancyUnresolved, 403, 1},
		{"row without namespace", "Bearer " + credA, "chk-no-ns", ErrForbidden, wire.ErrMissingNamespace, 403, 1},
		{"not an ssl check", "Bearer " + credA, "chk-http", ErrForbidden, ErrNotF61Check, 403, 1},
		{"disabled check", "Bearer " + credA, "chk-off", ErrForbidden, ErrCheckDisabled, 403, 1},
		{"lookup returned another row", "Bearer " + credA, "chk-mismatched", ErrUnavailable, ErrLookupMismatch, 503, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, l, a := setup(t)
			got, err := a.Authorize(context.Background(), tc.header, tc.check)
			if !errors.Is(err, tc.class) || !errors.Is(err, tc.detail) {
				t.Fatalf("got %v, want class %v detail %v", err, tc.class, tc.detail)
			}
			if got != (Admission{}) {
				t.Fatal("a refusal must return the zero Admission")
			}
			if StatusCode(err) != tc.status {
				t.Fatalf("status %d, want %d", StatusCode(err), tc.status)
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

// A missing check and another organization's check are indistinguishable to
// the caller.
func TestNotFoundAndForeignCheckLookTheSame(t *testing.T) {
	k, _, a := setup(t)
	cred, _ := credential(t, k, "org-a", probetoken.Declaration{})
	_, e1 := a.Authorize(context.Background(), "Bearer "+cred, "chk-missing")
	_, e2 := a.Authorize(context.Background(), "Bearer "+cred, "chk-b")
	if StatusCode(e1) != StatusCode(e2) || !errors.Is(e1, ErrForbidden) || !errors.Is(e2, ErrForbidden) {
		t.Fatal("not-found and foreign-org checks must share one class and status")
	}
}

func TestLookupFailureFailsClosed(t *testing.T) {
	k, l, a := setup(t)
	l.err = errors.New("connection refused")
	cred, _ := credential(t, k, "org-a", probetoken.Declaration{})
	_, err := a.Authorize(context.Background(), "Bearer "+cred, "chk-a")
	if !errors.Is(err, ErrUnavailable) || StatusCode(err) != 503 {
		t.Fatalf("got %v", err)
	}
}

// An agent install token, whether presented as issued by the gateway or
// re-wrapped with the probe prefix and signed with the probe key, never
// carries the dedicated scope's authority here.
func TestAgentInstallTokensAreRefused(t *testing.T) {
	_, l, a := setup(t)
	agentClaims := jwt.MapClaims{
		"typ":    "observex_agent_install",
		"org_id": "org-a",
		"scopes": []string{"agent:write", "agent:update", "metrics:write", "logs:write", "traces:write", "topology:write", "security:write"},
		"iat":    t0.Unix(),
		"exp":    t0.Add(time.Hour).Unix(),
		"jti":    "00000000-0000-0000-0000-000000000000",
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, agentClaims).SignedString([]byte(testKey))
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"Bearer oxat_" + signed, "Bearer " + probetoken.TokenPrefix + signed, "Bearer " + signed} {
		_, err := a.Authorize(context.Background(), h, "chk-a")
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("agent token was not refused: %v", err)
		}
	}
	if l.calls != 0 {
		t.Fatal("no check row may be read for an unauthenticated caller")
	}
}

func TestExpiredCredentialIsRefused(t *testing.T) {
	k, l, _ := setup(t)
	cred, _ := credential(t, k, "org-a", probetoken.Declaration{})
	a, err := New(k, l, &fakeRevocations{}, func() time.Time { return t0.Add(probetoken.DefaultTTL + time.Hour) })
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Authorize(context.Background(), "Bearer "+cred, "chk-a")
	if !errors.Is(err, ErrUnauthenticated) || !errors.Is(err, probetoken.ErrExpired) {
		t.Fatalf("got %v", err)
	}
}

func TestNewRefusesMissingDependencies(t *testing.T) {
	k := probetoken.NewKey(testKey)
	l := &fakeLookup{}
	now := func() time.Time { return t0 }
	rev := &fakeRevocations{}
	if _, err := New(k, l, nil, now); err == nil {
		t.Fatal("nil revocation checker accepted")
	}
	if _, err := New(nil, l, rev, now); err == nil {
		t.Fatal("nil verifier accepted")
	}
	if _, err := New(k, nil, rev, now); err == nil {
		t.Fatal("nil lookup accepted")
	}
	if _, err := New(k, l, rev, nil); err == nil {
		t.Fatal("nil clock accepted")
	}
	if _, err := New(probetoken.Key{}, l, rev, now); !errors.Is(err, probetoken.ErrKeyNotConfigured) {
		t.Fatalf("unconfigured key: %v", err)
	}
}

func TestStatusCodeDefault(t *testing.T) {
	if StatusCode(errors.New("other")) != 500 || StatusCode(nil) != 500 {
		t.Fatal("unclassified errors must map to 500")
	}
}

// Authorize accepts a context, the Authorization header value and a check ID,
// and nothing else: there is no parameter through which an organization,
// namespace or vantage could be supplied by the caller.
func TestAuthorizeSignatureAdmitsNoCallerIdentity(t *testing.T) {
	m, ok := reflect.TypeOf(&Authorizer{}).MethodByName("Authorize")
	if !ok {
		t.Fatal("Authorize not found")
	}
	ft := m.Type
	want := []reflect.Type{reflect.TypeOf(&Authorizer{}), reflect.TypeOf((*context.Context)(nil)).Elem(), reflect.TypeOf(""), reflect.TypeOf("")}
	if ft.NumIn() != len(want) {
		t.Fatalf("Authorize has %d inputs, want %d", ft.NumIn(), len(want))
	}
	for i, w := range want {
		if ft.In(i) != w {
			t.Fatalf("input %d is %v, want %v", i, ft.In(i), w)
		}
	}
	rt := reflect.TypeOf(CheckRecord{})
	for i := 0; i < rt.NumField(); i++ {
		switch n := rt.Field(i).Name; n {
		case "Row", "Type", "Enabled", "Locations", "IntervalSec", "TimeoutSec":
		default:
			t.Fatalf("CheckRecord gained field %q", n)
		}
	}
}
