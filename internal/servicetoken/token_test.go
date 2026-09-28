package servicetoken

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"testing"
)

// Test token values are built at run time and never printed by these tests.
var (
	testTokenA = strings.Repeat("a", MinLength) + "-servicetoken-test-A"
	testTokenB = strings.Repeat("b", MinLength) + "-servicetoken-test-B"
)

type fakeEnv map[string]string

func (e fakeEnv) lookup(k string) (string, bool) { v, ok := e[k]; return v, ok }

type fakeFiles map[string]fakeFile

type fakeFile struct {
	data []byte
	err  error
}

func (f fakeFiles) read(path string) ([]byte, error) {
	ff, ok := f[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return ff.data, ff.err
}

func TestLoadFrom(t *testing.T) {
	const path = "/run/secrets/internal-token"
	tests := []struct {
		name           string
		env            fakeEnv
		files          fakeFiles
		wantConfigured bool
		wantSource     Source
		wantReason     Reason
		wantMatch      string // token that must match, if configured
	}{
		{
			name:       "NothingSet",
			env:        fakeEnv{},
			wantSource: SourceNone, wantReason: ReasonNotSet,
		},
		{
			name:           "EnvOnly",
			env:            fakeEnv{EnvVar: testTokenA},
			wantConfigured: true, wantSource: SourceEnv, wantReason: ReasonNone, wantMatch: testTokenA,
		},
		{
			name:           "FileOnly",
			env:            fakeEnv{FileEnvVar: path},
			files:          fakeFiles{path: {data: []byte(testTokenA)}},
			wantConfigured: true, wantSource: SourceFile, wantReason: ReasonNone, wantMatch: testTokenA,
		},
		{
			name:           "FileWinsOverEnv",
			env:            fakeEnv{FileEnvVar: path, EnvVar: testTokenB},
			files:          fakeFiles{path: {data: []byte(testTokenA)}},
			wantConfigured: true, wantSource: SourceFile, wantReason: ReasonNone, wantMatch: testTokenA,
		},
		{
			name:           "FileTrailingNewlineTrimmed",
			env:            fakeEnv{FileEnvVar: path},
			files:          fakeFiles{path: {data: []byte("  " + testTokenA + "\n")}},
			wantConfigured: true, wantSource: SourceFile, wantReason: ReasonNone, wantMatch: testTokenA,
		},
		{
			name:       "FileMissing_NoFallbackToEnv",
			env:        fakeEnv{FileEnvVar: path, EnvVar: testTokenB},
			files:      fakeFiles{},
			wantSource: SourceFile, wantReason: ReasonFileMissing,
		},
		{
			name:       "FileUnreadable_NoFallbackToEnv",
			env:        fakeEnv{FileEnvVar: path, EnvVar: testTokenB},
			files:      fakeFiles{path: {err: errors.New("permission denied")}},
			wantSource: SourceFile, wantReason: ReasonFileUnreadable,
		},
		{
			name:       "FileEmpty_NoFallbackToEnv",
			env:        fakeEnv{FileEnvVar: path, EnvVar: testTokenB},
			files:      fakeFiles{path: {data: []byte("")}},
			wantSource: SourceFile, wantReason: ReasonEmpty,
		},
		{
			name:       "FileWhitespaceOnly_NoFallbackToEnv",
			env:        fakeEnv{FileEnvVar: path, EnvVar: testTokenB},
			files:      fakeFiles{path: {data: []byte(" \n\t ")}},
			wantSource: SourceFile, wantReason: ReasonEmpty,
		},
		{
			name:       "FileTooShort_NoFallbackToEnv",
			env:        fakeEnv{FileEnvVar: path, EnvVar: testTokenB},
			files:      fakeFiles{path: {data: []byte(strings.Repeat("x", MinLength-1))}},
			wantSource: SourceFile, wantReason: ReasonTooShort,
		},
		{
			name:       "EnvEmpty",
			env:        fakeEnv{EnvVar: ""},
			wantSource: SourceEnv, wantReason: ReasonEmpty,
		},
		{
			name:       "EnvWhitespaceOnly",
			env:        fakeEnv{EnvVar: "   "},
			wantSource: SourceEnv, wantReason: ReasonEmpty,
		},
		{
			name:       "EnvTooShort31Bytes",
			env:        fakeEnv{EnvVar: strings.Repeat("x", 31)},
			wantSource: SourceEnv, wantReason: ReasonTooShort,
		},
		{
			name:           "EnvExactly32Bytes",
			env:            fakeEnv{EnvVar: strings.Repeat("x", 32)},
			wantConfigured: true, wantSource: SourceEnv, wantReason: ReasonNone, wantMatch: strings.Repeat("x", 32),
		},
		{
			// "é" is 2 bytes: 16 of them are 16 characters but 32 bytes.
			name:           "LengthCountsBytesNotCharacters",
			env:            fakeEnv{EnvVar: strings.Repeat("é", 16)},
			wantConfigured: true, wantSource: SourceEnv, wantReason: ReasonNone, wantMatch: strings.Repeat("é", 16),
		},
		{
			name:       "EnvLengthMeasuredAfterTrim",
			env:        fakeEnv{EnvVar: "  " + strings.Repeat("x", 31) + "  "},
			wantSource: SourceEnv, wantReason: ReasonTooShort,
		},
		{
			name:           "EmptyFileVarIsNotSet_EnvUsed",
			env:            fakeEnv{FileEnvVar: "  ", EnvVar: testTokenA},
			wantConfigured: true, wantSource: SourceEnv, wantReason: ReasonNone, wantMatch: testTokenA,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := tt.files
			if files == nil {
				files = fakeFiles{}
			}
			tok := LoadFrom(tt.env.lookup, files.read)
			if tok.Configured() != tt.wantConfigured {
				t.Fatalf("Configured() = %v, want %v", tok.Configured(), tt.wantConfigured)
			}
			if tok.Source() != tt.wantSource {
				t.Fatalf("Source() = %q, want %q", tok.Source(), tt.wantSource)
			}
			if tok.Reason() != tt.wantReason {
				t.Fatalf("Reason() = %q, want %q", tok.Reason(), tt.wantReason)
			}
			if tt.wantConfigured && !tok.Matches(tt.wantMatch) {
				t.Fatal("configured token does not match the expected value")
			}
			if !tt.wantConfigured {
				for _, candidate := range []string{testTokenA, testTokenB, "", strings.Repeat("x", 31)} {
					if tok.Matches(candidate) {
						t.Fatal("unconfigured token matched a value")
					}
				}
			}
		})
	}
}

func TestFilePathReported(t *testing.T) {
	const path = "/run/secrets/internal-token"
	tok := LoadFrom(fakeEnv{FileEnvVar: " " + path + " "}.lookup, fakeFiles{}.read)
	if tok.FilePath() != path {
		t.Fatalf("FilePath() = %q, want %q", tok.FilePath(), path)
	}
	if env := LoadFrom(fakeEnv{EnvVar: testTokenA}.lookup, fakeFiles{}.read); env.FilePath() != "" {
		t.Fatalf("env token FilePath() = %q, want empty", env.FilePath())
	}
}

func TestMatches(t *testing.T) {
	tok := LoadFrom(fakeEnv{EnvVar: testTokenA}.lookup, fakeFiles{}.read)
	switch {
	case !tok.Matches(testTokenA):
		t.Fatal("exact value did not match")
	case tok.Matches(testTokenB):
		t.Fatal("different value matched")
	case tok.Matches(""):
		t.Fatal("empty value matched")
	case tok.Matches(testTokenA + "x"):
		t.Fatal("longer value matched")
	case tok.Matches(testTokenA[:len(testTokenA)-1]):
		t.Fatal("prefix matched")
	case tok.Matches(strings.ToUpper(testTokenA)):
		t.Fatal("case-changed value matched")
	case tok.Matches(" " + testTokenA):
		t.Fatal("presented value with whitespace matched")
	}
	var zero Token
	if zero.Configured() || zero.Matches("") || zero.Matches(testTokenA) {
		t.Fatal("zero Token must be unconfigured and match nothing")
	}
	if zero.Source() != SourceNone || zero.Reason() != ReasonNotSet {
		t.Fatalf("zero Token source/reason = %q/%q", zero.Source(), zero.Reason())
	}
}

func TestSetHeader(t *testing.T) {
	h := http.Header{}
	if (Token{}).SetHeader(h) || h.Get(Header) != "" {
		t.Fatal("unconfigured token set a header")
	}
	tok := LoadFrom(fakeEnv{EnvVar: testTokenA}.lookup, fakeFiles{}.read)
	if !tok.SetHeader(h) {
		t.Fatal("configured token did not set the header")
	}
	if !tok.Matches(h.Get(Header)) {
		t.Fatal("header value does not match the token")
	}
}

func TestTokenNeverRendersValue(t *testing.T) {
	tok := LoadFrom(fakeEnv{EnvVar: testTokenA}.lookup, fakeFiles{}.read)
	holder := struct {
		Token Token
		Name  string
	}{Token: tok, Name: "holder"}
	hidden := struct {
		tok  Token
		name string
	}{tok: tok, name: "hidden"}

	rendered := []string{
		tok.String(),
		tok.GoString(),
		fmt.Sprint(tok),
		fmt.Sprintf("%v %+v %#v %s %q %x %d", tok, tok, tok, tok, tok, tok, tok),
		fmt.Sprintf("%v %+v %#v", holder, holder, holder),
		fmt.Sprintf("%v %+v %#v", hidden, hidden, hidden),
		fmt.Sprintf("%v %+v", &hidden, &holder),
		fmt.Sprintf("%v", &tok),
	}
	if b, err := json.Marshal(holder); err == nil {
		rendered = append(rendered, string(b))
	} else {
		t.Fatalf("json.Marshal: %v", err)
	}
	if b, err := tok.MarshalText(); err == nil {
		rendered = append(rendered, string(b))
	}
	for i, s := range rendered {
		if strings.Contains(s, testTokenA) || strings.Contains(s, strings.Repeat("a", MinLength)) {
			t.Fatalf("rendering %d exposed the token value", i)
		}
	}
}
