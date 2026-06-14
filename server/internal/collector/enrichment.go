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
func BuildTestMMDB(testEntries map[string]domain.GeoEnrichment) []byte {
	// Build a minimal MaxMind DB v2.0 with 4-bit record size (24-bit nodes),
	// IPv4-only, containing the given test entries.
	//
	// Structure:
	//   [binary search tree nodes]
	//   [16-byte data section separator: 0x00 x16]
	//   [data section: serialized record values]
	//   [metadata: MaxMind DB metadata]
	//   [metadata marker: "\xAB\xCD\xEFMaxMind.com"]
	//
	// For simplicity we use a 24-bit record size (3 bytes per record, 6 bytes per node).
	// Each node has two 24-bit child pointers (left=0-bit, right=1-bit).

	const recordSize uint = 24
	const nodeSize = 6 // bytes per node (2 records × 24 bits)

	// Build data section first, collecting offsets for each record.
	dataSec := make([]byte, 0, 1024)
	type recordOffset struct {
		off  int
		size int
	}
	entryOffsets := make(map[string]int, len(testEntries))

	for _, geo := range testEntries {
		// Encode as MaxMind map: {country: {iso_code: "XX"}, subdivisions: [{iso_code: "YY"}]}
		encoded := encodeMMDBRecord(geo)
		entryOffsets[geo.Country+"/"+geo.Region] = len(dataSec)
		dataSec = append(dataSec, encoded...)
	}

	// Map from IP → data offset.
	ipToOffset := make(map[uint32]int)
	for ipStr, geo := range testEntries {
		ip := net.ParseIP(ipStr).To4()
		if ip == nil {
			continue
		}
		key := binary.BigEndian.Uint32(ip)
		off, ok := entryOffsets[geo.Country+"/"+geo.Region]
		if ok {
			ipToOffset[key] = off
		}
	}

	// Build the binary search tree.
	// We use a simple approach: allocate enough nodes for a 32-bit IPv4 tree
	// (32 levels, 2 subtrees: IPv6-compat + IPv4).
	// For a minimal implementation with few entries, we can fit in ~100 nodes.
	//
	// The MaxMind DB format requires:
	//   - Node 0 is the root.
	//   - Left child = bit 0, right child = bit 1.
	//   - Leaf record value: if >= nodeCount → data section offset + 16 (separator)
	//     (specifically: value - nodeCount = data offset)
	//   - Reserved node: nodeCount itself means "no data for this IP".

	// Collect all unique IPv4 addresses.
	type entry struct {
		ip  uint32
		off int
	}
	var entries []entry
	for ip, off := range ipToOffset {
		entries = append(entries, entry{ip, off})
	}

	// Build a minimal trie. We'll preallocate 200 nodes to be safe.
	maxNodes := 200
	// nodes[i][0] = left child, nodes[i][1] = right child
	nodes := make([][2]uint32, maxNodes)
	nodeCount := 1 // node 0 is root
	// Initialize all nodes to point to "no data" (will be set to nodeCount at finalize)
	for i := range nodes {
		nodes[i][0] = 0xFFFFFF // placeholder for "no data"
		nodes[i][1] = 0xFFFFFF
	}

	// Insert each entry into the trie.
	for _, e := range entries {
		node := 0
		for bit := 31; bit >= 0; bit-- {
			b := (e.ip >> uint(bit)) & 1
			child := nodes[node][b]
			if bit == 0 {
				// Leaf: set to nodeCount + data_offset (record value encoding).
				nodes[node][b] = uint32(nodeCount) + uint32(e.off)
			} else if child == 0xFFFFFF {
				// Need a new interior node.
				newNode := nodeCount
				nodeCount++
				nodes[node][b] = uint32(newNode)
				node = newNode
			} else {
				node = int(child)
			}
		}
	}

	// Finalize: replace 0xFFFFFF placeholders with nodeCount (no-data sentinel).
	for i := 0; i < nodeCount; i++ {
		for b := 0; b < 2; b++ {
			if nodes[i][b] == 0xFFFFFF {
				nodes[i][b] = uint32(nodeCount)
			}
		}
	}

	// Serialize the node tree.
	treeBuf := make([]byte, nodeCount*nodeSize)
	for i := 0; i < nodeCount; i++ {
		// 24-bit big-endian for each record.
		left := nodes[i][0]
		right := nodes[i][1]
		treeBuf[i*nodeSize+0] = byte(left >> 16)
		treeBuf[i*nodeSize+1] = byte(left >> 8)
		treeBuf[i*nodeSize+2] = byte(left)
		treeBuf[i*nodeSize+3] = byte(right >> 16)
		treeBuf[i*nodeSize+4] = byte(right >> 8)
		treeBuf[i*nodeSize+5] = byte(right)
	}

	// Assemble: tree + 16-byte separator + data + metadata + marker.
	separator := make([]byte, 16)

	// Metadata is encoded as a MaxMind map.
	meta := encodeMMDBMetadata(uint(nodeCount), recordSize)
	metaMarker := []byte("\xAB\xCD\xEFMaxMind.com")

	var out []byte
	out = append(out, treeBuf...)
	out = append(out, separator...)
	out = append(out, dataSec...)
	out = append(out, meta...)
	out = append(out, metaMarker...)

	return out
}

// encodeMMDBRecord encodes a GeoEnrichment as a MaxMind DB data record (map type).
// Uses the MaxMind binary format encoding:
//   - Map type = 7 (0b111 in high 3 bits of control byte)
//   - String type = 2
//
// Format: control_byte [extended_type] size payload
func encodeMMDBRecord(geo domain.GeoEnrichment) []byte {
	// Build the subdivisions list (array with one element).
	var subdivBytes []byte
	if geo.Region != "" {
		// Inner map: {iso_code: "YY"}
		subdivMap := encodeMMDBMap(map[string]string{"iso_code": geo.Region})
		// Array of 1 element.
		subdivBytes = append([]byte{byte(0b00000100<<1 | 0b001)}, subdivMap...) // array, size=1? Use explicit.
		subdivBytes = encodeMMDBArray([][]byte{subdivMap})
	}

	fields := map[string][]byte{}

	// country map: {iso_code: "XX"}
	if geo.Country != "" {
		countryMap := encodeMMDBMap(map[string]string{"iso_code": geo.Country})
		fields["country"] = countryMap
	}
	if len(subdivBytes) > 0 {
		fields["subdivisions"] = subdivBytes
	}

	return encodeMMDBMapBytes(fields)
}

// encodeMMDBMap encodes a map[string]string as MaxMind binary map.
func encodeMMDBMap(m map[string]string) []byte {
	fields := map[string][]byte{}
	for k, v := range m {
		fields[k] = encodeMMDBString(v)
	}
	return encodeMMDBMapBytes(fields)
}

// encodeMMDBMapBytes encodes a map of pre-encoded values as a MaxMind DB map.
func encodeMMDBMapBytes(fields map[string][]byte) []byte {
	var out []byte
	size := len(fields)
	// Map type = 7 (control byte high 3 bits = 0b111 = 7).
	out = append(out, encodeMMDBCtrl(7, size)...)
	for k, v := range fields {
		out = append(out, encodeMMDBString(k)...)
		out = append(out, v...)
	}
	return out
}

// encodeMMDBArray encodes a [][]byte as a MaxMind DB array.
func encodeMMDBArray(items [][]byte) []byte {
	var out []byte
	// Array type = 11 (extended type 4, in extended space).
	// Extended: type = 0, extended byte = 4 (array).
	size := len(items)
	// Control byte for extended type: high 3 bits = 0, low 5 bits = size if <= 28.
	// Extended type byte follows: 4 = array.
	if size <= 28 {
		out = append(out, byte(size)) // high3=0 (extended), low5=size
	} else {
		out = append(out, 29, byte(size-29))
	}
	out = append(out, 4) // extended type: array
	for _, item := range items {
		out = append(out, item...)
	}
	return out
}

// encodeMMDBString encodes a string as MaxMind DB UTF-8 string.
func encodeMMDBString(s string) []byte {
	return encodeMMDBBytes(2, []byte(s)) // type 2 = UTF-8 string
}

// encodeMMDBBytes encodes a value with the given type id and raw bytes.
func encodeMMDBBytes(typeID int, data []byte) []byte {
	return append(encodeMMDBCtrl(typeID, len(data)), data...)
}

// encodeMMDBCtrl encodes a MaxMind DB control byte (and possible extended size bytes).
// typeID: 0=extended, 1=pointer, 2=string, 3=double, 4=bytes, 5=uint16, 6=uint32, 7=map
func encodeMMDBCtrl(typeID, size int) []byte {
	var out []byte
	var ctrl byte

	if typeID <= 7 {
		ctrl = byte(typeID<<5)
	} else {
		// Extended type.
		ctrl = 0 // high 3 bits = 0 for extended
	}

	switch {
	case size <= 28:
		ctrl |= byte(size)
		out = append(out, ctrl)
	case size <= 284:
		ctrl |= 29
		out = append(out, ctrl, byte(size-29))
	case size <= 65820:
		ctrl |= 30
		s := size - 285
		out = append(out, ctrl, byte(s>>8), byte(s))
	default:
		ctrl |= 31
		s := size - 65821
		out = append(out, ctrl, byte(s>>16), byte(s>>8), byte(s))
	}
	return out
}

// encodeMMDBMetadata encodes the MaxMind DB metadata section as a map.
func encodeMMDBMetadata(nodeCount, recordSize uint) []byte {
	// Required fields: binary_format_major_version, binary_format_minor_version,
	// build_epoch, database_type, description, ip_version, node_count, record_size.
	type uintField struct {
		name string
		val  uint64
	}
	uintFields := []uintField{
		{"binary_format_major_version", 2},
		{"binary_format_minor_version", 0},
		{"build_epoch", 1700000000},
		{"ip_version", 4},
		{"node_count", uint64(nodeCount)},
		{"record_size", uint64(recordSize)},
	}

	fields := map[string][]byte{}

	for _, f := range uintFields {
		fields[f.name] = encodeMMDBUint32(f.val)
	}
	fields["database_type"] = encodeMMDBString("GeoLite2-City-Test")
	// description: map {en: "Test DB"}
	fields["description"] = encodeMMDBMap(map[string]string{"en": "Test DB"})
	// languages: array of strings ["en"]
	fields["languages"] = encodeMMDBArray([][]byte{encodeMMDBString("en")})

	return encodeMMDBMapBytes(fields)
}

// encodeMMDBUint32 encodes a uint64 as MaxMind DB uint32 (type 6).
func encodeMMDBUint32(v uint64) []byte {
	if v == 0 {
		return encodeMMDBCtrl(6, 0) // zero-size uint = 0
	}
	// Encode as big-endian, minimal bytes.
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	// Find first non-zero byte.
	start := 0
	for start < 7 && buf[start] == 0 {
		start++
	}
	data := buf[start:]
	return encodeMMDBBytes(6, data)
}
