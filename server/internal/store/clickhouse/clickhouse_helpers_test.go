// Package clickhouse — unit tests for the pure data-helper functions in
// clickhouse.go. These require no ClickHouse connection and run under the
// plain (non-integration) test build.
package clickhouse

import (
	"testing"
)

// ─── strFromData ─────────────────────────────────────────────────────────────

func TestStrFromData_KeyPresent(t *testing.T) {
	d := map[string]any{"publish_type": "rtmp"}
	got := strFromData(d, "publish_type")
	if got != "rtmp" {
		t.Errorf("strFromData: got %q, want %q", got, "rtmp")
	}
}

func TestStrFromData_KeyAbsent(t *testing.T) {
	d := map[string]any{"other_key": "value"}
	got := strFromData(d, "missing")
	if got != "" {
		t.Errorf("strFromData absent: got %q, want empty string", got)
	}
}

func TestStrFromData_NilMap(t *testing.T) {
	got := strFromData(nil, "key")
	if got != "" {
		t.Errorf("strFromData nil map: got %q, want empty string", got)
	}
}

func TestStrFromData_WrongType_Int(t *testing.T) {
	d := map[string]any{"key": 42}
	got := strFromData(d, "key")
	if got != "" {
		t.Errorf("strFromData wrong type int: got %q, want empty string", got)
	}
}

func TestStrFromData_WrongType_Float(t *testing.T) {
	d := map[string]any{"key": 3.14}
	got := strFromData(d, "key")
	if got != "" {
		t.Errorf("strFromData wrong type float64: got %q, want empty string", got)
	}
}

// ─── intFromData ─────────────────────────────────────────────────────────────

func TestIntFromData_IntValue(t *testing.T) {
	d := map[string]any{"viewer_count": 25}
	got := intFromData(d, "viewer_count")
	if got != 25 {
		t.Errorf("intFromData int: got %d, want 25", got)
	}
}

func TestIntFromData_Float64Value(t *testing.T) {
	// JSON unmarshaling typically produces float64 for numbers.
	d := map[string]any{"viewer_count": float64(99.9)}
	got := intFromData(d, "viewer_count")
	// float64(99.9) truncates to 99, not rounds to 100.
	if got != 99 {
		t.Errorf("intFromData float64: got %d, want 99", got)
	}
}

func TestIntFromData_Int64Value(t *testing.T) {
	d := map[string]any{"viewer_count": int64(1000)}
	got := intFromData(d, "viewer_count")
	if got != 1000 {
		t.Errorf("intFromData int64: got %d, want 1000", got)
	}
}

func TestIntFromData_KeyAbsent(t *testing.T) {
	d := map[string]any{"other": 5}
	got := intFromData(d, "missing")
	if got != 0 {
		t.Errorf("intFromData absent: got %d, want 0", got)
	}
}

func TestIntFromData_WrongType_String(t *testing.T) {
	d := map[string]any{"viewer_count": "not-a-number"}
	got := intFromData(d, "viewer_count")
	if got != 0 {
		t.Errorf("intFromData wrong type string: got %d, want 0", got)
	}
}

func TestIntFromData_WrongType_Bool(t *testing.T) {
	d := map[string]any{"viewer_count": true}
	got := intFromData(d, "viewer_count")
	if got != 0 {
		t.Errorf("intFromData wrong type bool: got %d, want 0", got)
	}
}

// ─── int64FromData ───────────────────────────────────────────────────────────

func TestInt64FromData_Int64Value(t *testing.T) {
	d := map[string]any{"size_bytes": int64(4294967296)} // 2^32, beyond int32 range
	got := int64FromData(d, "size_bytes")
	if got != 4294967296 {
		t.Errorf("int64FromData int64: got %d, want 4294967296", got)
	}
}

func TestInt64FromData_Float64Value(t *testing.T) {
	// JSON numbers unmarshaled as float64.
	d := map[string]any{"size_bytes": float64(100000.7)}
	got := int64FromData(d, "size_bytes")
	if got != 100000 {
		t.Errorf("int64FromData float64: got %d, want 100000", got)
	}
}

func TestInt64FromData_IntValue(t *testing.T) {
	d := map[string]any{"size_bytes": 512}
	got := int64FromData(d, "size_bytes")
	if got != 512 {
		t.Errorf("int64FromData int: got %d, want 512", got)
	}
}

func TestInt64FromData_KeyAbsent(t *testing.T) {
	d := map[string]any{"other": "value"}
	got := int64FromData(d, "missing")
	if got != 0 {
		t.Errorf("int64FromData absent: got %d, want 0", got)
	}
}

func TestInt64FromData_WrongType_String(t *testing.T) {
	d := map[string]any{"size_bytes": "1234"}
	got := int64FromData(d, "size_bytes")
	if got != 0 {
		t.Errorf("int64FromData wrong type string: got %d, want 0", got)
	}
}

// ─── floatFromData ───────────────────────────────────────────────────────────

func TestFloatFromData_Float64Value(t *testing.T) {
	d := map[string]any{"bitrate_kbps": float64(2500.5)}
	got := floatFromData(d, "bitrate_kbps")
	if got != 2500.5 {
		t.Errorf("floatFromData float64: got %v, want 2500.5", got)
	}
}

func TestFloatFromData_Float32Value(t *testing.T) {
	d := map[string]any{"bitrate_kbps": float32(1024.0)}
	got := floatFromData(d, "bitrate_kbps")
	// float32 → float64 conversion may introduce tiny epsilon; check approximately.
	if got < 1023.9 || got > 1024.1 {
		t.Errorf("floatFromData float32: got %v, want ~1024.0", got)
	}
}

func TestFloatFromData_IntValue(t *testing.T) {
	d := map[string]any{"bitrate_kbps": 500}
	got := floatFromData(d, "bitrate_kbps")
	if got != 500.0 {
		t.Errorf("floatFromData int: got %v, want 500.0", got)
	}
}

func TestFloatFromData_KeyAbsent(t *testing.T) {
	d := map[string]any{}
	got := floatFromData(d, "missing")
	if got != 0.0 {
		t.Errorf("floatFromData absent: got %v, want 0.0", got)
	}
}

func TestFloatFromData_WrongType_String(t *testing.T) {
	d := map[string]any{"bitrate_kbps": "fast"}
	got := floatFromData(d, "bitrate_kbps")
	if got != 0.0 {
		t.Errorf("floatFromData wrong type string: got %v, want 0.0", got)
	}
}

// ─── boolFromData ────────────────────────────────────────────────────────────

func TestBoolFromData_TrueValue(t *testing.T) {
	d := map[string]any{"fatal": true}
	got := boolFromData(d, "fatal")
	if !got {
		t.Errorf("boolFromData true: got false, want true")
	}
}

func TestBoolFromData_FalseValue(t *testing.T) {
	d := map[string]any{"fatal": false}
	got := boolFromData(d, "fatal")
	if got {
		t.Errorf("boolFromData false: got true, want false")
	}
}

func TestBoolFromData_KeyAbsent(t *testing.T) {
	d := map[string]any{}
	got := boolFromData(d, "missing")
	if got {
		t.Errorf("boolFromData absent: got true, want false")
	}
}

func TestBoolFromData_WrongType_Int(t *testing.T) {
	d := map[string]any{"fatal": 1}
	got := boolFromData(d, "fatal")
	if got {
		t.Errorf("boolFromData wrong type int: got true, want false")
	}
}

// ─── boolToUint8 ─────────────────────────────────────────────────────────────

func TestBoolToUint8_True(t *testing.T) {
	got := boolToUint8(true)
	if got != 1 {
		t.Errorf("boolToUint8(true): got %d, want 1", got)
	}
}

func TestBoolToUint8_False(t *testing.T) {
	got := boolToUint8(false)
	if got != 0 {
		t.Errorf("boolToUint8(false): got %d, want 0", got)
	}
}

// ─── intFromProtocol ─────────────────────────────────────────────────────────

func TestIntFromProtocol_ProtocolPresent(t *testing.T) {
	d := map[string]any{
		"viewer_count_by_protocol": map[string]any{
			"webrtc": 15,
			"hls":    float64(30),
		},
	}
	if got := intFromProtocol(d, "webrtc"); got != 15 {
		t.Errorf("intFromProtocol webrtc: got %d, want 15", got)
	}
	if got := intFromProtocol(d, "hls"); got != 30 {
		t.Errorf("intFromProtocol hls: got %d, want 30", got)
	}
}

func TestIntFromProtocol_ProtocolAbsent(t *testing.T) {
	d := map[string]any{
		"viewer_count_by_protocol": map[string]any{
			"webrtc": 5,
		},
	}
	got := intFromProtocol(d, "rtmp")
	if got != 0 {
		t.Errorf("intFromProtocol absent proto: got %d, want 0", got)
	}
}

func TestIntFromProtocol_NoProtocolMap(t *testing.T) {
	d := map[string]any{"viewer_count": 10}
	got := intFromProtocol(d, "webrtc")
	if got != 0 {
		t.Errorf("intFromProtocol no protocol map: got %d, want 0", got)
	}
}

func TestIntFromProtocol_WrongMapType(t *testing.T) {
	// viewer_count_by_protocol is a string instead of map[string]any — must not panic.
	d := map[string]any{"viewer_count_by_protocol": "not-a-map"}
	got := intFromProtocol(d, "webrtc")
	if got != 0 {
		t.Errorf("intFromProtocol wrong map type: got %d, want 0", got)
	}
}
