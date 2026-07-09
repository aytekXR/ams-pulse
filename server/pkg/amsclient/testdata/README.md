# amsclient/testdata — fixture provenance

Fixtures in this directory fall into two categories:

## Real AMS 3.0.3 Enterprise captures

Captured 2026-06-21 from a live AMS 3.0.3 Enterprise server. All captures were
sanitized before commit: real IPs replaced with 203.0.113.10 (TEST-NET-3 per RFC
5737), real stream URLs redacted, user credentials removed. No personally-identifying
information is present.

| File | Source endpoint | Notes |
|---|---|---|
| `broadcasts_real_liveapp.json` | GET /LiveApp/rest/v2/broadcasts/list/0/200 | 16 entries (15 finished, 1 broadcasting test123); bitrate=622312 bps for test123 |
| `broadcasts_real_test123_v303.json` | GET /LiveApp/rest/v2/broadcasts/list/0/200 (single-stream) | Captured at a different poll moment; bitrate=624016 bps — distinct non-round value |
| `broadcast_statistics_real.json` | GET /LiveApp/rest/v2/broadcasts/test123/broadcast-statistics | totalRTMPWatchersCount=-1 (real AMS quirk for inactive protocol) |
| `system_status.json` | GET /rest/v2/system-status | AMS 3.x real shape: ONLY {osName, osArch, javaVersion, processorCount} — NO cpu/mem/disk |
| `version.json` | GET /rest/v2/version | versionName=3.0.3, versionType=Enterprise Edition |
| `webrtc_client_stats_real_empty.json` | GET /LiveApp/rest/v2/broadcasts/test123/webrtc-client-stats/0/100 | Empty array `[]` — no WebRTC viewers at capture time |

**Duplication caveat**: `broadcasts_real_liveapp.json` and `broadcasts_real_test123_v303.json`
are sanitized copies of `agents/handoffs/real-ams-captures/LiveApp_list.json` and a single-stream
slice thereof. The `system_status.json`, `version.json`, and `webrtc_client_stats_real_empty.json`
are sanitized copies of the corresponding files in `agents/handoffs/real-ams-captures/`. If the
real AMS server is re-captured, update BOTH the `agents/handoffs/real-ams-captures/` originals AND
the sanitized copies here.

## Synthetic fixtures

Synthetic fixtures (crafted to test specific conditions or boundary values) are the remaining files:

| File | Purpose |
|---|---|
| `applications.json` | 4-app minimal list (synthetic; real 16-app capture is in agents/handoffs/) |
| `applications_object_form.json` | Applications response in object form (edge case) |
| `broadcasts_empty.json` | Empty broadcast list |
| `broadcasts_extra_fields.json` | Unknown JSON fields — exercises tolerant decode |
| `broadcasts_mixed_status.json` | Mix of broadcasting/finished/created entries |
| `broadcasts_page_full.json` | Full-page pagination boundary |
| `broadcasts_v2_10.json` | AMS v2.10 broadcast shape |
| `broadcasts_v2_14.json` | AMS v2.14 broadcast shape |
| `broadcasts_v3.json` | AMS v3 broadcast shape (pre-3.0.3) |
| `cluster_nodes.json` | Cluster node list |
| `webrtc_client_stats.json` | Synthetic WebRTC peer stats (2 peers; covers dual-track averaging) |
