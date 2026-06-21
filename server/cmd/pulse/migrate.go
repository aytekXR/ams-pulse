package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
)

// runClickHouseMigrations connects to ClickHouse and runs all pending migrations.
func runClickHouseMigrations(ctx context.Context, cfg EnvConfig, logger *slog.Logger) error {
	// Resolve migrations directory.
	migrationsDir := cfg.MigrationsDir
	if migrationsDir == "" {
		// Default: relative to the binary location, or use a source-relative path
		// for development.
		_, file, _, ok := runtime.Caller(0)
		if ok {
			// In dev: server/cmd/pulse → server/../contracts/db/clickhouse
			base := filepath.Dir(filepath.Dir(filepath.Dir(file)))
			migrationsDir = filepath.Join(base, "contracts", "db", "clickhouse")
		} else {
			migrationsDir = "contracts/db/clickhouse"
		}
	}

	// Connect with retry.
	// Use the 'default' database for initial connection — the target database
	// may not exist yet; migrations create it via CREATE DATABASE IF NOT EXISTS.
	opts, err := clickhouse.ParseDSN(cfg.ClickHouseDSN)
	if err != nil {
		return fmt.Errorf("parse DSN: %w", err)
	}
	// Override to default database so ClickHouse accepts the connection even
	// before the target database is created by the first migration.
	opts.Auth.Database = "default"

	var conn clickhouse.Conn
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		conn, err = clickhouse.Open(opts)
		if err != nil {
			logger.Warn("migrate: clickhouse connect failed, retrying", "attempt", i+1, "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = conn.Ping(pingCtx)
		cancel()
		if err == nil {
			break
		}
		_ = conn.Close()
		conn = nil
		logger.Warn("migrate: clickhouse ping failed, retrying", "attempt", i+1, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if conn == nil {
		return fmt.Errorf("failed to connect to ClickHouse after %d retries", maxRetries)
	}
	defer conn.Close()

	runner := migrations.New(conn, migrations.Config{
		MigrationsDir: migrationsDir,
		Database:      cfg.ClickHouseDatabase,
		RetentionDays: cfg.RetentionDays,
		RollupTTLDays: cfg.RollupTTLDays,
	}, logger)

	return runner.Run(ctx)
}

// maskDSN returns a DSN string with the password replaced by ***.
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	// D-030: redact the password in clickhouse://user:pass@host DSNs before logging.
	// The prior implementation returned the DSN unchanged, leaking the ClickHouse
	// password in plaintext to JSON logs on every migrate run and `pulse diag`.
	// url.URL.Redacted replaces the password with "xxxxx" (no URL-escaping).
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	return u.Redacted()
}

// checkClickHouse prints a connectivity status line.
func checkClickHouse(dsn string) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		fmt.Printf("ClickHouse: FAIL (parse DSN: %v)\n", err)
		return
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		fmt.Printf("ClickHouse: FAIL (open: %v)\n", err)
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		fmt.Printf("ClickHouse: FAIL (ping: %v)\n", err)
		return
	}
	fmt.Println("ClickHouse: OK")
}

// checkAMS prints a connectivity status line for AMS.
func checkAMS(baseURL string) {
	if baseURL == "" {
		fmt.Println("AMS: not configured")
		return
	}
	fmt.Printf("AMS URL: %s (connectivity check skipped in diag mode)\n", baseURL)
}
