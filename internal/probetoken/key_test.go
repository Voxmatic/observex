package probetoken

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

const (
	testKeyValue  = "probe-key-0123456789abcdef0123456789abcdef"
	otherKeyValue = "other-key-0123456789abcdef0123456789abcdef"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if v, ok := m[p]; ok {
			return []byte(v), nil
		}
		return nil, fs.ErrNotExist
	}
}

func TestLoadKeyPrecedenceAndReasons(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		files      map[string]string
		configured bool
		source     Source
		reason     Reason
	}{
		{"unset", nil, nil, false, SourceNone, ReasonNotSet},
		{"env", map[string]string{KeyEnvVar: testKeyValue}, nil, true, SourceEnv, ReasonNone},
		{"env trimmed", map[string]string{KeyEnvVar: "  " + testKeyValue + "\n"}, nil, true, SourceEnv, ReasonNone},
		{"env empty", map[string]string{KeyEnvVar: "   "}, nil, false, SourceEnv, ReasonEmpty},
		{"env short", map[string]string{KeyEnvVar: "short"}, nil, false, SourceEnv, ReasonTooShort},
		{"file wins", map[string]string{KeyFileEnvVar: "/k", KeyEnvVar: otherKeyValue}, map[string]string{"/k": testKeyValue + "\n"}, true, SourceFile, ReasonNone},
		{"missing file no fallback", map[string]string{KeyFileEnvVar: "/missing", KeyEnvVar: testKeyValue}, nil, false, SourceFile, ReasonFileMissing},
		{"short file no fallback", map[string]string{KeyFileEnvVar: "/k", KeyEnvVar: testKeyValue}, map[string]string{"/k": "short"}, false, SourceFile, ReasonTooShort},
		{"blank file var ignored", map[string]string{KeyFileEnvVar: "  ", KeyEnvVar: testKeyValue}, nil, true, SourceEnv, ReasonNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := LoadKeyFrom(env(tc.env), files(tc.files))
			if k.Configured() != tc.configured || k.Source() != tc.source || k.Reason() != tc.reason {
				t.Fatalf("got configured=%v source=%q reason=%q", k.Configured(), k.Source(), k.Reason())
			}
		})
	}
	unreadable := LoadKeyFrom(env(map[string]string{KeyFileEnvVar: "/k"}), func(string) ([]byte, error) { return nil, errors.New("permission denied") })
	if unreadable.Configured() || unreadable.Reason() != ReasonFileUnreadable {
		t.Fatalf("unreadable file: got %q", unreadable.Reason())
	}
}

func TestKeyNeverPrintsMaterial(t *testing.T) {
	k := NewKey(testKeyValue)
	if !k.Configured() {
		t.Fatal("key not configured")
	}
	type holder struct {
		K Key
		k Key
	}
	outputs := []string{
		k.String(), k.GoString(),
		fmt.Sprintf("%v %+v %#v %s %q %x", k, k, k, k, k, k),
		fmt.Sprintf("%v %+v %#v", holder{k, k}, holder{k, k}, holder{k, k}),
	}
	b, err := json.Marshal(holder{K: k})
	if err != nil {
		t.Fatal(err)
	}
	outputs = append(outputs, string(b))
	for _, out := range outputs {
		if strings.Contains(out, testKeyValue) || strings.Contains(out, "probe-key") {
			t.Fatal("key material appeared in formatted output")
		}
	}
}

func TestSameAs(t *testing.T) {
	k := NewKey(testKeyValue)
	if !k.SameAs(testKeyValue) || !k.SameAs(" "+testKeyValue+"\n") {
		t.Fatal("SameAs should match the same secret")
	}
	if k.SameAs(otherKeyValue) || k.SameAs("") {
		t.Fatal("SameAs matched a different or empty secret")
	}
	if (Key{}).SameAs(testKeyValue) {
		t.Fatal("unconfigured key matched")
	}
}

func TestDerivedKeysAreDistinctFromRawAndEachOther(t *testing.T) {
	k := NewKey(testKeyValue)
	if string(k.m.sign) == testKeyValue || string(k.m.bind) == testKeyValue || string(k.m.sign) == string(k.m.bind) {
		t.Fatal("derived keys must differ from the raw key and from each other")
	}
}
