// Command pulse is the single Pulse server binary (PRD §7.10: "single Go binary,
// stateless"). One binary, multiple run modes, so the default Docker Compose
// deployment runs everything in one container while large installs can split roles.
//
// Usage:
//
//	pulse serve            # all-in-one: collector + query API + alert evaluator + reports
//	pulse serve --role=collector|api|alerter
//	pulse migrate          # apply contracts/db migrations
//	pulse version          # print version info
//	pulse diag             # diagnostic bundle (config echo, connectivity checks)
//
// Assembly hooks for BE-02:
//
//	// HOOK(BE-02): config.Load is a skeleton stub; replace with real implementation.
//	// HOOK(BE-02): meta migration is delegated here; call meta.Migrate(db).
//	// HOOK(BE-02): api.NewServer wiring goes in serve.go startAPIServer().
//	// HOOK(BE-02): alert.NewEvaluator wiring goes in serve.go startAlerter().
//	// HOOK(BE-02): license.Check goes in serve.go before startAPIServer().
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/config"
	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// Version is set by the build system via -ldflags.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "serve":
		if err := runServe(args); err != nil {
			fmt.Fprintf(os.Stderr, "pulse serve: %v\n", err)
			os.Exit(1)
		}
	case "migrate":
		if err := runMigrate(args); err != nil {
			fmt.Fprintf(os.Stderr, "pulse migrate: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(versionString())
	case "diag":
		if err := runDiag(args); err != nil {
			fmt.Fprintf(os.Stderr, "pulse diag: %v\n", err)
			os.Exit(1)
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "pulse: unknown command %q\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Pulse — AMS analytics, QoE monitoring and alerting

Usage:
  pulse serve [--role=collector|api|alerter]  Start the server (default: all roles)
  pulse migrate [--migrations-dir=<path>]     Apply ClickHouse + meta migrations
  pulse version                               Print version
  pulse diag                                  Print config + connectivity diagnostics

Environment variables:
  PULSE_CLICKHOUSE_DSN    ClickHouse DSN (default: clickhouse://localhost:9000/pulse)
  PULSE_AMS_URL           AMS base URL (default: http://localhost:5080)
  PULSE_AMS_NODE_ID       AMS node identifier (default: standalone)
  PULSE_LISTEN_ADDR       HTTP listen address (default: :8090)
  PULSE_LOG_LEVEL         Log level: debug|info|warn|error (default: info)
  PULSE_MIGRATIONS_DIR    Path to ClickHouse SQL migrations (default: contracts/db/clickhouse)
`)
}

// ─── Serve ────────────────────────────────────────────────────────────────────

func runServe(args []string) error {
	_ = args // role parsing reserved for BE-02 / Wave 2

	logger := newLogger()

	// HOOK(BE-02): Replace this stub with config.Load(os.Args[1:]).
	// For now, read from environment variables directly.
	cfg, err := loadEnvConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// B10: redact userinfo from the AMS URL before logging.
	amsURLLog := redactURL(cfg.AMSBaseURL)
	logger.Info("pulse: starting",
		"version", Version,
		"listen", cfg.ListenAddr,
		"ams_url", amsURLLog,
	)

	// B5: warn if the AMS base URL uses plain HTTP against a non-local host.
	if warnURL, err := url.Parse(cfg.AMSBaseURL); err == nil &&
		strings.EqualFold(warnURL.Scheme, "http") {
		host := warnURL.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			logger.Warn("pulse: AMS bearer token will travel in cleartext; use https:// for remote AMS hosts",
				"ams_url", amsURLLog)
		}
	}

	srv, err := newServer(ctx, cfg, logger)
	if err != nil {
		return err
	}

	if err := srv.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()
	logger.Info("pulse: shutting down")
	srv.Stop()
	return nil
}

// ─── Migrate ─────────────────────────────────────────────────────────────────

func runMigrate(args []string) error {
	_ = args

	logger := newLogger()
	cfg, err := loadEnvConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var errs []error

	// ── 1. Meta store migrations (D-W1-003) ──────────────────────────────────
	// Runs unconditionally (does not require ClickHouse). The DDL is embedded in
	// the binary (meta.EmbeddedDDL) so PULSE_META_DDL_PATH is not required for a
	// working binary; the env var acts as an override for custom schemas.
	metaBackend, metaDSN := resolveMetaBackend(os.Getenv)
	metaSecretKey, secretErr := config.GetSecret("PULSE_SECRET_KEY")
	if secretErr != nil {
		return fmt.Errorf("PULSE_SECRET_KEY: %w", secretErr)
	}
	if metaDSN != ":memory:" && len(metaSecretKey) < 16 {
		if metaSecretKey == "" {
			return fmt.Errorf("PULSE_SECRET_KEY must be set (min 16 bytes); generate with: openssl rand -hex 32")
		}
		return fmt.Errorf("PULSE_SECRET_KEY is too short (%d bytes); minimum is 16 bytes; generate with: openssl rand -hex 32", len(metaSecretKey))
	}
	logger.Info("pulse migrate: running meta store migrations", "dsn", metaDSN, "backend", metaBackend)
	metaStore, metaErr := meta.New(ctx, metaBackend, metaDSN, metaSecretKey)
	if metaErr != nil {
		logger.Error("pulse migrate: meta store open failed", "error", metaErr)
		errs = append(errs, fmt.Errorf("meta store open: %w", metaErr))
	} else {
		defer metaStore.Close()
		metaDDLPath := os.Getenv("PULSE_META_DDL_PATH")
		var applyErr error
		if metaDDLPath != "" {
			logger.Info("pulse migrate: using explicit meta DDL path", "path", metaDDLPath)
			applyErr = metaStore.Migrate(ctx, metaDDLPath)
		} else {
			applyErr = metaStore.MigrateEmbedded(ctx, meta.EmbeddedDDL)
		}
		if applyErr != nil {
			logger.Error("pulse migrate: meta DDL apply failed", "error", applyErr)
			errs = append(errs, fmt.Errorf("meta migrations: %w", applyErr))
		} else {
			logger.Info("pulse migrate: meta store migrations done")
		}
	}

	// ── 2. ClickHouse migrations ──────────────────────────────────────────────
	// ClickHouse failures are logged but do not prevent a successful exit.
	// This allows operators to run `pulse migrate` to pre-populate the meta schema
	// before ClickHouse is available (common in staged deployment scenarios).
	// ClickHouse migrations will be retried the next time `pulse migrate` is run
	// or applied via `pulse serve` on start (which also applies schema).
	logger.Info("pulse migrate: running ClickHouse migrations",
		"dsn", maskDSN(cfg.ClickHouseDSN),
		"dir", cfg.MigrationsDir,
	)
	if chErr := runClickHouseMigrations(ctx, cfg, logger); chErr != nil {
		logger.Warn("pulse migrate: ClickHouse migrations failed (non-fatal; retry when ClickHouse is available)",
			"error", chErr)
	} else {
		logger.Info("pulse migrate: ClickHouse migrations done")
	}

	if len(errs) > 0 {
		// Return the first error (meta failure is fatal).
		return errs[0]
	}
	logger.Info("pulse migrate: done")
	return nil
}

// ─── Diag ─────────────────────────────────────────────────────────────────────

// printDiagSummary writes the pulse-diag config summary to w. The AMS URL and
// ClickHouse DSN are credential-redacted before printing (S73/D-136 [6]); extracted
// from runDiag so the redaction of this call site is unit-testable.
func printDiagSummary(w io.Writer, cfg EnvConfig) {
	fmt.Fprintf(w, "Version:        %s (%s)\n", Version, GitCommit)
	fmt.Fprintf(w, "ListenAddr:     %s\n", cfg.ListenAddr)
	fmt.Fprintf(w, "AMS URL:        %s\n", redactURL(cfg.AMSBaseURL))
	fmt.Fprintf(w, "AMS NodeID:     %s\n", cfg.AMSNodeID)
	fmt.Fprintf(w, "ClickHouse DSN: %s\n", maskDSN(cfg.ClickHouseDSN))
	fmt.Fprintf(w, "Migrations dir: %s\n", cfg.MigrationsDir)
	fmt.Fprintf(w, "Log level:      %s\n", cfg.LogLevel)
}

func runDiag(args []string) error {
	// Parse --reconcile flag.
	doReconcile := false
	for _, a := range args {
		if a == "--reconcile" || a == "-reconcile" {
			doReconcile = true
		}
	}

	cfg, err := loadEnvConfig()
	if err != nil {
		// Show what we have anyway.
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
	}

	fmt.Println("=== Pulse Diagnostic ===")
	printDiagSummary(os.Stdout, cfg)

	// Connectivity checks.
	fmt.Println("\n=== Connectivity ===")
	checkClickHouse(cfg.ClickHouseDSN)
	checkAMS(cfg.AMSBaseURL)

	// ─── Reconciliation check (WO-204, PRD F6): pulse diag --reconcile ────────
	if doReconcile {
		fmt.Println("\n=== Reconciliation (±1% budget) ===")
		if err := runReconcile(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "reconcile: %v\n", err)
			return err
		}
	}

	return nil
}

// runReconcile runs the ±1% reconciliation check against live ClickHouse data.
// Source: Accountant.Reconcile — compares rollup_audience_1d to raw viewer_sessions.
func runReconcile(cfg EnvConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Connect to ClickHouse.
	chCfg := clickhouse.Config{
		DSN:           cfg.ClickHouseDSN,
		Database:      cfg.ClickHouseDatabase,
		BatchSize:     1,
		FlushInterval: time.Second,
		MaxRetries:    1,
		RetryDelay:    time.Second,
	}
	chStore, err := clickhouse.New(ctx, chCfg, logger)
	if err != nil {
		return fmt.Errorf("connect ClickHouse: %w", err)
	}
	defer chStore.Close()

	// Connect to meta store (for tenant mapping).
	metaBackend, metaDSN := resolveMetaBackend(os.Getenv)
	metaSecretKey, secretErr := config.GetSecret("PULSE_SECRET_KEY")
	if secretErr != nil {
		return fmt.Errorf("PULSE_SECRET_KEY: %w", secretErr)
	}
	if metaDSN != ":memory:" && len(metaSecretKey) < 16 {
		if metaSecretKey == "" {
			return fmt.Errorf("PULSE_SECRET_KEY must be set (min 16 bytes); generate with: openssl rand -hex 32")
		}
		return fmt.Errorf("PULSE_SECRET_KEY is too short (%d bytes); minimum is 16 bytes; generate with: openssl rand -hex 32", len(metaSecretKey))
	}
	metaStore, err := meta.New(ctx, metaBackend, metaDSN, metaSecretKey)
	if err != nil {
		return fmt.Errorf("connect meta store: %w", err)
	}
	defer metaStore.Close()

	// Default: previous calendar month.
	now := time.Now().UTC()
	to := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, -1, 0)
	fmt.Printf("Period:         %s to %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	accountant := reports.NewAccountant(chStore.GetConn(), metaStore)
	result, err := accountant.Reconcile(ctx, from, to)
	if err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	fmt.Printf("Rollup minutes: %.4f\n", result.RollupViewerMinutes)
	fmt.Printf("Raw minutes:    %.4f\n", result.RawViewerMinutes)
	fmt.Printf("Data points:    %d\n", result.DataPoints)
	fmt.Printf("Drift:          %.4f%%\n", result.DriftPct)

	if result.WithinTolerance {
		fmt.Printf("Result:         PASS (drift %.4f%% ≤ 1.0%%)\n", result.DriftPct)
	} else {
		fmt.Printf("Result:         FAIL (drift %.4f%% > 1.0%% budget)\n", result.DriftPct)
		return fmt.Errorf("reconciliation drift %.4f%% exceeds ±1%% budget", result.DriftPct)
	}
	return nil
}

// ─── Logger ───────────────────────────────────────────────────────────────────

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if ll := os.Getenv("PULSE_LOG_LEVEL"); ll != "" {
		switch ll {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
