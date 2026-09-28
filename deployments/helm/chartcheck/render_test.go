package chartcheck

// A minimal stand-in for `helm template`, used ONLY so the static chart checks
// can run where no Helm binary exists. It is text/template plus the subset of
// Helm and Sprig functions this chart uses, with Helm's value-merging and
// include semantics. It does NOT render sub-charts (PostgreSQL, Redis, ...),
// does not run Helm's schema or lint checks, and approximates some functions.
// A pass here is static evidence, never evidence that Helm renders or that
// Kubernetes accepts or enforces anything.
//
// When OBSERVEX_HELM_TEMPLATE_OUTPUT names a file produced by the real
// `helm template` with testdata/f61-kind-values.yaml, the checks read that file
// instead and this stand-in is not used.

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"sigs.k8s.io/yaml"
)

const (
	chartDir    = "../observex"
	releaseName = "observex"
	namespace   = "observex"
	// HelmOutputEnv names a real `helm template` output to check instead.
	HelmOutputEnv = "OBSERVEX_HELM_TEMPLATE_OUTPUT"
)

func empty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int64, reflect.Int32:
		return rv.Int() == 0
	case reflect.Float64, reflect.Float32:
		return rv.Float() == 0
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// merge deep-merges src into dst (maps only; src wins), as Helm merges -f files.
func merge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				merge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

func readYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

// renderStandIn renders every template of the chart with values.yaml merged
// with the given override files and override maps, and returns the documents.
func renderStandIn(t *testing.T, overrides ...any) ([]map[string]any, error) {
	t.Helper()
	values := readYAML(t, filepath.Join(chartDir, "values.yaml"))
	for _, o := range overrides {
		switch v := o.(type) {
		case string:
			merge(values, readYAML(t, v))
		case map[string]any:
			merge(values, v)
		}
	}
	meta := readYAML(t, filepath.Join(chartDir, "Chart.yaml"))
	top := map[string]any{
		"Values":       values,
		"Release":      map[string]any{"Name": releaseName, "Namespace": namespace, "Service": "Helm", "IsInstall": true, "IsUpgrade": false},
		"Chart":        map[string]any{"Name": meta["name"], "Version": meta["version"], "AppVersion": meta["appVersion"]},
		"Template":     map[string]any{"BasePath": "observex/templates", "Name": ""},
		"Capabilities": map[string]any{},
	}
	tpl := template.New("chart")
	depth := 0
	tpl.Funcs(template.FuncMap{
		"include": func(name string, data any) (string, error) {
			depth++
			defer func() { depth-- }()
			if depth > 100 {
				return "", errors.New("include depth exceeded")
			}
			var b bytes.Buffer
			err := tpl.ExecuteTemplate(&b, name, data)
			return b.String(), err
		},
		"required": func(msg string, v any) (any, error) {
			if empty(v) {
				return nil, errors.New(msg)
			}
			return v, nil
		},
		"fail":   func(msg string) (string, error) { return "", errors.New(msg) },
		"toYaml": func(v any) string { b, _ := yaml.Marshal(v); return strings.TrimSuffix(string(b), "\n") },
		"toJson": func(v any) string { b, _ := json.Marshal(v); return string(b) },
		"fromYamlArray": func(s string) []any {
			var out []any
			if err := yaml.Unmarshal([]byte(s), &out); err != nil {
				return []any{err.Error()}
			}
			return out
		},
		"nindent": func(n int, s string) string {
			pad := strings.Repeat(" ", n)
			return "\n" + pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"indent": func(n int, s string) string {
			pad := strings.Repeat(" ", n)
			return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"quote": func(args ...any) string {
			var out []string
			for _, a := range args {
				if a != nil {
					out = append(out, strconv.Quote(fmt.Sprint(a)))
				}
			}
			return strings.Join(out, " ")
		},
		"default": func(d any, given ...any) any {
			if len(given) == 0 || empty(given[0]) {
				return d
			}
			return given[0]
		},
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			return m
		},
		"list":   func(v ...any) []any { return v },
		"append": func(l []any, v any) []any { return append(append([]any(nil), l...), v) },
		"concat": func(ls ...[]any) []any {
			var out []any
			for _, l := range ls {
				out = append(out, l...)
			}
			return out
		},
		"set":    func(m map[string]any, k string, v any) map[string]any { m[k] = v; return m },
		"hasKey": func(m map[string]any, k string) bool { _, ok := m[k]; return ok },
		"trunc": func(n int, s string) string {
			if len(s) > n {
				return s[:n]
			}
			return s
		},
		"trimSuffix": func(suf, s string) string { return strings.TrimSuffix(s, suf) },
		"trim":       strings.TrimSpace,
		"replace":    func(o, n, s string) string { return strings.ReplaceAll(s, o, n) },
		"contains":   func(sub, s string) bool { return strings.Contains(s, sub) },
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"toString":   func(v any) string { return fmt.Sprint(v) },
		"int": func(v any) int {
			switch x := v.(type) {
			case float64:
				return int(x)
			case int:
				return x
			case string:
				i, _ := strconv.Atoi(x)
				return i
			}
			return 0
		},
		"sha256sum":    func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) },
		"randAlphaNum": func(n int) string { return strings.Repeat("x", n) },
		"b64enc":       func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		"b64dec":       func(s string) string { b, _ := base64.StdEncoding.DecodeString(s); return string(b) },
		"lookup":       func(...any) map[string]any { return map[string]any{} },
		"regexMatch":   func(re, s string) bool { return regexp.MustCompile(re).MatchString(s) },
		"empty":        empty,
		"ternary": func(a, b any, c bool) any {
			if c {
				return a
			}
			return b
		},
	}).Option("missingkey=zero")

	files, _ := filepath.Glob(filepath.Join(chartDir, "templates", "*"))
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tpl.New("observex/templates/" + filepath.Base(f)).Parse(string(b)); err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(f), err)
		}
	}
	var out bytes.Buffer
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasPrefix(base, "_") || strings.HasSuffix(base, ".txt") {
			continue
		}
		if err := tpl.ExecuteTemplate(&out, "observex/templates/"+base, top); err != nil {
			return nil, err
		}
		out.WriteString("\n---\n")
	}
	return splitDocs(t, strings.ReplaceAll(out.String(), "<no value>", ""))
}

func splitDocs(t *testing.T, text string) ([]map[string]any, error) {
	t.Helper()
	var docs []map[string]any
	for _, part := range regexp.MustCompile(`(?m)^---\s*$`).Split(text, -1) {
		if strings.TrimSpace(part) == "" {
			continue
		}
		d := map[string]any{}
		if err := yaml.Unmarshal([]byte(part), &d); err != nil {
			return nil, fmt.Errorf("rendered YAML does not parse: %w", err)
		}
		if len(d) > 0 {
			docs = append(docs, d)
		}
	}
	return docs, nil
}

// f61Docs returns the documents for the shared F6.1 test configuration: from
// real Helm output when HelmOutputEnv is set, from the stand-in otherwise.
// The second result says which.
func f61Docs(t *testing.T) ([]map[string]any, string) {
	t.Helper()
	if p := os.Getenv(HelmOutputEnv); p != "" {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		docs, err := splitDocs(t, string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return docs, "helm template output " + p
	}
	docs, err := renderStandIn(t, "testdata/f61-kind-values.yaml")
	if err != nil {
		t.Fatalf("stand-in render: %v", err)
	}
	return docs, "stand-in renderer (NOT Helm)"
}
