package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
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
	listResp, err := http.Get(ts.URL + "/live/rest/v2/broadcasts/list/0/200")
	if err != nil {
		t.Fatalf("GET /live/rest/v2/broadcasts/list/0/200: %v", err)
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

// TestWSSignaling_HappyPath verifies the WO-B WebSocket signaling handler:
// upgrade on /{app}/websocket, read a play command, reply with a
// takeConfiguration/offer message carrying a non-empty SDP (D-072 verifier
// finding: the handler previously had zero test coverage).
func TestWSSignaling_HappyPath(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/live/websocket"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial %s: %v", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	play := map[string]interface{}{
		"command":   "play",
		"streamId":  "ws-test-stream",
		"token":     "",
		"trackList": []string{},
	}
	if err := wsjson.Write(ctx, conn, play); err != nil {
		t.Fatalf("write play: %v", err)
	}

	var reply struct {
		Command  string `json:"command"`
		StreamID string `json:"streamId"`
		Type     string `json:"type"`
		SDP      string `json:"sdp"`
	}
	// Real AMS sends notification messages before the offer (D-074); the mock
	// mirrors that — skip them like any real signaling client must.
	for {
		if err := wsjson.Read(ctx, conn, &reply); err != nil {
			t.Fatalf("read offer: %v", err)
		}
		if reply.Command != "notification" {
			break
		}
	}

	if reply.Command != "takeConfiguration" {
		t.Errorf("command = %q, want takeConfiguration", reply.Command)
	}
	if reply.Type != "offer" {
		t.Errorf("type = %q, want offer", reply.Type)
	}
	if reply.StreamID != "ws-test-stream" {
		t.Errorf("streamId = %q, want ws-test-stream", reply.StreamID)
	}
	if !strings.HasPrefix(reply.SDP, "v=0") {
		t.Errorf("sdp does not look like SDP: %q", reply.SDP)
	}
}

// newClusterTestServer creates a test server in cluster mode.
func newClusterTestServer(t *testing.T) (*httptest.Server, *State) {
	t.Helper()
	cfg := Config{AppName: "live", ClusterMode: true}
	state := NewState(cfg.AppName)
	srv := NewServer(cfg, state)
	return httptest.NewServer(srv), state
}

// TestClusterModeStatus_Standalone verifies that /rest/v2/cluster-mode-status
// returns success=false in the default (standalone) configuration.
func TestClusterModeStatus_Standalone(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/v2/cluster-mode-status")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster-mode-status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode cluster-mode-status: %v", err)
	}
	if result["success"] != false {
		t.Errorf("standalone: cluster-mode-status success = %v, want false", result["success"])
	}
}

// TestClusterModeStatus_Cluster verifies that /rest/v2/cluster-mode-status
// returns success=true when ClusterMode is enabled.
func TestClusterModeStatus_Cluster(t *testing.T) {
	ts, _ := newClusterTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/v2/cluster-mode-status")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster-mode-status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode cluster-mode-status: %v", err)
	}
	if result["success"] != true {
		t.Errorf("cluster mode: cluster-mode-status success = %v, want true", result["success"])
	}
}

// TestClusterNodes_FlatPath404 verifies that GET /rest/v2/cluster/nodes (flat path,
// absent on real AMS 3.x) returns 404 in both standalone and cluster mode.
func TestClusterNodes_FlatPath404(t *testing.T) {
	for _, name := range []string{"standalone", "cluster"} {
		t.Run(name, func(t *testing.T) {
			var ts *httptest.Server
			if name == "cluster" {
				ts, _ = newClusterTestServer(t)
			} else {
				ts, _ = newTestServer(t)
			}
			defer ts.Close()

			resp, err := http.Get(ts.URL + "/rest/v2/cluster/nodes")
			if err != nil {
				t.Fatalf("GET /rest/v2/cluster/nodes: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("flat cluster/nodes: want 404, got %d", resp.StatusCode)
			}
		})
	}
}

// TestClusterNodes_PaginatedPath_Standalone500 verifies that GET
// /rest/v2/cluster/nodes/0/50 returns HTTP 500 in standalone mode,
// matching the behavior of real AMS 3.0.3 (live-probed 2026-07-26).
func TestClusterNodes_PaginatedPath_Standalone500(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/v2/cluster/nodes/0/50")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster/nodes/0/50: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("standalone paginated cluster/nodes: want 500, got %d", resp.StatusCode)
	}
}

// TestClusterNodes_PaginatedPath_ClusterMode verifies that GET
// /rest/v2/cluster/nodes/{offset}/{size} returns paginated node fixtures
// in cluster mode, using the real AMS 3.x wire fields.
func TestClusterNodes_PaginatedPath_ClusterMode(t *testing.T) {
	ts, _ := newClusterTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/v2/cluster/nodes/0/50")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster/nodes/0/50: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cluster paginated nodes: want 200, got %d", resp.StatusCode)
	}
	var nodes []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatalf("decode cluster nodes page 0: %v", err)
	}
	// clusterNodeFixtures() returns 60 nodes; page 0 size 50 should return 50.
	if len(nodes) != 50 {
		t.Errorf("cluster nodes page 0/50: want 50 nodes, got %d", len(nodes))
	}
	// Verify real AMS 3.x wire fields are present on the first node.
	first := nodes[0]
	if _, ok := first["id"]; !ok {
		t.Errorf("cluster node missing 'id' field (real AMS 3.x wire field)")
	}
	if _, ok := first["ip"]; !ok {
		t.Errorf("cluster node missing 'ip' field")
	}
	if _, ok := first["status"]; !ok {
		t.Errorf("cluster node missing 'status' field")
	}
}

// TestClusterNodes_MultiPagePagination verifies that the paginated cluster/nodes
// endpoint correctly returns 60 total nodes across two pages (page 0: 50, page 1: 10)
// in cluster mode, exercising the pagination boundary.
func TestClusterNodes_MultiPagePagination(t *testing.T) {
	ts, _ := newClusterTestServer(t)
	defer ts.Close()

	// Page 0: expect 50 nodes (full page).
	resp0, err := http.Get(ts.URL + "/rest/v2/cluster/nodes/0/50")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster/nodes/0/50: %v", err)
	}
	defer resp0.Body.Close()
	var page0 []map[string]any
	if err := json.NewDecoder(resp0.Body).Decode(&page0); err != nil {
		t.Fatalf("decode page 0: %v", err)
	}
	if len(page0) != 50 {
		t.Errorf("page 0: want 50 nodes, got %d", len(page0))
	}

	// Page 1: expect 10 nodes (short page, terminates pagination).
	resp1, err := http.Get(ts.URL + "/rest/v2/cluster/nodes/50/50")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster/nodes/50/50: %v", err)
	}
	defer resp1.Body.Close()
	var page1 []map[string]any
	if err := json.NewDecoder(resp1.Body).Decode(&page1); err != nil {
		t.Fatalf("decode page 1: %v", err)
	}
	if len(page1) != 10 {
		t.Errorf("page 1: want 10 nodes, got %d", len(page1))
	}

	// Page 2: expect 0 nodes (past the end).
	resp2, err := http.Get(ts.URL + "/rest/v2/cluster/nodes/60/50")
	if err != nil {
		t.Fatalf("GET /rest/v2/cluster/nodes/60/50: %v", err)
	}
	defer resp2.Body.Close()
	var page2 []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&page2); err != nil {
		t.Fatalf("decode page 2: %v", err)
	}
	if len(page2) != 0 {
		t.Errorf("page 2 (past end): want 0 nodes, got %d", len(page2))
	}
}

// TestSystemStatus verifies that GET /rest/v2/system-status returns 200
// with cpuUsage and ramUsage fields.
func TestSystemStatus(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/v2/system-status")
	if err != nil {
		t.Fatalf("GET /rest/v2/system-status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode system-status: %v", err)
	}
	if _, ok := result["cpuUsage"]; !ok {
		t.Errorf("system-status: missing 'cpuUsage' field")
	}
	if _, ok := result["ramUsage"]; !ok {
		t.Errorf("system-status: missing 'ramUsage' field")
	}
}

// TestVersion verifies that GET /rest/v2/version returns 200 with
// versionName, versionType, and buildNumber fields.
func TestVersion(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/v2/version")
	if err != nil {
		t.Fatalf("GET /rest/v2/version: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if result["versionName"] != "3.0.3" {
		t.Errorf("versionName = %v, want 3.0.3", result["versionName"])
	}
	if result["versionType"] != "Enterprise" {
		t.Errorf("versionType = %v, want Enterprise", result["versionType"])
	}
}

// TestAuthenticate verifies that POST /rest/v2/users/authenticate returns 200,
// a success response body, and sets a JSESSIONID session cookie.
func TestAuthenticate(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	payload := `{"email":"admin@example.com","password":"secret"}`
	resp, err := http.Post(ts.URL+"/rest/v2/users/authenticate", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /rest/v2/users/authenticate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode authenticate response: %v", err)
	}
	if result["success"] != true {
		t.Errorf("authenticate success = %v, want true", result["success"])
	}
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "JSESSIONID" {
			found = true
			break
		}
	}
	if !found {
		t.Error("authenticate: JSESSIONID cookie not set in response")
	}
}

// TestVodsList verifies that GET /{app}/rest/v2/vods/list/{offset}/{size}
// returns 200 and a JSON array (empty by default).
func TestVodsList(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/live/rest/v2/vods/list/0/200")
	if err != nil {
		t.Fatalf("GET /live/rest/v2/vods/list/0/200: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode vods list: %v", err)
	}
}
