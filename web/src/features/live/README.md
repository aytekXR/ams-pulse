# Feature: Live Ops Dashboard (F1) — Phase 1

Live view of concurrent viewers (total / per app / per stream), active publishers,
node health, protocol mix. WebSocket-pushed, refreshing every few seconds.

Acceptance (PRD F1): new stream visible ≤10s after publish; viewer counts within
±2% of AMS `broadcast-statistics`; loads <2s with 500 concurrent streams (virtualized
stream list, no per-row polling).

Owner: FE-01. Data: `/live/overview`, `/live/streams`, `/live/ws`.
