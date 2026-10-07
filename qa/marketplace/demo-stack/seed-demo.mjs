#!/usr/bin/env node
/**
 * Seed the marketplace demo stack (qa/marketplace/demo-stack) with demo data that
 * flows through Pulse's REAL interfaces:
 *
 *   - streams     → the AMS simulator's control API (qa/mock-ams); Pulse's collector
 *                   then polls them over the AMS REST v2 wire format, exactly as it
 *                   polls a real Ant Media Server;
 *   - config      → Pulse's public REST API (/api/v1/alerts/*, /admin/tokens, /probes),
 *                   the same calls the web UI makes;
 *   - viewers     → synthetic player sessions POSTed to the public beacon ingest
 *                   endpoint (/ingest/beacon) in the beacon SDK's wire format.
 *
 * Nothing is written to ClickHouse or the meta store directly.
 *
 * Usage (Node 20+, no dependencies):
 *   node seed-demo.mjs setup      streams + channels + rules + ingest token + probes
 *   node seed-demo.mjs history    replay ~24 h of beacon sessions (ends at the current hour)
 *   node seed-demo.mjs live       run live traffic until killed (viewer drift + heartbeats)
 *   node seed-demo.mjs degrade    start the incident: ingest bitrate drop + QoE degradation
 *   node seed-demo.mjs recover    end the incident (alerts resolve on the next evaluations)
 *   node seed-demo.mjs truth      one known session → print what the API reports (accuracy check)
 *
 * Environment: DEMO_PULSE_PORT (18190), DEMO_MOCK_PORT (19190).
 */

import { readFileSync, writeFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { randomUUID, randomBytes } from "node:crypto";

const HERE = dirname(fileURLToPath(import.meta.url));
const STATE = join(HERE, ".state");
mkdirSync(STATE, { recursive: true });

const PULSE = `http://127.0.0.1:${process.env.DEMO_PULSE_PORT || 18190}`;
const API = `${PULSE}/api/v1`;
const MOCK = `http://127.0.0.1:${process.env.DEMO_MOCK_PORT || 19190}`;
const APP = "LiveApp";

// ── Demo catalogue ────────────────────────────────────────────────────────────
// Modest, plausible numbers for a single AMS node. `viewers` is the simulator's
// base figure: it reports `viewers` WebRTC + `viewers/3` HLS viewers per stream.
const STREAMS = [
  { id: "main-stage", viewers: 240, kbps: 4500, weight: 0.24 },
  { id: "keynote-hall", viewers: 165, kbps: 6000, weight: 0.17 },
  { id: "esports-cam-1", viewers: 130, kbps: 6000, weight: 0.13 },
  { id: "studio-a", viewers: 110, kbps: 3000, weight: 0.12 },
  { id: "studio-b", viewers: 92, kbps: 3500, weight: 0.11 },
  { id: "town-hall", viewers: 64, kbps: 2500, weight: 0.12 },
  { id: "lobby-cam", viewers: 9, kbps: 1800, weight: 0.06 },
  { id: "backstage-cam", viewers: 6, kbps: 2000, weight: 0.05 },
];
const INGEST_INCIDENT_STREAM = "studio-b"; // encoder starves → bitrate floor alert
const QOE_INCIDENT_STREAM = "keynote-hall"; // viewers rebuffer → QoE alert

// Realistic User-Agent mix → the device/OS/browser breakdowns.
const CLIENTS = [
  { w: 0.28, kind: "hls.js", ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36" },
  { w: 0.11, kind: "hls.js", ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15" },
  { w: 0.19, kind: "native", ua: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1" },
  { w: 0.20, kind: "ams-webrtc", ua: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36" },
  { w: 0.07, kind: "video.js", ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0" },
  { w: 0.06, kind: "ams-webrtc", ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0" },
  { w: 0.05, kind: "hls.js", ua: "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1" },
  { w: 0.04, kind: "native", ua: "Mozilla/5.0 (SMART-TV; LINUX; Tizen 8.0) AppleWebKit/537.36 (KHTML, like Gecko) 108.0.5359.1/8.0 TV Safari/537.36" },
];

// ── Small helpers ─────────────────────────────────────────────────────────────
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const rand = (a, b) => a + Math.random() * (b - a);
const pick = (items) => {
  let r = Math.random() * items.reduce((s, i) => s + (i.w ?? i.weight), 0);
  for (const i of items) {
    r -= i.w ?? i.weight;
    if (r <= 0) return i;
  }
  return items[items.length - 1];
};
// Log-normal sample with the given median and log-space sigma.
const lognormal = (median, sigma) => {
  const u = 1 - Math.random();
  const v = Math.random();
  const z = Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v);
  return median * Math.exp(sigma * z);
};
const readState = (name) => {
  const p = join(STATE, name);
  return existsSync(p) ? readFileSync(p, "utf8").trim() : "";
};
const writeState = (name, value) => writeFileSync(join(STATE, name), `${value}\n`, { mode: 0o600 });

function adminToken() {
  const t = readState("admin-token");
  if (!t) throw new Error(`no admin token in ${STATE}/admin-token — run up.sh first`);
  return t;
}

async function api(method, path, body) {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: { Authorization: `Bearer ${adminToken()}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  if (!res.ok) throw new Error(`${method} ${path} → HTTP ${res.status}: ${text.slice(0, 300)}`);
  return json;
}

async function mock(path, body) {
  const res = await fetch(`${MOCK}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`mock ${path} → HTTP ${res.status}`);
}

async function waitHealthy() {
  for (let i = 0; i < 100; i++) {
    try {
      const h = await (await fetch(`${PULSE}/healthz`)).json();
      const m = await fetch(`${MOCK}/healthz`);
      if (h && m.ok) return h;
    } catch {
      /* not up yet */
    }
    await sleep(3000);
  }
  throw new Error("Pulse or the AMS simulator never became reachable");
}

// ── Beacon traffic ────────────────────────────────────────────────────────────
let beaconToken = "";
async function postBeacon(batch, ua) {
  for (let attempt = 0; attempt < 5; attempt++) {
    const res = await fetch(`${PULSE}/ingest/beacon`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Pulse-Ingest-Token": beaconToken, "User-Agent": ua },
      body: JSON.stringify(batch),
    });
    if (res.status === 429) {
      await sleep(500 * (attempt + 1));
      continue;
    }
    if (!res.ok) throw new Error(`beacon → HTTP ${res.status}: ${(await res.text()).slice(0, 200)}`);
    return;
  }
  throw new Error("beacon: rate limited 5 times in a row");
}

/** Build the full event list for one viewer session (beacon SDK wire format). */
function sessionEvents({ stream, startMs, watchMs, degraded }) {
  const events = [];
  const startup = Math.round(Math.min(9000, lognormal(degraded ? 2400 : 820, degraded ? 0.45 : 0.5)));
  let bitrate = Math.round(stream.kbps * rand(0.55, 0.95));
  events.push({ type: "session_start", ts: startMs, data: { autoplay: Math.random() < 0.6 } });
  events.push({ type: "startup_complete", ts: startMs + startup, data: { startup_ms: startup, bitrate_kbps: bitrate } });
  let t = startMs + startup;
  let watched = 0;
  const HEARTBEAT = 30_000;
  while (watched < watchMs) {
    const step = Math.min(HEARTBEAT, watchMs - watched);
    // Rebuffering inside this interval.
    const pStall = degraded ? 0.85 : 0.06;
    if (Math.random() < pStall) {
      const dur = Math.round(degraded ? rand(1800, 4200) : rand(250, 1600));
      const at = t + Math.round(rand(0.2, 0.8) * step);
      events.push({ type: "rebuffer_start", ts: at });
      events.push({ type: "rebuffer_end", ts: at + dur, data: { duration_ms: dur } });
    }
    // Occasional ABR switch.
    if (Math.random() < 0.12) {
      const to = Math.round(Math.max(400, Math.min(stream.kbps, bitrate * rand(0.6, 1.4))));
      events.push({ type: "bitrate_change", ts: t + Math.round(step / 2), data: { from_kbps: bitrate, to_kbps: to } });
      bitrate = to;
    }
    t += step;
    watched += step;
    events.push({ type: "heartbeat", ts: t, data: { watch_ms: watched, bitrate_kbps: bitrate, buffer_ms: Math.round(rand(4000, 18000)) } });
  }
  if (Math.random() < (degraded ? 0.05 : 0.006)) {
    events.push({ type: "error", ts: t, data: { code: "MEDIA_ERR_NETWORK", fatal: false, message: "segment request timed out" } });
  }
  events.push({ type: "session_end", ts: t + 500 });
  return events;
}

async function sendSession(opts) {
  const client = opts.client ?? pick(CLIENTS);
  const events = sessionEvents(opts);
  const sid = opts.sessionId ?? randomUUID();
  // The SDK flushes in batches; stay well under the 100-events-per-batch cap.
  for (let i = 0; i < events.length; i += 80) {
    await postBeacon(
      {
        version: 1,
        session_id: sid,
        stream_id: opts.stream.id,
        app: APP,
        player: { kind: client.kind, sdk_version: "0.4.5" },
        events: events.slice(i, i + 80),
      },
      client.ua,
    );
  }
  return sid;
}

// ── Commands ──────────────────────────────────────────────────────────────────
async function setup() {
  const health = await waitHealthy();
  console.log("pulse /healthz:", JSON.stringify(health));

  for (const s of STREAMS) {
    await mock("/control/publish", { stream_id: s.id, viewers: s.viewers, bitrate: s.kbps * 1000 });
  }
  console.log(`published ${STREAMS.length} streams on the simulator (app ${APP})`);

  // Notification channels. E-mail goes to the local Mailpit sink; the webhook to the
  // local recorder. SMTP server fields are API-only (the UI form sets the recipient).
  const channels = (await api("GET", "/alerts/channels"))?.items ?? [];
  const byName = (n) => channels.find((c) => c.name === n);
  const email =
    byName("NOC on-call (email)") ??
    (await api("POST", "/alerts/channels", {
      type: "email",
      name: "NOC on-call (email)",
      config: { email_to: "noc@example.com", smtp_addr: "mailpit:1025", from: "pulse-alerts@example.com", starttls: false },
    }));
  const webhook =
    byName("Incident webhook") ??
    (await api("POST", "/alerts/channels", {
      type: "webhook",
      name: "Incident webhook",
      config: { webhook_url: "http://webhook-sink:8080/pulse-alerts", webhook_secret: randomBytes(24).toString("hex") },
    }));
  console.log("channels:", email.id, webhook.id);

  const rules = (await api("GET", "/alerts/rules"))?.items ?? [];
  const wanted = [
    {
      name: "Viewer QoE degraded — rebuffer ratio above 5%",
      metric: "rebuffer_ratio", operator: "gt", threshold: 0.05, window_s: 60,
      severity: "critical", cooldown_s: 600, channel_ids: [email.id, webhook.id],
    },
    {
      name: "Ingest bitrate below 1,500 kbps",
      metric: "ingest_bitrate_kbps", operator: "lt", threshold: 1500, window_s: 60,
      severity: "warning", cooldown_s: 600, channel_ids: [email.id, webhook.id],
    },
    {
      name: "Stream went offline",
      metric: "stream_offline", operator: "eq", threshold: 1, window_s: 30,
      severity: "critical", cooldown_s: 300, channel_ids: [email.id],
    },
    {
      name: "Audience drop — fewer than 5 viewers on main-stage",
      metric: "viewer_count", operator: "lt", threshold: 5, window_s: 300,
      severity: "info", cooldown_s: 900, scope: { app: APP, stream_id: "main-stage" }, channel_ids: [email.id],
    },
  ];
  for (const r of wanted) {
    if (!rules.find((x) => x.name === r.name)) await api("POST", "/alerts/rules", r);
  }
  console.log(`alert rules: ${wanted.length} demo rules present (+ the shipped default pack, muted)`);

  // Ingest token for the synthetic players.
  if (!readState("ingest-token")) {
    const tok = await api("POST", "/admin/tokens", { kind: "ingest", name: "demo-player-sdk" });
    writeState("ingest-token", tok.token);
  }
  console.log("ingest token: saved to .state/ingest-token");

  // Synthetic probes against the simulator's DASH and RTMP endpoints.
  const probes = (await api("GET", "/probes"))?.items ?? [];
  const wantedProbes = [
    { name: "main-stage — DASH manifest + segment", url: `http://mock-ams:9090/${APP}/streams/main-stage.mpd`, protocol: "dash", interval_s: 30 },
    { name: "main-stage — RTMP handshake", url: `rtmp://mock-ams:1935/${APP}/main-stage`, protocol: "rtmp", interval_s: 30 },
  ];
  for (const p of wantedProbes) {
    if (!probes.find((x) => x.name === p.name)) await api("POST", "/probes", p);
  }
  console.log("probes: DASH + RTMP probes present");
}

async function history() {
  beaconToken = readState("ingest-token");
  if (!beaconToken) throw new Error("run `setup` first");
  const now = Date.now();
  const hourStart = now - (now % 3_600_000);
  const from = hourStart - 24 * 3_600_000;
  let sent = 0;
  for (let h = from; h < hourStart; h += 3_600_000) {
    const hourOfDay = new Date(h).getUTCHours();
    // Diurnal audience curve: quiet overnight, peak in the evening (UTC).
    const level = 0.25 + 0.75 * Math.max(0, Math.sin(((hourOfDay - 7) / 24) * 2 * Math.PI)) ** 1.5;
    const sessions = Math.round(18 + 70 * level * rand(0.85, 1.15));
    const jobs = [];
    for (let i = 0; i < sessions; i++) {
      const stream = pick(STREAMS);
      const startMs = h + Math.round(rand(0, 3_300_000));
      const watchMs = Math.round(Math.min(40 * 60_000, Math.max(45_000, lognormal(8 * 60_000, 0.8))));
      // Leave the session inside its own hour so the QoE window stays clean.
      jobs.push(sendSession({ stream, startMs, watchMs: Math.min(watchMs, h + 3_590_000 - startMs), degraded: false }));
      if (jobs.length >= 12) {
        await Promise.all(jobs.splice(0));
      }
    }
    await Promise.all(jobs);
    sent += sessions;
    process.stdout.write(`\rreplayed ${sent} sessions up to ${new Date(h + 3_600_000).toISOString()}`);
  }
  console.log(`\nhistory: ${sent} sessions replayed through /ingest/beacon`);
}

function scenario() {
  try {
    return JSON.parse(readState("scenario.json") || "{}");
  } catch {
    return {};
  }
}

async function live() {
  beaconToken = readState("ingest-token");
  if (!beaconToken) throw new Error("run `setup` first");
  const viewers = Object.fromEntries(STREAMS.map((s) => [s.id, s.viewers]));
  console.log("live traffic running — Ctrl-C to stop");
  for (let tick = 0; ; tick++) {
    const sc = scenario();
    // Viewer drift on the simulator (bounded random walk around the base figure).
    for (const s of STREAMS) {
      const drift = Math.round(viewers[s.id] * rand(-0.04, 0.045));
      viewers[s.id] = Math.max(1, Math.round(Math.min(s.viewers * 1.35, Math.max(s.viewers * 0.7, viewers[s.id] + drift))));
      await mock("/control/set_viewers", { stream_id: s.id, viewers: viewers[s.id] });
      const kbps = sc.ingestDegraded && s.id === INGEST_INCIDENT_STREAM ? rand(780, 920) : s.kbps * rand(0.97, 1.03);
      await mock("/control/set_bitrate", { stream_id: s.id, bitrate: Math.round(kbps * 1000) });
    }
    // Short instrumented sessions finishing every tick keep QoE rollups current.
    const jobs = [];
    for (let i = 0; i < 6; i++) {
      const stream = pick(STREAMS);
      const degraded = Boolean(sc.qoeDegraded) && stream.id === QOE_INCIDENT_STREAM;
      jobs.push(sendSession({ stream, startMs: Date.now() - 95_000, watchMs: 90_000, degraded }));
    }
    if (sc.qoeDegraded) {
      // The incident stream gets its own steady flow of struggling players.
      const stream = STREAMS.find((s) => s.id === QOE_INCIDENT_STREAM);
      for (let i = 0; i < 3; i++) jobs.push(sendSession({ stream, startMs: Date.now() - 95_000, watchMs: 90_000, degraded: true }));
    }
    await Promise.all(jobs);
    if (tick % 6 === 0) console.log(new Date().toISOString(), "tick", tick, JSON.stringify(sc));
    await sleep(10_000);
  }
}

async function degrade() {
  writeState("scenario.json", JSON.stringify({ ingestDegraded: true, qoeDegraded: true, since: new Date().toISOString() }));
  await mock("/control/set_bitrate", { stream_id: INGEST_INCIDENT_STREAM, bitrate: 850_000 });
  console.log(`incident started: ${INGEST_INCIDENT_STREAM} ingest ≈ 850 kbps; ${QOE_INCIDENT_STREAM} viewers rebuffering`);
}

async function recover() {
  writeState("scenario.json", JSON.stringify({ ingestDegraded: false, qoeDegraded: false, since: new Date().toISOString() }));
  const s = STREAMS.find((x) => x.id === INGEST_INCIDENT_STREAM);
  await mock("/control/set_bitrate", { stream_id: s.id, bitrate: s.kbps * 1000 });
  console.log("incident cleared (rebuffer ratio stays elevated until the hourly QoE window rolls over)");
}

/**
 * Accuracy check: one session with a KNOWN ground truth, on a stream nobody else
 * uses, read back through the public API. Prints expected vs reported numbers.
 */
async function truth() {
  beaconToken = readState("ingest-token");
  if (!beaconToken) throw new Error("run `setup` first");
  const stream = { id: `truth-${Date.now()}`, kbps: 3000 };
  await mock("/control/publish", { stream_id: stream.id, viewers: 1, bitrate: 3_000_000 });
  const sid = randomUUID();
  const t0 = Date.now() - 11 * 60_000;
  const events = [
    { type: "session_start", ts: t0 },
    { type: "startup_complete", ts: t0 + 1000, data: { startup_ms: 1000, bitrate_kbps: 2500 } },
  ];
  // 10 minutes of playback: 20 heartbeats, cumulative watch_ms (as the SDK sends it),
  // exactly one 6 000 ms stall → true rebuffer ratio = 6000 / 600000 = 1.0 %.
  for (let i = 1; i <= 20; i++) {
    const ts = t0 + 1000 + i * 30_000;
    if (i === 10) {
      events.push({ type: "rebuffer_start", ts: ts - 10_000 });
      events.push({ type: "rebuffer_end", ts: ts - 4_000, data: { duration_ms: 6000 } });
    }
    events.push({ type: "heartbeat", ts, data: { watch_ms: i * 30_000, bitrate_kbps: 2500 } });
  }
  events.push({ type: "session_end", ts: t0 + 1000 + 20 * 30_000 + 500 });
  await postBeacon({ version: 1, session_id: sid, stream_id: stream.id, app: APP, player: { kind: "hls.js" }, events }, CLIENTS[0].ua);
  console.log(`truth session ${sid} on stream ${stream.id}: 1 view, 600 s watched, 6 000 ms stalled (1.0 %)`);
  await sleep(20_000); // flush interval + MV
  const from = t0 - 3_600_000;
  const to = Date.now() + 60_000;
  const qoe = await api("GET", `/qoe/summary?stream=${encodeURIComponent(stream.id)}&from=${from}&to=${to}`);
  const aud = await api("GET", `/analytics/audience?stream=${encodeURIComponent(stream.id)}&from=${from}&to=${to}`);
  console.log("API /qoe/summary totals:", JSON.stringify(qoe?.totals));
  console.log("API /analytics/audience totals:", JSON.stringify(aud?.totals));
  await mock("/control/unpublish", { stream_id: stream.id });
}

const cmd = process.argv[2] || "setup";
const commands = { setup, history, live, degrade, recover, truth };
if (!commands[cmd]) {
  console.error(`unknown command ${cmd}; one of: ${Object.keys(commands).join(", ")}`);
  process.exit(2);
}
commands[cmd]().catch((err) => {
  console.error(`seed-demo ${cmd}: ${err.message}`);
  process.exit(1);
});
