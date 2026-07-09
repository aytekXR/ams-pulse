package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer creates a test server and returns the httptest.Server and State.
// The caller is responsible for calling ts.Close().
func newTestServer(t *testing.T) (*httptest.Server, *State) {
	t.Helper()
	cfg := Config{AppName: "live"}
	state := NewState(cfg.AppName)
	srv := NewServer(cfg, state)
	return httptest.NewServer(srv), state
}

// TestSetBitRate_PublishedStream verifies that POST /control/set_bitrate on a published
// stream returns 200 and the subsequent broadcast list reflects the updated bitrate.
func TestSetBitRate_PublishedStream(t *testing.T) {
	ts, state := newTestServer(t)
	defer ts.Close()

	// Publish a stream so the control endpoint has something to update.
	state.Publish("s1", 5)

	payload := `{"stream_id":"s1","bitrate":2000000}`
	resp, err := http.Post(ts.URL+"/control/set_bitrate", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /control/set_bitrate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	// Verify the bitrate is visible in the broadcast list.
	listResp, err := http.Get(ts.URL + "/rest/v2/broadcasts/live/list")
	if err != nil {
		t.Fatalf("GET /rest/v2/broadcasts/live/list: %v", err)
	}
	defer listResp.Body.Close()

	var broadcasts []Broadcast
	if err := json.NewDecoder(listResp.Body).Decode(&broadcasts); err != nil {
		t.Fatalf("decode broadcast list: %v", err)
	}

	var found bool
	for _, b := range broadcasts {
		if b.StreamID == "s1" {
			found = true
			if b.BitRate != 2000000 {
				t.Errorf("want bitrate 2000000, got %v", b.BitRate)
			}
		}
	}
	if !found {
		t.Error("stream s1 not found in broadcast list after set_bitrate")
	}
}

// TestSetBitRate_UnknownStream verifies that POST /control/set_bitrate with an unknown
// stream_id returns 404 (no such stream to update).
func TestSetBitRate_UnknownStream(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	payload := `{"stream_id":"does-not-exist","bitrate":2000000}`
	resp, err := http.Post(ts.URL+"/control/set_bitrate", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /control/set_bitrate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream, got %d", resp.StatusCode)
	}
}

// TestSetBitRate_BadJSON verifies that malformed JSON returns 400.
func TestSetBitRate_BadJSON(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/control/set_bitrate", "application/json", bytes.NewBufferString("{bad json"))
	if err != nil {
		t.Fatalf("POST /control/set_bitrate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for bad JSON, got %d", resp.StatusCode)
	}
}

// TestSetBitRate_MissingStreamID verifies that a payload without stream_id returns 400.
func TestSetBitRate_MissingStreamID(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	payload := `{"bitrate":2000000}`
	resp, err := http.Post(ts.URL+"/control/set_bitrate", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /control/set_bitrate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for missing stream_id, got %d", resp.StatusCode)
	}
}

// TestAppPrefixedBroadcastList verifies that the app-prefixed path used by amsclient,
// GET /{app}/rest/v2/broadcasts/list/0/200, returns 200 and a JSON array containing
// the published stream. This is the path amsclient.ListBroadcasts actually calls.
func TestAppPrefixedBroadcastList(t *testing.T) {
	ts, state := newTestServer(t)
	defer ts.Close()

	state.Publish("stream-1", 10)

	resp, err := http.Get(ts.URL + "/live/rest/v2/broadcasts/list/0/200")
	if err != nil {
		t.Fatalf("GET /live/rest/v2/broadcasts/list/0/200: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var broadcasts []Broadcast
	if err := json.NewDecoder(resp.Body).Decode(&broadcasts); err != nil {
		t.Fatalf("decode broadcast list: %v", err)
	}
	var found bool
	for _, b := range broadcasts {
		if b.StreamID == "stream-1" {
			found = true
		}
	}
	if !found {
		t.Error("stream-1 not found in app-prefixed broadcast list")
	}
}

// TestAppPrefixedWebRTCClientStats verifies that the app-prefixed path used by amsclient,
// GET /{app}/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100, returns 200 and a JSON
// array. This is the path amsclient.WebRTCClientStats actually calls.
func TestAppPrefixedWebRTCClientStats(t *testing.T) {
	ts, state := newTestServer(t)
	defer ts.Close()

	state.Publish("stream-2", 5)

	resp, err := http.Get(ts.URL + "/live/rest/v2/broadcasts/stream-2/webrtc-client-stats/0/100")
	if err != nil {
		t.Fatalf("GET /live/rest/v2/broadcasts/stream-2/webrtc-client-stats/0/100: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var stats []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode webrtc stats: %v", err)
	}
	// stats may be empty — any valid JSON array is acceptable
}

// TestPagination_500Streams verifies that the /list route correctly paginates when there
// are 500 streams: page 0 → 200 items, page 200 → 200 items, page 400 → 100 items,
// page 500 → 0 items. This is the TDD red test for the pagination fix (BLOCKING bug:
// without the fix, every page returns all 500 items and ListBroadcastsPaged loops forever).
func TestPagination_500Streams(t *testing.T) {
	ts, state := newTestServer(t)
	defer ts.Close()

	// Publish 500 streams via state directly (bypasses HTTP control endpoint).
	for i := 1; i <= 500; i++ {
		state.Publish(fmt.Sprintf("pg-stream-%04d", i), 0)
	}

	cases := []struct {
		offset  int
		size    int
		wantLen int
	}{
		{0, 200, 200},
		{200, 200, 200},
		{400, 200, 100},
		{500, 200, 0},
	}

	for _, tc := range cases {
		url := fmt.Sprintf("%s/live/rest/v2/broadcasts/list/%d/%d", ts.URL, tc.offset, tc.size)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("offset=%d size=%d: want 200, got %d", tc.offset, tc.size, resp.StatusCode)
		}
		var broadcasts []Broadcast
		if err := json.NewDecoder(resp.Body).Decode(&broadcasts); err != nil {
			t.Fatalf("offset=%d size=%d: decode: %v", tc.offset, tc.size, err)
		}
		if len(broadcasts) != tc.wantLen {
			t.Errorf("offset=%d size=%d: want %d items, got %d", tc.offset, tc.size, tc.wantLen, len(broadcasts))
		}
	}
}

// TestBulkPublish verifies POST /control/bulk_publish: seeds N streams in one call,
// returns 200 with {"status":"ok","count":N}, and the /list endpoint reflects all streams.
// Bad body returns 400.
func TestBulkPublish(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	t.Run("happy path", func(t *testing.T) {
		payload := `{"count":10,"prefix":"bulk-","viewers_each":0}`
		resp, err := http.Post(ts.URL+"/control/bulk_publish", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("POST /control/bulk_publish: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if result["status"] != "ok" {
			t.Errorf("want status=ok, got %v", result["status"])
		}
		if int(result["count"].(float64)) != 10 {
			t.Errorf("want count=10, got %v", result["count"])
		}

		// Verify streams appear in list.
		listResp, err := http.Get(ts.URL + "/live/rest/v2/broadcasts/list/0/200")
		if err != nil {
			t.Fatalf("GET /list: %v", err)
		}
		defer listResp.Body.Close()
		var broadcasts []Broadcast
		if err := json.NewDecoder(listResp.Body).Decode(&broadcasts); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(broadcasts) != 10 {
			t.Errorf("want 10 streams in list, got %d", len(broadcasts))
		}
	})

	t.Run("bad json returns 400", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/control/bulk_publish", "application/json", bytes.NewBufferString("{bad json"))
		if err != nil {
			t.Fatalf("POST /control/bulk_publish: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400 for bad JSON, got %d", resp.StatusCode)
		}
	})

	t.Run("zero count returns 400", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/control/bulk_publish", "application/json", bytes.NewBufferString(`{"count":0,"prefix":"x-"}`))
		if err != nil {
			t.Fatalf("POST /control/bulk_publish: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400 for count=0, got %d", resp.StatusCode)
		}
	})
}
