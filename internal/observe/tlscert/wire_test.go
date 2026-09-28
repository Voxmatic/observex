package tlscert

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

var serverVantage = Vantage{Kind: "synthetic-probe", ID: "vtg_server_assigned"}

func bindingFor(o Observation) Binding {
	return Binding{CheckID: o.CheckID, Endpoint: o.Endpoint, Vantage: serverVantage}
}

// assertRoundTrip checks that decode(encode(o)) is o with the server vantage.
func assertRoundTrip(t *testing.T, o Observation) []byte {
	t.Helper()
	data, err := MarshalObservation(o)
	if err != nil {
		t.Fatalf("MarshalObservation(%s): %v", o.Outcome, err)
	}
	got, err := UnmarshalObservation(data, bindingFor(o))
	if err != nil {
		t.Fatalf("UnmarshalObservation(%s): %v", o.Outcome, err)
	}
	want := o
	want.Vantage = serverVantage
	want.ObservedAt = o.ObservedAt.UTC()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the observation:\n got %+v\nwant %+v", got, want)
	}
	if !reflect.DeepEqual(got.Certificates(), o.Certificates()) {
		t.Fatal("round trip changed the certificate chain")
	}
	return data
}

func probeOnce(t *testing.T, p Prober, endpoint string) Observation {
	t.Helper()
	obs, err := p.Probe(context.Background(), Target{CheckID: "chk-1", Endpoint: endpoint, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	return obs
}

// Every outcome Probe can produce survives transport unchanged, apart from
// the vantage, which is always the receiver's.
func TestWireRoundTripForEveryProbeOutcome(t *testing.T) {
	root := newCA(t, "root-ca", nil)
	inter := newCA(t, "intermediate-ca", root)

	t.Run("observed, full chain", func(t *testing.T) {
		der, key := newLeaf(t, inter, "observex.test", fixedNow.Add(-time.Hour), fixedNow.Add(30*24*time.Hour), nil, localIPs())
		s := startTLSServer(t, [][]byte{der, inter.der}, key)
		obs := probeOnce(t, proberFor(t, poolOf(root.cert), nil), s.addr)
		if obs.Outcome != Observed || len(obs.Certificates()) != 2 {
			t.Fatalf("fixture: %q with %d certificates", obs.Outcome, len(obs.Certificates()))
		}
		assertRoundTrip(t, obs)
	})
	t.Run("untrusted: hostname mismatch", func(t *testing.T) {
		der, key := newLeaf(t, inter, "other.example", fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), []string{"other.example"}, nil)
		s := startTLSServer(t, [][]byte{der, inter.der}, key)
		obs := probeOnce(t, proberFor(t, poolOf(root.cert), nil), s.addr)
		if obs.Trust.Reason != ReasonHostnameMismatch {
			t.Fatalf("fixture reason %q", obs.Trust.Reason)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("untrusted: unknown authority and hostname mismatch", func(t *testing.T) {
		rogue := newCA(t, "rogue", nil)
		der, key := newLeaf(t, rogue, "other.example", fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), []string{"other.example"}, nil)
		s := startTLSServer(t, [][]byte{der}, key)
		obs := probeOnce(t, proberFor(t, poolOf(root.cert), nil), s.addr)
		if obs.Trust.Reason != ReasonUnknownAuthority+" and "+ReasonHostnameMismatch {
			t.Fatalf("fixture reason %q", obs.Trust.Reason)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("untrusted: expired leaf", func(t *testing.T) {
		der, key := newLeaf(t, inter, "observex.test", fixedNow.Add(-48*time.Hour), fixedNow.Add(-time.Hour), nil, localIPs())
		s := startTLSServer(t, [][]byte{der, inter.der}, key)
		obs := probeOnce(t, proberFor(t, poolOf(root.cert), nil), s.addr)
		if obs.Trust.Reason != ReasonExpired {
			t.Fatalf("fixture reason %q", obs.Trust.Reason)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("untrusted: not yet valid", func(t *testing.T) {
		der, key := newLeaf(t, inter, "observex.test", fixedNow.Add(time.Hour), fixedNow.Add(48*time.Hour), nil, localIPs())
		s := startTLSServer(t, [][]byte{der, inter.der}, key)
		obs := probeOnce(t, proberFor(t, poolOf(root.cert), nil), s.addr)
		if obs.Trust.Reason != ReasonNotYetValid {
			t.Fatalf("fixture reason %q", obs.Trust.Reason)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("transport failure", func(t *testing.T) {
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		addr := ln.Addr().String()
		ln.Close()
		obs := probeOnce(t, proberFor(t, x509.NewCertPool(), nil), addr)
		if obs.Outcome != TransportFailure {
			t.Fatalf("fixture outcome %q", obs.Outcome)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("timeout", func(t *testing.T) {
		p := New(Config{Now: func() time.Time { return fixedNow }, Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Err: timeoutError{}}
		}})
		obs := probeOnce(t, p, "example.invalid:443")
		if obs.Outcome != Timeout {
			t.Fatalf("fixture outcome %q", obs.Outcome)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("malformed", func(t *testing.T) {
		obs := probeOnce(t, proberFor(t, x509.NewCertPool(), nil), startPlaintextServer(t))
		if obs.Outcome != Malformed {
			t.Fatalf("fixture outcome %q", obs.Outcome)
		}
		assertRoundTrip(t, obs)
	})
	t.Run("no certificates", func(t *testing.T) {
		// Not producible by a conforming server; built the way Probe builds it.
		assertRoundTrip(t, Observation{CheckID: "chk-1", Endpoint: "observex.test:443", ObservedAt: fixedNow,
			Outcome: NoCertificates, Detail: noCertificatesDetail})
	})
}

// ── the v1 format is pinned ──────────────────────────────────────────────────

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A document written by v1 decodes to the same Observation forever, and
// re-encodes to the same bytes. A change to field names, types or order fails
// here before it can strand a probe running an older build.
func TestWireGoldenV1(t *testing.T) {
	data := readGolden(t, "observation-v1-observed-untrusted.json")
	b := Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage}
	obs, err := UnmarshalObservation(data, b)
	if err != nil {
		t.Fatalf("golden v1 no longer decodes: %v", err)
	}
	leaf, ok := obs.Leaf()
	if !ok || obs.Outcome != ObservedUntrusted || obs.Trust.Reason != ReasonUnknownAuthority || !obs.Trust.HostnameVerified ||
		!leaf.NotAfter.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) || leaf.SerialNumber != "61061061" ||
		!strings.Contains(leaf.Subject, "CN=observex.test") || !reflect.DeepEqual(leaf.DNSNames(), []string{"observex.test"}) ||
		!obs.ObservedAt.Equal(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)) || obs.Duration != 42*time.Millisecond ||
		obs.Vantage != serverVantage || obs.CheckID != "chk-golden-1" {
		t.Fatalf("golden v1 decoded to an unexpected observation: %+v leaf %+v", obs, leaf)
	}
	again, err := MarshalObservation(obs)
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("golden v1 does not re-encode byte for byte (err %v)", err)
	}

	timeout := readGolden(t, "observation-v1-timeout.json")
	obs, err = UnmarshalObservation(timeout, Binding{CheckID: "chk-golden-2", Endpoint: "observex.test", Vantage: serverVantage})
	if err != nil || obs.Outcome != Timeout || obs.Detail != "timeout during handshake" || len(obs.Certificates()) != 0 {
		t.Fatalf("golden v1 timeout: %+v, %v", obs, err)
	}
	if again, _ := MarshalObservation(obs); !bytes.Equal(again, timeout) {
		t.Fatal("golden v1 timeout does not re-encode byte for byte")
	}
}

// ── identity never comes from the payload ────────────────────────────────────

func TestWireCarriesNoVantageAndBindsIdentity(t *testing.T) {
	data := readGolden(t, "observation-v1-observed-untrusted.json")
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for k := range m {
		switch k {
		case "vantage", "vantage_id", "org_id", "namespace", "subject", "service_id":
			t.Fatalf("payload carries identity field %q", k)
		}
	}
	good := Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage}
	cases := map[string]Binding{
		"other check":       {CheckID: "chk-other", Endpoint: good.Endpoint, Vantage: serverVantage},
		"other endpoint":    {CheckID: good.CheckID, Endpoint: "observex.test:8443", Vantage: serverVantage},
		"endpoint spelling": {CheckID: good.CheckID, Endpoint: "OBSERVEX.TEST:443", Vantage: serverVantage},
		"no vantage id":     {CheckID: good.CheckID, Endpoint: good.Endpoint, Vantage: Vantage{Kind: "synthetic-probe"}},
		"no vantage kind":   {CheckID: good.CheckID, Endpoint: good.Endpoint, Vantage: Vantage{ID: "x"}},
		"blank check":       {CheckID: " ", Endpoint: good.Endpoint, Vantage: serverVantage},
		"unusable endpoint": {CheckID: good.CheckID, Endpoint: "https://observex.test/", Vantage: serverVantage},
		"empty endpoint":    {CheckID: good.CheckID, Vantage: serverVantage},
	}
	for name, b := range cases {
		if obs, err := UnmarshalObservation(data, b); !errors.Is(err, ErrWireBinding) || obs.Outcome != "" {
			t.Errorf("%s: got %v, want ErrWireBinding and the zero Observation", name, err)
		}
	}
	// A vantage in the payload is not merely ignored: the document is refused.
	withVantage := mutate(t, data, func(m map[string]any) { m["vantage"] = map[string]any{"Kind": "external", "ID": "spoofed"} })
	if _, err := UnmarshalObservation(withVantage, good); !errors.Is(err, ErrWireMalformed) {
		t.Fatalf("payload with a vantage: %v", err)
	}
}

// ── versioning and strict decoding ───────────────────────────────────────────

func mutate(t *testing.T, data []byte, f func(map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		t.Fatal(err)
	}
	f(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWireVersionAndStrictness(t *testing.T) {
	data := readGolden(t, "observation-v1-observed-untrusted.json")
	b := Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage}
	cases := []struct {
		name string
		in   []byte
		want error
	}{
		{"version 2", mutate(t, data, func(m map[string]any) { m["v"] = 2 }), ErrWireVersion},
		{"version 2 with new fields", mutate(t, data, func(m map[string]any) { m["v"] = 2; m["future"] = true }), ErrWireVersion},
		{"version 0", mutate(t, data, func(m map[string]any) { m["v"] = 0 }), ErrWireVersion},
		{"no version", mutate(t, data, func(m map[string]any) { delete(m, "v") }), ErrWireMalformed},
		{"string version", mutate(t, data, func(m map[string]any) { m["v"] = "1" }), ErrWireMalformed},
		{"unknown field", mutate(t, data, func(m map[string]any) { m["not_after"] = "2099-01-01T00:00:00Z" }), ErrWireMalformed},
		{"wrong type", mutate(t, data, func(m map[string]any) { m["duration_ns"] = "42" }), ErrWireMalformed},
		{"certificates not base64", mutate(t, data, func(m map[string]any) { m["certificates"] = []any{"!!!"} }), ErrWireMalformed},
		{"trailing data", append(append([]byte{}, data...), []byte(` {"v":1}`)...), ErrWireMalformed},
		{"not json", []byte("certificate please"), ErrWireMalformed},
		{"array", []byte(`[1]`), ErrWireMalformed},
		{"null", []byte(`null`), ErrWireMalformed},
		{"empty", nil, ErrWireMalformed},
		{"too large", append(bytes.Repeat([]byte(" "), MaxWireBytes), data...), ErrWireTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := UnmarshalObservation(tc.in, b)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if obs.Outcome != "" || obs.Certificates() != nil {
				t.Fatal("a refused payload must yield the zero Observation")
			}
		})
	}
}

// ── consistency with what the prober could have produced ────────────────────

func TestWireRefusesInconsistentObservations(t *testing.T) {
	data := readGolden(t, "observation-v1-observed-untrusted.json")
	b := Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	der, _ := base64.StdEncoding.DecodeString(m["certificates"].([]any)[0].(string))
	b64 := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
	trust := func(chain, host bool, reason string) map[string]any {
		return map[string]any{"chain_verified": chain, "hostname_verified": host, "reason": reason}
	}
	failTo := func(outcome, detail string) func(map[string]any) {
		return func(m map[string]any) {
			m["outcome"], m["detail"] = outcome, detail
			m["trust"] = trust(false, false, "")
			delete(m, "certificates")
		}
	}
	cases := []struct {
		name string
		f    func(map[string]any)
		want error
	}{
		{"garbage DER", func(m map[string]any) { m["certificates"] = []any{b64([]byte("not a certificate"))} }, ErrWireInvalid},
		{"DER with trailing byte", func(m map[string]any) { m["certificates"] = []any{b64(append(append([]byte{}, der...), 0))} }, ErrWireInvalid},
		{"empty DER", func(m map[string]any) { m["certificates"] = []any{""} }, ErrWireInvalid},
		{"observed without certificates", func(m map[string]any) { delete(m, "certificates") }, ErrWireInvalid},
		{"hostname claim contradicts the leaf", func(m map[string]any) {
			m["trust"] = trust(false, false, ReasonUnknownAuthority+" and "+ReasonHostnameMismatch)
			m["detail"] = "observed 1 certificate(s); verification failed: " + ReasonUnknownAuthority + " and " + ReasonHostnameMismatch
		}, ErrWireInvalid},
		{"chain claimed verified outside validity", func(m map[string]any) {
			m["observed_at"] = "2027-06-01T00:00:00Z"
			m["outcome"], m["trust"] = "observed", trust(true, true, "")
			m["detail"] = "observed 1 certificate(s); verification passed"
		}, ErrWireInvalid},
		{"not-yet-valid inside validity", func(m map[string]any) {
			m["trust"] = trust(false, true, ReasonNotYetValid)
			m["detail"] = "observed 1 certificate(s); verification failed: " + ReasonNotYetValid
		}, ErrWireInvalid},
		{"expired before notBefore", func(m map[string]any) {
			m["observed_at"] = "2025-06-01T00:00:00Z"
			m["trust"] = trust(false, true, ReasonExpired)
			m["detail"] = "observed 1 certificate(s); verification failed: " + ReasonExpired
		}, ErrWireInvalid},
		{"reason outside the vocabulary", func(m map[string]any) {
			m["trust"] = trust(false, true, "attacker says <script>")
			m["detail"] = "observed 1 certificate(s); verification failed: attacker says <script>"
		}, ErrWireInvalid},
		{"reason on a verified chain", func(m map[string]any) {
			m["outcome"], m["trust"] = "observed", trust(true, true, ReasonExpired)
		}, ErrWireInvalid},
		{"outcome contradicts trust", func(m map[string]any) { m["outcome"] = "observed" }, ErrWireInvalid},
		{"detail rewritten", func(m map[string]any) { m["detail"] = "all good, nothing to see" }, ErrWireInvalid},
		{"unknown outcome", func(m map[string]any) { m["outcome"] = "fine" }, ErrWireInvalid},
		{"zero time", func(m map[string]any) { m["observed_at"] = "0001-01-01T00:00:00Z" }, ErrWireInvalid},
		{"negative duration", func(m map[string]any) { m["duration_ns"] = -1 }, ErrWireInvalid},
		{"duration beyond limit", func(m map[string]any) { m["duration_ns"] = int64(MaxWireDuration) + 1 }, ErrWireInvalid},
		{"failure carrying certificates", func(m map[string]any) {
			failTo("timeout", "timeout during dial")(m)
			m["certificates"] = []any{b64(der)}
		}, ErrWireInvalid},
		{"failure carrying trust", func(m map[string]any) {
			failTo("timeout", "timeout during dial")(m)
			m["trust"] = trust(true, false, "")
		}, ErrWireInvalid},
		{"failure with free-text detail", failTo("transport_failure", "connection refused by 10.0.0.7"), ErrWireInvalid},
		{"failure with another outcome's detail", failTo("malformed", "timeout during dial"), ErrWireInvalid},
		{"no-certificates with other detail", failTo("no_certificates", "no_certificates during dial"), ErrWireInvalid},
		{"too many certificates", func(m map[string]any) {
			many := make([]any, MaxWireCertificates+1)
			for i := range many {
				many[i] = b64(der)
			}
			m["certificates"] = many
		}, ErrWireTooLarge},
	}
	// Within the count limit but over the byte limit: a leaf padded with SANs.
	ca := newCA(t, "big-ca", nil)
	var names []string
	for i := 0; i < 120; i++ {
		names = append(names, strings.Repeat("n", 20)+".observex.test")
	}
	big, _ := newLeaf(t, ca, "observex.test", fixedNow.Add(-time.Hour), fixedNow.Add(time.Hour), names, nil)
	n := MaxWireChainBytes/len(big) + 1
	if n > MaxWireCertificates {
		t.Fatalf("fixture: %d certificates of %d bytes would hit the count limit first", n, len(big))
	}
	cases = append(cases, struct {
		name string
		f    func(map[string]any)
		want error
	}{"chain bytes too large", func(m map[string]any) {
		many := make([]any, n)
		for i := range many {
			many[i] = b64(big)
		}
		m["certificates"] = many
	}, ErrWireTooLarge})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := UnmarshalObservation(mutate(t, data, tc.f), b)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if obs.Outcome != "" {
				t.Fatal("a refused payload must yield the zero Observation")
			}
		})
	}
	// Control: the valid failure outcomes decode.
	for _, f := range []func(map[string]any){
		failTo("timeout", "timeout during dial"),
		failTo("malformed", "malformed during handshake"),
		failTo("transport_failure", "transport_failure during dial"),
		failTo("no_certificates", noCertificatesDetail),
	} {
		if _, err := UnmarshalObservation(mutate(t, data, f), b); err != nil {
			t.Fatalf("valid failure payload refused: %v", err)
		}
	}
}

func TestWireErrorsNeverQuotePayload(t *testing.T) {
	marker := "zz-marker-7f3a"
	data := mutate(t, readGolden(t, "observation-v1-observed-untrusted.json"), func(m map[string]any) {
		m["detail"] = marker
	})
	_, err := UnmarshalObservation(data, Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage})
	if err == nil || strings.Contains(err.Error(), marker) {
		t.Fatalf("error quotes the payload or is nil: %v", err)
	}
}

func TestMarshalRefusesWhatCannotBeDecoded(t *testing.T) {
	cases := map[string]Observation{
		"zero observation": {},
		"no check id":      {Endpoint: "observex.test", ObservedAt: fixedNow, Outcome: Timeout, Detail: "timeout during dial"},
		"bad endpoint":     {CheckID: "c", Endpoint: "https://x/", ObservedAt: fixedNow, Outcome: Timeout, Detail: "timeout during dial"},
		"free-text detail": {CheckID: "c", Endpoint: "observex.test", ObservedAt: fixedNow, Outcome: Timeout, Detail: "peer said hello"},
	}
	for name, o := range cases {
		if data, err := MarshalObservation(o); err == nil || data != nil {
			t.Errorf("%s: MarshalObservation succeeded", name)
		}
	}
}

// The decoder copies what it keeps: mutating the input afterwards changes
// nothing in the decoded observation.
func TestWireDecodedObservationIsIndependentOfInput(t *testing.T) {
	data := readGolden(t, "observation-v1-observed-untrusted.json")
	obs, err := UnmarshalObservation(data, Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage})
	if err != nil {
		t.Fatal(err)
	}
	before := obs.Certificates()[0].DER()
	for i := range data {
		data[i] = 'x'
	}
	if !bytes.Equal(obs.Certificates()[0].DER(), before) {
		t.Fatal("decoded certificate shares memory with the input")
	}
}

// FuzzUnmarshalObservation: arbitrary input never panics, and anything
// accepted re-encodes and decodes to the same observation.
func FuzzUnmarshalObservation(f *testing.F) {
	for _, name := range []string{"observation-v1-observed-untrusted.json", "observation-v1-timeout.json"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte(`{"v":1}`))
	f.Add([]byte(`{"v":2,"x":1}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, b := range []Binding{
			{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: serverVantage},
			{CheckID: "chk-golden-2", Endpoint: "observex.test", Vantage: serverVantage},
		} {
			obs, err := UnmarshalObservation(data, b)
			if err != nil {
				continue
			}
			again, err := MarshalObservation(obs)
			if err != nil {
				t.Fatalf("accepted observation does not re-encode: %v", err)
			}
			back, err := UnmarshalObservation(again, b)
			if err != nil || !reflect.DeepEqual(back, obs) {
				t.Fatalf("accepted observation is not stable across a round trip: %v", err)
			}
		}
	})
}
