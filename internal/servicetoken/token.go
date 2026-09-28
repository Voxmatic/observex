// Package servicetoken loads and checks the shared internal service token
// that ObserveX services use to authenticate calls to each other.
//
// Configuration (approved names, P-1):
//
//   - OBSERVEX_INTERNAL_TOKEN_FILE: path to a file holding the token. When this
//     variable is set to a non-empty path it always wins, even if the file is
//     missing, unreadable, empty or too short. In those cases the token is "not
//     configured" and OBSERVEX_INTERNAL_TOKEN is NOT used as a fallback.
//   - OBSERVEX_INTERNAL_TOKEN: the token value, used only when the file
//     variable is unset or empty.
//
// The token is trimmed with strings.TrimSpace (so a trailing newline in a
// secret file is ignored) and must then be at least MinLength bytes long.
// Length is measured in bytes of the UTF-8 string (Go len), not in characters.
//
// A Token never reveals its value: String, GoString and Format all print
// "[REDACTED]", and there is no exported accessor for the value.
package servicetoken

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

const (
	// EnvVar holds the token value.
	EnvVar = "OBSERVEX_INTERNAL_TOKEN"
	// FileEnvVar holds the path to a file containing the token. It takes
	// priority over EnvVar.
	FileEnvVar = "OBSERVEX_INTERNAL_TOKEN_FILE"
	// Header is the HTTP header that carries the token.
	Header = "X-ObserveX-Internal-Token"
	// MinLength is the minimum token length in bytes (len of the UTF-8
	// string) after surrounding whitespace is trimmed.
	MinLength = 32
)

// Source says where the token configuration came from.
type Source string

const (
	SourceNone Source = "none"
	SourceFile Source = "file"
	SourceEnv  Source = "env"
)

// Reason says why a token is not configured. It is empty when the token is
// configured.
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

// Token is a loaded internal service token. The zero value is not configured
// and matches nothing.
//
// The value is held behind a pointer so that printing a struct that embeds a
// Token in an unexported field shows only an address, never the value.
type Token struct {
	value  *string
	source Source
	reason Reason
	path   string
}

// Load reads the token configuration from the process environment.
func Load() Token {
	return LoadFrom(os.LookupEnv, os.ReadFile)
}

// LoadFrom reads the token configuration using the given environment lookup
// and file reader. It exists so tests can supply both.
func LoadFrom(lookupEnv func(string) (string, bool), readFile func(string) ([]byte, error)) Token {
	if path, ok := lookupEnv(FileEnvVar); ok && strings.TrimSpace(path) != "" {
		t := Token{source: SourceFile, path: strings.TrimSpace(path)}
		data, err := readFile(t.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				t.reason = ReasonFileMissing
			} else {
				t.reason = ReasonFileUnreadable
			}
			return t
		}
		return t.withValue(string(data))
	}
	if v, ok := lookupEnv(EnvVar); ok {
		return Token{source: SourceEnv}.withValue(v)
	}
	return Token{source: SourceNone, reason: ReasonNotSet}
}

func (t Token) withValue(raw string) Token {
	v := strings.TrimSpace(raw)
	switch {
	case v == "":
		t.reason = ReasonEmpty
	case len(v) < MinLength:
		t.reason = ReasonTooShort
	default:
		t.value = &v
		t.reason = ReasonNone
	}
	return t
}

// Configured reports whether a usable token was loaded.
func (t Token) Configured() bool { return t.value != nil && *t.value != "" }

// Source reports where the configuration came from.
func (t Token) Source() Source {
	if t.source == "" {
		return SourceNone
	}
	return t.source
}

// Reason reports why the token is not configured, or ReasonNone.
func (t Token) Reason() Reason {
	if !t.Configured() && t.reason == ReasonNone {
		return ReasonNotSet
	}
	return t.reason
}

// FilePath returns the path from OBSERVEX_INTERNAL_TOKEN_FILE, or "" when the
// token did not come from a file. The path is not secret; the contents are.
func (t Token) FilePath() string { return t.path }

// Matches reports whether presented equals the configured token, using a
// constant-time comparison. It is always false when no token is configured
// or presented is empty.
func (t Token) Matches(presented string) bool {
	if !t.Configured() || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(*t.value)) == 1
}

// SetHeader sets the token header on h when a token is configured, and
// reports whether it did.
func (t Token) SetHeader(h http.Header) bool {
	if !t.Configured() {
		return false
	}
	h.Set(Header, *t.value)
	return true
}

// String never returns the token value.
func (t Token) String() string { return redacted }

// GoString never returns the token value.
func (t Token) GoString() string { return redacted }

// Format never prints the token value, whatever the verb.
func (t Token) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }

// MarshalText never returns the token value.
func (t Token) MarshalText() ([]byte, error) { return []byte(redacted), nil }
