package dbtest

// MIG-1 tests: fresh initialisation, and upgrade of every database state the
// original migrations can have produced. They need a PostgreSQL server
// (OBSERVEX_TEST_POSTGRES_DSN) and the psql client, because the states being
// reproduced are exactly what psql -f produces, statement by statement.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func needPsql(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("psql")
	if err != nil {
		t.Skip("psql client not found")
	}
	return p
}

// psqlFile runs psql -f; stopOnError reproduces docker-entrypoint-initdb.d.
func psqlFile(t *testing.T, dsn, file string, stopOnError bool) error {
	t.Helper()
	args := []string{dsn, "-q", "-f", file}
	if stopOnError {
		args = append(args, "-v", "ON_ERROR_STOP=1")
	}
	cmd := exec.Command(needPsql(t), args...)
	cmd.Env = append(os.Environ(), "PGOPTIONS=-c client_min_messages=error")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("psql %s: %s", filepath.Base(file), firstLine(string(out)))
	}
	return err
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "ERROR") {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

func runMigrateScript(t *testing.T, dsn string) {
	t.Helper()
	needPsql(t)
	script := filepath.Join(MigrationsDir(), "..", "..", "..", "scripts", "db-migrate.sh")
	cmd := exec.Command("sh", script, MigrationsDir())
	cmd.Env = append(os.Environ(), "POSTGRES_DSN="+dsn, "DB_MIGRATE_WAIT_SECONDS=5")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("db-migrate.sh: %v\n%s", err, out)
	}
}

// fingerprint describes the public schema independently of creation order.
func fingerprint(t *testing.T, dsn string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var fp string
	err = conn.QueryRow(ctx, `
		SELECT string_agg(x, E'\n' ORDER BY x) FROM (
		  SELECT 'col ' || table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable || ' ' || coalesce(column_default, '') AS x
		    FROM information_schema.columns WHERE table_schema = 'public'
		  UNION ALL
		  SELECT 'con ' || c.relname || ' ' || k.conname || ' ' || pg_get_constraintdef(k.oid)
		    FROM pg_constraint k JOIN pg_class c ON c.oid = k.conrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		   WHERE n.nspname = 'public'
		  UNION ALL
		  SELECT 'idx ' || indexdef FROM pg_indexes WHERE schemaname = 'public'
		  UNION ALL
		  SELECT 'typ ' || t.typname || ' ' || coalesce((SELECT string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder) FROM pg_enum e WHERE e.enumtypid = t.oid), '')
		    FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype = 'e'
		  UNION ALL
		  SELECT 'fn ' || p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public'
		  UNION ALL
		  SELECT 'trg ' || tgname FROM pg_trigger WHERE NOT tgisinternal
		) s`).Scan(&fp)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

func freshFingerprint(t *testing.T) string {
	t.Helper()
	dsn := Empty(t)
	runMigrateScript(t, dsn)
	return fingerprint(t, dsn)
}

func TestFreshInitialisationSucceeds(t *testing.T) {
	dsn := Empty(t)
	for _, f := range MigrationFiles(t) {
		if err := psqlFile(t, dsn, f, true); err != nil {
			t.Fatalf("fresh initialisation stopped at %s", filepath.Base(f))
		}
	}
	fp := fingerprint(t, dsn)
	for _, want := range []string{
		"col slos.window text NO",
		"PRIMARY KEY (id, captured_at)",
		"col synthetic_checks.locations ARRAY NO",
		"col f61_tls_results.alert_state text NO",
		"con f61_tls_observations f61_tls_observations_check_id_org_id_fkey FOREIGN KEY (check_id, org_id) REFERENCES synthetic_checks(id, org_id) ON DELETE CASCADE",
		"col f61_vantage_revocations.not_before timestamp with time zone NO",
	} {
		if !strings.Contains(fp, want) {
			t.Errorf("fresh schema lacks %q", want)
		}
	}
}

// The two statements corrected in place could not be applied by the original
// files on this server — so no existing database can contain them.
func TestOriginalStatementsCannotSucceed(t *testing.T) {
	orig001 := filepath.Join("testdata", "pre-mig1", "001_initial.sql")
	orig005 := filepath.Join("testdata", "pre-mig1", "005_integrations_postmortems.sql")
	dsn := Empty(t)
	if err := psqlFile(t, dsn, orig001, true); err == nil {
		t.Fatal("original 001 applied; the reserved-word premise no longer holds")
	}
	dsn = Empty(t)
	for _, f := range Named(t, "001", "002", "003", "004") {
		if err := psqlFile(t, dsn, f, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := psqlFile(t, dsn, orig005, true); err == nil {
		t.Fatal("original 005 applied; the partition-key premise no longer holds")
	}
}

// E1: docker-entrypoint-initdb.d with ON_ERROR_STOP stopped inside original
// 001; the container restarts without re-running init. Re-applying the
// corrected set completes the schema exactly.
func TestUpgradeFromStoppedInitialisation(t *testing.T) {
	want := freshFingerprint(t)
	dsn := Empty(t)
	_ = psqlFile(t, dsn, filepath.Join("testdata", "pre-mig1", "001_initial.sql"), true)
	if fingerprint(t, dsn) == want {
		t.Fatal("fixture error: the stopped initialisation already has the full schema")
	}
	runMigrateScript(t, dsn)
	if got := fingerprint(t, dsn); got != want {
		t.Fatalf("upgraded schema differs from a fresh one:\n%s", diffLines(want, got))
	}
}

// E2: every original file applied without stopping on errors. Re-applying the
// corrected set completes the schema exactly.
func TestUpgradeFromTolerantApplication(t *testing.T) {
	want := freshFingerprint(t)
	dsn := Empty(t)
	for _, f := range MigrationFiles(t) {
		base := filepath.Base(f)
		if strings.HasPrefix(base, "009") {
			continue // did not exist before this change
		}
		if base == "001_initial.sql" || base == "005_integrations_postmortems.sql" {
			f = filepath.Join("testdata", "pre-mig1", base)
		}
		_ = psqlFile(t, dsn, f, false)
	}
	runMigrateScript(t, dsn)
	if got := fingerprint(t, dsn); got != want {
		t.Fatalf("upgraded schema differs from a fresh one:\n%s", diffLines(want, got))
	}
}

// Re-running the whole set leaves the SCHEMA unchanged, and schema_migrations
// records it. It is not a data no-op: see rerun_characterization_test.go.
func TestMigrationsAreIdempotent(t *testing.T) {
	dsn := Empty(t)
	runMigrateScript(t, dsn)
	first := fingerprint(t, dsn)
	runMigrateScript(t, dsn)
	if fingerprint(t, dsn) != first {
		t.Fatal("re-applying the migrations changed the schema")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var n int
	_ = conn.QueryRow(context.Background(), `SELECT count(*) FROM schema_migrations`).Scan(&n)
	if n != len(MigrationFiles(t)) {
		t.Fatalf("schema_migrations has %d rows, want %d", n, len(MigrationFiles(t)))
	}
}

func diffLines(want, got string) string {
	w := map[string]bool{}
	for _, l := range strings.Split(want, "\n") {
		w[l] = true
	}
	g := map[string]bool{}
	for _, l := range strings.Split(got, "\n") {
		g[l] = true
	}
	var b strings.Builder
	for l := range w {
		if !g[l] {
			b.WriteString("- " + l + "\n")
		}
	}
	for l := range g {
		if !w[l] {
			b.WriteString("+ " + l + "\n")
		}
	}
	return b.String()
}
