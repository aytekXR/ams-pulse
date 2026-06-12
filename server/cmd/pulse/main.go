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
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
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
		fmt.Printf("pulse %s (commit %s, built %s)\n", Version, GitCommit, BuildDate)
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

	logger.Info("pulse: starting",
		"version", Version,
		"listen", cfg.ListenAddr,
		"ams_url", cfg.AMSBaseURL,
	)

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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger.Info("pulse migrate: running ClickHouse migrations",
		"dsn", maskDSN(cfg.ClickHouseDSN),
		"dir", cfg.MigrationsDir,
	)

	if err := runClickHouseMigrations(ctx, cfg, logger); err != nil {
		return fmt.Errorf("clickhouse migrations: %w", err)
	}

	// HOOK(BE-02): Meta migrations go here.
	// When be-02 implements store/meta, uncomment:
	//   if err := meta.Migrate(cfg.MetaDSN); err != nil {
	//       return fmt.Errorf("meta migrations: %w", err)
	//   }
	logger.Info("pulse migrate: done (meta migrations: HOOK pending BE-02)")

	return nil
}

// ─── Diag ─────────────────────────────────────────────────────────────────────

func runDiag(args []string) error {
	_ = args

	cfg, err := loadEnvConfig()
	if err != nil {
		// Show what we have anyway.
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
	}

	fmt.Println("=== Pulse Diagnostic ===")
	fmt.Printf("Version:        %s (%s)\n", Version, GitCommit)
	fmt.Printf("ListenAddr:     %s\n", cfg.ListenAddr)
	fmt.Printf("AMS URL:        %s\n", cfg.AMSBaseURL)
	fmt.Printf("AMS NodeID:     %s\n", cfg.AMSNodeID)
	fmt.Printf("ClickHouse DSN: %s\n", maskDSN(cfg.ClickHouseDSN))
	fmt.Printf("Migrations dir: %s\n", cfg.MigrationsDir)
	fmt.Printf("Log level:      %s\n", cfg.LogLevel)

	// Connectivity checks.
	fmt.Println("\n=== Connectivity ===")
	checkClickHouse(cfg.ClickHouseDSN)
	checkAMS(cfg.AMSBaseURL)

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
