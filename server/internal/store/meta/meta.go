// Package meta implements the metadata store (config, users, API tokens, AMS
// sources, alert rules/channels/history, report schedules, license state).
//
// Backends: SQLite (default, single node, CGO-free via modernc.org/sqlite) and
// Postgres (HA option). DDL lives in contracts/db/meta/ and uses the common SQL
// subset. Secrets (AMS credentials, channel tokens) are AES-256-GCM encrypted
// at rest.
//
// # Encryption key derivation
//
// The encryption key is sourced (in preference order):
//  1. PULSE_SECRET_KEY env var (hex-encoded 32-byte key, or arbitrary string
//     padded/hashed with SHA-256).
//  2. A file at <meta_dir>/pulse_secret.key — generated randomly on first run,
//     stored in the same directory as the SQLite database.
//     Document: never commit this file; back it up with your database.
package meta

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // pure-Go Postgres driver; registers "pgx" driver name
	_ "modernc.org/sqlite"             // pure-Go SQLite driver
)

// parseKeysetCursor splits a "<int64>:<id>" keyset cursor string used for
// stable pagination over SQLite tables. Returns (0, "") on empty or malformed
// input so the caller silently falls back to the first page — matching the
// OpenAPI contract which treats cursor as opaque and mandates no error on
// invalid values.
func parseKeysetCursor(cursor string) (int64, string) {
	if cursor == "" {
		return 0, ""
	}
	i := strings.IndexByte(cursor, ':')
	if i < 0 {
		return 0, ""
	}
	ts, err := strconv.ParseInt(cursor[:i], 10, 64)
	if err != nil {
		return 0, ""
	}
	return ts, cursor[i+1:]
}

// AlertHistoryDefaultKeep is the default maximum number of alert_history rows
// retained per rule_id. CreateAlertHistory automatically prunes to this cap
// after every insert so that a flapping rule cannot grow the SQLite file
// unboundedly. Override for tests via SetAlertHistoryCap.
const AlertHistoryDefaultKeep = 1000

// Store is the metadata store — wraps SQLite or Postgres.
type Store struct {
	db              *sql.DB
	backend         string // "sqlite" or "postgres"
	cipherKey       [32]byte
	hmacKey         [32]byte // derived from cipherKey via domain-separated SHA-256
	hasExplicitKey  bool     // true when PULSE_SECRET_KEY was provided (not auto-generated)
	alertHistoryCap int      // 0 = use AlertHistoryDefaultKeep; overridable in tests
}

// SetAlertHistoryCap overrides the per-rule alert_history cap used by
// CreateAlertHistory. A value <= 0 resets to AlertHistoryDefaultKeep.
// Intended for tests only; not safe for concurrent use.
func (s *Store) SetAlertHistoryCap(n int) {
	s.alertHistoryCap = n
}

// New opens (and creates if needed) the meta store at the given DSN.
//
//   - SQLite: DSN is a file path (e.g. "/var/lib/pulse/meta.db") or ":memory:".
//   - Postgres: DSN is a postgres:// connection string.
//
// The secretKey is used to derive the AES-256-GCM encryption key for secrets
// at rest. If empty, the key is loaded from (or generated in) the same
// directory as the database file.
func New(ctx context.Context, backend, dsn, secretKey string) (*Store, error) {
	var db *sql.DB
	var err error

	switch backend {
	case "sqlite", "":
		db, err = sql.Open("sqlite", dsn+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
		if err != nil {
			return nil, fmt.Errorf("meta store: open sqlite %s: %w", dsn, err)
		}
		// SQLite should not be used with concurrent writers; single conn is fine
		db.SetMaxOpenConns(1)
		backend = "sqlite"
	case "postgres":
		// PULSE_SECRET_KEY is mandatory for Postgres: filepath.Dir of a postgres://
		// DSN is not a valid filesystem path, so key-file generation is impossible.
		// Fail loudly here rather than silently writing an ephemeral key that will
		// be lost on next restart.
		if secretKey == "" {
			return nil, fmt.Errorf("meta store: PULSE_SECRET_KEY is required for the postgres meta backend (cannot auto-generate a key file from a postgres:// DSN)")
		}
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("meta store: open postgres %s: %w", dsn, err)
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	default:
		return nil, fmt.Errorf("meta store: unknown backend %q (supported: sqlite, postgres)", backend)
	}

	// Ping.
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("meta store: ping: %w", err)
	}

	// Derive cipher key.
	cipherKey, err := deriveKey(secretKey, dsn)
	if err != nil {
		return nil, fmt.Errorf("meta store: derive encryption key: %w", err)
	}

	s := &Store{
		db:             db,
		backend:        backend,
		cipherKey:      cipherKey,
		hmacKey:        deriveHMACKey(cipherKey),
		hasExplicitKey: secretKey != "",
	}
	return s, nil
}

// Migrate applies the DDL from the given path idempotently.
// All CREATE TABLE / ALTER TABLE statements use IF NOT EXISTS so re-running is
// safe. After applying the DDL, applySchemaUpgrades runs idempotent ALTER TABLE
// statements so that existing SQLite databases (e.g. prod) gain new columns on
// next startup without requiring a separate migration command.
//
// For the Postgres backend, applySchemaUpgrades is a no-op (returns nil early);
// schema evolution is handled exclusively via numbered migration files.
func (s *Store) Migrate(ctx context.Context, ddlPath string) error {
	// File-path DDL override is a SQLite-only escape hatch. Applying an
	// arbitrary file to Postgres is a footgun (the sqlite DDL's 32-bit INTEGER
	// epoch-ms columns and INSERT OR IGNORE would corrupt or fail) — the
	// postgres backend migrates exclusively via the embedded PG DDL.
	if s.backend == "postgres" {
		return fmt.Errorf("meta migrate: PULSE_META_DDL_PATH is sqlite-only; the postgres backend uses embedded migrations — unset PULSE_META_DDL_PATH")
	}
	data, err := os.ReadFile(ddlPath)
	if err != nil {
		return fmt.Errorf("meta migrate: read ddl %s: %w", ddlPath, err)
	}

	// Split on semicolons and execute each statement.
	stmts := splitSQL(string(data))
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		if _, err := s.execContext(ctx, stmt); err != nil {
			// Tolerate PRAGMA errors on postgres-compat statements.
			if strings.Contains(err.Error(), "PRAGMA") || strings.Contains(err.Error(), "foreign_keys") {
				continue
			}
			return fmt.Errorf("meta migrate: exec %q: %w", abbreviate(stmt, 80), err)
		}
	}
	return s.applySchemaUpgrades(ctx)
}

// MigrateEmbedded applies the embedded meta DDL idempotently.
//
// Backend routing:
//   - "postgres": the passed ddl string is IGNORED; EmbeddedDDLPostgres
//     (0001_init + 0002_anomaly_alert_rule, both in PG syntax) is applied
//     instead. This ensures callers that pass the SQLite EmbeddedDDL constant
//     still get a correct PG schema automatically.
//   - "sqlite" (default): the passed ddl string is used as-is, then
//     applySchemaUpgrades runs idempotent ALTER TABLE statements for columns
//     added in later migrations.
func (s *Store) MigrateEmbedded(ctx context.Context, ddl string) error {
	if s.backend == "postgres" {
		ddl = EmbeddedDDLPostgres
	}
	stmts := splitSQL(ddl)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		if _, err := s.execContext(ctx, stmt); err != nil {
			if strings.Contains(err.Error(), "PRAGMA") || strings.Contains(err.Error(), "foreign_keys") {
				continue
			}
			return fmt.Errorf("meta migrate: exec %q: %w", abbreviate(stmt, 80), err)
		}
	}
	return s.applySchemaUpgrades(ctx)
}

// applySchemaUpgrades runs idempotent ALTER TABLE statements to add columns
// that are in the current schema but may be missing from databases created
// before those columns were added. Safe to call on every startup.
//
// Current upgrades:
//   - api_tokens.hash_alg (added in P2 hardening): stores 'hmac-sha256' or
//     'sha256' so auth can use the correct hash algorithm on lookup.
//   - ams_sources.webhook_secret_enc (added in B7, D-062): AES-256-GCM
//     encrypted per-source HMAC secret for /webhook/ams/{name} dispatch.
//   - alert_rules.rule_type, .sigma, .min_samples (S11 0002 migration): anomaly
//     rule columns added by 0002_anomaly_alert_rule.sql; the applySchemaUpgrades
//     path makes them available on all databases regardless of migration runner.
//
// Note: these upgrades are SQLite-specific. Postgres schemas are managed via
// dedicated migration tooling; the function returns nil early for Postgres
// (matching the hash_alg precedent).
func (s *Store) applySchemaUpgrades(ctx context.Context) error {
	// SQLite-specific PRAGMA-based upgrades only.
	if s.backend != "sqlite" {
		return nil
	}

	// ── api_tokens.hash_alg ──────────────────────────────────────────────────
	// Check for hash_alg column in api_tokens using PRAGMA table_info.
	// Table may not exist yet on a fresh DB before the DDL runs; harmless.
	rows, err := s.queryContext(ctx, "PRAGMA table_info(api_tokens)")
	if err == nil {
		hasHashAlg := false
		for rows.Next() {
			var cid int
			var name, ctype string
			var notNull int
			var dflt sql.NullString
			var pk int
			if scanErr := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); scanErr != nil {
				continue
			}
			if name == "hash_alg" {
				hasHashAlg = true
			}
		}
		_ = rows.Close()
		if !hasHashAlg {
			// DEFAULT 'sha256' labels all existing rows as legacy.
			if _, err := s.execContext(ctx,
				`ALTER TABLE api_tokens ADD COLUMN hash_alg TEXT NOT NULL DEFAULT 'sha256'`); err != nil {
				return fmt.Errorf("schema upgrade: add api_tokens.hash_alg: %w", err)
			}
		}
	}

	// ── ams_sources.webhook_secret_enc (B7) ──────────────────────────────────
	// Check for webhook_secret_enc column in ams_sources.
	rows2, err := s.queryContext(ctx, "PRAGMA table_info(ams_sources)")
	if err == nil {
		hasWebhookSecretEnc := false
		for rows2.Next() {
			var cid int
			var name, ctype string
			var notNull int
			var dflt sql.NullString
			var pk int
			if scanErr := rows2.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); scanErr != nil {
				continue
			}
			if name == "webhook_secret_enc" {
				hasWebhookSecretEnc = true
			}
		}
		_ = rows2.Close()
		if !hasWebhookSecretEnc {
			// NULL default: existing rows have no per-source secret (fall through
			// to SharedSecret on the /webhook/ams/{name} route).
			if _, err := s.execContext(ctx,
				`ALTER TABLE ams_sources ADD COLUMN webhook_secret_enc TEXT`); err != nil {
				return fmt.Errorf("schema upgrade: add ams_sources.webhook_secret_enc: %w", err)
			}
		}
	}

	// ── alert_rules anomaly columns (S11 0002) ────────────────────────────────
	// The 0002_anomaly_alert_rule.sql migration adds rule_type, sigma, and
	// min_samples. We add them here too so that databases initialised via
	// MigrateEmbedded (which runs only the 0001 DDL embed) also get the columns
	// after applySchemaUpgrades — no separate migration runner needed.
	rows3, err := s.queryContext(ctx, "PRAGMA table_info(alert_rules)")
	if err == nil {
		hasRuleType, hasSigma, hasMinSamples := false, false, false
		for rows3.Next() {
			var cid int
			var name, ctype string
			var notNull int
			var dflt sql.NullString
			var pk int
			if scanErr := rows3.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); scanErr != nil {
				continue
			}
			switch name {
			case "rule_type":
				hasRuleType = true
			case "sigma":
				hasSigma = true
			case "min_samples":
				hasMinSamples = true
			}
		}
		_ = rows3.Close()
		if !hasRuleType {
			if _, err := s.execContext(ctx,
				`ALTER TABLE alert_rules ADD COLUMN rule_type TEXT NOT NULL DEFAULT 'threshold'`); err != nil {
				return fmt.Errorf("schema upgrade: add alert_rules.rule_type: %w", err)
			}
		}
		if !hasSigma {
			if _, err := s.execContext(ctx,
				`ALTER TABLE alert_rules ADD COLUMN sigma REAL`); err != nil {
				return fmt.Errorf("schema upgrade: add alert_rules.sigma: %w", err)
			}
		}
		if !hasMinSamples {
			if _, err := s.execContext(ctx,
				`ALTER TABLE alert_rules ADD COLUMN min_samples INTEGER`); err != nil {
				return fmt.Errorf("schema upgrade: add alert_rules.min_samples: %w", err)
			}
		}
	}

	// ── vod_poll_state table (S23 0003) ──────────────────────────────────────
	// The 0003_vod_poll_state.sql migration creates the vod_poll_state seen-set
	// table for BUG-002. We create it here idempotently so that SQLite databases
	// initialised via MigrateEmbedded (which embeds only 0001) also get the
	// table on startup — no separate migration runner is needed.
	rows4, err := s.queryContext(ctx, "PRAGMA table_info(vod_poll_state)")
	if err == nil {
		hasTable := false
		for rows4.Next() {
			hasTable = true
			// Drain the rows — we only need to know if any columns exist.
			var cid int
			var name, ctype string
			var notNull int
			var dflt sql.NullString
			var pk int
			_ = rows4.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk)
		}
		_ = rows4.Close()
		if !hasTable {
			if _, err := s.execContext(ctx,
				`CREATE TABLE IF NOT EXISTS vod_poll_state (
				    app        TEXT    NOT NULL,
				    vod_id     TEXT    NOT NULL,
				    created_ms INTEGER NOT NULL DEFAULT 0,
				    PRIMARY KEY (app, vod_id)
				)`); err != nil {
				return fmt.Errorf("schema upgrade: create vod_poll_state: %w", err)
			}
		}
	}

	// ── audit_log table (S40 0004) ───────────────────────────────────────────
	// The 0004_audit_log.sql migration creates the append-only audit trail. We
	// create it (and its ts-desc index) here idempotently so that SQLite databases
	// initialised via MigrateEmbedded (which embeds only 0001) also get the table
	// on startup — no separate migration runner is needed.
	rows5, err := s.queryContext(ctx, "PRAGMA table_info(audit_log)")
	if err == nil {
		hasTable := false
		for rows5.Next() {
			hasTable = true
			var cid int
			var name, ctype string
			var notNull int
			var dflt sql.NullString
			var pk int
			_ = rows5.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk)
		}
		_ = rows5.Close()
		if !hasTable {
			if _, err := s.execContext(ctx,
				`CREATE TABLE IF NOT EXISTS audit_log (
				    id             TEXT    NOT NULL PRIMARY KEY,
				    ts             INTEGER NOT NULL,
				    actor_token_id TEXT    NOT NULL DEFAULT '',
				    actor_user_id  TEXT    NOT NULL DEFAULT '',
				    actor_name     TEXT    NOT NULL DEFAULT '',
				    action         TEXT    NOT NULL,
				    object_type    TEXT    NOT NULL,
				    object_id      TEXT    NOT NULL DEFAULT '',
				    remote_addr    TEXT    NOT NULL DEFAULT '',
				    detail_json    TEXT    NOT NULL DEFAULT ''
				)`); err != nil {
				return fmt.Errorf("schema upgrade: create audit_log: %w", err)
			}
			if _, err := s.execContext(ctx,
				`CREATE INDEX IF NOT EXISTS idx_audit_log_ts ON audit_log(ts DESC, id DESC)`); err != nil {
				return fmt.Errorf("schema upgrade: create idx_audit_log_ts: %w", err)
			}
		}
	}

	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping pings the database with the given context. Used by /healthz to check liveness.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// ─── Users ────────────────────────────────────────────────────────────────────

// User represents a local user account.
type User struct {
	ID        string
	Username  string
	PwHash    string
	Role      string // "admin" | "viewer"
	CreatedAt int64  // Unix epoch ms
	UpdatedAt int64  // Unix epoch ms
}

// CreateUser inserts a new user. pw_hash must be a bcrypt hash.
func (s *Store) CreateUser(ctx context.Context, u User) error {
	if u.ID == "" {
		u.ID = newUUID()
	}
	now := nowMS()
	_, err := s.execContext(ctx,
		`INSERT INTO users (id, username, pw_hash, role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PwHash, u.Role, now, now)
	return err
}

// GetUserByUsername fetches a user by username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, username, pw_hash, role, created_at, updated_at FROM users WHERE username = ?`,
		username)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PwHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetUserByID fetches a user by id. Returns (nil, nil) when not found.
func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, username, pw_hash, role, created_at, updated_at FROM users WHERE id = ?`,
		id)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PwHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// ListUsers returns users ordered by created_at ASC, id ASC.
// limit<=0 means no LIMIT (unbounded); cursor="" means first page.
func (s *Store) ListUsers(ctx context.Context, limit int, cursor string) ([]User, error) {
	q := `SELECT id, username, pw_hash, role, created_at, updated_at FROM users WHERE 1=1`
	var args []any
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at ASC, id ASC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PwHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// CountUsers returns the number of users in the store.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.queryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// UpdateUser updates an existing user.
func (s *Store) UpdateUser(ctx context.Context, id, username, role string) error {
	_, err := s.execContext(ctx,
		`UPDATE users SET username=?, role=?, updated_at=? WHERE id=?`,
		username, role, nowMS(), id)
	return err
}

// DeleteUser deletes a user by ID.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}

// ─── API Tokens ───────────────────────────────────────────────────────────────

// APIToken represents a bearer token record.
type APIToken struct {
	ID         string
	UserID     string
	Kind       string // "api" | "ingest"
	Name       string
	TokenHash  string // hex-encoded hash of raw token (see HashAlg)
	HashAlg    string // "hmac-sha256" | "sha256" (empty treated as "sha256")
	Scopes     []string
	ExpiresAt  *int64
	LastUsedAt *int64
	CreatedAt  int64
}

// CreateToken inserts a new API token.
// If HashAlg is empty, it defaults to "sha256" for backward compatibility.
func (s *Store) CreateToken(ctx context.Context, t APIToken) error {
	if t.ID == "" {
		t.ID = newUUID()
	}
	hashAlg := t.HashAlg
	if hashAlg == "" {
		hashAlg = "sha256"
	}
	scopesJSON, _ := json.Marshal(t.Scopes)
	// user_id must be NULL (not empty string) when not set to avoid FK violation.
	var userID any
	if t.UserID != "" {
		userID = t.UserID
	}
	_, err := s.execContext(ctx,
		`INSERT INTO api_tokens (id, user_id, kind, name, token_hash, hash_alg, scopes, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, userID, t.Kind, t.Name, t.TokenHash, hashAlg, string(scopesJSON), t.ExpiresAt, t.CreatedAt)
	return err
}

// GetTokenByHash looks up a token by its stored hash value (exact match).
// Callers that have a raw token should use LookupToken instead, which handles
// both HMAC-SHA256 (new tokens) and plain SHA-256 (legacy tokens) transparently.
func (s *Store) GetTokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, user_id, kind, name, token_hash, hash_alg, scopes, expires_at, last_used_at, created_at
		 FROM api_tokens WHERE token_hash = ?`, hash)
	return scanAPIToken(row)
}

// LookupToken resolves a raw bearer token to its record.
//
// When PULSE_SECRET_KEY is configured, HMAC-SHA256 lookup is tried first
// (new tokens). If not found, plain SHA-256 lookup runs as a fallback for
// legacy rows (including any live-prod admin token created before this
// upgrade). This makes upgrading from SHA-256 to HMAC fully transparent
// without requiring a database re-hash pass.
//
// In dev mode (no explicit PULSE_SECRET_KEY), only SHA-256 is tried.
func (s *Store) LookupToken(ctx context.Context, rawToken string) (*APIToken, error) {
	if s.hasExplicitKey {
		// Try HMAC hash first (new tokens hashed with store.HashToken).
		hmacHash := s.hmacHex(rawToken)
		tok, err := s.GetTokenByHash(ctx, hmacHash)
		if err != nil {
			return nil, err
		}
		if tok != nil {
			return tok, nil
		}
		// Fall through to SHA-256 for legacy rows.
	}
	// Dev mode or legacy fallback: plain SHA-256 lookup.
	sha256Hash := sha256HexLocal(rawToken)
	return s.GetTokenByHash(ctx, sha256Hash)
}

// HashToken returns the canonical hash and algorithm identifier for a raw token.
//
// When PULSE_SECRET_KEY is configured (hasExplicitKey), returns
// HMAC-SHA256(hmacKey, token) and "hmac-sha256". The HMAC key is derived from
// the store's cipher key via a domain-separated SHA-256 to avoid key reuse
// between the AES-GCM cipher and the HMAC token hash.
//
// In dev mode (no explicit PULSE_SECRET_KEY), returns plain SHA-256 and "sha256".
// This is documented behavior: without a key, tokens are not HMAC-protected but
// the dev DB is not typically accessible to attackers. Production MUST set
// PULSE_SECRET_KEY to get HMAC protection.
func (s *Store) HashToken(rawToken string) (hash, alg string) {
	if s.hasExplicitKey {
		return s.hmacHex(rawToken), "hmac-sha256"
	}
	return sha256HexLocal(rawToken), "sha256"
}

// ListTokens returns tokens ordered by created_at DESC, id DESC.
// kind filters by token kind when non-empty. limit<=0 means no LIMIT;
// cursor="" means first page. Cursor is DESC keyset (older rows on later pages).
func (s *Store) ListTokens(ctx context.Context, kind string, limit int, cursor string) ([]APIToken, error) {
	q := `SELECT id, user_id, kind, name, token_hash, hash_alg, scopes, expires_at, last_used_at, created_at
	      FROM api_tokens`
	var args []any
	hasWhere := false
	if kind != "" {
		q += " WHERE kind = ?"
		args = append(args, kind)
		hasWhere = true
	}
	if ts, id := parseKeysetCursor(cursor); id != "" {
		if hasWhere {
			q += " AND (created_at < ? OR (created_at = ? AND id < ?))"
		} else {
			q += " WHERE (created_at < ? OR (created_at = ? AND id < ?))"
		}
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at DESC, id DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, *t)
	}
	return tokens, rows.Err()
}

// TouchToken updates last_used_at for a token.
func (s *Store) TouchToken(ctx context.Context, id string) {
	now := nowMS()
	_, _ = s.execContext(ctx, `UPDATE api_tokens SET last_used_at=? WHERE id=?`, now, id)
}

// DeleteToken removes a token by ID (revoke).
func (s *Store) DeleteToken(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM api_tokens WHERE id=?`, id)
	return err
}

// CountTokens returns the number of tokens.
func (s *Store) CountTokens(ctx context.Context) (int, error) {
	var n int
	err := s.queryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens`).Scan(&n)
	return n, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAPIToken(row scanner) (*APIToken, error) {
	var t APIToken
	var scopesJSON string
	var userID sql.NullString
	if err := row.Scan(&t.ID, &userID, &t.Kind, &t.Name, &t.TokenHash, &t.HashAlg, &scopesJSON,
		&t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.UserID = userID.String
	_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
	return &t, nil
}

// ─── Alert Rules ──────────────────────────────────────────────────────────────

// AlertRuleRow mirrors the alert_rules table.
type AlertRuleRow struct {
	ID                 string
	Name               string // human-readable display name (CR-1)
	Metric             string
	Operator           string
	Threshold          float64
	WindowS            int
	ScopeJSON          string
	Severity           string
	CooldownS          int
	GroupBy            sql.NullString
	Enabled            bool // when false, rule is not evaluated at all (CR-2)
	Muted              bool
	MaintenanceWindows string // JSON array
	ChannelIDs         string // JSON array
	CreatedAt          int64
	UpdatedAt          int64
	// S11 WO-B: anomaly rule fields (0002 migration).
	// RuleType is "threshold" (default) or "anomaly".
	// Empty string is normalised to "threshold" in CreateAlertRule for backward compat.
	RuleType   string
	Sigma      float64 // effective sigma: >0 overrides anomaly.DefaultSigma
	MinSamples int     // effective min_samples: >0 overrides anomaly.MinSamples
}

// CreateAlertRule inserts a new rule.
func (s *Store) CreateAlertRule(ctx context.Context, r AlertRuleRow) (AlertRuleRow, error) {
	if r.ID == "" {
		r.ID = newUUID()
	}
	now := nowMS()
	r.CreatedAt = now
	r.UpdatedAt = now
	if r.CooldownS == 0 {
		r.CooldownS = 300
	}
	if r.MaintenanceWindows == "" {
		r.MaintenanceWindows = "[]"
	}
	if r.ChannelIDs == "" {
		r.ChannelIDs = "[]"
	}
	if r.ScopeJSON == "" {
		r.ScopeJSON = "{}"
	}
	// S11 WO-B: normalise empty RuleType to "threshold" for backward compat.
	if r.RuleType == "" {
		r.RuleType = "threshold"
	}
	_, err := s.execContext(ctx,
		`INSERT INTO alert_rules
		 (id, name, metric, operator, threshold, window_s, scope, severity, cooldown_s, group_by,
		  enabled, muted, maintenance_windows, channel_ids, created_at, updated_at,
		  rule_type, sigma, min_samples)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.Metric, r.Operator, r.Threshold, r.WindowS, r.ScopeJSON,
		r.Severity, r.CooldownS, r.GroupBy, boolToInt(r.Enabled), boolToInt(r.Muted),
		r.MaintenanceWindows, r.ChannelIDs, r.CreatedAt, r.UpdatedAt,
		r.RuleType, r.Sigma, r.MinSamples)
	return r, err
}

// GetAlertRule fetches a rule by ID.
func (s *Store) GetAlertRule(ctx context.Context, id string) (*AlertRuleRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, name, metric, operator, threshold, window_s, scope, severity, cooldown_s, group_by,
		        enabled, muted, maintenance_windows, channel_ids, created_at, updated_at,
		        COALESCE(rule_type,'threshold'), COALESCE(sigma,0), COALESCE(min_samples,0)
		 FROM alert_rules WHERE id=?`, id)
	return scanAlertRule(row)
}

// ListAlertRules returns alert rules ordered by created_at ASC, id ASC.
// limit<=0 means no LIMIT; cursor="" means first page.
func (s *Store) ListAlertRules(ctx context.Context, limit int, cursor string) ([]AlertRuleRow, error) {
	q := `SELECT id, name, metric, operator, threshold, window_s, scope, severity, cooldown_s, group_by,
	             enabled, muted, maintenance_windows, channel_ids, created_at, updated_at,
	             COALESCE(rule_type,'threshold'), COALESCE(sigma,0), COALESCE(min_samples,0)
	      FROM alert_rules WHERE 1=1`
	var args []any
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at ASC, id ASC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []AlertRuleRow
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, *r)
	}
	return rules, rows.Err()
}

// UpdateAlertRule updates a rule by ID.
func (s *Store) UpdateAlertRule(ctx context.Context, r AlertRuleRow) error {
	r.UpdatedAt = nowMS()
	// S11 WO-B: normalise empty RuleType to "threshold" for backward compat.
	if r.RuleType == "" {
		r.RuleType = "threshold"
	}
	_, err := s.execContext(ctx,
		`UPDATE alert_rules SET name=?, metric=?, operator=?, threshold=?, window_s=?, scope=?,
		  severity=?, cooldown_s=?, group_by=?, enabled=?, muted=?, maintenance_windows=?, channel_ids=?,
		  updated_at=?, rule_type=?, sigma=?, min_samples=? WHERE id=?`,
		r.Name, r.Metric, r.Operator, r.Threshold, r.WindowS, r.ScopeJSON,
		r.Severity, r.CooldownS, r.GroupBy, boolToInt(r.Enabled), boolToInt(r.Muted),
		r.MaintenanceWindows, r.ChannelIDs, r.UpdatedAt, r.RuleType, r.Sigma, r.MinSamples, r.ID)
	return err
}

// DeleteAlertRule removes a rule by ID.
func (s *Store) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM alert_rules WHERE id=?`, id)
	return err
}

func scanAlertRule(row scanner) (*AlertRuleRow, error) {
	var r AlertRuleRow
	var enabled, muted int
	if err := row.Scan(&r.ID, &r.Name, &r.Metric, &r.Operator, &r.Threshold, &r.WindowS,
		&r.ScopeJSON, &r.Severity, &r.CooldownS, &r.GroupBy, &enabled, &muted,
		&r.MaintenanceWindows, &r.ChannelIDs, &r.CreatedAt, &r.UpdatedAt,
		&r.RuleType, &r.Sigma, &r.MinSamples); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.Enabled = enabled != 0
	r.Muted = muted != 0
	return &r, nil
}

// ─── Alert Channels ───────────────────────────────────────────────────────────

// AlertChannelRow mirrors the alert_channels table.
type AlertChannelRow struct {
	ID           string
	Type         string
	Name         string
	ConfigEnc    string // encrypted
	ConfigPublic string // JSON
	CreatedAt    int64
	UpdatedAt    int64
}

// CreateAlertChannel inserts a new channel, encrypting the config.
func (s *Store) CreateAlertChannel(ctx context.Context, c AlertChannelRow) (AlertChannelRow, error) {
	if c.ID == "" {
		c.ID = newUUID()
	}
	now := nowMS()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.ConfigPublic == "" {
		c.ConfigPublic = "{}"
	}
	_, err := s.execContext(ctx,
		`INSERT INTO alert_channels (id, type, name, config_enc, config_public, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		c.ID, c.Type, c.Name, c.ConfigEnc, c.ConfigPublic, c.CreatedAt, c.UpdatedAt)
	return c, err
}

// GetAlertChannel fetches a channel by ID.
func (s *Store) GetAlertChannel(ctx context.Context, id string) (*AlertChannelRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, type, name, config_enc, config_public, created_at, updated_at
		 FROM alert_channels WHERE id=?`, id)
	return scanAlertChannel(row)
}

// ListAlertChannels returns channels ordered by created_at ASC, id ASC.
// limit<=0 means no LIMIT; cursor="" means first page.
func (s *Store) ListAlertChannels(ctx context.Context, limit int, cursor string) ([]AlertChannelRow, error) {
	q := `SELECT id, type, name, config_enc, config_public, created_at, updated_at
	      FROM alert_channels WHERE 1=1`
	var args []any
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at ASC, id ASC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []AlertChannelRow
	for rows.Next() {
		c, err := scanAlertChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, *c)
	}
	return channels, rows.Err()
}

// UpdateAlertChannel updates a channel by ID.
func (s *Store) UpdateAlertChannel(ctx context.Context, c AlertChannelRow) error {
	c.UpdatedAt = nowMS()
	_, err := s.execContext(ctx,
		`UPDATE alert_channels SET type=?, name=?, config_enc=?, config_public=?, updated_at=?
		 WHERE id=?`,
		c.Type, c.Name, c.ConfigEnc, c.ConfigPublic, c.UpdatedAt, c.ID)
	return err
}

// DeleteAlertChannel removes a channel by ID.
func (s *Store) DeleteAlertChannel(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM alert_channels WHERE id=?`, id)
	return err
}

func scanAlertChannel(row scanner) (*AlertChannelRow, error) {
	var c AlertChannelRow
	if err := row.Scan(&c.ID, &c.Type, &c.Name, &c.ConfigEnc, &c.ConfigPublic, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// ─── Alert History ────────────────────────────────────────────────────────────

// AlertHistoryRow mirrors the alert_history table.
type AlertHistoryRow struct {
	ID            string
	AlertID       string
	RuleID        string
	State         string // "firing" | "resolved"
	Severity      string
	TS            int64
	Metric        string
	Value         float64
	Threshold     float64
	ScopeJSON     string
	CooldownUntil *int64
	GroupKey      sql.NullString
	Test          bool
}

// CreateAlertHistory inserts a history row and then prunes older rows for the
// same rule_id to keep the table bounded. The cap is AlertHistoryDefaultKeep
// unless overridden by SetAlertHistoryCap (for tests). A prune error is
// non-fatal — the insert succeeds even if pruning fails.
func (s *Store) CreateAlertHistory(ctx context.Context, h AlertHistoryRow) error {
	if h.ID == "" {
		h.ID = newUUID()
	}
	if h.ScopeJSON == "" {
		h.ScopeJSON = "{}"
	}
	_, err := s.execContext(ctx,
		`INSERT INTO alert_history
		 (id, alert_id, rule_id, state, severity, ts, metric, value, threshold,
		  scope, cooldown_until, group_key, test)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.AlertID, h.RuleID, h.State, h.Severity, h.TS,
		h.Metric, h.Value, h.Threshold, h.ScopeJSON, h.CooldownUntil, h.GroupKey,
		boolToInt(h.Test))
	if err != nil {
		return err
	}

	cap := s.alertHistoryCap
	if cap <= 0 {
		cap = AlertHistoryDefaultKeep
	}
	// Non-fatal: log-and-continue on prune failure; the insert already committed.
	_ = s.PruneAlertHistory(ctx, h.RuleID, cap)
	return nil
}

// PruneAlertHistory deletes all but the newest `keep` rows for the given
// rule_id. Keep-newest semantics: rows with the lowest ts values are removed
// first; equal-ts tiebreak is backend-specific (see below).
//
// keep <= 0 is a safe no-op — no rows are deleted. This prevents accidental
// mass-deletion if the cap is misconfigured.
//
// Implementation: COUNT the rows first; skip the DELETE entirely when no
// pruning is needed (common path: first `keep` inserts). When excess rows exist,
// DELETE the oldest `excess` rows. O(excess) per call.
// At n=2000 and keep=1000 on :memory: SQLite this completes in well under 10 ms.
//
// TIEBREAK DIVERGENCE (ts equal):
//   - SQLite: uses rowid (implicit auto-increment integer) — exact insertion
//     order. Test TestAlertHistory_PruneEqualTsDeterministic verifies this and
//     is SQLite-only (no //go:build tag, runs in the default test pass).
//   - Postgres: uses (ts ASC, id ASC) where id is a UUID (text). UUID ordering
//     does NOT guarantee insertion order for UUIDv4 random values. The
//     keep-newest semantic (drop oldest ts) is preserved; equal-ts order is
//     non-deterministic. No parity with the SQLite rowid tiebreak test.
func (s *Store) PruneAlertHistory(ctx context.Context, ruleID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	var total int
	if err := s.queryRowContext(ctx,
		"SELECT COUNT(*) FROM alert_history WHERE rule_id = ?", ruleID).Scan(&total); err != nil {
		return err
	}
	excess := total - keep
	if excess <= 0 {
		return nil
	}

	var deleteSQL string
	if s.backend == "postgres" {
		// Postgres has no rowid pseudo-column. Use id (UUID) as tiebreak.
		// Keep-newest semantics preserved: lowest ts rows are pruned first.
		deleteSQL = `DELETE FROM alert_history
		             WHERE id IN (
		                 SELECT id FROM alert_history
		                 WHERE rule_id = ?
		                 ORDER BY ts ASC, id ASC
		                 LIMIT ?
		             )`
	} else {
		// SQLite: rowid reflects exact insertion order — deterministic tiebreak.
		deleteSQL = `DELETE FROM alert_history
		             WHERE rowid IN (
		                 SELECT rowid FROM alert_history
		                 WHERE rule_id = ?
		                 ORDER BY ts ASC, rowid ASC
		                 LIMIT ?
		             )`
	}
	_, err := s.execContext(ctx, deleteSQL, ruleID, excess)
	return err
}

// ListAlertHistory returns history entries ordered by ts DESC, id DESC.
// Filtered by optional ruleID, state, from/to time range. limit<=0 means no
// LIMIT; cursor="" means first page. Cursor is DESC keyset (older rows later).
func (s *Store) ListAlertHistory(ctx context.Context, ruleID, state string, from, to int64, limit int, cursor string) ([]AlertHistoryRow, error) {
	q := `SELECT id, alert_id, rule_id, state, severity, ts, metric, value, threshold,
	             scope, cooldown_until, group_key, test
	      FROM alert_history WHERE 1=1`
	var args []any
	if ruleID != "" {
		q += " AND rule_id = ?"
		args = append(args, ruleID)
	}
	if state != "" {
		q += " AND state = ?"
		args = append(args, state)
	}
	if from > 0 {
		q += " AND ts >= ?"
		args = append(args, from)
	}
	if to > 0 {
		q += " AND ts <= ?"
		args = append(args, to)
	}
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (ts < ? OR (ts = ? AND id < ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY ts DESC, id DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hist []AlertHistoryRow
	for rows.Next() {
		var h AlertHistoryRow
		var testInt int
		if err := rows.Scan(&h.ID, &h.AlertID, &h.RuleID, &h.State, &h.Severity, &h.TS,
			&h.Metric, &h.Value, &h.Threshold, &h.ScopeJSON, &h.CooldownUntil, &h.GroupKey,
			&testInt); err != nil {
			return nil, err
		}
		h.Test = testInt != 0
		hist = append(hist, h)
	}
	return hist, rows.Err()
}

// ─── License ──────────────────────────────────────────────────────────────────

// LicenseRow mirrors the license table (singleton row, id='singleton').
type LicenseRow struct {
	ID          string
	LicenseKey  sql.NullString
	Tier        string // "free" | "pro" | "enterprise"
	Signature   sql.NullString
	ClaimsJSON  string
	OfflinePath sql.NullString
	Valid       bool
	ExpiresAt   *int64
	ActivatedAt *int64
	UpdatedAt   int64
}

// GetLicense fetches the singleton license row, or creates a free-tier default.
func (s *Store) GetLicense(ctx context.Context) (*LicenseRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, license_key, tier, signature, claims, offline_path, valid, expires_at, activated_at, updated_at
		 FROM license WHERE id='singleton'`)
	var l LicenseRow
	var valid int
	err := row.Scan(&l.ID, &l.LicenseKey, &l.Tier, &l.Signature, &l.ClaimsJSON,
		&l.OfflinePath, &valid, &l.ExpiresAt, &l.ActivatedAt, &l.UpdatedAt)
	if err == sql.ErrNoRows {
		// Bootstrap free tier. Use ON CONFLICT DO NOTHING (portable: SQLite 3.24+
		// and PostgreSQL) instead of INSERT OR IGNORE to work on both backends.
		now := nowMS()
		l = LicenseRow{
			ID:         "singleton",
			Tier:       "free",
			ClaimsJSON: "{}",
			Valid:      true,
			UpdatedAt:  now,
		}
		_, err2 := s.execContext(ctx,
			`INSERT INTO license (id, tier, claims, valid, updated_at)
			 VALUES ('singleton', 'free', '{}', 1, ?)
			 ON CONFLICT (id) DO NOTHING`, now)
		return &l, err2
	}
	if err != nil {
		return nil, err
	}
	l.Valid = valid != 0
	return &l, nil
}

// UpsertLicense updates or inserts the singleton license row.
func (s *Store) UpsertLicense(ctx context.Context, l LicenseRow) error {
	l.UpdatedAt = nowMS()
	_, err := s.execContext(ctx,
		`INSERT INTO license (id, license_key, tier, signature, claims, offline_path, valid, expires_at, activated_at, updated_at)
		 VALUES ('singleton', ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   license_key=excluded.license_key, tier=excluded.tier,
		   signature=excluded.signature, claims=excluded.claims,
		   offline_path=excluded.offline_path, valid=excluded.valid,
		   expires_at=excluded.expires_at, activated_at=excluded.activated_at,
		   updated_at=excluded.updated_at`,
		l.LicenseKey, l.Tier, l.Signature, l.ClaimsJSON, l.OfflinePath,
		boolToInt(l.Valid), l.ExpiresAt, l.ActivatedAt, l.UpdatedAt)
	return err
}

// ─── AMS Sources ─────────────────────────────────────────────────────────────

// AMSSourceRow mirrors the ams_sources table.
type AMSSourceRow struct {
	ID               string
	Name             string
	SourceType       string
	RestURL          sql.NullString
	RestUser         sql.NullString
	CredentialEnc    sql.NullString
	CredentialEnvRef sql.NullString
	LogPath          sql.NullString
	KafkaBrokers     sql.NullString // JSON
	WebhookPath      sql.NullString
	WebhookSecretEnc sql.NullString // AES-256-GCM encrypted per-source HMAC secret (B7)
	Enabled          bool
	CreatedAt        int64
	UpdatedAt        int64
}

// CreateAMSSource inserts a new source.
func (s *Store) CreateAMSSource(ctx context.Context, src AMSSourceRow) (AMSSourceRow, error) {
	if src.ID == "" {
		src.ID = newUUID()
	}
	now := nowMS()
	src.CreatedAt = now
	src.UpdatedAt = now
	_, err := s.execContext(ctx,
		`INSERT INTO ams_sources
		 (id, name, source_type, rest_url, rest_user, credential_enc, credential_env_ref,
		  log_path, kafka_brokers, webhook_path, webhook_secret_enc, enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		src.ID, src.Name, src.SourceType, src.RestURL, src.RestUser,
		src.CredentialEnc, src.CredentialEnvRef, src.LogPath, src.KafkaBrokers,
		src.WebhookPath, src.WebhookSecretEnc, boolToInt(src.Enabled), src.CreatedAt, src.UpdatedAt)
	return src, err
}

// GetAMSSource fetches a source by ID.
func (s *Store) GetAMSSource(ctx context.Context, id string) (*AMSSourceRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, name, source_type, rest_url, rest_user, credential_enc, credential_env_ref,
		        log_path, kafka_brokers, webhook_path, webhook_secret_enc, enabled, created_at, updated_at
		 FROM ams_sources WHERE id=?`, id)
	return scanAMSSource(row)
}

// ListAMSSources returns sources ordered by created_at ASC, id ASC.
// limit<=0 means no LIMIT; cursor="" means first page.
func (s *Store) ListAMSSources(ctx context.Context, limit int, cursor string) ([]AMSSourceRow, error) {
	q := `SELECT id, name, source_type, rest_url, rest_user, credential_enc, credential_env_ref,
	             log_path, kafka_brokers, webhook_path, webhook_secret_enc, enabled, created_at, updated_at
	      FROM ams_sources WHERE 1=1`
	var args []any
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at ASC, id ASC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []AMSSourceRow
	for rows.Next() {
		src, err := scanAMSSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, *src)
	}
	return sources, rows.Err()
}

// UpdateAMSSource updates a source by ID.
func (s *Store) UpdateAMSSource(ctx context.Context, src AMSSourceRow) error {
	src.UpdatedAt = nowMS()
	_, err := s.execContext(ctx,
		`UPDATE ams_sources SET name=?, source_type=?, rest_url=?, rest_user=?, credential_enc=?,
		  credential_env_ref=?, log_path=?, kafka_brokers=?, webhook_path=?, webhook_secret_enc=?,
		  enabled=?, updated_at=?
		 WHERE id=?`,
		src.Name, src.SourceType, src.RestURL, src.RestUser, src.CredentialEnc,
		src.CredentialEnvRef, src.LogPath, src.KafkaBrokers, src.WebhookPath, src.WebhookSecretEnc,
		boolToInt(src.Enabled), src.UpdatedAt, src.ID)
	return err
}

// DeleteAMSSource removes a source by ID.
func (s *Store) DeleteAMSSource(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM ams_sources WHERE id=?`, id)
	return err
}

func scanAMSSource(row scanner) (*AMSSourceRow, error) {
	var src AMSSourceRow
	var enabled int
	if err := row.Scan(&src.ID, &src.Name, &src.SourceType, &src.RestURL, &src.RestUser,
		&src.CredentialEnc, &src.CredentialEnvRef, &src.LogPath, &src.KafkaBrokers,
		&src.WebhookPath, &src.WebhookSecretEnc, &enabled, &src.CreatedAt, &src.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	src.Enabled = enabled != 0
	return &src, nil
}

// ─── Tenants ─────────────────────────────────────────────────────────────────

// TenantRow mirrors the tenants table (F6 multi-tenant billing mapping).
type TenantRow struct {
	ID            string
	Name          string
	StreamPattern string // LIKE glob on stream_id; empty = not used
	MetaTagKey    string // beacon meta field key; empty = not used
	MetaTagValue  string // beacon meta field value to match
	CreatedAt     int64
	UpdatedAt     int64
}

// CreateTenant inserts a new tenant.
func (s *Store) CreateTenant(ctx context.Context, t TenantRow) (TenantRow, error) {
	if t.ID == "" {
		t.ID = newUUID()
	}
	now := nowMS()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := s.execContext(ctx,
		`INSERT INTO tenants (id, name, stream_pattern, meta_tag_key, meta_tag_value, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.StreamPattern, t.MetaTagKey, t.MetaTagValue, t.CreatedAt, t.UpdatedAt)
	return t, err
}

// GetTenant fetches a tenant by ID.
func (s *Store) GetTenant(ctx context.Context, id string) (*TenantRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, name, stream_pattern, meta_tag_key, meta_tag_value, created_at, updated_at
		 FROM tenants WHERE id=?`, id)
	return scanTenant(row)
}

// GetTenantByName fetches a tenant by name.
func (s *Store) GetTenantByName(ctx context.Context, name string) (*TenantRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, name, stream_pattern, meta_tag_key, meta_tag_value, created_at, updated_at
		 FROM tenants WHERE name=?`, name)
	return scanTenant(row)
}

// ListTenants returns tenants ordered by created_at ASC, id ASC.
// limit<=0 means no LIMIT; cursor="" means first page.
func (s *Store) ListTenants(ctx context.Context, limit int, cursor string) ([]TenantRow, error) {
	q := `SELECT id, name, stream_pattern, meta_tag_key, meta_tag_value, created_at, updated_at
	      FROM tenants WHERE 1=1`
	var args []any
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at ASC, id ASC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tenants []TenantRow
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, *t)
	}
	return tenants, rows.Err()
}

// UpdateTenant updates a tenant by ID.
func (s *Store) UpdateTenant(ctx context.Context, t TenantRow) error {
	t.UpdatedAt = nowMS()
	_, err := s.execContext(ctx,
		`UPDATE tenants SET name=?, stream_pattern=?, meta_tag_key=?, meta_tag_value=?, updated_at=?
		 WHERE id=?`,
		t.Name, t.StreamPattern, t.MetaTagKey, t.MetaTagValue, t.UpdatedAt, t.ID)
	return err
}

// DeleteTenant removes a tenant by ID.
func (s *Store) DeleteTenant(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM tenants WHERE id=?`, id)
	return err
}

func scanTenant(row scanner) (*TenantRow, error) {
	var t TenantRow
	if err := row.Scan(&t.ID, &t.Name, &t.StreamPattern, &t.MetaTagKey, &t.MetaTagValue, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// ─── Report Schedules ─────────────────────────────────────────────────────────

// ReportScheduleRow mirrors the report_schedules table.
type ReportScheduleRow struct {
	ID               string
	Cron             string
	Format           string // "csv" | "pdf"
	ScopeJSON        string // {"app": ..., "tenant": ...}
	TenantMapping    sql.NullString
	WhitelabelHeader sql.NullString // JSON
	LastRunAt        *int64
	NextRunAt        *int64
	CreatedAt        int64
	UpdatedAt        int64
}

// CreateReportSchedule inserts a new report schedule.
func (s *Store) CreateReportSchedule(ctx context.Context, r ReportScheduleRow) (ReportScheduleRow, error) {
	if r.ID == "" {
		r.ID = newUUID()
	}
	now := nowMS()
	r.CreatedAt = now
	r.UpdatedAt = now
	if r.ScopeJSON == "" {
		r.ScopeJSON = "{}"
	}
	_, err := s.execContext(ctx,
		`INSERT INTO report_schedules
		 (id, cron, format, scope, tenant_mapping, whitelabel_header, last_run_at, next_run_at, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Cron, r.Format, r.ScopeJSON, r.TenantMapping, r.WhitelabelHeader,
		r.LastRunAt, r.NextRunAt, r.CreatedAt, r.UpdatedAt)
	return r, err
}

// GetReportSchedule fetches a schedule by ID.
func (s *Store) GetReportSchedule(ctx context.Context, id string) (*ReportScheduleRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, cron, format, scope, tenant_mapping, whitelabel_header, last_run_at, next_run_at, created_at, updated_at
		 FROM report_schedules WHERE id=?`, id)
	return scanReportSchedule(row)
}

// ListReportSchedules returns schedules ordered by created_at ASC, id ASC.
// limit<=0 means no LIMIT; cursor="" means first page.
func (s *Store) ListReportSchedules(ctx context.Context, limit int, cursor string) ([]ReportScheduleRow, error) {
	q := `SELECT id, cron, format, scope, tenant_mapping, whitelabel_header, last_run_at, next_run_at, created_at, updated_at
	      FROM report_schedules WHERE 1=1`
	var args []any
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY created_at ASC, id ASC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schedules []ReportScheduleRow
	for rows.Next() {
		r, err := scanReportSchedule(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, *r)
	}
	return schedules, rows.Err()
}

// UpdateReportSchedule updates a schedule by ID.
func (s *Store) UpdateReportSchedule(ctx context.Context, r ReportScheduleRow) error {
	r.UpdatedAt = nowMS()
	_, err := s.execContext(ctx,
		`UPDATE report_schedules SET cron=?, format=?, scope=?, tenant_mapping=?, whitelabel_header=?,
		  last_run_at=?, next_run_at=?, updated_at=? WHERE id=?`,
		r.Cron, r.Format, r.ScopeJSON, r.TenantMapping, r.WhitelabelHeader,
		r.LastRunAt, r.NextRunAt, r.UpdatedAt, r.ID)
	return err
}

// DeleteReportSchedule removes a schedule by ID.
func (s *Store) DeleteReportSchedule(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM report_schedules WHERE id=?`, id)
	return err
}

// ListDueReportSchedules returns schedules whose next_run_at <= now (Unix ms).
func (s *Store) ListDueReportSchedules(ctx context.Context, nowMS int64) ([]ReportScheduleRow, error) {
	rows, err := s.queryContext(ctx,
		`SELECT id, cron, format, scope, tenant_mapping, whitelabel_header, last_run_at, next_run_at, created_at, updated_at
		 FROM report_schedules WHERE next_run_at IS NOT NULL AND next_run_at <= ?
		 ORDER BY next_run_at`, nowMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schedules []ReportScheduleRow
	for rows.Next() {
		r, err := scanReportSchedule(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, *r)
	}
	return schedules, rows.Err()
}

// MarkScheduleRan updates last_run_at and next_run_at for a completed schedule.
func (s *Store) MarkScheduleRan(ctx context.Context, id string, lastRunAt, nextRunAt int64) error {
	_, err := s.execContext(ctx,
		`UPDATE report_schedules SET last_run_at=?, next_run_at=?, updated_at=? WHERE id=?`,
		lastRunAt, nextRunAt, nowMS(), id)
	return err
}

func scanReportSchedule(row scanner) (*ReportScheduleRow, error) {
	var r ReportScheduleRow
	if err := row.Scan(&r.ID, &r.Cron, &r.Format, &r.ScopeJSON, &r.TenantMapping,
		&r.WhitelabelHeader, &r.LastRunAt, &r.NextRunAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// ─── Encryption helpers ───────────────────────────────────────────────────────

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64 ciphertext.
func (s *Store) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(s.cipherKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt decrypts a base64 AES-256-GCM ciphertext.
func (s *Store) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt: base64: %w", err)
	}
	block, err := aes.NewCipher(s.cipherKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("decrypt: ciphertext too short")
	}
	nonce := data[:gcm.NonceSize()]
	ct := data[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}

// ─── Token hashing helpers ────────────────────────────────────────────────────

// hmacHex returns HMAC-SHA256(s.hmacKey, rawToken) as a hex string.
// The hmacKey is derived from the cipher key via deriveHMACKey.
func (s *Store) hmacHex(rawToken string) string {
	mac := hmac.New(sha256.New, s.hmacKey[:])
	mac.Write([]byte(rawToken))
	return hex.EncodeToString(mac.Sum(nil))
}

// sha256HexLocal returns the plain SHA-256 hex of s (for legacy token lookup).
func sha256HexLocal(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// deriveHMACKey derives a 32-byte HMAC key from the AES cipher key using a
// domain-separated SHA-256 hash. This avoids key reuse between the AES-GCM
// cipher and the HMAC-SHA256 token hash — a security best practice.
//
// Domain separator: "pulse-token-hmac-v1\x00" (null-terminated to prevent
// length-extension confusion).
func deriveHMACKey(cipherKey [32]byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("pulse-token-hmac-v1\x00"))
	h.Write(cipherKey[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// ─── Key derivation ───────────────────────────────────────────────────────────

// deriveKey derives a 32-byte AES key from the secretKey string.
// If secretKey is a 64-char hex string, it is decoded directly (exact 32 bytes).
// Otherwise it is hashed with SHA-256.
// If secretKey is empty, a persisted random key is loaded from/created in the
// database directory.
func deriveKey(secretKey, dsn string) ([32]byte, error) {
	var key [32]byte

	if secretKey != "" {
		// Try hex decode first (user may supply 64-char hex key).
		if len(secretKey) == 64 {
			if b, err := hex.DecodeString(secretKey); err == nil && len(b) == 32 {
				copy(key[:], b)
				return key, nil
			}
		}
		// SHA-256 of arbitrary string.
		sum := sha256.Sum256([]byte(secretKey))
		copy(key[:], sum[:])
		return key, nil
	}

	// No env key — load or generate a persistent key file next to the DB.
	// For :memory: or empty, generate an ephemeral key.
	if dsn == ":memory:" || dsn == "" {
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return key, err
		}
		return key, nil
	}

	// Determine key file path: same dir as SQLite db file.
	dir := filepath.Dir(dsn)
	// Strip any query string from dsn path.
	if idx := strings.IndexByte(dir, '?'); idx >= 0 {
		dir = dir[:idx]
	}
	keyFile := filepath.Join(dir, "pulse_secret.key")

	data, err := os.ReadFile(keyFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return key, fmt.Errorf("read key file %s: %w", keyFile, err)
		}
		// Generate a new key.
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return key, err
		}
		hexKey := hex.EncodeToString(key[:])
		_ = os.MkdirAll(dir, 0o750)
		if err := os.WriteFile(keyFile, []byte(hexKey), 0o600); err != nil {
			// Non-fatal: warn via return but proceed.
			return key, fmt.Errorf("write key file %s: %w (use PULSE_SECRET_KEY env var as alternative)", keyFile, err)
		}
		return key, nil
	}

	// Parse existing key file.
	hexKey := strings.TrimSpace(string(data))
	if len(hexKey) == 64 {
		if b, err := hex.DecodeString(hexKey); err == nil && len(b) == 32 {
			copy(key[:], b)
			return key, nil
		}
	}
	// If malformed, fall back to SHA-256 of the file contents.
	sum := sha256.Sum256(data)
	copy(key[:], sum[:])
	return key, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func nowMS() int64 {
	return time.Now().UnixMilli()
}

func newUUID() string {
	return uuid.New().String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func abbreviate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// splitSQL splits a SQL script into individual statements on semicolons,
// respecting that a semicolon inside a -- comment should not split.
func splitSQL(script string) []string {
	var stmts []string
	var buf strings.Builder
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			// Skip comment lines for statement splitting purposes.
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			stmts = append(stmts, buf.String())
			buf.Reset()
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}
