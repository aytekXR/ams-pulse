// Package license validates the Pulse license key and exposes tier entitlements.
//
// # License format
//
// A Pulse license key is a base64-encoded JSON blob containing license claims,
// followed by a period and a base64-encoded ed25519 signature:
//
//	<base64(claims_json)>.<base64(ed25519_signature)>
//
// Claims JSON shape:
//
//	{
//	  "tier":           "pro" | "business" | "enterprise",
//	  "max_nodes":      3,        // null = unlimited
//	  "max_streams":    null,
//	  "retention_days": 365,      // null = unlimited
//	  "data_api":       true,
//	  "white_label":    false,
//	  "expires_at":     1800000000000   // Unix epoch ms; omit = perpetual
//	}
//
// # Dev public key
//
// The embedded dev public key is used for testing. Production keys must be
// signed with the Pulse vendor private key (not distributed). Set the
// PULSE_LICENSE_PUBKEY env var to override the embedded key with a hex-encoded
// ed25519 public key (for CI/staging environments).
//
// # Free tier (no key)
//
// Free tier is the default when no license key is configured:
//   - 1 AMS node monitored
//   - 7-day data retention
//   - Email alerts only (no Slack/PagerDuty/webhook)
//   - No Data API access
//   - No white-label reports
//
// # Fail-open / fail-closed
//
// Per ARCHITECTURE §6:
//   - Fails open for reading already-collected data.
//   - Fails closed for gated features (API returns 402 Payment Required or 403).
package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// now is the package-level time source. Overridden in tests via SetNow (export_test.go).
var now = time.Now

// generateKey is the package-level ed25519 key generator, seamed so the dev-mode
// fallback in New (malformed PULSE_LICENSE_PUBKEY) is testable. Overridden in tests
// via SetGenerateKey (export_test.go); mirrors the `now` pattern above.
var generateKey = ed25519.GenerateKey

// licenseLog is the package-level logger for license lifecycle events.
// Use SetLogger to override (typically in tests or at process startup).
var licenseLog atomic.Pointer[slog.Logger]

func init() {
	licenseLog.Store(slog.Default())
}

// SetLogger replaces the package-level logger used for license events.
// Safe for concurrent use. Intended for tests and process-start configuration.
func SetLogger(l *slog.Logger) { licenseLog.Store(l) }

// Tier represents a license tier.
type Tier string

const (
	TierFree       Tier = "free"
	TierPro        Tier = "pro"
	TierBusiness   Tier = "business"
	TierEnterprise Tier = "enterprise"
)

// Entitlements is the set of permissions for a tier.
type Entitlements struct {
	// MaxNodes is the maximum number of monitored AMS nodes. -1 = unlimited.
	MaxNodes int

	// MaxStreams is the maximum concurrent streams. -1 = unlimited.
	MaxStreams int

	// RetentionDays is the maximum data retention period. -1 = unlimited.
	RetentionDays int

	// DataAPI enables the Data API (F8) for external data access.
	DataAPI bool

	// WhiteLabel enables PDF white-label report headers.
	WhiteLabel bool

	// Channels is the set of allowed notification channel types.
	Channels []string
}

// Free tier defaults per PRD §7.11.
var freeTierEntitlements = Entitlements{
	MaxNodes:      1,
	MaxStreams:    -1, // unlimited
	RetentionDays: 7,
	DataAPI:       false,
	WhiteLabel:    false,
	Channels:      []string{"email"},
}

// Pro tier (§7.11): adds Slack+Telegram, beacons/QoE, 90-day retention, CSV export.
// PagerDuty+webhook require Business/Enterprise tier.
var proTierEntitlements = Entitlements{
	MaxNodes:      10,
	MaxStreams:    -1,
	RetentionDays: 90,
	DataAPI:       true,
	WhiteLabel:    false,
	Channels:      []string{"email", "slack", "telegram"},
}

// Business tier: up to 50 nodes, 13-month retention, PagerDuty+webhook,
// usage/billing reports, multi-tenant billing, API+Prometheus. $299/month.
// Persona-consistent ladder: Free 1 / Pro 10 / Business 50 / Enterprise unlimited.
// PRD §7.11 listed a 5-node ceiling that is superseded by the current pricing
// sign-off; final pricing remains an operator item.
var businessTierEntitlements = Entitlements{
	MaxNodes:      50,
	MaxStreams:    -1,
	RetentionDays: 396, // 13 months ≈ 396 days
	DataAPI:       true,
	WhiteLabel:    false,
	Channels:      []string{"email", "slack", "telegram", "pagerduty", "webhook"},
}

var enterpriseTierEntitlements = Entitlements{
	MaxNodes:      -1,
	MaxStreams:    -1,
	RetentionDays: -1,
	DataAPI:       true,
	WhiteLabel:    true,
	Channels:      []string{"email", "slack", "pagerduty", "telegram", "webhook"},
}

// claims is the parsed JSON payload from a license key.
type claims struct {
	Tier          string `json:"tier"`
	MaxNodes      *int   `json:"max_nodes"`
	MaxStreams    *int   `json:"max_streams"`
	RetentionDays *int   `json:"retention_days"`
	DataAPI       bool   `json:"data_api"`
	WhiteLabel    bool   `json:"white_label"`
	ExpiresAt     *int64 `json:"expires_at"` // Unix epoch ms
}

// Manager resolves and caches the active license.
type Manager struct {
	mu            sync.RWMutex
	tier          Tier
	entitlements  Entitlements
	expiresAt     *time.Time
	valid         bool
	rawKey        string
	offlineFile   string
	pubKey        ed25519.PublicKey
	degradedByExp bool // true after mid-run expiry downgrade; makes maybeExpireLocked idempotent
}

// devPublicKeyHex is the embedded dev/test public key (ed25519).
// Retained for self-signing workflows and test environments that generate their
// own key pair. NOT used as the runtime default in production builds.
const devPublicKeyHex = "3dab4e90e91c3d58f37cf4a4bc7c71254c78348d11b52e57fec31a8a8b4d89b3"

// officialPublicKeyHex is the embedded production ed25519 public key used to
// verify Pulse license signatures when PULSE_LICENSE_PUBKEY is not set in the
// environment. Set PULSE_LICENSE_PUBKEY to override for self-signed/CI keys.
const officialPublicKeyHex = "6403d7b49951d0220c7219e491b6525971edf10f0e64616b17023eab002ab4ba"

// New creates a Manager and loads the license from the provided key or file.
// If both are empty, Free tier is assumed.
func New(licenseKey, offlineFile string) (*Manager, error) {
	m := &Manager{
		rawKey:      licenseKey,
		offlineFile: offlineFile,
	}

	// Load public key.
	pubKeyHex := os.Getenv("PULSE_LICENSE_PUBKEY")
	if pubKeyHex == "" {
		pubKeyHex = officialPublicKeyHex
		if licenseKey != "" {
			licenseLog.Load().Info("license: PULSE_LICENSE_PUBKEY unset — verifying with embedded official pubkey",
				"pubkey", officialPublicKeyHex)
		}
	}
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		// Fallback: generate a random key so the manager still works (dev mode).
		pubKeyBytes2, privKey, err2 := generateKey(nil)
		if err2 != nil {
			// Wrap err2 (the actual GenerateKey failure), not err — in the
			// length-mismatch sub-path err is nil, so wrapping it would yield the
			// opaque "init public key: <nil>" (D-133/S71 [24]).
			return nil, fmt.Errorf("license: init public key: %w", err2)
		}
		_ = privKey
		m.pubKey = pubKeyBytes2
	} else {
		m.pubKey = ed25519.PublicKey(pubKeyBytes)
	}

	// Try to load a license key.
	if licenseKey != "" {
		if err := m.activate(licenseKey); err != nil {
			// Fail open — use Free tier, but record the rejection so the operator can
			// tell "key rejected" apart from "no key configured" (D-133/S71 [12]).
			m.setFree()
			licenseLog.Load().Warn("license: activation failed, degrading to free tier", "error", err)
		}
	} else if offlineFile != "" {
		data, err := os.ReadFile(offlineFile)
		if err == nil {
			if err2 := m.activate(strings.TrimSpace(string(data))); err2 != nil {
				m.setFree()
				licenseLog.Load().Warn("license: offline file activation failed, degrading to free tier",
					"file", offlineFile, "error", err2)
			}
		} else {
			m.setFree()
			licenseLog.Load().Warn("license: offline file unreadable, degrading to free tier",
				"file", offlineFile, "error", err)
		}
	} else {
		m.setFree()
	}

	return m, nil
}

// Refresh reloads the license (e.g. after key update via API).
func (m *Manager) Refresh(_ context.Context, key string) error {
	if key == "" {
		m.setFree()
		return nil
	}
	return m.activate(key)
}

// maybeExpireLocked checks for mid-run expiry and lazily downgrades the Manager
// to Free tier. Must be called while m.mu (write lock) is held.
// It is idempotent: after the first downgrade it sets degradedByExp=true and
// subsequent calls return immediately without re-logging.
func (m *Manager) maybeExpireLocked() {
	if m.degradedByExp {
		return // already degraded; idempotent
	}
	if m.expiresAt == nil {
		return // perpetual key or no-key path
	}
	if !now().After(*m.expiresAt) {
		return // not yet expired
	}
	prevTier := m.tier
	m.tier = TierFree
	m.entitlements = freeTierEntitlements
	m.valid = false
	// RETAIN m.expiresAt so callers can distinguish "expired trial" from "no key".
	m.degradedByExp = true
	licenseLog.Load().Warn("license: expired — degraded to free tier",
		"prev_tier", string(prevTier),
		"expired_at", m.expiresAt.Format(time.RFC3339))
}

// Tier returns the current license tier.
// Triggers a lazy expiry check on every call.
func (m *Manager) Tier() Tier {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeExpireLocked()
	return m.tier
}

// Entitlements returns the current tier entitlements.
// Triggers a lazy expiry check on every call.
func (m *Manager) Entitlements() Entitlements {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeExpireLocked()
	return m.entitlements
}

// Valid returns whether the license is valid (not expired, signature OK).
// Free tier with no key is always valid; an expired trial key returns false.
func (m *Manager) Valid() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeExpireLocked()
	return m.valid
}

// ExpiresAt returns the expiration time, or nil for perpetual/no-key.
// After expiry, the past timestamp is RETAINED so callers can distinguish
// "expired trial" (non-nil past) from "no key" (nil).
func (m *Manager) ExpiresAt() *time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeExpireLocked()
	return m.expiresAt
}

// ─── Entitlement checks ───────────────────────────────────────────────────────

// CheckNodeLimit returns an error if the given node count exceeds the limit.
// Fails closed: returns error if limit exceeded.
func (m *Manager) CheckNodeLimit(count int) error {
	e := m.Entitlements()
	if e.MaxNodes < 0 {
		return nil // unlimited
	}
	if count > e.MaxNodes {
		return fmt.Errorf("node limit exceeded: tier %q allows %d node(s), got %d (upgrade required)",
			m.Tier(), e.MaxNodes, count)
	}
	return nil
}

// CheckRetention caps the retention duration to the tier's limit.
// Returns the effective retention in days: the minimum of requested and limit.
// Fails open for reads: if license is invalid, returns the Free tier limit.
func (m *Manager) CheckRetention(requestedDays int) int {
	e := m.Entitlements()
	if e.RetentionDays < 0 {
		return requestedDays // unlimited
	}
	if requestedDays <= 0 || requestedDays > e.RetentionDays {
		return e.RetentionDays
	}
	return requestedDays
}

// CheckChannelAllowed returns nil if the channel type is allowed by the tier.
func (m *Manager) CheckChannelAllowed(channelType string) error {
	e := m.Entitlements()
	for _, c := range e.Channels {
		if c == channelType {
			return nil
		}
	}
	return fmt.Errorf("channel type %q not allowed on tier %q (upgrade required)", channelType, m.Tier())
}

// CheckDataAPI returns nil if the tier includes Data API access.
func (m *Manager) CheckDataAPI() error {
	if !m.Entitlements().DataAPI {
		return fmt.Errorf("Data API (F8) requires Pro tier or higher (current: %q)", m.Tier())
	}
	return nil
}

// CheckProbes returns nil if the tier includes synthetic probe access (F10).
// Probes require Pro tier or higher (§7.11 pricing table).
func (m *Manager) CheckProbes() error {
	t := m.Tier()
	// Positive membership (matches the 5 sibling checks) — an unknown tier string is
	// blocked, not silently granted access as the old `t == TierFree` gate did (D-133/S71 [23]).
	if t != TierPro && t != TierBusiness && t != TierEnterprise {
		return fmt.Errorf("synthetic probes (F10) require Pro tier or higher (current: %q)", t)
	}
	return nil
}

// CheckAnomalies returns nil if the tier includes anomaly detection access (F9).
// Anomaly detection requires Enterprise tier (§7.11 pricing table).
func (m *Manager) CheckAnomalies() error {
	t := m.Tier()
	if t != TierEnterprise {
		return fmt.Errorf("anomaly detection (F9) requires Enterprise tier (current: %q)", t)
	}
	return nil
}

// CheckMultiTenant returns nil if the tier includes multi-tenant billing (F6 tenant CRUD).
// Multi-tenant billing requires Business tier or higher (PRD §7.11 table); Free and Pro → 403.
func (m *Manager) CheckMultiTenant() error {
	t := m.Tier()
	if t != TierBusiness && t != TierEnterprise {
		return fmt.Errorf("multi-tenant billing (F6) requires Business tier or higher (current: %q)", t)
	}
	return nil
}

// CheckReports returns nil if the tier includes usage/billing reports (F6).
// Reports require Business tier or higher (PRD §7.11: "usage reports" is a Business+ feature).
// Free and Pro tiers receive 403.
func (m *Manager) CheckReports() error {
	t := m.Tier()
	if t != TierBusiness && t != TierEnterprise {
		return fmt.Errorf("usage/billing reports (F6) require Business tier or higher (current: %q)", t)
	}
	return nil
}

// CheckBeaconIngest returns nil if the tier includes QoE beacon ingest (F3).
// Beacon ingest (player-side QoE data) requires Pro tier or higher (PRD §7.11 table).
// Free tier returns 403.
func (m *Manager) CheckBeaconIngest() error {
	t := m.Tier()
	// Positive membership (matches the 5 sibling checks) — an unknown tier string is
	// blocked, not silently granted access as the old `t == TierFree` gate did (D-133/S71 [23]).
	if t != TierPro && t != TierBusiness && t != TierEnterprise {
		return fmt.Errorf("QoE beacon ingest (F3) requires Pro tier or higher (current: %q)", t)
	}
	return nil
}

// CheckPrometheus returns nil if the tier includes the Prometheus /metrics endpoint (F8).
// /metrics (Prometheus endpoint F8) requires Business+.
// Free and Pro tiers are blocked by this gate (the handler returns 403 LICENSE_REQUIRED);
// there is no unauthenticated /metrics fallback path.
func (m *Manager) CheckPrometheus() error {
	t := m.Tier()
	if t != TierBusiness && t != TierEnterprise {
		return fmt.Errorf("Prometheus endpoint (F8) requires Business tier or higher (current: %q)", t)
	}
	return nil
}

// CheckSSO returns nil if the tier includes SSO / OIDC login.
// SSO is an Enterprise-only feature (PRD §7.11 Enterprise row). Free, Pro and
// Business are blocked — the OIDC login/callback routes return 403 and the
// status endpoint reports SSO disabled so the SPA does not offer the button.
func (m *Manager) CheckSSO() error {
	if m.Tier() != TierEnterprise {
		return fmt.Errorf("SSO / OIDC login requires Enterprise tier (current: %q)", m.Tier())
	}
	return nil
}

// CheckWhiteLabel returns nil if the tier includes white-label PDF report headers.
// Gated on the WhiteLabel entitlement (Enterprise per the tier tables, but a
// custom key may grant it), mirroring CheckDataAPI. Callers must apply this
// before honouring a schedule's white-label header, else any tier could brand
// its reports.
func (m *Manager) CheckWhiteLabel() error {
	if !m.Entitlements().WhiteLabel {
		return fmt.Errorf("white-label report headers require a license with white_label enabled (current tier: %q)", m.Tier())
	}
	return nil
}

// ─── Activation ───────────────────────────────────────────────────────────────

// activate parses and validates a license key.
func (m *Manager) activate(key string) error {
	parts := strings.Split(key, ".")
	if len(parts) != 2 {
		return fmt.Errorf("license: invalid key format (expected <claims>.<signature>)")
	}

	claimsData, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("license: decode claims: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("license: decode signature: %w", err)
	}

	// Verify ed25519 signature.
	if !ed25519.Verify(m.pubKey, claimsData, sig) {
		return fmt.Errorf("license: signature verification failed")
	}

	var c claims
	if err := json.Unmarshal(claimsData, &c); err != nil {
		return fmt.Errorf("license: parse claims: %w", err)
	}

	// Validate the tier against the known set. An unrecognized tier (a vendor-side
	// typo, e.g. "enterprise_lite") must NOT be trusted: the negative-gated checks
	// would treat any non-"free" string as paid, and buildEntitlements maps absent
	// node/retention claims to unlimited. Rejecting here fails the activation, so New
	// falls open to Free rather than granting a privileged unknown tier (D-133/S71 [23]).
	switch Tier(c.Tier) {
	case TierFree, TierPro, TierBusiness, TierEnterprise:
		// known tier
	default:
		return fmt.Errorf("license: unknown tier %q", c.Tier)
	}

	// Parse expiry (if present). Expiry enforcement is done lazily via
	// maybeExpireLocked() on the first reader call, so that mid-run expiry
	// is also caught and so that the expiresAt timestamp is always preserved.
	var expiresAt *time.Time
	if c.ExpiresAt != nil {
		t := time.UnixMilli(*c.ExpiresAt).UTC()
		expiresAt = &t
		// NOTE: do NOT error here for already-expired keys. The signature has
		// already been verified above; we set expiresAt and let maybeExpireLocked
		// produce the honest state on the first read.
	}

	// Build entitlements from claims.
	ent := buildEntitlements(c)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.tier = Tier(c.Tier)
	m.entitlements = ent
	m.expiresAt = expiresAt
	m.valid = true
	m.rawKey = key
	m.degradedByExp = false // reset so mid-run expiry can fire after a Refresh
	return nil
}

func (m *Manager) setFree() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tier = TierFree
	m.entitlements = freeTierEntitlements
	m.expiresAt = nil
	m.valid = true
	m.degradedByExp = false
}

func buildEntitlements(c claims) Entitlements {
	ent := Entitlements{
		DataAPI:    c.DataAPI,
		WhiteLabel: c.WhiteLabel,
	}

	if c.MaxNodes != nil {
		ent.MaxNodes = *c.MaxNodes
		if ent.MaxNodes == 0 {
			ent.MaxNodes = -1 // 0 in claims = unlimited
		}
	} else {
		ent.MaxNodes = -1
	}

	if c.MaxStreams != nil {
		ent.MaxStreams = *c.MaxStreams
		if ent.MaxStreams == 0 {
			ent.MaxStreams = -1
		}
	} else {
		ent.MaxStreams = -1
	}

	if c.RetentionDays != nil {
		ent.RetentionDays = *c.RetentionDays
		if ent.RetentionDays == 0 {
			ent.RetentionDays = -1
		}
	} else {
		ent.RetentionDays = -1
	}

	// Channels by tier (§7.11 tier matrix):
	//   Free:       email only
	//   Pro:        email, slack, telegram
	//   Business:   email, slack, telegram, pagerduty, webhook
	//   Enterprise: all channels (email, slack, telegram, pagerduty, webhook)
	switch Tier(c.Tier) {
	case TierPro:
		ent.Channels = proTierEntitlements.Channels
	case TierBusiness:
		ent.Channels = businessTierEntitlements.Channels
	case TierEnterprise:
		ent.Channels = enterpriseTierEntitlements.Channels
	default:
		ent.Channels = freeTierEntitlements.Channels
	}

	return ent
}
