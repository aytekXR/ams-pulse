package amsclient_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// TestListVods_FixtureDecode decodes the live-captured vods_list.json fixture
// (GET /pulse-test/rest/v2/vods/list/0/5 on AMS 3.0.3, 2026-07-12) and asserts
// every declared VodDTO field matches the capture exactly.
func TestListVods_FixtureDecode(t *testing.T) {
	fixture := mustReadFixture(t, "vods_list.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	vods, err := c.ListVods(context.Background(), "pulse-test", 0, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vods) != 1 {
		t.Fatalf("expected 1 vod, got %d", len(vods))
	}
	v := vods[0]
	// vodId: stable unique string id (dedup key)
	if v.VodID != "SiJJzyAJEDhSMd7nmaDmgIbz" {
		t.Errorf("VodID = %q, want SiJJzyAJEDhSMd7nmaDmgIbz", v.VodID)
	}
	// vodName: file name
	if v.VodName != "val-vodgen-s17.mp4" {
		t.Errorf("VodName = %q, want val-vodgen-s17.mp4", v.VodName)
	}
	// streamId: originating stream id (NOT streamName which is the file name)
	if v.StreamID != "val-vodgen-s17" {
		t.Errorf("StreamID = %q, want val-vodgen-s17", v.StreamID)
	}
	// filePath: relative path
	if v.FilePath != "streams/val-vodgen-s17.mp4" {
		t.Errorf("FilePath = %q, want streams/val-vodgen-s17.mp4", v.FilePath)
	}
	// fileSize: bytes (int64)
	if v.FileSize != 3125555 {
		t.Errorf("FileSize = %d, want 3125555", v.FileSize)
	}
	// creationDate: Unix epoch MILLISECONDS
	if v.CreationDate != 1783770838091 {
		t.Errorf("CreationDate = %d, want 1783770838091", v.CreationDate)
	}
	// duration: MILLISECONDS (43025 for a ~43s VoD — NOT seconds)
	if v.Duration != 43025 {
		t.Errorf("Duration = %d, want 43025", v.Duration)
	}
	// type: VoD origin type
	if v.Type != "streamVod" {
		t.Errorf("Type = %q, want streamVod", v.Type)
	}
}

// TestListVodsPaged_TwoPages verifies that ListVodsPaged makes exactly 2 HTTP
// calls when the first page is full (200 items) and the second page is partial
// (1 item), returns 201 total items, and passes correct offsets in request paths.
func TestListVodsPaged_TwoPages(t *testing.T) {
	var callCount atomic.Int32
	var mu sync.Mutex
	var paths []string

	// Generate a full 200-item page.
	page0 := make([]amsclient.VodDTO, 200)
	for i := range page0 {
		page0[i] = amsclient.VodDTO{VodID: fmt.Sprintf("vod-%03d", i)}
	}
	page0JSON, _ := json.Marshal(page0)

	// One-item second page.
	page1 := []amsclient.VodDTO{{VodID: "vod-last"}}
	page1JSON, _ := json.Marshal(page1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write(page0JSON)
		} else {
			_, _ = w.Write(page1JSON)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	vods, err := c.ListVodsPaged(context.Background(), "pulse-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vods) != 201 {
		t.Errorf("expected 201 vods total, got %d", len(vods))
	}
	if n := callCount.Load(); n != 2 {
		t.Errorf("expected exactly 2 HTTP calls, got %d", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) < 2 {
		t.Fatalf("expected at least 2 recorded paths, got %d", len(paths))
	}
	// First request must use offset 0.
	if !strings.Contains(paths[0], "/0/200") {
		t.Errorf("first request path %q must contain /0/200", paths[0])
	}
	// Second request must use offset 200.
	if !strings.Contains(paths[1], "/200/200") {
		t.Errorf("second request path %q must contain /200/200", paths[1])
	}
}

// TestListVods_EmptyList verifies that an app with zero VoDs returns an empty
// (non-nil) slice and a nil error.
func TestListVods_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	vods, err := c.ListVods(context.Background(), "empty-app", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vods) != 0 {
		t.Errorf("expected 0 vods, got %d", len(vods))
	}
}

// TestListVods_Non2xx_ReturnsError verifies that a 500 response from the VoD
// list endpoint surfaces as a non-nil error containing the HTTP status code.
func TestListVods_Non2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal server error"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ListVods(context.Background(), "pulse-test", 0, 200)
	if err == nil {
		t.Fatal("expected non-nil error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}
