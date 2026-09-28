// Package chartcheck holds STATIC checks of the ObserveX Helm chart's F6.1
// resources: the probe endpoint and certificate identity (D1), the migration
// Job's network policy (D2), and which Secret each database client reads.
//
// Static means: rendered manifests are inspected; nothing is deployed. The
// policy evaluation below implements the NetworkPolicy selection and union
// rules for the label and port cases this chart uses. It says what a
// conforming network plugin WOULD allow; it cannot show that a cluster's
// plugin enforces anything. See render_test.go for how manifests are produced.
package chartcheck

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

type doc = map[string]any

func get(m any, path ...string) any {
	for _, p := range path {
		mm, ok := m.(map[string]any)
		if !ok {
			return nil
		}
		m = mm[p]
	}
	return m
}

func str(m any, path ...string) string { s, _ := get(m, path...).(string); return s }

func list(m any, path ...string) []any { l, _ := get(m, path...).([]any); return l }

func find(docs []doc, kind, name string) doc {
	for _, d := range docs {
		if d["kind"] == kind && str(d, "metadata", "name") == name {
			return d
		}
	}
	return nil
}

func mustFind(t *testing.T, docs []doc, kind, name string) doc {
	t.Helper()
	d := find(docs, kind, name)
	if d == nil {
		t.Fatalf("%s %s not rendered", kind, name)
	}
	return d
}

func labelsOf(m any) map[string]string {
	out := map[string]string{}
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

func envOf(container any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, e := range list(container, "env") {
		em := e.(map[string]any)
		out[str(em, "name")] = em
	}
	return out
}

// selects implements a podSelector (matchLabels and matchExpressions).
func selects(selector any, labels map[string]string) bool {
	for k, v := range labelsOf(get(selector, "matchLabels")) {
		if labels[k] != v {
			return false
		}
	}
	for _, e := range list(selector, "matchExpressions") {
		key, op := str(e, "key"), str(e, "operator")
		val, has := labels[key]
		in := false
		for _, x := range list(e, "values") {
			if fmt.Sprint(x) == val {
				in = true
			}
		}
		switch op {
		case "In":
			if !has || !in {
				return false
			}
		case "NotIn":
			if has && in {
				return false
			}
		case "Exists":
			if !has {
				return false
			}
		case "DoesNotExist":
			if has {
				return false
			}
		}
	}
	return true
}

func hasType(np doc, typ string) bool {
	for _, x := range list(np, "spec", "policyTypes") {
		if x == typ {
			return true
		}
	}
	return false
}

// endpoint is a destination (or source): either a pod in the release
// namespace, identified by labels, or an address outside any pod.
type endpoint struct {
	pod map[string]string
	ip  string
}

func portMatches(rulePorts []any, port int, proto string) bool {
	if len(rulePorts) == 0 {
		return true // no ports: all ports
	}
	for _, p := range rulePorts {
		rp := fmt.Sprint(get(p, "port"))
		rproto := str(p, "protocol")
		if rproto == "" {
			rproto = "TCP"
		}
		if rp == fmt.Sprint(port) && rproto == proto {
			return true
		}
	}
	return false
}

func peerMatches(peers []any, e endpoint) bool {
	if len(peers) == 0 {
		return true // no peers: everywhere
	}
	for _, p := range peers {
		if sel := get(p, "podSelector"); sel != nil && e.pod != nil && get(p, "namespaceSelector") == nil && selects(sel, e.pod) {
			return true
		}
		if blk := get(p, "ipBlock"); blk != nil && e.ip != "" && e.pod == nil {
			return true // this chart's ipBlocks are 0.0.0.0/0-style; exceptions are checked separately
		}
	}
	return false
}

// policiesFor returns the policies of the given type that select the pod.
func policiesFor(docs []doc, pod map[string]string, typ string) []doc {
	var out []doc
	for _, d := range docs {
		if d["kind"] == "NetworkPolicy" && hasType(d, typ) && selects(get(d, "spec", "podSelector"), pod) {
			out = append(out, d)
		}
	}
	return out
}

// egressAllowed: the union of the egress rules of every policy that selects
// src; a pod selected by no egress policy is unrestricted.
func egressAllowed(docs []doc, src map[string]string, dst endpoint, port int, proto string) (bool, []string) {
	pols := policiesFor(docs, src, "Egress")
	var names []string
	for _, p := range pols {
		names = append(names, str(p, "metadata", "name"))
	}
	if len(pols) == 0 {
		return true, names
	}
	for _, p := range pols {
		for _, r := range list(p, "spec", "egress") {
			if portMatches(list(r, "ports"), port, proto) && peerMatches(list(r, "to"), dst) {
				return true, names
			}
		}
	}
	return false, names
}

func ingressAllowed(docs []doc, dst map[string]string, src endpoint, port int, proto string) bool {
	pols := policiesFor(docs, dst, "Ingress")
	if len(pols) == 0 {
		return true
	}
	for _, p := range pols {
		for _, r := range list(p, "spec", "ingress") {
			if portMatches(list(r, "ports"), port, proto) && peerMatches(list(r, "from"), src) {
				return true
			}
		}
	}
	return false
}

// Bitnami PostgreSQL primary pod labels (the sub-chart is not rendered here;
// the chart's own policies select PostgreSQL by app.kubernetes.io/name).
var postgresPod = map[string]string{"app.kubernetes.io/name": "postgresql", "app.kubernetes.io/instance": releaseName, "app.kubernetes.io/component": "primary"}

func podLabels(t *testing.T, d doc) map[string]string {
	t.Helper()
	l := labelsOf(get(d, "spec", "template", "metadata", "labels"))
	if len(l) == 0 {
		t.Fatalf("%s has no pod labels", str(d, "metadata", "name"))
	}
	return l
}

// ── D1: probe endpoint and certificate identity ─────────────────────────────

func TestD1ProbeDialsTheCertificateName(t *testing.T) {
	docs, source := f61Docs(t)
	t.Logf("manifests from %s", source)
	svc := mustFind(t, docs, "Service", "observex-processor-probe-intake")
	tlsName := str(svc, "metadata", "annotations", "observex.io/tls-server-name")
	want := "observex-processor-probe-intake." + namespace + ".svc"
	if tlsName != want {
		t.Fatalf("intake Service tls-server-name annotation %q, want %q", tlsName, want)
	}
	if ns := str(svc, "metadata", "namespace"); ns != namespace {
		t.Fatalf("intake Service namespace %q", ns)
	}
	svcPort := fmt.Sprint(get(list(svc, "spec", "ports")[0], "port"))

	probe := mustFind(t, docs, "Deployment", "observex-probe-kind-a")
	c := list(probe, "spec", "template", "spec", "containers")[0]
	u, err := url.Parse(str(envOf(c)["OBSERVEX_PROCESSOR_URL"], "value"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" {
		t.Fatalf("probe URL scheme %q, want https", u.Scheme)
	}
	if u.Hostname() != tlsName {
		t.Fatalf("probe dials %q but the certificate name is %q (D1)", u.Hostname(), tlsName)
	}
	if u.Port() != svcPort {
		t.Fatalf("probe port %s, Service port %s", u.Port(), svcPort)
	}
	// The name is the Service's own name plus namespace and "svc": it names
	// no cluster domain.
	if strings.Count(u.Hostname(), ".") != 2 || !strings.HasPrefix(u.Hostname(), str(svc, "metadata", "name")+".") {
		t.Fatalf("probe host %q is not <service>.<namespace>.svc", u.Hostname())
	}
	if env := envOf(c); env["OBSERVEX_PROCESSOR_CA_FILE"] == nil || env["OBSERVEX_PROBE_ALLOW_PLAINTEXT"] != nil {
		t.Fatal("probe must verify the listener with the configured CA and must not allow plaintext")
	}
}

// Stand-in only: other supported modes keep the same identity rules.
func TestD1OtherModes(t *testing.T) {
	base := "testdata/f61-kind-values.yaml"
	probeURL := func(docs []doc) string {
		c := list(mustFind(t, docs, "Deployment", "observex-probe-kind-a"), "spec", "template", "spec", "containers")[0]
		return str(envOf(c)["OBSERVEX_PROCESSOR_URL"], "value")
	}
	// Development plaintext: same host, http.
	docs, err := renderStandIn(t, base, map[string]any{"f61": map[string]any{"intake": map[string]any{"tlsSecret": "", "insecurePlaintext": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := probeURL(docs); got != "http://observex-processor-probe-intake.observex.svc:8443" {
		t.Fatalf("plaintext mode URL %q", got)
	}
	// An explicit processor URL (probes outside the cluster) is used as given.
	docs, err = renderStandIn(t, base, map[string]any{"f61": map[string]any{"probe": map[string]any{"processorUrl": "https://probes.example.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := probeURL(docs); got != "https://probes.example.test" {
		t.Fatalf("override URL %q", got)
	}
	// A long release fullname: the Service name stays a valid DNS label and
	// the TLS name is derived from the same (truncated) name.
	long := strings.Repeat("a", 50)
	docs, err = renderStandIn(t, base, map[string]any{"fullnameOverride": long})
	if err != nil {
		t.Fatal(err)
	}
	var svc doc
	for _, d := range docs {
		if d["kind"] == "Service" && str(d, "metadata", "labels", "observex.io/f61") == "probe-intake" {
			svc = d
		}
	}
	if svc == nil {
		t.Fatal("intake Service not rendered")
	}
	name := str(svc, "metadata", "name")
	if len(name) > 63 || strings.HasSuffix(name, "-") {
		t.Fatalf("intake Service name %q is not a valid DNS label", name)
	}
	c := list(mustFind(t, docs, "Deployment", long+"-probe-kind-a"), "spec", "template", "spec", "containers")[0]
	u, _ := url.Parse(str(envOf(c)["OBSERVEX_PROCESSOR_URL"], "value"))
	if u.Hostname() != name+".observex.svc" || str(svc, "metadata", "annotations", "observex.io/tls-server-name") != u.Hostname() {
		t.Fatalf("long name: probe host %q, Service %q", u.Hostname(), name)
	}
}

// ── D2: migration Job network access ────────────────────────────────────────

func TestD2MigrationJobReachesPostgreSQLOnly(t *testing.T) {
	docs, source := f61Docs(t)
	t.Logf("manifests from %s", source)
	job := mustFind(t, docs, "Job", "observex-db-migrate")
	pod := podLabels(t, job)

	ok, pols := egressAllowed(docs, pod, endpoint{pod: postgresPod}, 5432, "TCP")
	t.Logf("egress policies selecting the Job pod: %v", pols)
	if !ok {
		t.Fatal("migration Job cannot reach PostgreSQL on 5432 (D2)")
	}
	if ok, _ := egressAllowed(docs, pod, endpoint{ip: "10.96.0.10"}, 53, "UDP"); !ok {
		t.Fatal("migration Job cannot resolve names (DNS 53/UDP)")
	}
	// Nothing else: not other chart pods, not other ports, not outside addresses.
	for _, c := range []struct {
		dst   endpoint
		port  int
		proto string
		what  string
	}{
		{endpoint{pod: map[string]string{"app.kubernetes.io/name": "observex", "app.kubernetes.io/instance": releaseName, "app.kubernetes.io/component": "processor"}}, 5432, "TCP", "processor pod on 5432"},
		{endpoint{pod: map[string]string{"app.kubernetes.io/name": "observex", "app.kubernetes.io/instance": releaseName, "app.kubernetes.io/component": "processor"}}, 8443, "TCP", "processor probe listener"},
		{endpoint{pod: postgresPod}, 5433, "TCP", "PostgreSQL pod on another port"},
		{endpoint{ip: "93.184.216.34"}, 443, "TCP", "an outside address on 443"},
		{endpoint{ip: "169.254.169.254"}, 80, "TCP", "the cloud metadata address"},
		{endpoint{ip: "10.0.0.5"}, 5432, "TCP", "a PostgreSQL port at an arbitrary address"},
	} {
		if ok, _ := egressAllowed(docs, pod, c.dst, c.port, c.proto); ok {
			t.Errorf("migration Job may reach %s", c.what)
		}
	}
	// PostgreSQL's side: the storage policy admits chart pods, the Job included.
	if !ingressAllowed(docs, postgresPod, endpoint{pod: pod}, 5432, "TCP") {
		t.Fatal("PostgreSQL ingress does not admit the migration Job")
	}
	// The probes must still not reach PostgreSQL.
	probe := podLabels(t, mustFind(t, docs, "Deployment", "observex-probe-kind-a"))
	if ok, _ := egressAllowed(docs, probe, endpoint{pod: postgresPod}, 5432, "TCP"); ok {
		t.Fatal("probe pods may reach PostgreSQL")
	}
}

// Stand-in only: the Job policy exists only with both the Job and policies on.
func TestD2PolicyOnlyWhenJobAndPoliciesEnabled(t *testing.T) {
	base := "testdata/f61-kind-values.yaml"
	for name, over := range map[string]map[string]any{
		"job off":      {"f61": map[string]any{"migrations": map[string]any{"job": map[string]any{"enabled": false}}}},
		"policies off": {"networkPolicy": map[string]any{"enabled": false}},
	} {
		docs, err := renderStandIn(t, base, over)
		if err != nil {
			t.Fatal(err)
		}
		if find(docs, "NetworkPolicy", "observex-f61-db-migrate") != nil {
			t.Errorf("%s: migration Job policy rendered", name)
		}
	}
	// Defaults (values.yaml alone): nothing F6.1 at all.
	docs, err := renderStandIn(t, map[string]any{"image": map[string]any{"tag": "x"}, "trivyScanner": map[string]any{"enabled": false}})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if strings.Contains(str(d, "metadata", "name"), "f61") || d["kind"] == "Job" {
			t.Errorf("default render contains %s %s", d["kind"], str(d, "metadata", "name"))
		}
	}
}

// ── Database credential sources (Workstream D, characterization) ────────────

// Records which Secret every chart-managed database client reads, and what the
// parent chart passes to the PostgreSQL sub-chart. It does not render the
// sub-chart; see docs/validation/f61-ops-1-kind-validation.md for what the
// sub-chart does with these values.
func TestCharacterizationDatabaseCredentialSources(t *testing.T) {
	docs, source := f61Docs(t)
	t.Logf("manifests from %s", source)
	ref := func(env map[string]map[string]any, name string) string {
		e := env[name]
		if e == nil {
			return ""
		}
		if _, literal := e["value"]; literal {
			t.Fatalf("%s is a literal value, not a Secret reference", name)
		}
		return str(e, "valueFrom", "secretKeyRef", "name") + "/" + str(e, "valueFrom", "secretKeyRef", "key")
	}
	gw := envOf(list(mustFind(t, docs, "Deployment", "observex-api-gateway"), "spec", "template", "spec", "containers")[0])
	proc := envOf(list(mustFind(t, docs, "Deployment", "observex-processor"), "spec", "template", "spec", "containers")[0])
	job := envOf(list(mustFind(t, docs, "Job", "observex-db-migrate"), "spec", "template", "spec", "containers")[0])
	g, p, j := ref(gw, "POSTGRES_PASSWORD"), ref(proc, "F61_POSTGRES_PASSWORD"), ref(job, "F61_POSTGRES_PASSWORD")
	if g != "observex-secrets/postgres-password" || p != g || j != g {
		t.Fatalf("database password sources differ: gateway %q, processor %q, job %q", g, p, j)
	}
	if dsn := str(proc["OBSERVEX_F61_POSTGRES_DSN"], "value"); !strings.Contains(dsn, "$(F61_POSTGRES_PASSWORD)") {
		t.Fatal("processor DSN does not take the password from the Secret-backed variable")
	}
	if _, ok := proc["POSTGRES_DSN"]; ok {
		t.Fatal("processor has POSTGRES_DSN (would start the legacy in-process scheduler)")
	}
	// No password-like variable anywhere is a literal.
	for _, d := range docs {
		for _, c := range list(d, "spec", "template", "spec", "containers") {
			for name, e := range envOf(c) {
				sensitive := strings.Contains(name, "PASSWORD") || strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET")
				if sensitive && !strings.HasSuffix(name, "_FILE") { // *_FILE values are paths, not secrets
					if _, literal := e["value"]; literal {
						t.Errorf("%s: %s is a literal value", str(d, "metadata", "name"), name)
					}
				}
			}
		}
	}
	// What the parent passes to the PostgreSQL sub-chart (values.yaml only).
	values := readYAML(t, chartDir+"/values.yaml")
	auth := get(values, "postgresql", "auth")
	t.Logf("CURRENT: postgresql.auth.username=%q password-set=%v existingSecret=%q",
		str(auth, "username"), str(auth, "password") != "", str(auth, "existingSecret"))
	if str(auth, "password") != "" || str(auth, "existingSecret") != "" {
		t.Fatal("characterization changed: the parent now passes a credential to the sub-chart; update the report and this test")
	}
}

// Stand-in only: the disposable kind configuration renders the F6.1 pieces
// and leaves out the components the runbook disables.
func TestKindRuntimeValuesRender(t *testing.T) {
	docs, err := renderStandIn(t, "testdata/f61-kind-values.yaml", "testdata/f61-kind-runtime-values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][2]string{
		{"Deployment", "observex-api-gateway"}, {"Deployment", "observex-processor"}, {"Deployment", "observex-probe-kind-a"},
		{"Service", "observex-processor-probe-intake"}, {"Job", "observex-db-migrate"},
		{"NetworkPolicy", "observex-f61-probe"}, {"NetworkPolicy", "observex-f61-probe-private-targets"},
		{"NetworkPolicy", "observex-f61-processor"}, {"NetworkPolicy", "observex-f61-db-migrate"},
	} {
		mustFind(t, docs, want[0], want[1])
	}
	for _, gone := range []string{"observex-ingestor", "observex-ai-agent", "observex-query-engine"} {
		if find(docs, "Deployment", gone) != nil {
			t.Errorf("%s rendered although disabled", gone)
		}
	}
	for _, d := range docs {
		if d["kind"] == "DaemonSet" {
			t.Errorf("DaemonSet %s rendered although oneagent is disabled", str(d, "metadata", "name"))
		}
	}
	// The kind vantage is explicitly internal; the probe image tag is the local build.
	probe := mustFind(t, docs, "Deployment", "observex-probe-kind-a")
	if str(probe, "spec", "template", "metadata", "labels", "observex.io/private-targets") != "true" {
		t.Fatal("kind vantage is not marked for private targets")
	}
	c := list(probe, "spec", "template", "spec", "containers")[0]
	if !strings.HasSuffix(str(c, "image"), ":f61-validation") {
		t.Fatalf("probe image %q", str(c, "image"))
	}
}
