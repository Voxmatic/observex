package probetoken

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var t0 = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func issue(t *testing.T, k Key, req IssueRequest) Issued {
	t.Helper()
	out, err := k.Issue(req, t0, fixedRandom(3))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return out
}

// sign builds a credential with arbitrary claims and method, signed with the
// derived signing key unless key is given. It lets tests present values the
// issuer never produces.
func sign(t *testing.T, method jwt.SigningMethod, key any, c claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, c).SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return TokenPrefix + s
}

func validClaims(t *testing.T, k Key, org string) claims {
	t.Helper()
	id, err := k.NewVantageID(org, fixedRandom(5))
	if err != nil {
		t.Fatal(err)
	}
	return claims{
		Type: TokenType, OrgID: org, VantageID: id, Scopes: []string{Scope},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(t0),
			ExpiresAt: jwt.NewNumericDate(t0.Add(DefaultTTL)),
		},
	}
}

func payloadKeys(t *testing.T, cred string) []string {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(cred, TokenPrefix), ".")
	if len(parts) != 3 {
		t.Fatalf("credential is not a compact JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestIssueVerifyRoundTrip(t *testing.T) {
	k := NewKey(testKeyValue)
	decl := Declaration{NetworkZone: "eu-west-edge", ClusterName: "probe-eu-1", Environment: "production", HostGroup: "synthetic"}
	out := issue(t, k, IssueRequest{OrgID: "org-a", Declared: decl})
	if out.Rotated || out.OrgID != "org-a" || !k.VantageIDBoundTo(out.VantageID, "org-a") {
		t.Fatalf("unexpected issue result: rotated=%v org=%q", out.Rotated, out.OrgID)
	}
	if !out.IssuedAt.Equal(t0) || !out.ExpiresAt.Equal(t0.Add(DefaultTTL)) {
		t.Fatal("unexpected lifetime")
	}
	cred := out.Credential.Reveal()
	if !strings.HasPrefix(cred, TokenPrefix) {
		t.Fatal("missing prefix")
	}
	p, err := k.Verify(cred, t0.Add(time.Minute))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	want := Principal{OrgID: "org-a", VantageID: out.VantageID, Declared: decl, IssuedAt: t0, ExpiresAt: t0.Add(DefaultTTL)}
	if p != want {
		t.Fatalf("principal mismatch:\n got %+v\nwant %+v", p, want)
	}
}

func TestIssuedPayloadHasNoSubOrJTIAndExactlyOneScope(t *testing.T) {
	k := NewKey(testKeyValue)
	out := issue(t, k, IssueRequest{OrgID: "org-a"})
	got := payloadKeys(t, out.Credential.Reveal())
	want := []string{"exp", "iat", "org_id", "scopes", "typ", "vantage_id"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("payload keys = %v, want %v", got, want)
	}
}

func TestRotationKeepsVantageAndRefusesForeignIDs(t *testing.T) {
	k := NewKey(testKeyValue)
	first := issue(t, k, IssueRequest{OrgID: "org-a"})
	second, err := k.Issue(IssueRequest{OrgID: "org-a", VantageID: first.VantageID}, t0.Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if !second.Rotated || second.VantageID != first.VantageID {
		t.Fatal("rotation must keep the vantage id")
	}
	if second.Credential.Reveal() == first.Credential.Reveal() {
		t.Fatal("rotation must issue a new credential")
	}
	// An ID minted for org-a cannot be re-issued under org-b.
	if _, err := k.Issue(IssueRequest{OrgID: "org-b", VantageID: first.VantageID}, t0, nil); !errors.Is(err, ErrVantageID) {
		t.Fatalf("foreign vantage id: %v", err)
	}
	// A caller-chosen string is not a vantage ID.
	if _, err := k.Issue(IssueRequest{OrgID: "org-a", VantageID: "my-probe-1"}, t0, nil); !errors.Is(err, ErrVantageID) {
		t.Fatalf("caller-chosen id: %v", err)
	}
}

func TestIssueFailsClosed(t *testing.T) {
	k := NewKey(testKeyValue)
	cases := []struct {
		name string
		key  Key
		req  IssueRequest
		now  time.Time
		want error
	}{
		{"no key", Key{}, IssueRequest{OrgID: "org-a"}, t0, ErrKeyNotConfigured},
		{"no clock", k, IssueRequest{OrgID: "org-a"}, time.Time{}, ErrNoClock},
		{"blank org", k, IssueRequest{OrgID: " "}, t0, ErrOrg},
		{"control char org", k, IssueRequest{OrgID: "org\x00a"}, t0, ErrOrg},
		{"long org", k, IssueRequest{OrgID: strings.Repeat("o", MaxOrgIDLength+1)}, t0, ErrOrg},
		{"ttl too short", k, IssueRequest{OrgID: "org-a", TTL: time.Minute}, t0, ErrLifetime},
		{"ttl too long", k, IssueRequest{OrgID: "org-a", TTL: MaxTTL + time.Hour}, t0, ErrLifetime},
		{"negative ttl", k, IssueRequest{OrgID: "org-a", TTL: -time.Hour}, t0, ErrLifetime},
		{"control char declaration", k, IssueRequest{OrgID: "org-a", Declared: Declaration{NetworkZone: "a\nb"}}, t0, ErrDeclaration},
		{"long declaration", k, IssueRequest{OrgID: "org-a", Declared: Declaration{HostGroup: strings.Repeat("h", MaxDeclarationLength+1)}}, t0, ErrDeclaration},
		{"invalid utf8 declaration", k, IssueRequest{OrgID: "org-a", Declared: Declaration{Environment: "\xff"}}, t0, ErrDeclaration},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.key.Issue(tc.req, tc.now, fixedRandom(1))
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if out.Credential.Reveal() != "" || out.VantageID != "" {
				t.Fatal("a failed issue must return nothing")
			}
		})
	}
	if _, err := k.Issue(IssueRequest{OrgID: "org-a", TTL: MaxTTL}, t0, fixedRandom(1)); err != nil {
		t.Fatalf("MaxTTL must be accepted: %v", err)
	}
}

func TestVerifyFailsClosed(t *testing.T) {
	k := NewKey(testKeyValue)
	good := validClaims(t, k, "org-a")
	mut := func(f func(*claims)) claims {
		c := good
		c.Scopes = append([]string(nil), good.Scopes...)
		f(&c)
		return c
	}
	hs := jwt.SigningMethodHS256
	cases := []struct {
		name string
		cred string
		now  time.Time
		want error
	}{
		{"valid control", sign(t, hs, k.m.sign, good), t0, nil},
		{"empty", "", t0, ErrMalformed},
		{"prefix only", TokenPrefix, t0, ErrMalformed},
		{"no prefix", strings.TrimPrefix(sign(t, hs, k.m.sign, good), TokenPrefix), t0, ErrMalformed},
		{"agent prefix", "oxat_" + strings.TrimPrefix(sign(t, hs, k.m.sign, good), TokenPrefix), t0, ErrMalformed},
		{"too long", TokenPrefix + strings.Repeat("a", MaxTokenLength), t0, ErrMalformed},
		{"garbage", TokenPrefix + "not.a.jwt", t0, ErrMalformed},
		{"signed with raw key", sign(t, hs, []byte(testKeyValue), good), t0, ErrSignature},
		{"signed with other key", sign(t, hs, NewKey(otherKeyValue).m.sign, good), t0, ErrSignature},
		{"HS384", sign(t, jwt.SigningMethodHS384, k.m.sign, good), t0, ErrSignature},
		{"HS512", sign(t, jwt.SigningMethodHS512, k.m.sign, good), t0, ErrSignature},
		{"alg none", sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, good), t0, ErrSignature},
		{"agent install typ", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Type = "observex_agent_install" })), t0, ErrWrongType},
		{"missing typ", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Type = "" })), t0, ErrWrongType},
		{"no scopes", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Scopes = nil })), t0, ErrScope},
		{"wildcard scope", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Scopes = []string{"*"} })), t0, ErrScope},
		{"telemetry scope", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Scopes = []string{"metrics:write"} })), t0, ErrScope},
		{"extra scope", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Scopes = []string{Scope, "metrics:write"} })), t0, ErrScope},
		{"duplicate scope", sign(t, hs, k.m.sign, mut(func(c *claims) { c.Scopes = []string{Scope, Scope} })), t0, ErrScope},
		{"missing org", sign(t, hs, k.m.sign, mut(func(c *claims) { c.OrgID = "" })), t0, ErrOrg},
		{"blank org", sign(t, hs, k.m.sign, mut(func(c *claims) { c.OrgID = "  " })), t0, ErrOrg},
		{"org changed from binding", sign(t, hs, k.m.sign, mut(func(c *claims) { c.OrgID = "org-b" })), t0, ErrVantageID},
		{"missing vantage", sign(t, hs, k.m.sign, mut(func(c *claims) { c.VantageID = "" })), t0, ErrVantageID},
		{"self-chosen vantage", sign(t, hs, k.m.sign, mut(func(c *claims) { c.VantageID = "agent-1234" })), t0, ErrVantageID},
		{"bad declaration", sign(t, hs, k.m.sign, mut(func(c *claims) { c.ClusterName = "x\x1by" })), t0, ErrDeclaration},
		{"expired", sign(t, hs, k.m.sign, good), t0.Add(DefaultTTL + ClockSkew + time.Second), ErrExpired},
		{"expiry within skew", sign(t, hs, k.m.sign, good), t0.Add(DefaultTTL + ClockSkew - time.Second), nil},
		{"issued in future", sign(t, hs, k.m.sign, good), t0.Add(-ClockSkew - time.Second), ErrNotYetValid},
		{"not before in future", sign(t, hs, k.m.sign, mut(func(c *claims) { c.NotBefore = jwt.NewNumericDate(t0.Add(time.Hour)) })), t0, ErrNotYetValid},
		{"missing exp", sign(t, hs, k.m.sign, mut(func(c *claims) { c.ExpiresAt = nil })), t0, ErrLifetime},
		{"missing iat", sign(t, hs, k.m.sign, mut(func(c *claims) { c.IssuedAt = nil })), t0, ErrLifetime},
		{"lifetime too long", sign(t, hs, k.m.sign, mut(func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(t0.Add(MaxTTL + time.Second)) })), t0, ErrLifetime},
		{"exp before iat", sign(t, hs, k.m.sign, mut(func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(t0.Add(-time.Second)) })), t0.Add(-2 * time.Second), ErrLifetime},
		{"zero clock", sign(t, hs, k.m.sign, good), time.Time{}, ErrNoClock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := k.Verify(tc.cred, tc.now)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if p != (Principal{}) {
				t.Fatal("a failed verification must return the zero Principal")
			}
			if tc.cred != "" && len(tc.cred) > 12 && strings.Contains(err.Error(), tc.cred[len(TokenPrefix):len(TokenPrefix)+8]) {
				t.Fatal("error text contains part of the credential")
			}
		})
	}
}

func TestTamperedPayloadIsRejected(t *testing.T) {
	k := NewKey(testKeyValue)
	cred := issue(t, k, IssueRequest{OrgID: "org-a"}).Credential.Reveal()
	parts := strings.Split(strings.TrimPrefix(cred, TokenPrefix), ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	forged := strings.Replace(string(raw), `"org_id":"org-a"`, `"org_id":"org-b"`, 1)
	if forged == string(raw) {
		t.Fatal("test did not alter the payload")
	}
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(forged))
	if _, err := k.Verify(TokenPrefix+strings.Join(parts, "."), t0); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered payload: %v", err)
	}
}

func TestVerifyWithoutKey(t *testing.T) {
	cred := issue(t, NewKey(testKeyValue), IssueRequest{OrgID: "org-a"}).Credential.Reveal()
	if _, err := (Key{}).Verify(cred, t0); !errors.Is(err, ErrKeyNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestCredentialNeverPrintsValue(t *testing.T) {
	out := issue(t, NewKey(testKeyValue), IssueRequest{OrgID: "org-a"})
	val := out.Credential.Reveal()
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		fmt.Sprintf("%v %+v %#v %s %q", out, out, out, out.Credential, out.Credential),
		out.Credential.String(), out.Credential.GoString(), string(b),
	} {
		if strings.Contains(s, val) || strings.Contains(s, TokenPrefix) {
			t.Fatal("credential value appeared in formatted output")
		}
	}
	if (Credential{}).Reveal() != "" {
		t.Fatal("zero credential must reveal nothing")
	}
}

func TestParseCredential(t *testing.T) {
	cred := issue(t, NewKey(testKeyValue), IssueRequest{OrgID: "org-a"}).Credential.Reveal()
	for _, in := range []string{cred, cred + "\n", "  " + cred + "\r\n"} {
		c, err := ParseCredential(in)
		if err != nil || c.Reveal() != cred {
			t.Fatalf("ParseCredential rejected a well-formed credential: %v", err)
		}
		if fmt.Sprint(c) != redacted || strings.Contains(fmt.Sprintf("%+v %#v", c, c), cred) {
			t.Fatal("parsed credential prints its value")
		}
	}
	bad := []string{
		"", TokenPrefix, strings.TrimPrefix(cred, TokenPrefix), "oxat_" + strings.TrimPrefix(cred, TokenPrefix),
		cred[:10] + " " + cred[10:], TokenPrefix + "a.b", TokenPrefix + strings.Repeat("a", MaxTokenLength),
	}
	for i, in := range bad {
		if c, err := ParseCredential(in); !errors.Is(err, ErrCredentialFormat) || c.Reveal() != "" {
			t.Errorf("case %d: accepted a malformed credential", i)
		}
	}
}
