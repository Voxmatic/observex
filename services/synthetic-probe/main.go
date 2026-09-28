// Command synthetic-probe is the F6.1 dedicated synthetic probe service (G-1:
// one Deployment per vantage).
//
// It has two modes.
//
// serve (the Deployment's mode) pulls this vantage's work from the processor,
// probes each assigned check once per due slot and reports every observation
// (PROPOSED FANOUT-1). The processor decides the work from the verified
// credential (PROPOSED LOC-1); the probe asserts nothing about who it is. It
// drops all work when the processor rejects the credential or when work cannot
// be refreshed for ten minutes, and re-reads the credential file after a
// rejection, so a rotated Secret is used without a restart.
//
//	synthetic-probe serve [-processor-url URL] [-processor-ca-file PEM]
//	                      [-allow-private-targets] [-max-in-flight N]
//	                      [-health-addr ADDR] [-allow-plaintext]
//
// The one-shot mode probes one endpoint once, reports it, prints one JSON line
// and exits; use it by hand to exercise the path.
//
//	synthetic-probe -check-id ID -endpoint HOST[:PORT] [-timeout 10s]
//	                [-processor-url URL] [-processor-ca-file PEM]
//	                [-allow-private-targets] [-allow-plaintext]
//
// Environment (flags win):
//
//	OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL_FILE  path to this vantage's credential (wins)
//	OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL       the credential value
//	OBSERVEX_PROCESSOR_URL                    processor probe origin (https://…:8443)
//	OBSERVEX_PROCESSOR_CA_FILE                PEM roots for the processor's certificate
//	                                          (default: system roots)
//	OBSERVEX_PROBE_ALLOW_PRIVATE_TARGETS      "true" permits probing private,
//	                                          loopback and link-local addresses
//	OBSERVEX_PROBE_ALLOW_PLAINTEXT            "true" permits an http:// processor URL
//	                                          (development only)
//	OBSERVEX_PROBE_MAX_IN_FLIGHT              concurrent probes, 1–64 (default 8)
//	OBSERVEX_PROBE_HEALTH_ADDR                health listener (default :8081; "off" disables)
//
// Probe targets are dialled through an SSRF guard (PROPOSED EGRESS-1): unless
// private targets are allowed, a name that resolves only to non-public
// addresses is not dialled and the observation is a transport failure.
// Certificates of targets are verified against the system roots.
//
// Exit status (one-shot): 0 accepted by the processor; 1 probed but not
// accepted or not delivered; 2 usage or configuration error. serve exits 0 on
// SIGTERM and 2 on a configuration error. The credential is never printed.
package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	probef61 "github.com/observex/platform/internal/probe/f61"
	"github.com/observex/platform/internal/probetoken"
)

const (
	processorURLEnvVar   = "OBSERVEX_PROCESSOR_URL"
	processorCAEnvVar    = "OBSERVEX_PROCESSOR_CA_FILE"
	allowPrivateEnvVar   = "OBSERVEX_PROBE_ALLOW_PRIVATE_TARGETS"
	allowPlaintextEnvVar = "OBSERVEX_PROBE_ALLOW_PLAINTEXT"
	maxInFlightEnvVar    = "OBSERVEX_PROBE_MAX_IN_FLIGHT"
	healthAddrEnvVar     = "OBSERVEX_PROBE_HEALTH_ADDR"
	maxProbeTimeout      = 60 * time.Second
	defaultHealthAddr    = ":8081"
)

// result is the one line written to stdout by the one-shot mode.
type result struct {
	CheckID      string `json:"check_id"`
	Endpoint     string `json:"endpoint"`
	Outcome      string `json:"outcome,omitempty"`
	Certificates int    `json:"certificates"`
	Delivered    bool   `json:"delivered"`
	Status       int    `json:"status,omitempty"`
	Accepted     bool   `json:"accepted"`
	Stored       bool   `json:"stored"`
	Evaluated    bool   `json:"evaluated"`
	ResultStatus string `json:"result_status,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Error        string `json:"error,omitempty"`
}

// deps are the process's interfaces to the outside world.
type deps struct {
	lookupEnv func(string) (string, bool)
	readFile  func(string) ([]byte, error)
	// newProber builds the target prober; allowPrivate lifts the SSRF guard.
	newProber func(allowPrivate bool) tlscert.Prober
	stdout    io.Writer
	stderr    io.Writer
	// listen opens the health listener.
	listen func(network, addr string) (net.Listener, error)
}

func defaultProber(allowPrivate bool) tlscert.Prober {
	return tlscert.New(tlscert.Config{Dial: probef61.GuardedDialer{AllowPrivate: allowPrivate}.Dial})
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], deps{
		lookupEnv: os.LookupEnv, readFile: os.ReadFile, newProber: defaultProber,
		stdout: os.Stdout, stderr: os.Stderr, listen: net.Listen,
	}))
}

func run(ctx context.Context, args []string, d deps) int {
	if len(args) > 0 && args[0] == "serve" {
		return serve(ctx, args[1:], d)
	}
	return oneShot(ctx, args, d)
}

func envString(d deps, key string) string {
	v, _ := d.lookupEnv(key)
	return v
}

// envTrue is true only for exactly "true": a typo does not relax a guard.
func envTrue(d deps, key string) bool { return envString(d, key) == "true" }

// common holds the flags both modes share.
type common struct {
	processorURL, caFile         *string
	allowPlaintext, allowPrivate *bool
}

func commonFlags(fs *flag.FlagSet, d deps) common {
	return common{
		processorURL:   fs.String("processor-url", envString(d, processorURLEnvVar), "processor probe origin; defaults to $"+processorURLEnvVar),
		caFile:         fs.String("processor-ca-file", envString(d, processorCAEnvVar), "PEM roots for the processor's certificate; defaults to $"+processorCAEnvVar),
		allowPlaintext: fs.Bool("allow-plaintext", envTrue(d, allowPlaintextEnvVar), "permit an http:// processor URL (the credential is then sent in clear)"),
		allowPrivate:   fs.Bool("allow-private-targets", envTrue(d, allowPrivateEnvVar), "permit probing private, loopback and link-local addresses"),
	}
}

// client loads the credential and builds the processor client.
func (c common) client(d deps) (*probef61.Client, error) {
	cred, err := probef61.LoadCredentialFrom(d.lookupEnv, d.readFile)
	if err != nil {
		return nil, err
	}
	var roots *x509.CertPool
	if *c.caFile != "" {
		pem, err := d.readFile(*c.caFile)
		if err != nil {
			return nil, fmt.Errorf("processor CA file could not be read: %w", err)
		}
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("processor CA file holds no PEM certificate")
		}
	}
	return probef61.NewClient(probef61.ClientConfig{ProcessorURL: *c.processorURL, Credential: cred,
		AllowPlaintext: *c.allowPlaintext, RootCAs: roots})
}

func oneShot(ctx context.Context, args []string, d deps) int {
	fs := flag.NewFlagSet("synthetic-probe", flag.ContinueOnError)
	fs.SetOutput(d.stderr)
	checkID := fs.String("check-id", "", "synthetic check ID (required)")
	endpoint := fs.String("endpoint", "", "HOST or HOST:PORT, exactly as in the check's target (required)")
	timeout := fs.Duration("timeout", 10*time.Second, "probe timeout, dial plus handshake (max 60s)")
	cf := commonFlags(fs, d)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *checkID == "" || *endpoint == "" || *timeout <= 0 || *timeout > maxProbeTimeout {
		fmt.Fprintln(d.stderr, "synthetic-probe: -check-id and -endpoint are required, and -timeout must be in (0, 60s]")
		return 2
	}
	client, err := cf.client(d)
	if err != nil {
		fmt.Fprintln(d.stderr, "synthetic-probe:", err)
		return 2
	}

	out := result{CheckID: *checkID, Endpoint: *endpoint}
	obs, receipt, err := probef61.ProbeAndReport(ctx, d.newProber(*cf.allowPrivate), client,
		tlscert.Target{CheckID: *checkID, Endpoint: *endpoint, Timeout: *timeout})
	if errors.Is(err, tlscert.ErrBadAddress) || errors.Is(err, tlscert.ErrNoEndpoint) || errors.Is(err, tlscert.ErrNoTimeout) {
		fmt.Fprintln(d.stderr, "synthetic-probe:", err)
		return 2
	}
	out.Outcome = string(obs.Outcome)
	out.Certificates = len(obs.Certificates())
	out.Status, out.Accepted, out.Stored, out.Evaluated = receipt.Status, receipt.Accepted, receipt.Stored, receipt.Evaluated
	out.ResultStatus, out.Reason = receipt.ResultStatus, receipt.Reason
	out.Delivered = receipt.Status != 0
	if err != nil {
		out.Error = err.Error()
	}
	line, _ := json.Marshal(out)
	fmt.Fprintln(d.stdout, string(line))
	if receipt.Accepted {
		return 0
	}
	return 1
}

func serve(ctx context.Context, args []string, d deps) int {
	fs := flag.NewFlagSet("synthetic-probe serve", flag.ContinueOnError)
	fs.SetOutput(d.stderr)
	cf := commonFlags(fs, d)
	defInFlight := 8
	if v := envString(d, maxInFlightEnvVar); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintln(d.stderr, "synthetic-probe: "+maxInFlightEnvVar+" must be an integer")
			return 2
		}
		defInFlight = n
	}
	maxInFlight := fs.Int("max-in-flight", defInFlight, "concurrent probes (1–64)")
	defHealth := defaultHealthAddr
	if v, ok := d.lookupEnv(healthAddrEnvVar); ok {
		defHealth = v
	}
	healthAddr := fs.String("health-addr", defHealth, `health listener address ("off" disables)`)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *maxInFlight < 1 || *maxInFlight > 64 {
		fmt.Fprintln(d.stderr, "synthetic-probe: serve takes no arguments, and -max-in-flight must be in [1, 64]")
		return 2
	}
	client, err := cf.client(d)
	if err != nil {
		fmt.Fprintln(d.stderr, "synthetic-probe:", err)
		return 2
	}
	logger := slog.New(slog.NewJSONHandler(d.stderr, nil))
	runner := &probef61.Runner{
		Client: client, Prober: d.newProber(*cf.allowPrivate), MaxInFlight: *maxInFlight,
		ReloadCredential: func() (probetoken.Credential, error) { return probef61.LoadCredentialFrom(d.lookupEnv, d.readFile) },
		Log:              func(msg string, kv ...any) { logger.Info(msg, kv...) },
	}

	if *healthAddr != "" && *healthAddr != "off" {
		ln, err := d.listen("tcp", *healthAddr)
		if err != nil {
			fmt.Fprintln(d.stderr, "synthetic-probe: health listener:", err)
			return 2
		}
		srv := &http.Server{Handler: healthHandler(runner), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = srv.Serve(ln) }()
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(sctx)
		}()
		logger.Info("health endpoint listening", "addr", ln.Addr().String())
	}
	logger.Info("synthetic probe serving", "work_url", client.WorkURL(), "max_in_flight", *maxInFlight,
		"allow_private_targets", *cf.allowPrivate)
	if err := runner.Run(ctx); err != nil {
		fmt.Fprintln(d.stderr, "synthetic-probe:", err)
		return 2
	}
	logger.Info("synthetic probe stopped")
	return 0
}

// healthHandler: /healthz is liveness (the process runs); /readyz says
// whether the probe holds work accepted by the processor within the stale
// bound. Neither reveals the credential or the assignments.
func healthHandler(r *probef61.Runner) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !r.Ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "no current work\n")
			return
		}
		_, _ = io.WriteString(w, "ok\n")
	})
	return mux
}
