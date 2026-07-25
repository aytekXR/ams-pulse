# WO-B1 (S105) — AMS integration docs: Kafka truth, auth-mode mapping, B7 rewrite, version claim

Companion doc work for code changes being made in parallel (WO-A1 kafka, WO-A2 webhook,
WO-A3 amsclient). Write docs describing the POST-fix state below as fact.

**Code changes you document (being implemented in parallel):**
- Kafka: default topics now `ams-instance-stats,ams-webrtc-stats` (official AMS topics, verified
  from AMS `StatsCollector.java`); new env `PULSE_KAFKA_TOPICS` (comma-separated override);
  normalizer routes by topic and parses the official nested shapes (`instanceId`, nested
  `cpuUsage`/`systemMemoryInfo`/`fileSystemInfo`); fixture-tested against source-derived
  messages. STILL not validated against a live AMS Kafka producer (AV-15) — the feature stays
  **experimental/preview** until that runs.
- Webhook: parser now accepts the official AMS payload (`id` as stream id fallback, vodReady
  `vodId`+`duration`, form-urlencoded bodies). AMS webhooks remain UNSIGNED in AMS 3.0.3 —
  the "do not point AMS directly at Pulse's HMAC listener" guidance in §4.5 stays; the supported
  pattern is a signing proxy.
- amsclient: static-token mode now sends `Authorization: Bearer <jwt>` AND `ProxyAuthorization:
  <jwt>` on every GET, so management-scope endpoints work with `server.jwtServerControlEnabled`
  tokens; app-scope works with app-level `jwtControlEnabled` tokens.

## Scope — you may ONLY edit these files
- `docs/AMS-INTEGRATION.md`
- `docs/kafka-integration.md`
- `docs/compatibility.md`
- `docs/admin-guide.md`
- `docs/known-limitations.md`

No git commands. Other agents edit other files concurrently — expected. Keep this repo's
honest-limitations tone; never claim live validation that hasn't happened.

## Work items

1. **`docs/AMS-INTEGRATION.md`:**
   - §2 (auth): add a short table mapping each Pulse auth mode to the AMS setting it matches:
     cookie-session (default, live-validated) ↔ AMS default panel auth;
     `PULSE_AMS_AUTH_TOKEN` ↔ app-scope `jwtControlEnabled` (Authorization: Bearer) AND
     management-scope `server.jwtServerControlEnabled` (ProxyAuthorization) — Pulse sends both
     headers automatically as of this change.
   - §2.4: replace the "v2.8 and above are supported" claim with the standardized claim:
     **"Validated live on AMS 3.0.3 Enterprise (current release); best-effort compatibility with
     AMS 2.10+ via version-tolerance tests (mock profiles)."** (Issue 12 — 2.8 appears in no
     test matrix anywhere in the repo.)
   - §3.7 (Kafka): document the new defaults + `PULSE_KAFKA_TOPICS`, the official topic names and
     the red5.properties `server.kafka_brokers` enable step, with an explicit
     EXPERIMENTAL/PREVIEW banner pending AV-15.
   - §7 B7 (~lines 755-763): the "Concrete operator example" currently instructs entering a
     "Webhook secret" and "Header name: X-Ams-Signature" in the AMS Management Console — fields
     that DO NOT EXIST in AMS 3.0.3 (contradicts §4.5 of the same file). Rewrite B7 to the real
     pattern: per-source secret is configured on the PULSE side; a small signing proxy between
     AMS and Pulse computes the HMAC over the raw body and adds X-Ams-Signature; AMS itself only
     gets `listenerHookURL` pointed at the proxy. Note AMS retries on non-200
     (webhookRetryCount/webhookRetryDelay), which is why Pulse's listener answers 200 even on
     parse failures. Also note the parser now accepts official-payload field names and
     form-urlencoded bodies (proxies can forward verbatim).

2. **`docs/kafka-integration.md`:** correct the topic story (the doc currently concludes Pulse's
   `ams-server-events` is authoritative — it was wrong; AMS publishes `ams-instance-stats` /
   `ams-webrtc-stats`); correct the config file name (**`conf/red5.properties`**, not
   application.properties); document `PULSE_KAFKA_TOPICS`; describe the official nested message
   shapes; update the AV-15 status section: consumer now aligned to the official shapes and
   fixture-tested + broker-integration-tested, live AMS-producer validation still pending.

3. **`docs/compatibility.md`:** align any AMS-version claim to the standardized claim above; in
   the Kafka row/section, note the consumer is now aligned to official topics/shapes
   (source-derived fixtures) but remains not live-validated (AV-15).

4. **`docs/admin-guide.md`:** add `PULSE_KAFKA_TOPICS` to the env-var reference (comma-separated,
   default `ams-instance-stats,ams-webrtc-stats`), next to PULSE_KAFKA_BROKERS/GROUP_ID; fix any
   stale topic mention; in the AMS-connection section, note token mode sends both auth headers
   (one line).

5. **`docs/known-limitations.md`:** stamp the header **v0.4.1** (lines ~3 and ~8 still say
   v0.4.0); update LIM-01 and LIM-19: the consumer-side topic/shape mismatch is FIXED
   (aligned to AMS source, fixture-tested); what remains open is live validation against a real
   AMS Kafka producer (AV-15) — keep the limitation entry but narrow it honestly.

## Definition of done
Every claim you write matches the WO-A1/A2/A3 changes described above; no doc claims live Kafka
validation. Return: files changed, sections touched, any inconsistency you found but could not
fix inside your scope (report, don't fix).
