// internal/db/store/store.go
// PostgreSQL data access layer for ObserveX.
// All stores are thin wrappers over pgx connection pool.
package store

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	m "github.com/observex/platform/internal/db/models"
)

// ── DB ────────────────────────────────────────────────────────────────────────

type DBConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

func DefaultConfig() DBConfig {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			envOr("POSTGRES_USER", "observex"),
			envOr("POSTGRES_PASSWORD", "observex"),
			envOr("POSTGRES_HOST", "postgres"),
			envOr("POSTGRES_PORT", "5432"),
			envOr("POSTGRES_DB", "observex"),
		)
	}
	return DBConfig{
		DSN: dsn, MaxConns: 30, MinConns: 2,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
}

type DB struct {
	Pool *pgxpool.Pool
	log  *zap.Logger
}

func Open(ctx context.Context, cfg DBConfig, log *zap.Logger) (*DB, error) {
	config, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}
	config.MaxConns = cfg.MaxConns
	config.MinConns = cfg.MinConns
	config.MaxConnLifetime = cfg.MaxConnLifetime
	config.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	log.Info("database connected", zap.String("dsn", maskDSN(cfg.DSN)))
	return &DB{Pool: pool, log: log}, nil
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

func (db *DB) Close() {
	db.Pool.Close()
}

func maskDSN(dsn string) string {
	if len(dsn) > 20 { return dsn[:20] + "***" }
	return "***"
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" { return v }
	return d
}

// ── User types ────────────────────────────────────────────────────────────────

type User struct {
	ID           string     `db:"id"`
	OrgID        string     `db:"org_id"`
	Email        string     `db:"email"`
	Name         string     `db:"name"`
	PasswordHash string     `db:"password_hash"`
	Role         m.Role     `db:"role"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
}

func (u *User) Public() m.UserPublic {
	return m.UserPublic{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, OrgID: u.OrgID, CreatedAt: u.CreatedAt}
}

// ── Org types ─────────────────────────────────────────────────────────────────

type Org struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	Plan      string    `db:"plan"`
	Settings  string    `db:"settings"`
	CreatedAt time.Time `db:"created_at"`
}

// ── Team types ────────────────────────────────────────────────────────────────

type Team struct {
	ID        string    `db:"id"`
	OrgID     string    `db:"org_id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

type TeamMember struct {
	TeamID string `db:"team_id"`
	UserID string `db:"user_id"`
	Role   m.Role `db:"role"`
}

// ── Session types ─────────────────────────────────────────────────────────────

type Session struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	OrgID     string    `db:"org_id"`
	Token     string    `db:"token"`
	IPAddress string    `db:"ip_address"`
	UserAgent string    `db:"user_agent"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}

// ── API Key types ─────────────────────────────────────────────────────────────

type APIKey struct {
	ID        string     `db:"id"`
	UserID    string     `db:"user_id"`
	OrgID     string     `db:"org_id"`
	Name      string     `db:"name"`
	KeyHash   string     `db:"key_hash"`
	Scopes    []string   `db:"scopes"`
	ExpiresAt *time.Time `db:"expires_at"`
	CreatedAt time.Time  `db:"created_at"`
}

// ── Invitation types ──────────────────────────────────────────────────────────

type Invitation struct {
	ID        string     `db:"id"`
	OrgID     string     `db:"org_id"`
	Email     string     `db:"email"`
	Role      m.Role     `db:"role"`
	TeamID    *string    `db:"team_id"`
	Token     string     `db:"token"`
	ExpiresAt time.Time  `db:"expires_at"`
	AcceptedAt *time.Time `db:"accepted_at"`
	CreatedAt  time.Time  `db:"created_at"`
}

// ── NamespacePerm types ───────────────────────────────────────────────────────

type NamespacePerm struct {
	TeamID    string    `db:"team_id"`
	OrgID     string    `db:"org_id"`
	Namespace string    `db:"namespace"`
	Access    m.Access  `db:"access"`
}

// ── AuthContext types ─────────────────────────────────────────────────────────

type AuthContext struct {
	UserID          string
	UserEmail       string
	Email           string
	Name            string
	OrgID           string
	Role            m.Role
	EffectiveRole   m.Role
	TeamIDs         []string
	NamespaceAccess map[string]m.Access
	IsOrgAdmin      bool
}

func (a *AuthContext) Get(ns string) m.Access   { return a.NamespaceAccess[ns] }
func (a *AuthContext) Post(ns string) bool       { return a.NamespaceAccess[ns] == m.AccessWrite }
func (a *AuthContext) Put(ns string) bool        { return a.NamespaceAccess[ns] == m.AccessWrite }

// ── PasswordReset types ───────────────────────────────────────────────────────

type PasswordReset struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	Token     string    `db:"token"`
	ExpiresAt time.Time `db:"expires_at"`
	UsedAt    *time.Time `db:"used_at"`
}

