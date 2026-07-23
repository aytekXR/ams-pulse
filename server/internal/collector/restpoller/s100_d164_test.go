// S100 / D-164 — poll-loop freshness reporting.
//
// The poller is the only component that knows whether AMS is actually
// answering. Before D-164 it kept that knowledge to itself (a WARN log per
// failed poll) while /healthz reported the collector "ok" off a stale snapshot.
// These tests pin the freshness state machine PollHealth exposes.
package restpoller

import (
	"errors"
	"testing"
	"time"
)

func TestD164_PollHealth_BeforeAnyPoll_HasNoSuccess(t *testing.T) {
	p := &Poller{cfg: Config{PollInterval: 5 * time.Second}}

	h := p.PollHealth()
	if !h.LastSuccess.IsZero() {
		t.Errorf("LastSuccess: want zero before any poll, got %v", h.LastSuccess)
	}
	if h.LastError != "" {
		t.Errorf("LastError: want empty before any poll, got %q", h.LastError)
	}
}

func TestD164_PollHealth_RecordsSuccess(t *testing.T) {
	p := &Poller{cfg: Config{PollInterval: 5 * time.Second}}
	before := time.Now()

	p.recordPoll(nil)

	h := p.PollHealth()
	if h.LastSuccess.Before(before) {
		t.Errorf("LastSuccess: want >= %v after a successful poll, got %v", before, h.LastSuccess)
	}
	if h.LastError != "" {
		t.Errorf("LastError: want empty after a successful poll, got %q", h.LastError)
	}
}

// A failing poll must NOT advance LastSuccess — that is precisely the bug the
// old liveness proxy had: it treated "still running" as "still working".
func TestD164_PollHealth_FailureDoesNotAdvanceLastSuccess(t *testing.T) {
	p := &Poller{cfg: Config{PollInterval: 5 * time.Second}}
	p.recordPoll(nil)
	success := p.PollHealth().LastSuccess

	time.Sleep(2 * time.Millisecond)
	p.recordPoll(errors.New("list applications: dial tcp: lookup mock-ams: server misbehaving"))

	h := p.PollHealth()
	if !h.LastSuccess.Equal(success) {
		t.Errorf("LastSuccess moved on a FAILED poll: was %v, now %v", success, h.LastSuccess)
	}
	if h.LastError == "" {
		t.Error("LastError: want the poll error recorded, got empty")
	}
}

// Recovery clears the error so /healthz does not keep naming a stale cause.
func TestD164_PollHealth_SuccessClearsLastError(t *testing.T) {
	p := &Poller{cfg: Config{PollInterval: 5 * time.Second}}
	p.recordPoll(errors.New("connection refused"))
	if p.PollHealth().LastError == "" {
		t.Fatal("precondition: expected LastError to be set")
	}

	p.recordPoll(nil)

	if got := p.PollHealth().LastError; got != "" {
		t.Errorf("LastError: want cleared after recovery, got %q", got)
	}
}

// StaleAfter is 3 missed intervals, floored so the 5 s default cadence does not
// flap the health signal on one slow response.
func TestD164_StaleAfter_ThreeIntervalsWithFloor(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{"default 5s cadence is floored", 5 * time.Second, 30 * time.Second},
		{"exactly at the floor", 10 * time.Second, 30 * time.Second},
		{"slow cadence scales to 3x", 60 * time.Second, 180 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Poller{cfg: Config{PollInterval: tc.interval}}
			if got := p.PollHealth().StaleAfter; got != tc.want {
				t.Errorf("StaleAfter for a %v interval: want %v, got %v", tc.interval, tc.want, got)
			}
		})
	}
}

// PollHealth is read by the API handler goroutine while Run's goroutine writes.
// Run with -race (CI does) — this fails loudly if the lock is dropped.
func TestD164_PollHealth_ConcurrentReadWrite(t *testing.T) {
	p := &Poller{cfg: Config{PollInterval: 5 * time.Second}}
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			if i%2 == 0 {
				p.recordPoll(nil)
			} else {
				p.recordPoll(errors.New("boom"))
			}
		}
	}()
	for i := 0; i < 500; i++ {
		_ = p.PollHealth()
	}
	<-done
}
