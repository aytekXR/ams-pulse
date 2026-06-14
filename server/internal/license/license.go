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
//	  "tier":           "pro" | "enterprise",
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
	"os"
	"strings"
	"sync"
	"time"
)

// Tier represents a license tier.
type Tier string

const (
	TierFree       Tier = "free"
	TierPro        Tier = "pro"
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
	Tier          string  `json:"tier"`
	MaxNodes      *int    `json:"max_nodes"`
	MaxStreams     *int    `json:"max_streams"`
	RetentionDays *int    `json:"retention_days"`
	DataAPI       bool    `json:"data_api"`
	WhiteLabel    bool    `json:"white_label"`
	ExpiresAt     *int64  `json:"expires_at"` // Unix epoch ms
}

// Manager resolves and caches the active license.
type Manager struct {
	mu           sync.RWMutex
	tier         Tier
	entitlements Entitlements
	expiresAt    *time.Time
	valid        bool
	rawKey       string
	offlineFile  string
	pubKey       ed25519.PublicKey
}

// devPublicKeyHex is the embedded dev/test public key (ed25519).
// Replace with the real vendor public key for production builds.
// This key was generated for Pulse development only and does NOT authorize
// production use.
const devPublicKeyHex = "3dab4e90e91c3d58f37cf4a4bc7c71254c78348d11b52e57fec31a8a8b4d89b3"

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
		pubKeyHex = devPublicKeyHex
	}
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		// Fallback: generate a random key so the manager still works (dev mode).
		pubKeyBytes2, privKey, err2 := ed25519.GenerateKey(nil)
		if err2 != nil {
			return nil, fmt.Errorf("license: init public key: %w", err)
		}
		_ = privKey
		m.pubKey = pubKeyBytes2
	} else {
		m.pubKey = ed25519.PublicKey(pubKeyBytes)
	}

	// Try to load a license key.
	if licenseKey != "" {
		if err := m.activate(licenseKey); err != nil {
			// Fail open — use Free tier but record the error in logs.
			m.setFree()
			_ = err // callers can ignore; free tier is still usable
		}
	} else if offlineFile != "" {
		data, err := os.ReadFile(offlineFile)
		if err == nil {
			if err2 := m.activate(strings.TrimSpace(string(data))); err2 != nil {
				m.setFree()
			}
		} else {
			m.setFree()
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

// Tier returns the current license tier.
func (m *Manager) Tier() Tier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tier
}

// Entitlements returns the current tier entitlements.
func (m *Manager) Entitlements() Entitlements {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entitlements
}

// Valid returns whether the license is valid (not expired, signature OK).
// Free tier is always valid.
func (m *Manager) Valid() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.valid
}

// ExpiresAt returns the expiration time, or nil for perpetual/free.
func (m *Manager) ExpiresAt() *time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	if t == TierFree {
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
// Multi-tenant billing requires Enterprise tier (§7.11 — "Business/Enterprise" in PRD maps
// to TierEnterprise in the license model; Free and Pro → 403).
func (m *Manager) CheckMultiTenant() error {
	t := m.Tier()
	if t != TierEnterprise {
		return fmt.Errorf("multi-tenant billing (F6) requires Enterprise tier (current: %q)", t)
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

	// Check expiry.
	var expiresAt *time.Time
	if c.ExpiresAt != nil {
		t := time.UnixMilli(*c.ExpiresAt).UTC()
		expiresAt = &t
		if time.Now().After(t) {
			return fmt.Errorf("license: expired at %s", t.Format(time.RFC3339))
		}
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
	return nil
}

func (m *Manager) setFree() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tier = TierFree
	m.entitlements = freeTierEntitlements
	m.expiresAt = nil
	m.valid = true
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
	//   Enterprise: all channels (email, slack, telegram, pagerduty, webhook)
	switch Tier(c.Tier) {
	case TierPro:
		ent.Channels = proTierEntitlements.Channels
	case TierEnterprise:
		ent.Channels = enterpriseTierEntitlements.Channels
	default:
		ent.Channels = freeTierEntitlements.Channels
	}

	return ent
}
