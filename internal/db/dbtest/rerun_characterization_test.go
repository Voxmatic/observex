package dbtest

// Characterization of the CURRENT interim migration runner
// (scripts/db-migrate.sh, PROPOSED MIG-1). The runner applies every file on
// every run; schema_migrations is written after each file but never read.
// These tests record what an ordinary later run does to live data.
//
// They document behaviour; they do not endorse it. The once-only ledger
// proposed in MIG-1a (UNRESOLVED, not approved) would invert the four
// TestCharacterizationRerun* assertions. Until that decision is made, these
// tests keep the current behaviour visible, so a change to it cannot go
// unnoticed. TestFailedMigrationIsNotRecorded holds under both policies.
//
// Like the other tests in this package they need OBSERVEX_TEST_POSTGRES_DSN
// and the psql client, and they skip without them.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// runMigrateDir runs scripts/db-migrate.sh against dir and returns its error
// and output instead of failing the test.
func runMigrateDir(t *testing.T, dsn, dir string) (string, error) {
	t.Helper()
	needPsql(t)
	script := filepath.Join(MigrationsDir(), "..", "..", "..", "scripts", "db-migrate.sh")
	cmd := exec.Command("sh", script, dir)
	cmd.Env = append(os.Environ(), "POSTGRES_DSN="+dsn, "DB_MIGRATE_WAIT_SECONDS=5")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func exec1(t *testing.T, conn *pgx.Conn, sql string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", strings.Fields(sql)[0], err)
	}
}

func count(t *testing.T, conn *pgx.Conn, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// migrated returns a freshly migrated database and a connection to it.
func migrated(t *testing.T) (string, *pgx.Conn) {
	t.Helper()
	dsn := Empty(t)
	runMigrateScript(t, dsn)
	return dsn, connect(t, dsn)
}

// A version recorded as applied is executed again: 001's template-dashboard
// seed reappears after the rows were deleted.
func TestCharacterizationRerunIgnoresLedger(t *testing.T) {
	dsn, conn := migrated(t)
	if count(t, conn, `SELECT count(*) FROM schema_migrations WHERE version = '001_initial'`) != 1 {
		t.Fatal("001_initial not recorded after the first run")
	}
	exec1(t, conn, `DELETE FROM dashboards WHERE is_template`)
	runMigrateScript(t, dsn)
	if n := count(t, conn, `SELECT count(*) FROM dashboards WHERE is_template`); n == 0 {
		t.Fatal("current behaviour changed: 001 was not re-executed on the second run")
	} else {
		t.Logf("CURRENT BEHAVIOUR: 001 re-executed although recorded; %d template dashboards re-created", n)
	}
}

// A deleted default administrator is re-created by 001's seed, with role
// admin and 001's fixed password hash.
func TestCharacterizationRerunRecreatesDeletedDefaultAdmin(t *testing.T) {
	dsn, conn := migrated(t)
	exec1(t, conn, `INSERT INTO users (id, email, name, password_hash, role) VALUES ('u-owner', 'owner@example.test', 'Owner', 'x', 'admin')`)
	exec1(t, conn, `UPDATE orgs SET owner_id = 'u-owner' WHERE owner_id = 'usr-admin-default'`)
	exec1(t, conn, `DELETE FROM dashboards WHERE owner_id = 'usr-admin-default'`)
	exec1(t, conn, `DELETE FROM users WHERE email = 'admin@observex.io'`)
	runMigrateScript(t, dsn)
	var role, hash string
	err := conn.QueryRow(context.Background(), `SELECT role, password_hash FROM users WHERE email = 'admin@observex.io'`).Scan(&role, &hash)
	if err != nil {
		t.Fatalf("current behaviour changed: the default admin was not re-created (%v)", err)
	}
	if role != "admin" || !strings.HasPrefix(hash, "$2a$12$LQv3c1yq") {
		t.Fatalf("re-created admin differs from 001's seed: role %q", role)
	}
	t.Log("CURRENT BEHAVIOUR: deleted default admin re-created with role admin and 001's fixed hash")
}

// A user without an organization is assigned to org-default by 002's backfill.
// UserStore.Create inserts users without org_id.
func TestCharacterizationRerunAssignsOrglessUserToDefaultOrg(t *testing.T) {
	dsn, conn := migrated(t)
	exec1(t, conn, `INSERT INTO users (id, email, name, password_hash, role) VALUES ('u-pending', 'pending@example.test', 'Pending', 'x', 'viewer')`)
	runMigrateScript(t, dsn)
	var org *string
	if err := conn.QueryRow(context.Background(), `SELECT org_id FROM users WHERE id = 'u-pending'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if org == nil || *org != "org-default" {
		t.Fatalf("current behaviour changed: org_id after re-run is %v", org)
	}
	t.Log("CURRENT BEHAVIOUR: user without an organization moved into org-default by the re-run")
}

// A team membership removed by an administrator is re-created from
// users.team_id by 002's backfill, with the user's organization role.
func TestCharacterizationRerunRestoresRemovedTeamMembership(t *testing.T) {
	dsn, conn := migrated(t)
	exec1(t, conn, `INSERT INTO orgs (id, name, owner_id) VALUES ('org-b', 'tenant-b', 'u-b1')`)
	exec1(t, conn, `INSERT INTO teams (id, name, org_id) VALUES ('t-b', 'team-b', 'org-b')`)
	exec1(t, conn, `INSERT INTO users (id, email, name, password_hash, role, team_id, org_id) VALUES ('u-b1', 'b1@example.test', 'B1', 'x', 'admin', 't-b', 'org-b')`)
	exec1(t, conn, `INSERT INTO team_members (team_id, user_id, role) VALUES ('t-b', 'u-b1', 'viewer') ON CONFLICT DO NOTHING`)
	exec1(t, conn, `DELETE FROM team_members WHERE user_id = 'u-b1'`)
	runMigrateScript(t, dsn)
	var role string
	if err := conn.QueryRow(context.Background(), `SELECT role FROM team_members WHERE team_id = 't-b' AND user_id = 'u-b1'`).Scan(&role); err != nil {
		t.Fatalf("current behaviour changed: the removed membership was not restored (%v)", err)
	}
	if role != "admin" {
		t.Fatalf("restored membership role %q, want the user's org role admin", role)
	}
	t.Log("CURRENT BEHAVIOUR: removed team membership restored with role admin")
}

// A failing migration is rolled back as a whole, gets no ledger entry, and
// stops the run before later files. Required under both policies.
func TestFailedMigrationIsNotRecorded(t *testing.T) {
	dir := t.TempDir()
	for _, f := range MigrationFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	broken := "CREATE TABLE f61char_marker (id int);\nSELECT this_is_not_sql;\n"
	if err := os.WriteFile(filepath.Join(dir, "010_broken.sql"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "011_after.sql"), []byte("CREATE TABLE f61char_after (id int);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dsn := Empty(t)
	if out, err := runMigrateDir(t, dsn, dir); err == nil {
		t.Fatalf("a failing migration did not stop the run:\n%s", out)
	}
	conn := connect(t, dsn)
	if n := count(t, conn, `SELECT count(*) FROM schema_migrations WHERE version IN ('010_broken', '011_after')`); n != 0 {
		t.Fatalf("%d ledger entries for the failed or skipped files", n)
	}
	if n := count(t, conn, `SELECT count(*) FROM pg_tables WHERE tablename IN ('f61char_marker', 'f61char_after')`); n != 0 {
		t.Fatal("the failed file was not rolled back as a whole, or a later file ran")
	}
	if n := count(t, conn, `SELECT count(*) FROM schema_migrations`); n != len(MigrationFiles(t)) {
		t.Fatalf("ledger has %d entries, want the %d files before the failure", n, len(MigrationFiles(t)))
	}
}
