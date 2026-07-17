# SESSION-78 — planned at S77 close (D-139)

> Written by SESSION-77 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S73-AUDIT-FINDINGS.md`** (8-finding ledger; 6 shipped, ALL HIGH done).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-79

Re-verify each finding against the code before building — take the verified CORE. Do NOT stop after one session — at
close, update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete
or a human/operator is genuinely required. **Ultracode is on** — [7] is a SECURITY + auth-protocol surface →
adversarial-review is mandatory. **Workflow-script gotcha:** no backticks in workflow prompt prose. `gofmt -l` before
every push. **Web gotchas (D-139):** `vi.hoisted` for vi.mock top-level refs; `ApiError(status,{code,message})`; run
full `npm test` for the coverage gate; the binary embeds `web/dist` (web change → prod roll-forward).

## Goal — [7] MEDIUM (security, operator-flagged): admin token in the Live-dashboard WS URL → proxy access logs

`S73-AUDIT-FINDINGS.md`: 2 MEDIUM remain ([5]/[7]). Lead = [7] (security-relevant; the operator has been told it's
pending and to treat Caddy/docker logs as containing a live admin credential — closing it retires that note):

### [7] MEDIUM — admin bearer token passed as `?token=` in the WebSocket upgrade URL → recorded in Caddy access logs
- **loc:** `web/src/api/client.ts:570` (LiveSocket.connect builds `/live/ws?token=<bearer>`) + `deploy/config/Caddyfile.prod:82-86`
  (unfiltered `format json` logs full URIs). The Go `loggingMiddleware` (server.go:799) only logs `r.URL.Path`, but Caddy
  (and any proxy/SIEM) logs the full URL incl. the query string.
- **Re-verify at open:** confirm the WS handler (server side — find the `/live/ws` upgrade handler; grep `?token=` /
  `q.Get("token")` / websocket upgrade in server.go) still authenticates via the URL `token` param, and that
  `LiveSocket.connect` still appends `?token=`.
- **Design options — choose the verified CORE at open (do NOT touch the do-not-commit `Caddyfile.prod`):**
  - **(a) Short-lived single-use ticket (RECOMMENDED — most secure):** `POST /auth/ws-ticket` (authed via the normal
    bearer) → `{ ticket, expires_in: 30 }`. Server keeps a small in-memory ticket store (map ticket→expiry, single-use;
    a mutex + lazy expiry sweep; NOT persisted). The WS upgrade accepts `?ticket=` instead of `?token=`, validates +
    **consumes** it (delete on first use). Web: `LiveSocket.connect` first `POST /auth/ws-ticket`, then connects with
    `?ticket=`. So a leaked ticket in logs is useless (single-use + 30 s). OpenAPI: new endpoint + response schema →
    regen `web/src/lib/api/schema.d.ts` (+ redocly lint) + a param-conformance registry entry if a new query param.
  - **(b) Token as a WebSocket subprotocol (least change):** browser `new WebSocket(url, ["pulse.v1", token])` → sent as
    the `Sec-WebSocket-Protocol` HEADER (not the URL → not in Caddy's URL log). Server reads the header, validates the
    token, and MUST echo the selected subprotocol (`Sec-WebSocket-Protocol: pulse.v1`) or the handshake fails. Caveat:
    the long-lived token still travels (in a header, not URL) — better than the URL but not single-use; verify the token
    charset is a valid subprotocol token (`plt_<hex>` is fine). No new endpoint/state.
  - (c) first-WS-frame auth — more protocol change; least preferred.
  - **Recommendation:** (a) ticket if the in-memory store is acceptable (it is — WS auth is ephemeral); it fully closes
    the exposure. (b) is a fine minimal alternative if a new endpoint is deemed heavy. Weigh at open; take the CORE.
- **Test:** ticket path — a ticket validates once then is rejected on reuse + after expiry (401 on the upgrade); the web
  client fetches a ticket before connecting. Mutation: skip the consume/expiry → reuse succeeds → RED. For (b): the
  server reads+validates the subprotocol header and rejects a bad/absent one.
- **Adversarial review MANDATORY** (auth surface): ticket forgeability/replay/expiry races; does the WS upgrade still
  reject an unauthenticated/expired/reused credential; any downgrade (does `?token=` still work as a fallback and thus
  not actually close the exposure? — it must NOT).

## Later cluster (the last S73 finding)
- **[5] MEDIUM** `QoEForStream` cross-tenant QoE — ⚠ WIDER: `AlertScope`/`AlertRuleRow`/`LiveStream` have NO Tenant
  field; thread Tenant through the aggregator → those structs, THEN the `QoEReader` signature. Multi-tenant-only impact
  (primary single-tenant model unaffected — low real-world severity). Scope carefully; may split into a
  "tenant-in-live-pipeline" prerequisite. **After [5], the S73 audit is COMPLETE** (8/8 dispositioned) — flip ROADMAP
  §2.32 to ✅ COMPLETE and re-survey ROADMAP §2 for the next arc (the §2.7 CI-promotion gate unlocks ≥ 2026-07-23).

## Pipeline (the S62→S77 loop; server+web variant)
1. **Verify-at-open:** git state clean (only Caddyfile dirty). Record **D-140 IN PROGRESS** in `decisions.md`. Branch
   `s78-d140`. **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Re-verify vs code** (`mcp__codegraph__codegraph_explore` for the WS handler + LiveSocket); choose the design CORE.
3. **Fix → test.** Go: `/tmp/pulsemut` mutation. Web: in-place mutate + `npm test`. If OpenAPI changes, regen schema.d.ts.
4. **Full Go suite** (25 pkgs) + web (`npm test` + `npm run build` + typecheck + lint).
5. **Adversarial review (MANDATORY — auth surface).**
6. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (server + web source): `config -q` → tag `pulse-prod-pulse:pre-d140` → backup rc0 → STAMPED
   build → assert stamp ≠ dev → `up -d pulse` → 5-check smoke (+ `GET /` SPA 200; consider a WS-connect smoke).
8. **Close docs:** D-140 SHIPPED, CHANGELOG, ledger [7] ✅ DONE, ROADMAP §2.32 count, RESUME rotation,
   `operator-expected.md` (**retire the WS-token log-exposure heads-up** once shipped), SESSION-78 CLOSED, SESSION-79
   written (lead: [5], the last finding). Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker** (25 pkgs); mutation copy `/tmp/pulsemut` (not `/mut`). **CodeQL** flags `InsecureSkipVerify` in
  PRODUCTION Go only. Any OpenAPI change → regen `web/src/lib/api/schema.d.ts` (committed; web CI drift check) + a
  param-conformance registry entry for any new documented param (the gate fails otherwise — S75).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-91-g7e272f6**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
