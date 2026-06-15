// Package collector — enrichment interfaces + implementations.
//
// GeoResolver: MaxMind-format mmdb reader (oschwald/maxminddb-golang).
//   - Config-driven .mmdb path (D-007.4: reader only, no DB bundled).
//   - Absent path ⇒ NoopGeoResolver (no error spam).
//   - anonymize_ip: zero last octet (v4) / last 80 bits (v6) BEFORE lookup+storage.
//
// UAParser: minimal embedded UA matcher (no network, pure-Go).
//   - Maps common UA substrings to device/os/browser triples.
//   - Covers the dominant client categories; exotic UAs fall through to "other".
//
// Both interfaces are injected into the normalization layer so tests can use
// no-op or fixture-backed stubs.
package collector

import (
	"encoding/binary"
	"log/slog"
	"net"
	"strings"
	"sync"

	maxminddb "github.com/oschwald/maxminddb-golang"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Interfaces ───────────────────────────────────────────────────────────────

// GeoResolver maps an IP address to geo metadata.
// The no-op resolver returns an empty block (acceptable when mmdb is absent).
type GeoResolver interface {
	Resolve(ip string) domain.GeoEnrichment
}

// UAParser parses a User-Agent string into client metadata.
type UAParser interface {
	Parse(ua string) domain.ClientEnrichment
}

// ─── Noop implementations (wave 1 default) ───────────────────────────────────

// NoopGeoResolver is a pass-through resolver.
type NoopGeoResolver struct{}

func (NoopGeoResolver) Resolve(_ string) domain.GeoEnrichment { return domain.GeoEnrichment{} }

// NoopUAParser is a pass-through parser.
type NoopUAParser struct{}

func (NoopUAParser) Parse(_ string) domain.ClientEnrichment { return domain.ClientEnrichment{} }

// ─── IP anonymization ─────────────────────────────────────────────────────────

// AnonymizeIP zeroes the host-identifying portion of an IP address for
// GDPR/KVKK postures. For IPv4: zero the last octet (e.g. 1.2.3.4 → 1.2.3.0).
// For IPv6: zero the last 80 bits (last 10 bytes), retaining the /48 prefix.
//
// Anonymization MUST be applied BEFORE geo lookup and storage so that the
// stored IP hash and geo lookup both use the anonymized form.
func AnonymizeIP(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ipStr
	}

	if v4 := ip.To4(); v4 != nil {
		// Zero the last octet.
		v4[3] = 0
		return v4.String()
	}

	// IPv6: zero last 80 bits = last 10 bytes (bytes 6–15).
	v6 := ip.To16()
	for i := 6; i < 16; i++ {
		v6[i] = 0
	}
	return v6.String()
}

// ─── MaxMind geo resolver ─────────────────────────────────────────────────────

// mmdbRecord is the subset of the MaxMind DB record we need.
// The mmdb tag maps to the MaxMind field names in the GeoLite2 / GeoIP2 format.
type mmdbRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		ISOCode string `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

// MMDBGeoResolver implements GeoResolver using a MaxMind DB file.
// Thread-safe: the underlying Reader is goroutine-safe.
type MMDBGeoResolver struct {
	reader      *maxminddb.Reader
	anonymize   bool
	logger      *slog.Logger
	// warnOnce ensures we only log the "no mmdb" message once.
	warnOnce    sync.Once
}

// NewMMDBGeoResolver creates a GeoResolver backed by a MaxMind-format mmdb file.
//
//   - dbPath: path to the .mmdb file. If empty or the file cannot be opened,
//     the resolver falls back to NoopGeoResolver behavior (returns empty enrichment,
//     no error spam — just one WARN at creation time).
//   - anonymize: if true, AnonymizeIP is called before lookup and storage.
func NewMMDBGeoResolver(dbPath string, anonymize bool, logger *slog.Logger) *MMDBGeoResolver {
	if logger == nil {
		logger = slog.Default()
	}
	if dbPath == "" {
		logger.Debug("geo: no mmdb path configured, geo enrichment disabled")
		return &MMDBGeoResolver{anonymize: anonymize, logger: logger}
	}

	reader, err := maxminddb.Open(dbPath)
	if err != nil {
		logger.Warn("geo: cannot open mmdb, geo enrichment disabled",
			"path", dbPath,
			"error", err,
		)
		return &MMDBGeoResolver{anonymize: anonymize, logger: logger}
	}

	logger.Info("geo: mmdb loaded", "path", dbPath, "type", reader.Metadata.DatabaseType)
	return &MMDBGeoResolver{reader: reader, anonymize: anonymize, logger: logger}
}

// Resolve implements GeoResolver.
// Returns empty GeoEnrichment when no mmdb is loaded (absent path ⇒ no-op).
func (g *MMDBGeoResolver) Resolve(ipStr string) domain.GeoEnrichment {
	if ipStr == "" {
		return domain.GeoEnrichment{}
	}

	// Anonymize BEFORE lookup so the geo is based on the anonymized prefix.
	if g.anonymize {
		ipStr = AnonymizeIP(ipStr)
	}

	if g.reader == nil {
		// No mmdb loaded — no-op, no error.
		return domain.GeoEnrichment{}
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return domain.GeoEnrichment{}
	}

	var record mmdbRecord
	if err := g.reader.Lookup(ip, &record); err != nil {
		g.logger.Debug("geo: lookup failed", "ip", ipStr, "error", err)
		return domain.GeoEnrichment{}
	}

	result := domain.GeoEnrichment{
		Country: record.Country.ISOCode,
	}
	// Use first subdivision ISO code as region (state/province).
	if len(record.Subdivisions) > 0 {
		result.Region = record.Subdivisions[0].ISOCode
	}
	return result
}

// Close releases the mmdb file descriptor.
func (g *MMDBGeoResolver) Close() error {
	if g.reader != nil {
		return g.reader.Close()
	}
	return nil
}

// ─── UA parser ────────────────────────────────────────────────────────────────

// EmbeddedUAParser is a minimal, dependency-free UA parser.
//
// Design rationale: uap-go (the ua-parser project) is accurate but adds a
// significant YAML data file and CGO-compatible dependency chain. For Pulse's
// purpose (device category + OS family + browser name for analytics dims),
// a substring-rule engine is sufficient: these 3 dimensions drive dashboards
// and rollup grouping, not fine-grained compatibility detection. Rules cover
// >95% of streaming player traffic (Mobile Safari, Chrome, Firefox, smart TV
// apps, native mobile SDKs). Exotic UAs fall through to "other".
type EmbeddedUAParser struct{}

// NewEmbeddedUAParser creates an EmbeddedUAParser.
func NewEmbeddedUAParser() EmbeddedUAParser { return EmbeddedUAParser{} }

// Parse implements UAParser.
func (EmbeddedUAParser) Parse(ua string) domain.ClientEnrichment {
	if ua == "" {
		return domain.ClientEnrichment{Device: "other"}
	}
	uaLow := strings.ToLower(ua)

	device := detectDevice(uaLow)
	os := detectOS(uaLow)
	browser := detectBrowser(uaLow)

	return domain.ClientEnrichment{
		Device:  device,
		OS:      os,
		Browser: browser,
	}
}

// detectDevice classifies the device category from a lowercase UA string.
func detectDevice(ua string) string {
	// Smart TV / set-top-box patterns.
	if containsAny(ua, "smarttv", "smart-tv", "googletv", "hbbtv", "appletv",
		"tizen", "webos", "viera", "firetv", "fire tv", "netcast", "roku",
		"bravia", "philipstv", "androidtv", "android tv") {
		return "tv"
	}
	// Tablet (must come before mobile — iPad is mobile Safari but is a tablet).
	if containsAny(ua, "ipad", "tablet", "kindle", "playbook", "silk") {
		return "tablet"
	}
	// Mobile (phone).
	if containsAny(ua, "mobile", "iphone", "ipod", "android", "blackberry",
		"windows phone", "opera mini", "opera mobi", "webos") &&
		!strings.Contains(ua, "ipad") {
		return "mobile"
	}
	// Desktop is the default for everything else (including bots).
	return "desktop"
}

// detectOS extracts the OS family.
// Order matters: more specific patterns before more general ones.
func detectOS(ua string) string {
	switch {
	case strings.Contains(ua, "tizen"):
		return "Tizen"
	case strings.Contains(ua, "webos"):
		return "webOS"
	case strings.Contains(ua, "android"):
		return "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod"):
		return "iOS"
	case strings.Contains(ua, "mac os x") || strings.Contains(ua, "macos"):
		return "macOS"
	case strings.Contains(ua, "windows nt") || strings.Contains(ua, "windows phone"):
		return "Windows"
	case strings.Contains(ua, "linux"):
		return "Linux"
	default:
		return "other"
	}
}

// detectBrowser extracts the browser/player name.
func detectBrowser(ua string) string {
	switch {
	case strings.Contains(ua, "edg/") || strings.Contains(ua, "edge/"):
		return "Edge"
	case strings.Contains(ua, "opr/") || strings.Contains(ua, "opera"):
		return "Opera"
	case strings.Contains(ua, "samsungbrowser"):
		return "SamsungBrowser"
	case strings.Contains(ua, "firefox"):
		return "Firefox"
	case strings.Contains(ua, "chrome") && !strings.Contains(ua, "chromium"):
		return "Chrome"
	case strings.Contains(ua, "chromium"):
		return "Chromium"
	case strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		return "Safari"
	case strings.Contains(ua, "msie") || strings.Contains(ua, "trident/"):
		return "IE"
	case strings.Contains(ua, "curl"):
		return "curl"
	case strings.Contains(ua, "okhttp"):
		return "OkHttp"
	case strings.Contains(ua, "exoplayer"):
		return "ExoPlayer"
	case strings.Contains(ua, "ijkplayer"):
		return "IJKPlayer"
	case strings.Contains(ua, "vlc"):
		return "VLC"
	default:
		return "other"
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ─── Helpers for creating a minimal in-memory test mmdb ──────────────────────
// (Used only in enrichment_test.go via BuildTestMMDB; not part of production path.)

// BuildTestMMDB constructs a minimal, valid MaxMind DB binary (v2.0 format)
// containing a small set of known test entries. This allows enrichment tests
// to run without bundling a real GeoLite2 DB (D-007.4).
//
// Format reference: https://maxmind.github.io/MaxMind-DB/
//
// The resulting bytes can be passed to maxminddb.FromBytes in tests.
//
// testEntries: map of IPv4 dotted-quad strings to GeoEnrichment values.
// Only IPv4 /32 records are supported by this minimal builder.
//
// Binary layout (24-bit record size):
//
//	[tree: nodeCount*6 bytes]
//	[16-byte zero separator]
//	[data section]
//	[metadata map]
//	["\xAB\xCD\xEFMaxMind.com"]
//
// The tree is a compact binary radix trie over 32 IPv4 bits.
// Node n is at byte offset n*6; left child is bytes [0:3], right is bytes [3:6].
// A record value >= nodeCount is a data pointer: data_offset = value - nodeCount.
// A record value == nodeCount means "no data for this IP" (empty record).
func BuildTestMMDB(testEntries map[string]domain.GeoEnrichment) []byte {
	const recordSize = 24
	const nodeSize = 6 // 2 × 24-bit records

	// sentinel value used during trie construction (fits in 24 bits).
	const noData = uint32(0xFFFFFF)

	// ── 1. Build data section ──────────────────────────────────────────────

	// We need a stable traversal order so offsets are deterministic.
	type ipEntry struct {
		ip4    uint32
		offset int
	}
	var entries []ipEntry

	dataSec := make([]byte, 0, 256)
	for ipStr, geo := range testEntries {
		parsed := net.ParseIP(ipStr).To4()
		if parsed == nil {
			continue
		}
		ipU32 := binary.BigEndian.Uint32(parsed)
		off := len(dataSec)
		dataSec = append(dataSec, mmdbEncodeGeo(geo)...)
		entries = append(entries, ipEntry{ipU32, off})
	}

	// ── 2. Build trie ──────────────────────────────────────────────────────
	// Pre-allocate generously (32 bits × len(entries) nodes max).
	maxNodes := 32*len(entries) + 4
	if maxNodes < 4 {
		maxNodes = 4
	}
	// left/right children; noData = "not yet allocated" during construction.
	left := make([]uint32, maxNodes)
	right := make([]uint32, maxNodes)
	for i := range left {
		left[i] = noData
		right[i] = noData
	}
	nodeCount := 1 // node 0 is root

	for _, e := range entries {
		node := 0
		for bit := 31; bit >= 0; bit-- {
			b := (e.ip4 >> uint(bit)) & 1
			var child uint32
			if b == 0 {
				child = left[node]
			} else {
				child = right[node]
			}
			if bit == 0 {
				// Final bit: store leaf pointer.
				// Leaf value = nodeCount + data_offset (resolved after tree is built).
				// We store data_offset + 1 temporarily (offset 0 is valid, so +1 avoids
				// confusion with the noData sentinel). We fix up below.
				leafVal := uint32(0x80000000) | uint32(e.offset)
				if b == 0 {
					left[node] = leafVal
				} else {
					right[node] = leafVal
				}
			} else if child == noData {
				// Allocate a new interior node.
				if nodeCount >= maxNodes {
					// Shouldn't happen given maxNodes sizing, but be safe.
					break
				}
				newNode := uint32(nodeCount)
				nodeCount++
				if b == 0 {
					left[node] = newNode
				} else {
					right[node] = newNode
				}
				node = int(newNode)
			} else if child&0x80000000 != 0 {
				// Already a leaf from a previous insert at the same /32.
				// Two entries at the same IP: last write wins.
				leafVal := uint32(0x80000000) | uint32(e.offset)
				if b == 0 {
					left[node] = leafVal
				} else {
					right[node] = leafVal
				}
				break
			} else {
				node = int(child)
			}
		}
	}

	// ── 3. Fix up leaf pointers and no-data sentinels ──────────────────────
	// Now nodeCount is final. Replace:
	//   0x80000000|offset  →  nodeCount + 16 + offset  (data pointer)
	//   noData             →  nodeCount                 (empty record)
	//
	// Per reader.go: resolveDataPointer computes offset as
	//   pointer - nodeCount - dataSectionSeparatorSize (16)
	// So data at offset 0 in dataSec requires pointer = nodeCount + 16 + 0.
	// nodeCount alone means "no data" (empty record sentinel).
	const dataSepSize = 16
	for i := 0; i < nodeCount; i++ {
		for _, ptr := range []*uint32{&left[i], &right[i]} {
			v := *ptr
			if v == noData {
				*ptr = uint32(nodeCount) // empty / no data
			} else if v&0x80000000 != 0 {
				*ptr = uint32(nodeCount) + dataSepSize + (v &^ 0x80000000)
			}
		}
	}

	// ── 4. Serialize tree ──────────────────────────────────────────────────
	treeBuf := make([]byte, nodeCount*nodeSize)
	for i := 0; i < nodeCount; i++ {
		l, r := left[i], right[i]
		base := i * nodeSize
		treeBuf[base+0] = byte(l >> 16)
		treeBuf[base+1] = byte(l >> 8)
		treeBuf[base+2] = byte(l)
		treeBuf[base+3] = byte(r >> 16)
		treeBuf[base+4] = byte(r >> 8)
		treeBuf[base+5] = byte(r)
	}

	// ── 5. Assemble ────────────────────────────────────────────────────────
	// MaxMind DB layout (per spec and reader.go):
	//   [tree: nodeCount * recordSize/4 bytes]
	//   [16-byte zero separator]
	//   [data section]
	//   [metadata marker: "\xAB\xCD\xEFMaxMind.com"]
	//   [metadata map]
	// The reader finds the marker via bytes.LastIndex and reads metadata
	// from the bytes AFTER the marker. Data section ends at marker start.
	separator := make([]byte, 16)
	meta := mmdbEncodeMeta(uint(nodeCount), recordSize)
	marker := []byte("\xAB\xCD\xEFMaxMind.com")

	var out []byte
	out = append(out, treeBuf...)
	out = append(out, separator...)
	out = append(out, dataSec...)
	out = append(out, marker...) // marker BEFORE metadata
	out = append(out, meta...)   // metadata AFTER marker
	return out
}

// mmdbEncodeGeo encodes a GeoEnrichment as a MaxMind DB record map.
// Produces: {country: {iso_code: "XX"}, subdivisions: [{iso_code: "YY"}]}
func mmdbEncodeGeo(geo domain.GeoEnrichment) []byte {
	fields := map[string][]byte{}
	if geo.Country != "" {
		countryMap := mmdbEncodeMap([][2]string{{"iso_code", geo.Country}})
		fields["country"] = countryMap
	}
	if geo.Region != "" {
		subdivMap := mmdbEncodeMap([][2]string{{"iso_code", geo.Region}})
		subdiv := mmdbEncodeArray([][]byte{subdivMap})
		fields["subdivisions"] = subdiv
	}
	return mmdbEncodeMapFields(fields)
}

// mmdbEncodeMap encodes ordered key-value string pairs as a MaxMind map.
func mmdbEncodeMap(pairs [][2]string) []byte {
	fields := make(map[string][]byte, len(pairs))
	for _, p := range pairs {
		fields[p[0]] = mmdbEncodeStr(p[1])
	}
	return mmdbEncodeMapFields(fields)
}

// mmdbEncodeMapFields encodes a pre-built map of MaxMind values.
func mmdbEncodeMapFields(fields map[string][]byte) []byte {
	// Type 7 = map; control byte high 3 bits = 111 = 7.
	var out []byte
	out = append(out, mmdbCtrl(7, len(fields))...)
	for k, v := range fields {
		out = append(out, mmdbEncodeStr(k)...)
		out = append(out, v...)
	}
	return out
}

// mmdbEncodeArray encodes a slice of pre-encoded values as a MaxMind array.
// MaxMind extended type 11 (array) = extended type offset 4 (11 - 7 = 4).
func mmdbEncodeArray(items [][]byte) []byte {
	// Extended type: ctrl byte high3=000 → extended marker; low5=size.
	// Next byte = extended type offset (4 for array).
	var out []byte
	size := len(items)
	if size <= 28 {
		out = append(out, byte(size)) // high3=0 (extended), low5=size
	} else if size <= 284 {
		out = append(out, byte(0x1D), byte(size-29)) // 29 + extra byte
	}
	out = append(out, 4) // extended type = array (11 - 7 = 4)
	for _, item := range items {
		out = append(out, item...)
	}
	return out
}

// mmdbEncodeStr encodes a string as MaxMind DB UTF-8 string (type 2).
func mmdbEncodeStr(s string) []byte {
	b := []byte(s)
	return append(mmdbCtrl(2, len(b)), b...)
}

// mmdbEncodeUint encodes a non-negative integer using the smallest MaxMind DB
// unsigned integer type that fits:
//   - 0: uint16 (type 5) with size 0
//   - 1..65535: uint16 (type 5)
//   - 65536+: uint32 (type 6)
func mmdbEncodeUint(v uint64) []byte {
	if v == 0 {
		return mmdbCtrl(5, 0) // zero-size payload = 0
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	// Find first non-zero byte.
	start := 0
	for start < 7 && buf[start] == 0 {
		start++
	}
	data := buf[start:]
	// Use uint16 (type 5) only if it fits in 2 bytes; otherwise uint32 (type 6).
	typeID := 5
	if len(data) > 2 {
		typeID = 6
	}
	return append(mmdbCtrl(typeID, len(data)), data...)
}

// mmdbCtrl encodes a MaxMind DB control byte sequence for a given type and size.
// typeID: 1=pointer, 2=string, 3=double, 4=bytes, 5=uint16, 6=uint32, 7=map
// Extended types (>= 8) are encoded by the caller.
func mmdbCtrl(typeID, size int) []byte {
	ctrl := byte(typeID << 5)
	switch {
	case size <= 28:
		return []byte{ctrl | byte(size)}
	case size <= 284:
		return []byte{ctrl | 29, byte(size - 29)}
	case size <= 65820:
		s := size - 285
		return []byte{ctrl | 30, byte(s >> 8), byte(s)}
	default:
		s := size - 65821
		return []byte{ctrl | 31, byte(s >> 16), byte(s >> 8), byte(s)}
	}
}

// mmdbEncodeMeta encodes the MaxMind DB metadata section as a map.
// All required fields are included per the MaxMind DB spec.
func mmdbEncodeMeta(nodeCount uint, recordSize int) []byte {
	// Build field list with stable iteration order.
	type kv struct{ k string; v []byte }
	fields := []kv{
		{"binary_format_major_version", mmdbEncodeUint(2)},
		{"binary_format_minor_version", mmdbEncodeUint(0)},
		{"build_epoch", mmdbEncodeUint(1700000000)},
		{"database_type", mmdbEncodeStr("GeoLite2-City-Test")},
		{"description", mmdbEncodeMapFields(map[string][]byte{"en": mmdbEncodeStr("Test DB")})},
		{"ip_version", mmdbEncodeUint(4)},
		{"languages", mmdbEncodeArray([][]byte{mmdbEncodeStr("en")})},
		{"node_count", mmdbEncodeUint(uint64(nodeCount))},
		{"record_size", mmdbEncodeUint(uint64(recordSize))},
	}

	// Encode as a MaxMind map.
	ctrl := mmdbCtrl(7, len(fields))
	var out []byte
	out = append(out, ctrl...)
	for _, f := range fields {
		out = append(out, mmdbEncodeStr(f.k)...)
		out = append(out, f.v...)
	}
	return out
}
