//go:build integration

package meta_test

// meta_pg_integration_test.go — Postgres parity suite for the meta store.
//
// Gated on env PULSE_META_TEST_PG_DSN. When that variable is unset the entire
// file's tests are skipped with a loud message so CI can distinguish a skipped
// run (missing infra) from a failed one.
//
// CLEANUP STRATEGY: each call to openPGStore drops and recreates the public
// schema, giving every test a pristine database with a fresh migration.
//
// PARITY REQUIREMENT: schema_migrations must contain exactly {'0001','0002'}
// after a fresh PG migration — identical version strings to a fresh SQLite DB
// migrated with both 0001 and 0002 SQL files.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

const pgSecretKey = "integration-test-secret-key-32b!"

// openPGStore opens a Postgres meta store against a fresh schema (drops and
// recreates public schema) and applies the embedded PG DDL. Skips if
// PULSE_META_TEST_PG_DSN is unset.
func openPGStore(t *testing.T) *meta.Store {
	t.Helper()
	dsn := pgDSN(t)
	ctx := context.Background()

	// Reset schema so every test starts pristine.
	rawDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("openPGStore: raw open: %v", err)
	}
	if _, err := rawDB.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		_ = rawDB.Close()
		t.Fatalf("openPGStore: reset schema: %v", err)
	}
	_ = rawDB.Close()

	s, err := meta.New(ctx, "postgres", dsn, pgSecretKey)
	if err != nil {
		t.Fatalf("meta.New postgres: %v", err)
	}
	// MigrateEmbedded routes to EmbeddedDDLPostgres when backend=="postgres",
	// so the caller-supplied DDL value is irrelevant for PG.
	if err := s.MigrateEmbedded(ctx, meta.EmbeddedDDL); err != nil {
		_ = s.Close()
		t.Fatalf("MigrateEmbedded: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// pgDSN returns PULSE_META_TEST_PG_DSN or skips the test.
func pgDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("PULSE_META_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PULSE_META_TEST_PG_DSN not set — skipping Postgres integration tests (set the variable to run them)")
	}
	return dsn
}

// pgSchemaVersions queries schema_migrations.version from the PG test database.
func pgSchemaVersions(t *testing.T) []string {
	t.Helper()
	dsn := os.Getenv("PULSE_META_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PULSE_META_TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("pgSchemaVersions: open: %v", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(context.Background(), "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("pgSchemaVersions: query: %v", err)
	}
	defer rows.Close()
	var vs []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("pgSchemaVersions: scan: %v", err)
		}
		vs = append(vs, v)
	}
	return vs
}

// sqliteSchemaVersions creates an in-memory SQLite store, applies both 0001
// and 0002 DDL files in a single MigrateEmbedded call (concatenated), and
// returns the resulting schema_migrations version strings.
//
// WHY concatenated: MigrateEmbedded calls applySchemaUpgrades after running
// the DDL. applySchemaUpgrades proactively adds the 0002 columns (rule_type,
// sigma, min_samples) via ALTER TABLE. If 0001 and 0002 are applied in two
// separate MigrateEmbedded / Migrate calls, the second call fails with
// "duplicate column name" because applySchemaUpgrades already added them.
// Concatenating DDL0001 + DDL0002 into one MigrateEmbedded call applies both
// in statement-execution order before applySchemaUpgrades runs — the same
// semantics as applying a fresh set of migrations sequentially.
func sqliteSchemaVersions(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()

	ddl0001, err := os.ReadFile("../../../../contracts/db/meta/0001_init.sql")
	if err != nil {
		t.Skipf("cannot read 0001_init.sql: %v", err)
	}
	ddl0002, err := os.ReadFile("../../../../contracts/db/meta/0002_anomaly_alert_rule.sql")
	if err != nil {
		t.Skipf("cannot read 0002_anomaly_alert_rule.sql: %v", err)
	}

	// Use a temp file so we can read schema_migrations via a raw connection.
	tmp, err := os.CreateTemp("", "meta_parity_*.db")
	if err != nil {
		t.Fatalf("sqliteSchemaVersions: tmp: %v", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	s, err := meta.New(ctx, "sqlite", tmpPath, pgSecretKey)
	if err != nil {
		t.Fatalf("sqliteSchemaVersions: New: %v", err)
	}
	// Apply both DDL files in one call to avoid the applySchemaUpgrades
	// double-column issue (see function doc above).
	combined := string(ddl0001) + "\n" + string(ddl0002)
	if err := s.MigrateEmbedded(ctx, combined); err != nil {
		_ = s.Close()
		t.Fatalf("sqliteSchemaVersions: MigrateEmbedded combined: %v", err)
	}
	_ = s.Close()

	// Read schema_migrations via a fresh raw SQLite connection.
	db, err := sql.Open("sqlite", tmpPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sqliteSchemaVersions: raw open: %v", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("sqliteSchemaVersions: query: %v", err)
	}
	defer rows.Close()
	var vs []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("sqliteSchemaVersions: scan: %v", err)
		}
		vs = append(vs, v)
	}
	return vs
}

// ─── Migration parity ─────────────────────────────────────────────────────────

// TestPG_MigrationParity checks that after migrating a fresh PG database,
// schema_migrations contains exactly the same version strings as a fresh SQLite
// database migrated with both 0001 and 0002 SQL files.
func TestPG_MigrationParity(t *testing.T) {
	openPGStore(t) // resets schema and migrates

	pgVersions := pgSchemaVersions(t)
	sqliteVersions := sqliteSchemaVersions(t)

	sort.Strings(pgVersions)
	sort.Strings(sqliteVersions)

	t.Logf("PG schema_migrations:     %v", pgVersions)
	t.Logf("SQLite schema_migrations: %v", sqliteVersions)

	if len(pgVersions) != len(sqliteVersions) {
		t.Fatalf("version count mismatch: pg=%d sqlite=%d", len(pgVersions), len(sqliteVersions))
	}
	for i := range pgVersions {
		if pgVersions[i] != sqliteVersions[i] {
			t.Errorf("version[%d]: pg=%q sqlite=%q", i, pgVersions[i], sqliteVersions[i])
		}
	}
	t.Logf("PASS: schema_migrations parity — both have %v", pgVersions)
}

// TestPG_MigrateIdempotent verifies that running MigrateEmbedded twice is a
// no-op: schema_migrations row count is unchanged after the second call.
func TestPG_MigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	before := pgSchemaVersions(t)
	if err := s.MigrateEmbedded(ctx, meta.EmbeddedDDL); err != nil {
		t.Fatalf("second MigrateEmbedded: %v", err)
	}
	after := pgSchemaVersions(t)

	if len(before) != len(after) {
		t.Fatalf("idempotency violated: before=%d after=%d", len(before), len(after))
	}
	t.Logf("PASS: migrate idempotent — %d version(s) unchanged after second run", len(after))
}

// TestPG_SecretKeyRequired verifies the hard error when secretKey is empty.
func TestPG_SecretKeyRequired(t *testing.T) {
	dsn := pgDSN(t)
	ctx := context.Background()
	_, err := meta.New(ctx, "postgres", dsn, "")
	if err == nil {
		t.Fatal("expected error when secretKey is empty for postgres backend, got nil")
	}
	t.Logf("PASS: got expected error: %v", err)
}

// ─── Users ────────────────────────────────────────────────────────────────────

func TestPG_Users_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	u := meta.User{
		Username:  "pg-alice",
		PwHash:    "bcrypt:$2a$10$test",
		Role:      "admin",
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUserByUsername(ctx, "pg-alice")
	if err != nil || got == nil {
		t.Fatalf("GetUserByUsername: err=%v got=%v", err, got)
	}
	if got.Role != "admin" || got.ID == "" {
		t.Errorf("user fields: %+v", got)
	}

	list, err := s.ListUsers(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListUsers: err=%v len=%d", err, len(list))
	}

	n, err := s.CountUsers(ctx)
	if err != nil || n != 1 {
		t.Fatalf("CountUsers: err=%v n=%d", err, n)
	}

	if err := s.UpdateUser(ctx, got.ID, "pg-alice-updated", "viewer"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	got2, _ := s.GetUserByUsername(ctx, "pg-alice-updated")
	if got2 == nil || got2.Role != "viewer" {
		t.Errorf("UpdateUser: got %v", got2)
	}

	if err := s.DeleteUser(ctx, got.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	list2, _ := s.ListUsers(ctx)
	if len(list2) != 0 {
		t.Errorf("expected 0 users after delete, got %d", len(list2))
	}
	t.Log("PASS: Users round-trip")
}

// ─── API Tokens ───────────────────────────────────────────────────────────────

func TestPG_Tokens_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	hash, alg := s.HashToken("rawtoken123")
	tok := meta.APIToken{
		Kind:      "api",
		Name:      "pg-test-token",
		TokenHash: hash,
		HashAlg:   alg,
		Scopes:    []string{"read", "write"},
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.CreateToken(ctx, tok); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	found, err := s.LookupToken(ctx, "rawtoken123")
	if err != nil || found == nil {
		t.Fatalf("LookupToken: err=%v found=%v", err, found)
	}
	if found.Name != "pg-test-token" {
		t.Errorf("name mismatch: %q", found.Name)
	}

	s.TouchToken(ctx, found.ID)

	list, err := s.ListTokens(ctx, "api")
	if err != nil || len(list) < 1 {
		t.Fatalf("ListTokens: err=%v len=%d", err, len(list))
	}

	n, err := s.CountTokens(ctx)
	if err != nil || n < 1 {
		t.Fatalf("CountTokens: err=%v n=%d", err, n)
	}

	if err := s.DeleteToken(ctx, found.ID); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	gone, err := s.LookupToken(ctx, "rawtoken123")
	if err != nil {
		t.Fatalf("LookupToken after delete: %v", err)
	}
	if gone != nil {
		t.Error("expected nil token after delete")
	}
	t.Log("PASS: API tokens round-trip")
}

func TestPG_Token_HMACAndExpiry(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	// HMAC token.
	rawToken := "my-super-secret-token-value"
	hash, alg := s.HashToken(rawToken)
	if alg != "hmac-sha256" {
		t.Errorf("expected hmac-sha256, got %q", alg)
	}
	tok := meta.APIToken{
		Kind: "api", Name: "hmac-tok", TokenHash: hash, HashAlg: alg,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.CreateToken(ctx, tok); err != nil {
		t.Fatalf("CreateToken HMAC: %v", err)
	}
	found, err := s.LookupToken(ctx, rawToken)
	if err != nil || found == nil {
		t.Fatalf("LookupToken HMAC: err=%v", err)
	}
	t.Logf("HMAC token lookup OK (alg=%s)", found.HashAlg)

	// Token with expiry stored.
	expiredAt := int64(1) // epoch ms = far past
	expiredTok := meta.APIToken{
		Kind: "api", Name: "expired-tok", TokenHash: "sha256:deadbeef01", HashAlg: "sha256",
		ExpiresAt: &expiredAt, CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.CreateToken(ctx, expiredTok); err != nil {
		t.Fatalf("CreateToken expired: %v", err)
	}
	rec, err := s.GetTokenByHash(ctx, "sha256:deadbeef01")
	if err != nil || rec == nil {
		t.Fatalf("GetTokenByHash expired: err=%v rec=%v", err, rec)
	}
	if rec.ExpiresAt == nil || *rec.ExpiresAt != expiredAt {
		t.Errorf("expiry not stored correctly: %v", rec.ExpiresAt)
	}
	t.Log("PASS: HMAC + expiry token")
}

// ─── Alert Rules ──────────────────────────────────────────────────────────────

func TestPG_AlertRules_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	r := meta.AlertRuleRow{
		Name: "pg-rule", Metric: "bitrate_kbps", Operator: "lt",
		Threshold: 500, WindowS: 60, Severity: "warning", Enabled: true,
		RuleType: "threshold",
	}
	created, err := s.CreateAlertRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	got, err := s.GetAlertRule(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetAlertRule: err=%v", err)
	}
	if got.Name != "pg-rule" {
		t.Errorf("name mismatch: %q", got.Name)
	}

	list, err := s.ListAlertRules(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListAlertRules: %v", err)
	}

	got.Name = "pg-rule-updated"
	if err := s.UpdateAlertRule(ctx, *got); err != nil {
		t.Fatalf("UpdateAlertRule: %v", err)
	}

	// Anomaly rule type round-trip.
	sigma := 3.5
	anom := meta.AlertRuleRow{
		Name: "pg-anom-rule", Metric: "rebuffer_ratio", Operator: "gt",
		Threshold: 0, WindowS: 300, Severity: "critical", Enabled: true,
		RuleType: "anomaly", Sigma: sigma, MinSamples: 50,
	}
	anomCreated, err := s.CreateAlertRule(ctx, anom)
	if err != nil {
		t.Fatalf("CreateAlertRule anomaly: %v", err)
	}
	anomGot, err := s.GetAlertRule(ctx, anomCreated.ID)
	if err != nil || anomGot == nil {
		t.Fatalf("GetAlertRule anomaly: %v", err)
	}
	if anomGot.RuleType != "anomaly" || anomGot.Sigma != sigma || anomGot.MinSamples != 50 {
		t.Errorf("anomaly rule fields: %+v", anomGot)
	}

	if err := s.DeleteAlertRule(ctx, created.ID); err != nil {
		t.Fatalf("DeleteAlertRule: %v", err)
	}
	if err := s.DeleteAlertRule(ctx, anomCreated.ID); err != nil {
		t.Fatalf("DeleteAlertRule anomaly: %v", err)
	}
	t.Log("PASS: Alert rules round-trip (threshold + anomaly)")
}

// ─── Alert Channels ───────────────────────────────────────────────────────────

func TestPG_AlertChannels_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	c := meta.AlertChannelRow{
		Type: "email", Name: "pg-email", ConfigPublic: `{"address":"a@b.com"}`,
	}
	created, err := s.CreateAlertChannel(ctx, c)
	if err != nil {
		t.Fatalf("CreateAlertChannel: %v", err)
	}

	got, err := s.GetAlertChannel(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetAlertChannel: %v", err)
	}

	list, err := s.ListAlertChannels(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListAlertChannels: %v", err)
	}

	got.Name = "pg-email-updated"
	if err := s.UpdateAlertChannel(ctx, *got); err != nil {
		t.Fatalf("UpdateAlertChannel: %v", err)
	}

	if err := s.DeleteAlertChannel(ctx, created.ID); err != nil {
		t.Fatalf("DeleteAlertChannel: %v", err)
	}
	t.Log("PASS: Alert channels round-trip")
}

// ─── Alert History ────────────────────────────────────────────────────────────

func TestPG_AlertHistory_CreateListPrune(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	rule := makeRule(t, s, "pg-hist-rule")
	s.SetAlertHistoryCap(100000) // disable auto-prune during fill

	const n = 20
	baseTS := int64(100_000)
	for i := 0; i < n; i++ {
		h := meta.AlertHistoryRow{
			AlertID: fmt.Sprintf("pg-alert-%03d", i), RuleID: rule,
			State: "firing", Severity: "warning",
			TS: baseTS + int64(i), Metric: "bitrate_kbps", Value: 100, Threshold: 500,
		}
		if err := s.CreateAlertHistory(ctx, h); err != nil {
			t.Fatalf("CreateAlertHistory[%d]: %v", i, err)
		}
	}

	all, err := s.ListAlertHistory(ctx, rule, "", 0, 0, 0)
	if err != nil || len(all) != n {
		t.Fatalf("ListAlertHistory: err=%v len=%d want %d", err, len(all), n)
	}

	// Filtered by state.
	byState, err := s.ListAlertHistory(ctx, rule, "firing", 0, 0, 0)
	if err != nil || len(byState) != n {
		t.Fatalf("ListAlertHistory (by state): err=%v len=%d", err, len(byState))
	}

	// Prune to keep newest 10.
	const keep = 10
	if err := s.PruneAlertHistory(ctx, rule, keep); err != nil {
		t.Fatalf("PruneAlertHistory: %v", err)
	}
	after, err := s.ListAlertHistory(ctx, rule, "", 0, 0, 0)
	if err != nil || len(after) != keep {
		t.Fatalf("after prune: err=%v len=%d want %d", err, len(after), keep)
	}
	// All kept rows must have ts >= baseTS+10 (the 10 newest).
	minTS := baseTS + int64(n-keep)
	for _, row := range after {
		if row.TS < minTS {
			t.Errorf("wrong row kept: ts=%d < minTS=%d", row.TS, minTS)
		}
	}
	t.Log("PASS: Alert history create/list/prune")
}

// ─── License ──────────────────────────────────────────────────────────────────

func TestPG_License_BootstrapAndUpsert(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	// Bootstrap: GetLicense on empty license table creates a free-tier row.
	l, err := s.GetLicense(ctx)
	if err != nil {
		t.Fatalf("GetLicense bootstrap: %v", err)
	}
	if l.Tier != "free" || !l.Valid {
		t.Errorf("bootstrap: got tier=%q valid=%v", l.Tier, l.Valid)
	}

	// Second call is idempotent (ON CONFLICT DO NOTHING).
	l2, err := s.GetLicense(ctx)
	if err != nil || l2.Tier != "free" {
		t.Fatalf("GetLicense idempotent: err=%v tier=%q", err, l2.Tier)
	}

	// Upsert a pro license.
	expiresAt := time.Now().Add(24 * time.Hour).UnixMilli()
	pro := meta.LicenseRow{
		ID: "singleton", Tier: "pro", ClaimsJSON: `{"tier":"pro"}`,
		Valid: true, ExpiresAt: &expiresAt,
	}
	if err := s.UpsertLicense(ctx, pro); err != nil {
		t.Fatalf("UpsertLicense: %v", err)
	}
	got, err := s.GetLicense(ctx)
	if err != nil || got.Tier != "pro" {
		t.Fatalf("GetLicense after upsert: err=%v tier=%q", err, got.Tier)
	}
	t.Log("PASS: License bootstrap + upsert")
}

// ─── AMS Sources ──────────────────────────────────────────────────────────────

func TestPG_AMSSources_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	src := meta.AMSSourceRow{
		Name: "pg-ams", SourceType: "rest_poll", Enabled: true,
	}
	created, err := s.CreateAMSSource(ctx, src)
	if err != nil {
		t.Fatalf("CreateAMSSource: %v", err)
	}

	got, err := s.GetAMSSource(ctx, created.ID)
	if err != nil || got == nil || got.Name != "pg-ams" {
		t.Fatalf("GetAMSSource: err=%v got=%v", err, got)
	}

	list, err := s.ListAMSSources(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListAMSSources: %v", err)
	}

	got.Name = "pg-ams-updated"
	if err := s.UpdateAMSSource(ctx, *got); err != nil {
		t.Fatalf("UpdateAMSSource: %v", err)
	}

	if err := s.DeleteAMSSource(ctx, created.ID); err != nil {
		t.Fatalf("DeleteAMSSource: %v", err)
	}
	list2, _ := s.ListAMSSources(ctx)
	if len(list2) != 0 {
		t.Errorf("expected 0 sources after delete, got %d", len(list2))
	}
	t.Log("PASS: AMS sources round-trip")
}

// ─── Tenants ──────────────────────────────────────────────────────────────────

func TestPG_Tenants_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	ten := meta.TenantRow{Name: "pg-tenant", StreamPattern: "live/*"}
	created, err := s.CreateTenant(ctx, ten)
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	got, err := s.GetTenant(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetTenant: %v", err)
	}

	got2, err := s.GetTenantByName(ctx, "pg-tenant")
	if err != nil || got2 == nil {
		t.Fatalf("GetTenantByName: %v", err)
	}

	list, err := s.ListTenants(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListTenants: %v", err)
	}

	got.Name = "pg-tenant-updated"
	if err := s.UpdateTenant(ctx, *got); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}

	if err := s.DeleteTenant(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}
	t.Log("PASS: Tenants round-trip")
}

// ─── Report Schedules ─────────────────────────────────────────────────────────

func TestPG_ReportSchedules_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	nextRunAt := time.Now().Add(time.Hour).UnixMilli()
	rs := meta.ReportScheduleRow{
		Cron: "0 0 * * *", Format: "csv", ScopeJSON: `{}`,
		NextRunAt: &nextRunAt,
	}
	created, err := s.CreateReportSchedule(ctx, rs)
	if err != nil {
		t.Fatalf("CreateReportSchedule: %v", err)
	}

	got, err := s.GetReportSchedule(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetReportSchedule: %v", err)
	}

	list, err := s.ListReportSchedules(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListReportSchedules: %v", err)
	}

	due, err := s.ListDueReportSchedules(ctx, nextRunAt+1)
	if err != nil || len(due) < 1 {
		t.Fatalf("ListDueReportSchedules: err=%v len=%d", err, len(due))
	}

	lastRunAt := time.Now().UnixMilli()
	nextRunAt2 := lastRunAt + 86400_000
	if err := s.MarkScheduleRan(ctx, created.ID, lastRunAt, nextRunAt2); err != nil {
		t.Fatalf("MarkScheduleRan: %v", err)
	}

	got.Cron = "0 1 * * *"
	if err := s.UpdateReportSchedule(ctx, *got); err != nil {
		t.Fatalf("UpdateReportSchedule: %v", err)
	}

	if err := s.DeleteReportSchedule(ctx, created.ID); err != nil {
		t.Fatalf("DeleteReportSchedule: %v", err)
	}
	t.Log("PASS: Report schedules round-trip")
}

// ─── Encryption ───────────────────────────────────────────────────────────────

func TestPG_Encryption_RoundTrip(t *testing.T) {
	s := openPGStore(t)

	plain := "super-secret-ams-password"
	enc, err := s.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "" || enc == plain {
		t.Errorf("Encrypt: bad ciphertext %q", enc)
	}
	dec, err := s.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plain {
		t.Errorf("Decrypt: got %q want %q", dec, plain)
	}
	t.Log("PASS: Encrypt/Decrypt round-trip")
}

// ─── Anomaly Baselines ────────────────────────────────────────────────────────

func TestPG_AnomalyBaselines_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	row := anomaly.AnomalyBaselineRow{
		Metric: "rebuffer_ratio", Scope: "global", WindowS: 300,
		Mean: 0.02, Stddev: 0.005, SampleCount: 100,
		LastUpdated: time.Now().UnixMilli(),
	}
	if err := s.UpsertAnomalyBaseline(ctx, row); err != nil {
		t.Fatalf("UpsertAnomalyBaseline: %v", err)
	}

	got, err := s.GetAnomalyBaseline(ctx, "rebuffer_ratio", "global", 300)
	if err != nil || got == nil {
		t.Fatalf("GetAnomalyBaseline: err=%v got=%v", err, got)
	}
	if got.Mean != 0.02 {
		t.Errorf("mean: got %v", got.Mean)
	}

	// Upsert to update.
	row.Mean = 0.03
	row.SampleCount = 200
	if err := s.UpsertAnomalyBaseline(ctx, row); err != nil {
		t.Fatalf("UpsertAnomalyBaseline update: %v", err)
	}
	got2, _ := s.GetAnomalyBaseline(ctx, "rebuffer_ratio", "global", 300)
	if got2 == nil || got2.Mean != 0.03 {
		t.Errorf("upsert update: mean=%v", got2)
	}

	list, err := s.ListAnomalyBaselines(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}

	if err := s.DeleteAnomalyBaseline(ctx, got2.ID); err != nil {
		t.Fatalf("DeleteAnomalyBaseline: %v", err)
	}
	gone, _ := s.GetAnomalyBaseline(ctx, "rebuffer_ratio", "global", 300)
	if gone != nil {
		t.Error("expected nil after delete")
	}
	t.Log("PASS: Anomaly baselines round-trip")
}

// ─── Probes + MetaProbeConfigSource ──────────────────────────────────────────

func TestPG_Probes_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openPGStore(t)

	p := meta.ProbeRow{
		Name: "pg-probe", URL: "http://example.com", Protocol: "http",
		IntervalS: 30, TimeoutS: 5, Enabled: true,
	}
	created, err := s.CreateProbe(ctx, p)
	if err != nil {
		t.Fatalf("CreateProbe: %v", err)
	}

	got, err := s.GetProbe(ctx, created.ID)
	if err != nil || got == nil || got.Name != "pg-probe" {
		t.Fatalf("GetProbe: err=%v got=%v", err, got)
	}

	list, err := s.ListProbes(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("ListProbes: %v", err)
	}

	got.Name = "pg-probe-updated"
	if err := s.UpdateProbe(ctx, *got); err != nil {
		t.Fatalf("UpdateProbe: %v", err)
	}

	// MetaProbeConfigSource.ListEnabled.
	src := meta.NewProbeConfigSource(s)
	enabled, err := src.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("ListEnabled: expected 1, got %d", len(enabled))
	}
	if enabled[0].Name != "pg-probe-updated" {
		t.Errorf("ListEnabled[0].Name: %q", enabled[0].Name)
	}

	// MetaProbeConfigSource.RecordResult.
	result := domain.ProbeResult{
		ID:      "result-001",
		ProbeID: created.ID,
		Success: true,
		TS:      time.Now(),
	}
	if err := src.RecordResult(ctx, result); err != nil {
		t.Fatalf("RecordResult: %v", err)
	}

	if err := s.DeleteProbe(ctx, created.ID); err != nil {
		t.Fatalf("DeleteProbe: %v", err)
	}
	list2, _ := s.ListProbes(ctx)
	if len(list2) != 0 {
		t.Errorf("expected 0 probes after delete, got %d", len(list2))
	}
	t.Log("PASS: Probes + MetaProbeConfigSource round-trip")
}

// ─── Ping ─────────────────────────────────────────────────────────────────────

func TestPG_Ping(t *testing.T) {
	s := openPGStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	t.Log("PASS: Ping")
}
