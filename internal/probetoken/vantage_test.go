package probetoken

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func fixedRandom(b byte) *bytes.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{b}, 64))
}

func TestNewVantageIDFormatAndBinding(t *testing.T) {
	k := NewKey(testKeyValue)
	id, err := k.NewVantageID("org-a", fixedRandom(7))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, VantageIDPrefix) || len(id) != vantageIDLen {
		t.Fatalf("unexpected vantage id shape (len %d)", len(id))
	}
	if strings.ToLower(id) != id {
		t.Fatal("vantage id must be lower case")
	}
	if !k.VantageIDBoundTo(id, "org-a") {
		t.Fatal("minted id is not bound to its org")
	}
	if k.VantageIDBoundTo(id, "org-b") {
		t.Fatal("id bound to org-a must not be bound to org-b")
	}
	if NewKey(otherKeyValue).VantageIDBoundTo(id, "org-a") {
		t.Fatal("id minted under one key must not verify under another")
	}
	if (Key{}).VantageIDBoundTo(id, "org-a") {
		t.Fatal("unconfigured key must bind nothing")
	}
}

func TestVantageIDIsUnambiguousAcrossOrgNonceBoundary(t *testing.T) {
	k := NewKey(testKeyValue)
	a, _ := k.NewVantageID("org", fixedRandom(1))
	if k.VantageIDBoundTo(a, "org\x01") || k.VantageIDBoundTo(a, "or") {
		t.Fatal("org/nonce boundary must be unambiguous")
	}
}

func TestVantageIDRejectsTamperingAndNonCanonicalForms(t *testing.T) {
	k := NewKey(testKeyValue)
	id, _ := k.NewVantageID("org-a", fixedRandom(9))
	flip := func(s string, i int) string {
		b := []byte(s)
		if b[i] == 'a' {
			b[i] = 'b'
		} else {
			b[i] = 'a'
		}
		return string(b)
	}
	bad := []string{
		"",
		VantageIDPrefix,
		id[:len(id)-1],
		id + "0",
		"vtx_" + id[4:],
		strings.ToUpper(id[:4]) + id[4:],
		id[:4] + strings.ToUpper(id[4:]),
		flip(id, 5),             // nonce
		flip(id, len(id)-1),     // tag
		id[:10] + "g" + id[11:], // non-hex
		" " + id,
	}
	for i, b := range bad {
		if k.VantageIDBoundTo(b, "org-a") {
			t.Errorf("case %d: tampered or non-canonical id accepted", i)
		}
	}
}

func TestNewVantageIDFailures(t *testing.T) {
	k := NewKey(testKeyValue)
	if _, err := (Key{}).NewVantageID("org-a", fixedRandom(1)); !errors.Is(err, ErrKeyNotConfigured) {
		t.Fatalf("unconfigured key: %v", err)
	}
	if _, err := k.NewVantageID("  ", fixedRandom(1)); !errors.Is(err, ErrOrg) {
		t.Fatalf("blank org: %v", err)
	}
	if _, err := k.NewVantageID("org-a", nil); !errors.Is(err, ErrRandom) {
		t.Fatalf("nil random: %v", err)
	}
	if _, err := k.NewVantageID("org-a", bytes.NewReader([]byte{1, 2, 3})); !errors.Is(err, ErrRandom) {
		t.Fatalf("short random: %v", err)
	}
	a, _ := k.NewVantageID("org-a", fixedRandom(1))
	b, _ := k.NewVantageID("org-a", fixedRandom(2))
	if a == b {
		t.Fatal("different nonces must give different ids")
	}
}
