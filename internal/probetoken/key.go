package probetoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const (
	// KeyEnvVar holds the probe credential key value.
	KeyEnvVar = "OBSERVEX_SYNTHETIC_PROBE_KEY"
	// KeyFileEnvVar holds the path to a file containing the key. When set to a
	// non-empty path it always wins over KeyEnvVar.
	KeyFileEnvVar = "OBSERVEX_SYNTHETIC_PROBE_KEY_FILE"
	// MinKeyLength is the minimum key length in bytes after trimming.
	MinKeyLength = 32
)

// Derivation labels. Changing either invalidates every issued credential.
const (
	signLabel = "observex/f6.1/synthetic-probe-credential/v1/jwt-hs256"
	bindLabel = "observex/f6.1/synthetic-probe-credential/v1/vantage-id"
)

// Source says where the key configuration came from.
type Source string

const (
	SourceNone  Source = "none"
	SourceFile  Source = "file"
	SourceEnv   Source = "env"
	SourceValue Source = "value"
)

// Reason says why a key is not configured. It is empty when it is configured.
type Reason string

const (
	ReasonNone           Reason = ""
	ReasonNotSet         Reason = "not_set"
	ReasonFileMissing    Reason = "file_missing"
	ReasonFileUnreadable Reason = "file_unreadable"
	ReasonEmpty          Reason = "empty"
	ReasonTooShort       Reason = "too_short"
)

const redacted = "[REDACTED]"

// keyMaterial is held behind a pointer so that printing a struct that embeds
// a Key shows an address, never key bytes.
type keyMaterial struct {
	raw  []byte
	sign []byte
	bind []byte
}

// Key is a loaded probe credential key. The zero value is not configured:
// it issues nothing and verifies nothing.
type Key struct {
	m      *keyMaterial
	source Source
	reason Reason
	path   string
}

// LoadKey reads the key configuration from the process environment.
func LoadKey() Key {
	return LoadKeyFrom(os.LookupEnv, os.ReadFile)
}

// LoadKeyFrom reads the key configuration using the given environment lookup
// and file reader, with the same precedence rules as internal/servicetoken.
func LoadKeyFrom(lookupEnv func(string) (string, bool), readFile func(string) ([]byte, error)) Key {
	if path, ok := lookupEnv(KeyFileEnvVar); ok && strings.TrimSpace(path) != "" {
		k := Key{source: SourceFile, path: strings.TrimSpace(path)}
		data, err := readFile(k.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				k.reason = ReasonFileMissing
			} else {
				k.reason = ReasonFileUnreadable
			}
			return k
		}
		return k.withValue(string(data))
	}
	if v, ok := lookupEnv(KeyEnvVar); ok {
		return Key{source: SourceEnv}.withValue(v)
	}
	return Key{source: SourceNone, reason: ReasonNotSet}
}

// NewKey builds a Key from a value, applying the same trimming and length
// rules as LoadKeyFrom. It exists for callers that hold the value already,
// such as tests.
func NewKey(value string) Key {
	return Key{source: SourceValue}.withValue(value)
}

func (k Key) withValue(raw string) Key {
	v := strings.TrimSpace(raw)
	switch {
	case v == "":
		k.reason = ReasonEmpty
	case len(v) < MinKeyLength:
		k.reason = ReasonTooShort
	default:
		rawBytes := []byte(v)
		k.m = &keyMaterial{
			raw:  rawBytes,
			sign: derive(rawBytes, signLabel),
			bind: derive(rawBytes, bindLabel),
		}
		k.reason = ReasonNone
	}
	return k
}

func derive(raw []byte, label string) []byte {
	mac := hmac.New(sha256.New, raw)
	mac.Write([]byte(label))
	return mac.Sum(nil)
}

// Configured reports whether a usable key was loaded.
func (k Key) Configured() bool { return k.m != nil && len(k.m.raw) > 0 }

// Source reports where the configuration came from.
func (k Key) Source() Source {
	if k.source == "" {
		return SourceNone
	}
	return k.source
}

// Reason reports why the key is not configured, or ReasonNone.
func (k Key) Reason() Reason {
	if !k.Configured() && k.reason == ReasonNone {
		return ReasonNotSet
	}
	return k.reason
}

// FilePath returns the path from KeyFileEnvVar, or "". The path is not secret.
func (k Key) FilePath() string { return k.path }

// SameAs reports, in constant time, whether secret (trimmed) equals this key.
// Issuers use it to refuse a key that is also the session or agent secret.
// It is false when the key is not configured or secret is blank.
func (k Key) SameAs(secret string) bool {
	s := strings.TrimSpace(secret)
	if !k.Configured() || s == "" {
		return false
	}
	return subtle.ConstantTimeCompare(k.m.raw, []byte(s)) == 1
}

// String never returns key material.
func (k Key) String() string { return redacted }

// GoString never returns key material.
func (k Key) GoString() string { return redacted }

// Format never prints key material, whatever the verb.
func (k Key) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }

// MarshalText never returns key material.
func (k Key) MarshalText() ([]byte, error) { return []byte(redacted), nil }
