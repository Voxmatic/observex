// Package dbtest provides disposable PostgreSQL databases for integration
// tests. It is used only by tests; nothing in a service imports it.
//
// Tests that need a database call NewDatabase, which skips the test unless
// OBSERVEX_TEST_POSTGRES_DSN points at a PostgreSQL server where the user may
// create and drop databases. Each call creates a fresh database, applies the
// repository's migrations in order exactly as internal/db/migrations holds
// them, and drops the database when the test ends.
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvDSN names the environment variable holding the server DSN.
const EnvDSN = "OBSERVEX_TEST_POSTGRES_DSN"

// MigrationsDir returns the repository's migrations directory.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "migrations")
}

// MigrationFiles returns the migration files in application order.
func MigrationFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(MigrationsDir(), "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	return files
}

// Empty creates an empty database and returns its DSN. It skips the test
// when no server is configured.
func Empty(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skip("set " + EnvDSN + " to a disposable PostgreSQL server to run this test")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	name := fmt.Sprintf("f61test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close(ctx)
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close(context.Background())
	})
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatalf("%s must be a postgres:// URL", EnvDSN)
	}
	u.Path = "/" + name
	return u.String()
}

// Apply runs each file's full text as one simple-protocol execution, the way
// psql -f with ON_ERROR_STOP would, and fails the test on the first error.
func Apply(t testing.TB, dsn string, files []string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
}

// NewDatabase creates a database with every migration applied and returns a
// pool on it.
func NewDatabase(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := Empty(t)
	Apply(t, dsn, MigrationFiles(t))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Named returns the migration files whose base names start with any prefix.
func Named(t testing.TB, prefixes ...string) []string {
	t.Helper()
	var out []string
	for _, f := range MigrationFiles(t) {
		for _, p := range prefixes {
			if strings.HasPrefix(filepath.Base(f), p) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}
