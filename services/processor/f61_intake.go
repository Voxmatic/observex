package main

// F6.1 synthetic probe service endpoints: the processor end of the
// probe-to-processor path (G-1: a dedicated probe service, one Deployment per
// vantage; F6.1-INTAKE-1(a): the processor owns intake).
//
// They are served on a DEDICATED listener (PROPOSED EGRESS-1), never on the
// internal :8080 app, whose other routes are unauthenticated:
//
//	GET  /v1/synthetic/probe/work                      work for the caller's vantage (FANOUT-1, LOC-1)
//	POST /v1/synthetic/checks/:id/tls-observations     one tlscert wire document (v1)
//
// Both require a probe credential (Authorization: Bearer oxpt_…, scope
// synthetic:report) and check the vantage's revocation bound. Every decision
// is made by internal/intake/f61; accepted observations are recorded,
// evaluated and turned into results by internal/result/f61 in one
// transaction.
//
// Configuration (environment):
//
//	OBSERVEX_SYNTHETIC_PROBE_KEY[_FILE]      probe credential key (required)
//	OBSERVEX_F61_POSTGRES_DSN                database for F6.1 only (preferred), or
//	POSTGRES_DSN                             the processor's shared database
//	OBSERVEX_F61_INTAKE_ADDR                 listen address (default :8443)
//	OBSERVEX_F61_INTAKE_TLS_CERT_FILE/_KEY_FILE   server certificate (reloaded on change)
//	OBSERVEX_F61_INTAKE_INSECURE_PLAINTEXT   "true" serves plaintext (development only)
//	OBSERVEX_F61_HORIZON                     expiry horizon, Go duration (default 720h)
//
// Without a certificate and without the explicit plaintext switch the
// listener is not started at all. Without the key, the database or a valid
// horizon, it answers 503 to everything.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	evaluate "github.com/observex/platform/internal/evaluate/f61"
	intakef61 "github.com/observex/platform/internal/intake/f61"
	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
	resultf61 "github.com/observex/platform/internal/result/f61"
)

const (
	f61WorkRoute        = "/v1/synthetic/probe/work"
	f61ObservationRoute = "/v1/synthetic/checks/:id/tls-observations"
	f61LookupTimeout    = 3 * time.Second
	f61RecordTimeout    = 10 * time.Second
	f61DefaultAddr      = ":8443"
)

// f61CheckQuery reads the columns the intake decision needs, by id alone. The
// org comparison is made by intakef61.Authorizer against the credential, so a
// report on another tenant's check is refused and logged as such (and answered
// exactly like a missing check).
const f61CheckQuery = `SELECT id, org_id, namespace, target, type, enabled, locations, interval_sec, timeout_sec
	FROM synthetic_checks WHERE id = $1`

// f61WorkQuery lists one organization's enabled ssl checks for work delivery.
const f61WorkQuery = `SELECT id, org_id, namespace, target, type, enabled, locations, interval_sec, timeout_sec
	FROM synthetic_checks WHERE org_id = $1 AND type = 'ssl' AND enabled ORDER BY id LIMIT 2000`

// pgSyntheticCheckLookup is the authoritative CheckLookup and WorkLister.
type pgSyntheticCheckLookup struct{ db resultf61.DB }

func scanCheck(row pgx.Row) (intakef61.CheckRecord, error) {
	var r intakef61.CheckRecord
	err := row.Scan(&r.Row.ID, &r.Row.OrgID, &r.Row.Namespace, &r.Row.Target, &r.Type, &r.Enabled,
		&r.Locations, &r.IntervalSec, &r.TimeoutSec)
	return r, err
}

func (l pgSyntheticCheckLookup) LookupCheck(ctx context.Context, id string) (intakef61.CheckRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, f61LookupTimeout)
	defer cancel()
	r, err := scanCheck(l.db.QueryRow(ctx, f61CheckQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return intakef61.CheckRecord{}, intakef61.ErrCheckNotFound
	}
	if err != nil {
		return intakef61.CheckRecord{}, err
	}
	return r, nil
}

func (l pgSyntheticCheckLookup) ListChecks(ctx context.Context, orgID string) ([]intakef61.CheckRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, f61LookupTimeout)
	defer cancel()
	rows, err := l.db.Query(ctx, f61WorkQuery, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intakef61.CheckRecord
	for rows.Next() {
		r, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// f61StoreReceiver records accepted observations and evaluates the result.
type f61StoreReceiver struct {
	store   *resultf61.Store
	horizon time.Duration
	logger  *zap.Logger
}

func (r f61StoreReceiver) Receive(ctx context.Context, adm intakef61.Admission, obs tlscert.Observation, receivedAt time.Time) (intakef61.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, f61RecordTimeout)
	defer cancel()
	out, err := r.store.Record(ctx, resultf61.Accepted{
		Subject: adm.Subject, IntervalSec: adm.IntervalSec, Observation: obs, ReceivedAt: receivedAt,
	}, r.horizon)
	if err != nil {
		return intakef61.Receipt{}, err
	}
	if out.Event != "" {
		r.logger.Info("f61 result transition",
			zap.String("org_id", adm.Subject.OrgID), zap.String("namespace", adm.Subject.Namespace),
			zap.String("check_id", adm.Subject.CheckID), zap.String("event", out.Event),
			zap.String("status", string(out.Verdict.Status)))
	}
	return intakef61.Receipt{Stored: out.Stored, Evaluated: out.Stored, Status: string(out.Verdict.Status)}, nil
}

// f61Deps is what the endpoints need.
type f61Deps struct {
	logger  *zap.Logger
	key     probetoken.Key
	db      resultf61.DB // nil without POSTGRES_DSN
	horizon time.Duration
	now     func() time.Time
}

// f61HorizonFromEnv reads OBSERVEX_F61_HORIZON (PROPOSED HORIZON-1).
func f61HorizonFromEnv(lookup func(string) (string, bool)) (time.Duration, error) {
	v, ok := lookup("OBSERVEX_F61_HORIZON")
	if !ok || strings.TrimSpace(v) == "" {
		return evaluate.DefaultHorizon, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < evaluate.MinHorizon || d > evaluate.MaxHorizon {
		return 0, fmt.Errorf("OBSERVEX_F61_HORIZON must be a duration between %s and %s", evaluate.MinHorizon, evaluate.MaxHorizon)
	}
	return d, nil
}

func f61Unavailable(c *fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
		"error": "synthetic probe intake is not configured", "reason": "unavailable",
	})
}

// newF61Handlers builds the work and intake handlers. Without a usable key,
// database or horizon both refuse everything with 503.
func newF61Handlers(d f61Deps) (work, intake fiber.Handler) {
	if !d.key.Configured() {
		d.logger.Warn("f61 probe endpoints disabled: probe credential key not configured",
			zap.String("source", string(d.key.Source())), zap.String("reason", string(d.key.Reason())))
		return f61Unavailable, f61Unavailable
	}
	if d.db == nil {
		d.logger.Warn("f61 probe endpoints disabled: POSTGRES_DSN not configured")
		return f61Unavailable, f61Unavailable
	}
	if (evaluate.Policy{Horizon: d.horizon, Span: time.Minute}).Validate() != nil {
		d.logger.Error("f61 probe endpoints disabled: invalid horizon")
		return f61Unavailable, f61Unavailable
	}
	store := resultf61.New(d.db)
	lookup := pgSyntheticCheckLookup{db: d.db}
	authz, err := intakef61.New(d.key, lookup, store, d.now)
	if err != nil {
		d.logger.Error("f61 probe endpoints disabled", zap.Error(err))
		return f61Unavailable, f61Unavailable
	}
	in, err := intakef61.NewIntake(authz, f61StoreReceiver{store: store, horizon: d.horizon, logger: d.logger})
	if err != nil {
		d.logger.Error("f61 probe endpoints disabled", zap.Error(err))
		return f61Unavailable, f61Unavailable
	}

	refuse := func(c *fiber.Ctx, err error, checkID string) error {
		status, reason := intakef61.StatusCode(err), intakef61.Reason(err)
		// The error chain holds sentinels and, for a database failure, the
		// driver's message. It never holds the credential or the payload.
		d.logger.Warn("f61 probe request refused", zap.String("route", c.Route().Path), zap.Int("status", status),
			zap.String("reason", reason), zap.String("check_id", boundedLogValue(checkID)), zap.Error(err))
		if status == http.StatusUnauthorized {
			c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
		}
		return c.Status(status).JSON(fiber.Map{"error": http.StatusText(status), "reason": reason})
	}

	work = func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		ws, err := authz.Work(c.UserContext(), c.Get(fiber.HeaderAuthorization), lookup)
		if err != nil {
			return refuse(c, err, "")
		}
		return c.JSON(ws)
	}
	intake = func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		_, receipt, err := in.Accept(c.UserContext(), c.Get(fiber.HeaderAuthorization), c.Params("id"), c.Body())
		if err != nil {
			return refuse(c, err, c.Params("id"))
		}
		if !receipt.Stored {
			return c.Status(http.StatusAccepted).JSON(fiber.Map{"status": "ignored", "reason": "not_newer", "stored": false, "evaluated": false})
		}
		return c.Status(http.StatusAccepted).JSON(fiber.Map{"status": "accepted", "stored": true, "evaluated": true, "result_status": receipt.Status})
	}
	d.logger.Info("f61 probe endpoints enabled", zap.Duration("horizon", d.horizon))
	return work, intake
}

// newF61IntakeApp builds the dedicated app: the two probe routes and nothing
// else.
func newF61IntakeApp(d f61Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "ObserveX Processor F6.1 probe intake", DisableStartupMessage: true,
		BodyLimit: tlscert.MaxWireBytes + 1, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	})
	work, intake := newF61Handlers(d)
	app.Get(f61WorkRoute, work)
	app.Post(f61ObservationRoute, intake)
	return app
}

// f61ListenerConfig is the dedicated listener's configuration.
type f61ListenerConfig struct {
	addr, certFile, keyFile string
	plaintext               bool
}

func f61ListenerFromEnv() f61ListenerConfig {
	return f61ListenerConfig{
		addr:     envOr("OBSERVEX_F61_INTAKE_ADDR", f61DefaultAddr),
		certFile: os.Getenv("OBSERVEX_F61_INTAKE_TLS_CERT_FILE"),
		keyFile:  os.Getenv("OBSERVEX_F61_INTAKE_TLS_KEY_FILE"),
		// Exactly "true": a typo or "1" does not turn TLS off.
		plaintext: os.Getenv("OBSERVEX_F61_INTAKE_INSECURE_PLAINTEXT") == "true",
	}
}

// errF61NotStarted: neither a certificate nor the explicit plaintext switch
// is configured, so no listener is opened.
var errF61NotStarted = errors.New("f61 probe listener not started: set OBSERVEX_F61_INTAKE_TLS_CERT_FILE and _KEY_FILE")

// listenF61 opens the dedicated listener: TLS when a certificate pair is
// configured (it wins over the plaintext switch), plaintext only when
// explicitly enabled, and nothing otherwise.
func listenF61(cfg f61ListenerConfig) (ln net.Listener, tlsOn bool, err error) {
	var reloader *certReloader
	switch {
	case cfg.certFile != "" && cfg.keyFile != "":
		if reloader, err = newCertReloader(cfg.certFile, cfg.keyFile); err != nil {
			return nil, false, fmt.Errorf("f61 probe listener: TLS certificate unusable: %w", err)
		}
	case cfg.plaintext:
	default:
		return nil, false, errF61NotStarted
	}
	if ln, err = net.Listen("tcp", cfg.addr); err != nil {
		return nil, false, err
	}
	if reloader == nil {
		return ln, false, nil
	}
	return tls.NewListener(ln, &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: reloader.get}), true, nil
}

// startF61Intake starts the dedicated listener from the environment and
// returns a function that stops it. It never starts a plaintext listener
// unless OBSERVEX_F61_INTAKE_INSECURE_PLAINTEXT is exactly "true".
func startF61Intake(logger *zap.Logger, pool *pgxpool.Pool) func() {
	var db resultf61.DB
	if pool != nil {
		db = pool // never a typed-nil interface
	}
	// A dedicated DSN lets a deployment enable F6.1 without setting
	// POSTGRES_DSN, which would also start the processor's older in-process
	// synthetic scheduler (it probes every enabled check from the processor,
	// ignores locations and has no SSRF guard).
	var ownPool *pgxpool.Pool
	if dsn := strings.TrimSpace(os.Getenv("OBSERVEX_F61_POSTGRES_DSN")); dsn != "" {
		p, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			// The error names the DSN's structure, never the password.
			logger.Error("f61 probe endpoints: OBSERVEX_F61_POSTGRES_DSN unusable")
		} else {
			ownPool, db = p, p
		}
	}
	horizon, err := f61HorizonFromEnv(os.LookupEnv)
	if err != nil {
		logger.Error("f61 probe endpoints", zap.Error(err))
	}
	closeOwn := func() {
		if ownPool != nil {
			ownPool.Close()
		}
	}
	cfg := f61ListenerFromEnv()
	ln, tlsOn, err := listenF61(cfg)
	if err != nil {
		if errors.Is(err, errF61NotStarted) {
			logger.Warn(err.Error())
		} else {
			logger.Error("f61 probe listener not started", zap.String("addr", cfg.addr), zap.Error(err))
		}
		closeOwn()
		return func() {}
	}
	if tlsOn {
		logger.Info("f61 probe listener started (TLS)", zap.String("addr", cfg.addr))
	} else {
		logger.Warn("f61 probe listener started WITHOUT TLS: probe credentials cross the network in clear (development only)",
			zap.String("addr", cfg.addr))
	}
	app := newF61IntakeApp(f61Deps{logger: logger, key: probetoken.LoadKey(), db: db, horizon: horizon, now: time.Now})
	go func() {
		if err := app.Listener(ln); err != nil {
			logger.Error("f61 probe listener stopped", zap.Error(err))
		}
	}()
	return func() {
		_ = app.ShutdownWithTimeout(10 * time.Second)
		closeOwn()
	}
}

// certReloader serves the certificate from files and picks up a renewed pair
// (for example a rotated Kubernetes Secret) without a restart.
type certReloader struct {
	certFile, keyFile string
	mu                sync.Mutex
	cert              *tls.Certificate
	modTime           time.Time
	checked           time.Time
}

func newCertReloader(certFile, keyFile string) (*certReloader, error) {
	r := &certReloader{certFile: certFile, keyFile: keyFile}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *certReloader) load() error {
	st, err := os.Stat(r.certFile)
	if err != nil {
		return err
	}
	pair, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return err
	}
	r.cert, r.modTime = &pair, st.ModTime()
	return nil
}

func (r *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.checked) > 30*time.Second {
		r.checked = time.Now()
		if st, err := os.Stat(r.certFile); err == nil && !st.ModTime().Equal(r.modTime) {
			_ = r.load() // on failure keep serving the previous pair
		}
	}
	return r.cert, nil
}

// boundedLogValue keeps a caller-chosen path segment from filling a log line.
func boundedLogValue(s string) string {
	const max = 128
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
