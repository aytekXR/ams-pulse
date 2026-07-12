// B7 TDD tests — AMSSource webhook_secret_enc CRUD and schema-upgrade path
// (D-062 WO-3). Written FIRST (red) before the column and Go field existed.
package meta_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// TestAMSSource_WebhookSecretEnc_RoundTrip verifies that webhook_secret_enc is
// written by CreateAMSSource and read back by GetAMSSource / ListAMSSources.
func TestAMSSource_WebhookSecretEnc_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Encrypt a plaintext secret and store it in the row.
	plaintext := "my-per-source-secret"
	enc, err := s.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	row := meta.AMSSourceRow{
		Name:             "webhook-src",
		SourceType:       "webhook",
		WebhookSecretEnc: sql.NullString{String: enc, Valid: true},
		Enabled:          true,
	}
	created, err := s.CreateAMSSource(ctx, row)
	if err != nil {
		t.Fatalf("CreateAMSSource: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateAMSSource: expected non-empty ID")
	}

	// Fetch by ID.
	got, err := s.GetAMSSource(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAMSSource: %v", err)
	}
	if got == nil {
		t.Fatal("GetAMSSource: nil result")
	}
	if !got.WebhookSecretEnc.Valid || got.WebhookSecretEnc.String == "" {
		t.Fatalf("GetAMSSource: WebhookSecretEnc not stored; got %+v", got.WebhookSecretEnc)
	}

	// Decrypt and compare.
	plain, err := s.Decrypt(got.WebhookSecretEnc.String)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != plaintext {
		t.Errorf("round-trip mismatch: want %q, got %q", plaintext, plain)
	}

	// Verify ListAMSSources also returns the field.
	list, err := s.ListAMSSources(ctx, 0, "")
	if err != nil {
		t.Fatalf("ListAMSSources: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListAMSSources: expected 1 row, got %d", len(list))
	}
	if !list[0].WebhookSecretEnc.Valid || list[0].WebhookSecretEnc.String != enc {
		t.Errorf("ListAMSSources: WebhookSecretEnc mismatch")
	}

	// Update: change to a different encrypted secret.
	newPlain := "rotated-secret"
	newEnc, _ := s.Encrypt(newPlain)
	got.WebhookSecretEnc = sql.NullString{String: newEnc, Valid: true}
	if err := s.UpdateAMSSource(ctx, *got); err != nil {
		t.Fatalf("UpdateAMSSource: %v", err)
	}
	after, err := s.GetAMSSource(ctx, created.ID)
	if err != nil || after == nil {
		t.Fatalf("GetAMSSource after update: err=%v, row=%v", err, after)
	}
	plain2, _ := s.Decrypt(after.WebhookSecretEnc.String)
	if plain2 != newPlain {
		t.Errorf("after update: want %q, got %q", newPlain, plain2)
	}
}

// TestAMSSource_WebhookSecretEnc_SchemaUpgrade verifies the ALTER TABLE upgrade
// path for existing databases that pre-date the webhook_secret_enc column:
//  1. Create a SQLite file from old DDL (webhook_secret_enc column stripped).
//  2. Insert a row via raw SQL (simulates a live prod row without the column).
//  3. Re-open via meta.New + MigrateEmbedded (triggers applySchemaUpgrades).
//  4. Assert: GetAMSSource finds the legacy row; WebhookSecretEnc is nil/empty.
func TestAMSSource_WebhookSecretEnc_SchemaUpgrade(t *testing.T) {
	currentDDL := readMetaDDL(t)
	oldDDL := stripWebhookSecretEncColumn(currentDDL)
	if hasWebhookSecretEncColumn(oldDDL) {
		t.Skip("could not build old DDL without webhook_secret_enc column for migration test")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "upgrade_b7.db")
	ctx := context.Background()

	// Step 1: apply old DDL and insert a row without webhook_secret_enc.
	func() {
		rawDB, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
		if err != nil {
			t.Fatalf("sql.Open (old schema): %v", err)
		}
		defer rawDB.Close()

		for _, stmt := range splitSQLStatements(oldDDL) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
				if strings.Contains(err.Error(), "PRAGMA") {
					continue
				}
				t.Fatalf("apply old DDL stmt %q: %v", stmt[:min(len(stmt), 60)], err)
			}
		}

		_, err = rawDB.ExecContext(ctx,
			`INSERT INTO ams_sources
			 (id, name, source_type, enabled, created_at, updated_at)
			 VALUES ('legacy-src-001', 'old-webhook', 'webhook', 1, 1000, 1000)`)
		if err != nil {
			t.Fatalf("insert legacy ams_sources row: %v", err)
		}
	}()

	// Step 2: re-open with meta.New + apply NEW DDL (triggers ALTER TABLE upgrade).
	newStore, err := meta.New(ctx, "sqlite", dbPath, "upgrade-secret-key")
	if err != nil {
		t.Fatalf("meta.New (re-open): %v", err)
	}
	defer newStore.Close()
	if err := newStore.MigrateEmbedded(ctx, currentDDL); err != nil {
		t.Fatalf("MigrateEmbedded: %v — schema upgrade for webhook_secret_enc failed", err)
	}

	// Step 3: legacy row is still accessible via GetAMSSource.
	got, err := newStore.GetAMSSource(ctx, "legacy-src-001")
	if err != nil {
		t.Fatalf("GetAMSSource (legacy after upgrade): %v", err)
	}
	if got == nil {
		t.Fatal("GetAMSSource (legacy after upgrade): expected row, got nil")
	}
	if got.Name != "old-webhook" {
		t.Errorf("unexpected name: %q", got.Name)
	}
	// The old row has no webhook_secret_enc value → NULL in column → not valid.
	if got.WebhookSecretEnc.Valid && got.WebhookSecretEnc.String != "" {
		t.Errorf("expected legacy row to have no webhook_secret_enc, got %q", got.WebhookSecretEnc.String)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// stripWebhookSecretEncColumn removes the webhook_secret_enc column declaration
// from the ams_sources DDL block to simulate a pre-B7 schema file.
func stripWebhookSecretEncColumn(ddl string) string {
	var out strings.Builder
	for _, line := range strings.Split(ddl, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "webhook_secret_enc") {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// hasWebhookSecretEncColumn reports whether the DDL contains the
// webhook_secret_enc column declaration.
func hasWebhookSecretEncColumn(ddl string) bool {
	for _, line := range strings.Split(ddl, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "webhook_secret_enc") {
			return true
		}
	}
	return false
}
