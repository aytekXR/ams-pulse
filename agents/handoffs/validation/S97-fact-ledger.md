<!-- S97/D-161 evidence record: the Phase-0 verified fact ledger that grounds the marketplace docs pack. Produced by 8 parallel verification agents against the code on 2026-07-22. -->
# S97 Phase-0 fact ledger (verified 2026-07-22, D-161)

Citation base for all marketplace docs. Each item: verdict vs the pre-audit claim, verified fact, code evidence.

## api.counts  [CORRECTED]
- CLAIM CHECKED: contracts/openapi/pulse-api.yaml has 32 paths, 46 operations, 66 component schemas.
- FACT: The file has 42 paths, 59 operations, and 73 component schemas. Exact counts verified by parsing with PyYAML: paths keys = 42, HTTP method entries = 59, components.schemas keys = 73.
- EVIDENCE: /home/aytek/repo/ams-pulse/contracts/openapi/pulse-api.yaml — all three numbers were machine-counted; claim is wrong on all three figures.
- DOC IMPACT: Marketplace docs must state the API surface as 42 paths, 59 operations, and 73 component schemas.

## api.groups  [CONFIRMED]
- CLAIM CHECKED: Path groups by tag are live, analytics, qoe, alerts, reports, admin, fleet, anomalies, probes, operational, ingest, auth.
- FACT: All 12 claimed tags exist. Operation counts per tag: live=3, analytics=3, qoe=2, alerts=10, reports=6, admin=20, fleet=1, anomalies=1, probes=5, operational=2, ingest=1, auth=5. Total=59.
- EVIDENCE: /home/aytek/repo/ams-pulse/contracts/openapi/pulse-api.yaml lines 38-62 (tag declarations) and machine-counted per-tag operation breakdown confirms all 12 tags are present.
- DOC IMPACT: Marketplace docs listing tag groups are correct; optionally add per-tag operation counts (live 3, analytics 3, qoe 2, alerts 10, reports 6, admin 20, fleet 1, anomalies 1, probes 5, operational 2, ingest 1, auth 5).

## api.healthz  [CONFIRMED]
- CLAIM CHECKED: GET /healthz returns JSON with status, components, and ams_env_configured fields and is unauthenticated. Check the handler in server/internal/api/server.go and list the exact component names it reports.
- FACT: The handler at server/internal/api/server.go:892 (handleHealthz) writes exactly those three top-level fields. It is registered outside any auth middleware at line 425 (comment: 'Operational (unauthenticated)'). Component names always present: clickhouse, meta_store, collector. A fourth component, kafka, is included only when a KafkaStatsProvider is wired at runtime (s.kafkaStats != nil, line 956).
- EVIDENCE: /home/aytek/repo/ams-pulse/server/internal/api/server.go lines 425 (route registration, unauthenticated), 892-991 (handler body): writeJSON at line 987-991 writes {"status", "components", "ams_env_configured"}; component keys set at lines 905/911/919 (clickhouse), 931/937/944 (meta_store), 952 (collector), 966 (kafka, conditional).
- DOC IMPACT: Marketplace docs must list the three guaranteed component names (clickhouse, meta_store, collector) and note kafka appears only when a Kafka source is configured; all docs must confirm /healthz requires no authentication.

## api.ws  [CONFIRMED]
- CLAIM CHECKED: GET /api/v1/live/ws upgrades to WebSocket pushing snapshot, delta, heartbeat message types.
- FACT: The route is registered at server/internal/api/server.go:519 under downloadAuthMiddleware. handleLiveWS (line 1122) sends a snapshot message immediately on connect (line 1165). The background wsPushLoop (started at line 359, defined at line 1177) broadcasts delta messages on each live-feed update (line 1193) and heartbeat messages on a 30-second ticker (line 1196). All three message types use the wsMessage struct {Type, TS, Payload}.
- EVIDENCE: /home/aytek/repo/ams-pulse/server/internal/api/server.go: route at line 519; initial snapshot at line 1165 (Type: "snapshot"); delta broadcast at line 1193 (Type: "delta"); heartbeat at line 1196 (Type: "heartbeat", 30s ticker line 1180). The endpoint requires authentication (bearer/cookie/?token= via downloadAuthMiddleware).
- DOC IMPACT: Marketplace docs must document all three WebSocket message types (snapshot, delta, heartbeat) and note that the endpoint is authenticated (bearer token, pulse_session cookie, or ?token= query parameter).

## ui.users-tab  [CONFIRMED]
- CLAIM CHECKED: The Settings Users tab is a stub whose UI says user management is coming in a future update while the /admin/users API is fully implemented. Quote the exact stub text from web/src/features/settings/SettingsPage.tsx.
- FACT: The Users tab renders exactly this text at SettingsPage.tsx:754: "User management — coming in a future update." The server-side /admin/users API has all four routes registered: GET /admin/users, POST /admin/users, PUT /admin/users/{userId}, DELETE /admin/users/{userId} (server/internal/api/server.go:500-503).
- EVIDENCE: web/src/features/settings/SettingsPage.tsx:751-756 (Users tabpanel renders a single div with that literal string); server/internal/api/server.go:500-503 (four CRUD routes registered for /admin/users).
- DOC IMPACT: Documentation must quote the exact stub string 'User management — coming in a future update.' and note that the backend CRUD API for users is live even though the UI management surface is deferred.

## ui.version-display  [CONFIRMED]
- CLAIM CHECKED: The web UI displays NO Pulse binary version anywhere. Search web/src for version display.
- FACT: No Pulse binary version is displayed anywhere in web/src. The only 'version' occurrences are: (a) AMS node version rendered in FleetPage.tsx:126-127 and :336 (shows the remote AMS server version, not Pulse), and (b) the connection-verified message in OnboardingWizard.tsx:119 which may append 'AMS {version}' to a connection test result. No component reads or renders a Pulse binary version string.
- EVIDENCE: grep of 'version' across web/src TSX/TS files (excluding test files) returns only FleetPage.tsx and OnboardingWizard.tsx hits, all referring to AMS/remote-node versions, not the Pulse binary itself.
- DOC IMPACT: Documentation must not claim the dashboard shows which version of Pulse is running; there is no such display in the current UI.

## ui.tier-gates  [CONFIRMED]
- CLAIM CHECKED: ReportsPage requires business tier, AnomaliesPage requires enterprise, ProbesPage requires pro, each showing a TierGate upgrade card. Cite file:line for each.
- FACT: All three pages render a <TierGate> component when the license check fails. Headings are: (1) ReportsPage.tsx:797 heading='Usage Reports requires Business tier' — gate condition: tier is neither 'business' nor 'enterprise'; (2) AnomaliesPage.tsx:267 heading='Anomaly Detection requires Enterprise tier' — gate condition: tier is not 'enterprise'; (3) ProbesPage.tsx:1286 heading='Synthetic Probes requires Pro tier' — gate condition: tier is 'free'. Enterprise tier passes all three gates; business tier passes reports and probes gates.
- EVIDENCE: web/src/features/reports/ReportsPage.tsx:711-715 (isGated logic), :797 (TierGate heading); web/src/features/anomalies/AnomaliesPage.tsx:247 (gate condition), :267 (TierGate heading); web/src/features/probes/ProbesPage.tsx:1263 (gate condition), :1286 (TierGate heading); web/src/components/TierGate.tsx (shared upgrade card component).
- DOC IMPACT: Documentation must list the minimum tier for each gated feature: Business for Usage Reports, Enterprise for Anomaly Detection, Pro for Synthetic Probes — noting that higher tiers also satisfy each gate.

## ui.onboarding  [CORRECTED]
- CLAIM CHECKED: The onboarding wizard has 4 steps: welcome, add AMS source, verify connection, done. Quote the step labels.
- FACT: The wizard does have 4 steps, but the step indicator labels displayed in the UI strip are: 'Welcome', 'Add source', 'Verify', 'Done' (OnboardingWizard.tsx:139). The claim's labels 'add AMS source' and 'verify connection' are the headings of the respective step cards (lines 167 'Add AMS source' and 295 'Verify connection'), not the step indicator labels.
- EVIDENCE: web/src/features/settings/OnboardingWizard.tsx:139: ["Welcome", "Add source", "Verify", "Done"].map(...); step card h2 at :167 reads 'Add AMS source' and at :295 reads 'Verify connection'.
- DOC IMPACT: Documentation must use the step indicator labels exactly as rendered: 'Welcome', 'Add source', 'Verify', 'Done' — not 'add AMS source' or 'verify connection', which are internal card headings.

## ui.authgate  [CORRECTED]
- CLAIM CHECKED: Login is an inline AuthGate card asking for an API token with placeholder plt_ prefix, plus an SSO button when OIDC is enabled.
- FACT: The AuthGate is a full-page centered form (minHeight: '100vh'), not an inline card. It replaces the entire viewport when no token is present (App.tsx:110 wraps all routes). The login form asks 'Enter your API token to continue', uses input type='password' with placeholder 'plt_…' (confirming the plt_ prefix), and appends a 'Sign in with SSO' button (AuthGate.tsx:194) only when the GET /auth/oidc/status endpoint returns enabled:true. The 'inline' characterization is wrong; it is a full-screen modal overlay.
- EVIDENCE: web/src/components/AuthGate.tsx:104-110 (outer div minHeight:'100vh'); :127-129 ('Enter your API token to continue' subtitle); :138 (placeholder 'plt_…'); :177-196 (SSO button rendered only when oidcEnabled state is true); web/src/App.tsx:110 (<AuthGate> wraps all protected routes).
- DOC IMPACT: Documentation must describe the login screen as a full-page centered card that takes over the viewport, not as an inline component; the token placeholder is 'plt_…' and an SSO option appears when OIDC is configured on the server.

## ui.trial-banner  [CONFIRMED]
- CLAIM CHECKED: A license expiry banner appears when expiry is within 14 days or expired. Quote the banner strings.
- FACT: TrialBanner.tsx implements two variants: (1) When isTrialExpired is true (non-dismissable error strip): 'License expired — Pulse is running on Free tier limits. Activate a license in Settings › License.' (2) When daysRemaining is between 1 and 14 inclusive (dismissable warning strip): 'License expires in {N} day(s) — activate a key in Settings › License.' The threshold of 14 days is exact (condition at TrialBanner.tsx:61: daysRemaining > 0 && daysRemaining <= 14).
- EVIDENCE: web/src/components/TrialBanner.tsx:36-58 (expired branch, banner text at :54-55); :61-106 (warning branch, banner text at :83-85); visibility rules documented in file header comments :8-15.
- DOC IMPACT: Documentation must reproduce both banner strings exactly: the expired string mentions 'Free tier limits' and the warning string says 'activate a key' (not 'activate a license'); both direct users to 'Settings › License'.

## reports.export.formats  [CONFIRMED]
- CLAIM CHECKED: The GET /api/v1/reports/export endpoint serves CSV only; interactive PDF export was removed (LIM-24).
- FACT: server/internal/api/export.go L40–44: any request with format != "csv" returns HTTP 501 NOT_IMPLEMENTED with message "PDF export is not yet implemented (Phase 3 feature); use format=csv". The Export PDF button was removed from the UI (changelog D-094 in known-limitations.md). The endpoint has never served PDF; the button was removed rather than leaving a 501-returning affordance.
- EVIDENCE: server/internal/api/export.go:40-44 (501 for non-csv); server/internal/api/server.go:518 (route registration); docs/known-limitations.md:542 changelog D-094
- DOC IMPACT: Documentation must state that GET /api/v1/reports/export accepts only format=csv and returns HTTP 501 NOT_IMPLEMENTED for any other format value, including format=pdf.

## reports.ui.buttons  [CONFIRMED]
- CLAIM CHECKED: The web Reports page has an Export CSV button but NO Export PDF button.
- FACT: web/src/features/reports/ReportsPage.tsx L828–842 renders a single export button labelled "Export CSV" that calls reportsApi.downloadExport({ format: "csv" }). There is no "Export PDF" button anywhere in the file.
- EVIDENCE: web/src/features/reports/ReportsPage.tsx:828-842 (Export CSV button); entire file searched — no PDF button present
- DOC IMPACT: Documentation must state that the Reports UI exposes only an Export CSV button; no PDF export button exists.

## reports.scheduled.formats  [CORRECTED]
- CLAIM CHECKED: SCHEDULED reports write both pulse-usage-*.csv and pulse-usage-*.pdf artifacts.
- FACT: Each scheduled run produces exactly ONE artifact — either pulse-usage-<from>-to-<to>.csv or pulse-usage-<from>-to-<to>.pdf — determined by the schedule's format field (scheduler.go L255–257: defaults to FormatCSV if empty). The isReportArtifact() prune filter at L172–175 recognises both extensions, but that is only for pruning existing files; it does not imply both are written per run. A single schedule never writes both formats simultaneously.
- EVIDENCE: server/internal/reports/scheduler.go:255-277 (format selection and single GenerateStatement call); server/internal/reports/statement.go:67-77 (GenerateStatement returns one artifact); server/internal/reports/statement.go:194 (csv filename), 265 (pdf filename)
- DOC IMPACT: Documentation must state that each scheduled report run produces a single artifact whose format (CSV or PDF) matches the schedule's configured format field.

## reports.whitelabel  [CORRECTED]
- CLAIM CHECKED: Enterprise white-label PDF exists gated by a white_label license claim and PULSE_REPORT_LOGO_PATH sets the logo.
- FACT: PDF format for scheduled reports is a Business-tier feature (gated by CheckReports(), not CheckWhiteLabel()). The white-label header block — company name and address printed inside the PDF — is the Enterprise-only feature, gated by the white_label license claim via CheckWhiteLabel() (license.go L438–445; only enterpriseTierEntitlements.WhiteLabel=true at L148). PULSE_REPORT_LOGO_PATH sets the default logo (SchedulerConfig.LogoPath, scheduler.go L42–43) used in ALL scheduled PDF exports regardless of tier — not just Enterprise. The white_label claim gates only the custom whitelabel_header JSON field (company name/address) stored on the schedule.
- EVIDENCE: server/internal/reports/scheduler.go:42-43 (LogoPath comment), 224-230 (CheckReports gate), 260-265 (CheckWhiteLabel gates whitelabel_header only); server/internal/license/license.go:104-105,139,148 (WhiteLabel entitlement, Business=false, Enterprise=true), 438-445 (CheckWhiteLabel); server/internal/reports/statement.go:45-48 (LogoPath set from PULSE_REPORT_LOGO_PATH), 214 (ResolveLogo applied to all PDFs)
- DOC IMPACT: Documentation must state that scheduled PDF reports are a Business-tier feature; the white-label header (company name, address) in the PDF requires an Enterprise license with the white_label claim; PULSE_REPORT_LOGO_PATH sets the logo for all scheduled PDF exports at any tier that has reports access.

## reports.lim24  [CONFIRMED]
- CLAIM CHECKED: docs/known-limitations.md LIM-24 says PDF export is not implemented. Quote its exact current wording in fact.
- FACT: LIM-24 heading (docs/known-limitations.md L512): "PDF export is not implemented (Phase 3 feature)". Body (L514–517): "The Reports page offers only CSV export (`GET /api/v1/reports/export?format=csv`). The \"Export PDF\" button has been removed. Requesting `format=pdf` returns `501 NOT_IMPLEMENTED`.". Note: the LIM-24 text refers only to the interactive export endpoint; it does not mention that scheduled PDF generation (format=pdf on a schedule) is implemented and functional.
- EVIDENCE: docs/known-limitations.md:512-531 (LIM-24 full text); changelog entry L542: "D-094 (S32, 2026-07-14) | Added LIM-24: PDF export not implemented (Phase 3); removed Export PDF button from Reports page; implemented GET /api/v1/reports/export?format=csv; count 23 → 24"
- DOC IMPACT: Documentation should quote LIM-24 accurately and clarify that the limitation applies to the interactive GET /api/v1/reports/export endpoint only — scheduled PDF generation via report schedules is a separate implemented feature.

## ver.describe  [CONFIRMED]
- CLAIM CHECKED: git describe --tags --always on the current branch yields a v0.4.0-based string.
- FACT: git describe --tags --always returns: v0.4.0-136-g327e5f0
- EVIDENCE: Ran `git describe --tags --always` in /home/aytek/repo/ams-pulse on branch s96-future-roadmap; output is v0.4.0-136-g327e5f0 (136 commits after tag v0.4.0, commit g327e5f0).
- DOC IMPACT: Documentation must state the current build is 136 commits ahead of the v0.4.0 release tag, identified as v0.4.0-136-g327e5f0.

## ver.tags  [CONFIRMED]
- CLAIM CHECKED: Release tags v0.1.0 v0.2.0 v0.3.0 v0.4.0 exist.
- FACT: git tag --list (sorted) shows exactly: v0.1.0, v0.2.0, v0.3.0, v0.4.0 — all four tags present, no others.
- EVIDENCE: Ran `git tag --list | sort -V` in /home/aytek/repo/ams-pulse; output lists all four tags and nothing else.
- DOC IMPACT: Documentation may accurately list v0.1.0 through v0.4.0 as the complete set of published release tags.

## ver.file  [CORRECTED]
- CLAIM CHECKED: The VERSION file contains 0.1.0.
- FACT: The VERSION file at /home/aytek/repo/ams-pulse/VERSION contains: 0.4.0
- EVIDENCE: Read /home/aytek/repo/ams-pulse/VERSION; content is '0.4.0', not '0.1.0'.
- DOC IMPACT: Documentation must state the VERSION file records the current release as 0.4.0, not 0.1.0.

## ghcr.visibility  [CONFIRMED]
- CLAIM CHECKED: ghcr.io/aytekxr/ams-pulse is PRIVATE (anonymous pull fails).
- FACT: Anonymous token request to https://ghcr.io/token?service=ghcr.io&scope=repository:aytekxr/ams-pulse:pull returns HTTP error with body {"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}. Subsequent manifest request for tag 0.4.0 using an empty bearer token returns HTTP 403.
- EVIDENCE: curl -sv of the token endpoint returned UNAUTHORIZED (no anonymous token issued); curl of https://ghcr.io/v2/aytekxr/ams-pulse/manifests/0.4.0 with the empty bearer returned HTTP 403, confirming the image is private and anonymous pull is denied.
- DOC IMPACT: Documentation must state that ghcr.io/aytekxr/ams-pulse is a private registry and users must authenticate (docker login ghcr.io) with a valid GitHub personal access token (read:packages scope) before pulling.

## ams.latest  [CONFIRMED]
- CLAIM CHECKED: The latest Ant Media Server release is 3.0.3 from May 2026.
- FACT: gh api repos/ant-media/Ant-Media-Server/releases/latest returns tag_name: ams-v3.0.3, published_at: 2026-05-05T10:55:50Z. The three most recent releases are: ams-v3.0.3 (2026-05-05), ams-v3.0.2 (2026-05-04), ams-v3.0.1 (2026-04-19).
- EVIDENCE: Ran `gh api repos/ant-media/Ant-Media-Server/releases/latest --jq .tag_name` → ams-v3.0.3; ran `gh api repos/ant-media/Ant-Media-Server/releases --jq '.[0:3] | .[] | {tag, published_at}'` → three entries above. Version 3.0.3 was published in May 2026, confirming both the version number and the month.
- DOC IMPACT: Documentation may state the minimum tested/compatible Ant Media Server version is 3.0.3 (tag ams-v3.0.3, released 2026-05-05); note the tag format uses 'ams-v' prefix, not bare 'v'.

## tiers.entitlements  [CONFIRMED]
- CLAIM CHECKED: server/internal/license/license.go defines Free MaxNodes=1 retention 7d, Pro MaxNodes=10 retention 90d, Business MaxNodes=5 retention 396d, Enterprise unlimited.
- FACT: Exact values from license.go lines 112-150:
| Tier       | MaxNodes       | MaxStreams      | RetentionDays  | DataAPI | WhiteLabel | Channels                                      |
|------------|----------------|-----------------|----------------|---------|------------|-----------------------------------------------|
| Free       | 1              | -1 (unlimited)  | 7              | false   | false      | email                                         |
| Pro        | 10             | -1 (unlimited)  | 90             | true    | false      | email, slack, telegram                        |
| Business   | 5              | -1 (unlimited)  | 396            | true    | false      | email, slack, telegram, pagerduty, webhook    |
| Enterprise | -1 (unlimited) | -1 (unlimited)  | -1 (unlimited) | true    | true       | email, slack, pagerduty, telegram, webhook    |

MaxStreams is -1 (unlimited) for every tier. DataAPI is false only for Free; WhiteLabel is true only for Enterprise. The claim's stated node and retention numbers are exactly correct.
- EVIDENCE: server/internal/license/license.go lines 112-150: freeTierEntitlements (MaxNodes=1, MaxStreams=-1, RetentionDays=7, DataAPI=false, WhiteLabel=false), proTierEntitlements (MaxNodes=10, MaxStreams=-1, RetentionDays=90, DataAPI=true), businessTierEntitlements (MaxNodes=5, MaxStreams=-1, RetentionDays=396 '// 13 months', DataAPI=true), enterpriseTierEntitlements (MaxNodes=-1, MaxStreams=-1, RetentionDays=-1, DataAPI=true, WhiteLabel=true).
- DOC IMPACT: Marketplace docs must publish the full six-column entitlement table (MaxNodes, MaxStreams, RetentionDays, DataAPI, WhiteLabel, Channels) for all four tiers; MaxStreams is unlimited on every tier and must not be listed as a differentiator.

## tiers.gates  [CONFIRMED]
- CLAIM CHECKED: Feature gates are: beacon ingest + probes + data API = Pro+; Prometheus metrics + reports + multi-tenant = Business+; anomalies + SSO + white-label = Enterprise.
- FACT: All eleven Check functions in license.go implement exactly the claimed gates. CheckBeaconIngest (line 405) and CheckProbes (line 361) gate on Pro/Business/Enterprise. CheckDataAPI (line 352) gates on the DataAPI bool which is true for Pro, Business, Enterprise. CheckPrometheus (line 419), CheckReports (line 394), CheckMultiTenant (line 383) gate on Business/Enterprise. CheckAnomalies (line 373), CheckSSO (line 431), CheckWhiteLabel (line 443) gate on Enterprise only (or the WhiteLabel entitlement field which is only set for Enterprise in the default tables).
- EVIDENCE: server/internal/license/license.go: CheckBeaconIngest line 405 (Pro+), CheckProbes line 361 (Pro+), CheckDataAPI line 352 (DataAPI bool, Pro+), CheckPrometheus line 419 (Business+), CheckReports line 394 (Business+), CheckMultiTenant line 383 (Business+), CheckAnomalies line 373 (Enterprise), CheckSSO line 431 (Enterprise), CheckWhiteLabel line 443 (WhiteLabel entitlement, Enterprise only by default).
- DOC IMPACT: Marketplace feature-comparison table must list all nine gated capabilities against the three tiers (Pro+, Business+, Enterprise) exactly as implemented in CheckBeaconIngest through CheckWhiteLabel.

## tiers.channels  [CONFIRMED]
- CLAIM CHECKED: Alert channels gate as email=Free+, Slack+Telegram=Pro+, PagerDuty+generic webhook=Business+.
- FACT: Channel gating is implemented in license.go via the Channels []string field of Entitlements (lines 118, 129, 140, 149) and enforced by CheckChannelAllowed (line 341). Free: [email]. Pro: [email, slack, telegram]. Business: [email, slack, telegram, pagerduty, webhook]. Enterprise: [email, slack, pagerduty, telegram, webhook] (same set as Business). The alert/channels/ package (channels.go) contains no tier checks; all gating resides in license.go.
- EVIDENCE: server/internal/license/license.go lines 118 (freeTierEntitlements.Channels=["email"]), 129 (proTierEntitlements.Channels=["email","slack","telegram"]), 140 (businessTierEntitlements.Channels=["email","slack","telegram","pagerduty","webhook"]), 149 (enterpriseTierEntitlements same as Business), 341 CheckChannelAllowed iterates e.Channels. server/internal/alert/channels/channels.go contains no license or tier references.
- DOC IMPACT: Docs must note that channel tier enforcement is applied at rule-creation and notification-dispatch time via CheckChannelAllowed in the license package, not inside the channel adapter code; operators cannot bypass the gate by configuring a channel adapter directly.

## tiers.maxnodes-inversion  [CONFIRMED]
- CLAIM CHECKED: Pro MaxNodes(10) exceeds Business MaxNodes(5) which is a product inconsistency flagged for the operator.
- FACT: proTierEntitlements.MaxNodes=10 (line 124) and businessTierEntitlements.MaxNodes=5 (line 135). A Business-tier operator is limited to fewer nodes than a Pro-tier operator. No compile-time or runtime assertion flags this inversion; it is an intentional pricing decision encoded in the static entitlement tables. The code comment on businessTierEntitlements reads 'up to 5 nodes'.
- EVIDENCE: server/internal/license/license.go line 124: proTierEntitlements MaxNodes=10; line 134-135: businessTierEntitlements MaxNodes=5. No cross-tier assertion exists anywhere in the file.
- DOC IMPACT: Marketplace tier comparison must explicitly state that the Pro tier allows up to 10 monitored nodes while Business allows up to 5; this counter-intuitive ordering must be explained in the pricing rationale to prevent operator confusion when choosing a tier.

## readme.f6  [CONFIRMED]
- CLAIM CHECKED: README.md feature table row F6 says CSV + PDF which conflicts or aligns with actual export behavior - quote the row.
- FACT: README.md line 93 reads exactly: '| Usage / billing reports | F6 | **Shipped** | Business+ tier required; CSV + PDF; tenant mapping; S3 export; ±1% reconciliation; 5-field cron schedules work; `peak_concurrency` sourced from true windowed max (`rollup_concurrency_1d`) |'. The row unambiguously states both CSV and PDF export formats are available at Business+ tier.
- EVIDENCE: /home/aytek/repo/ams-pulse/README.md line 93: full row text as quoted above.
- DOC IMPACT: Marketplace docs must state that usage/billing reports are exported in both CSV and PDF formats, require Business tier or higher, and support S3 upload; no feature-flag or separate add-on is described.

## tiers.activation  [CORRECTED]
- CLAIM CHECKED: License activates via PULSE_LICENSE_KEY env, PULSE_LICENSE_FILE for air-gapped, or PUT /api/v1/admin/license hot-reload; invalid/expired keys degrade to Free without crashing.
- FACT: Three parts of the claim are exactly correct: (1) PULSE_LICENSE_KEY is read directly in serve.go:316 and passed to license.New. (2) PULSE_LICENSE_FILE is also read directly in serve.go:316 as the second argument to license.New (the offline/air-gapped file path). (3) The hot-reload endpoint is PUT /api/v1/admin/license (server.go:494), which calls Manager.Refresh (license.go:240). (4) Invalid/expired keys trigger m.setFree() + a WARN log with no panic (license.go lines 217, 228, 263-270). One discrepancy: server/internal/config/config.go lines 342-346 defines a separate config field read from the env var PULSE_LICENSE_OFFLINE_FILE, but this LicenseConfig.OfflineFile value is never passed to license.New in any production code path; serve.go bypasses the config package and reads PULSE_LICENSE_FILE directly. The effective env var for air-gapped is PULSE_LICENSE_FILE (serve.go path), not PULSE_LICENSE_OFFLINE_FILE (config.go path). PULSE_LICENSE_OFFLINE_FILE is a dead config field.
- EVIDENCE: server/cmd/pulse/serve.go line 316: `license.New(os.Getenv("PULSE_LICENSE_KEY"), os.Getenv("PULSE_LICENSE_FILE"))`. server/internal/api/server.go line 494: `r.Put("/admin/license", s.handleActivateLicense)`. server/internal/license/license.go lines 215-234 (fail-open to Free on bad key), lines 263-270 (mid-run expiry downgrades to Free with WARN). server/internal/config/config.go lines 342-346: reads PULSE_LICENSE_KEY and PULSE_LICENSE_OFFLINE_FILE but this struct is never passed to license.New.
- DOC IMPACT: Docs must specify PULSE_LICENSE_FILE (not PULSE_LICENSE_OFFLINE_FILE) as the env var for air-gapped deployments; the config.go field PULSE_LICENSE_OFFLINE_FILE is silently ignored and must not be documented as an activation path.

## sdk.js-size  [CONFIRMED]
- CLAIM CHECKED: The beacon-js SDK gzip size is about 3.5 KB against a 15 KB gate.
- FACT: npm run size reports 3.52 kB with all dependencies, minified and gzipped, against a 15 kB size-limit gate (package.json size-limit config). The 'about 3.5 KB' characterization is accurate.
- EVIDENCE: /home/aytek/repo/ams-pulse/sdk/beacon-js/package.json lines 17-23 (size-limit config, 15 KB gate); live npm run size output: 'Size: 3.52 kB with all dependencies, minified and gzipped'
- DOC IMPACT: Documentation must state the beacon-js SDK gzip size is 3.52 KB against a 15 KB budget gate.

## sdk.js-tests  [CONFIRMED]
- CLAIM CHECKED: The JS SDK has 65 tests and is MIT licensed with zero runtime dependencies.
- FACT: vitest run reports '65 passed (65)' across 5 test files. package.json has 'license': 'MIT' and no 'dependencies' field (only 'devDependencies'), confirming zero runtime dependencies.
- EVIDENCE: /home/aytek/repo/ams-pulse/sdk/beacon-js/package.json line 5 (license: MIT), no 'dependencies' key present; vitest output: 'Tests 65 passed (65)' across src/__tests__/{hls,pulse,schema,session,transport}.test.ts
- DOC IMPACT: Documentation must state 65 tests, MIT license, and zero runtime dependencies for the beacon-js SDK.

## sdk.swift  [CONFIRMED]
- CLAIM CHECKED: The Swift SDK PulseBeacon is Phase 1 only: cross-platform core using Foundation+Dispatch, 22 tests, Linux CI, no URLSession transport yet.
- FACT: README.md explicitly labels the package 'Phase 1' and defers URLSession background config to 'Phase 2 (needs Xcode/an Apple CI runner)'. Package.swift uses only Foundation/Dispatch (no third-party dependencies). grep of all 4 test files counts exactly 22 'func test' functions (TransportTests: 8, PulseBeaconTests: 5, TypesTests: 5, SessionTests: 4). README states Linux CI gates the build.
- EVIDENCE: /home/aytek/repo/ams-pulse/sdk/beacon-swift/README.md lines 96-99 (Phase 1/2 status), lines 74-78 (Foundation+Dispatch, Linux CI), line 93 ('swift test — 22 tests, Linux-clean'); /home/aytek/repo/ams-pulse/sdk/beacon-swift/Package.swift (no external dependencies); test file grep total = 22
- DOC IMPACT: Documentation must state the Swift SDK is Phase 1 (Foundation+Dispatch core, 22 tests, Linux CI); URLSession transport and iOS background flush are Phase 2 and not yet shipped.

## docs.alerting-diagram  [CORRECTED]
- CLAIM CHECKED: docs/runbooks/alerting.md contains NO architecture/flow diagram (no mermaid block).
- FACT: The file contains two plain-code-block flow diagrams: a three-state alert state machine (lines 22–26: `pending → firing → resolved` with cooldown loop) and a latency flow (lines 141–143: `AMS REST poll → aggregator update → evaluator tick → channel.Send`). There is no mermaid-fenced block, but the file is not diagram-free.
- EVIDENCE: /home/aytek/repo/ams-pulse/docs/runbooks/alerting.md lines 22–26 and 141–143
- DOC IMPACT: External documentation must note that the alerting runbook contains ASCII-art flow diagrams for the alert state machine and detection-to-notification latency path, but no mermaid diagram.

## docs.beacon-diagram  [CONFIRMED]
- CLAIM CHECKED: docs/beacon-sdk.md contains NO flow diagram.
- FACT: The file (453 lines) contains tables, fenced code snippets, and prose sections but no flow diagram of any kind — neither mermaid nor ASCII art.
- EVIDENCE: /home/aytek/repo/ams-pulse/docs/beacon-sdk.md — full file read; no diagram block found.
- DOC IMPACT: External documentation may state that the beacon SDK guide contains no flow or sequence diagram; architecture context must come from README.md system-overview ASCII diagram.

## docs.compat-ams  [CONFIRMED]
- CLAIM CHECKED: docs/compatibility.md states AMS 3.0.3 Enterprise is live-validated and has a TBD capacity row for load validation.
- FACT: AMS-version support statement (line 12): '| 3.0.3 Enterprise (build 20260504_1443) | **LIVE-VALIDATED** | **Supported — primary target** | 46/50 scenario scripts PASS, qa/realams S17–S18, D-079/D-080 |'. Capacity TBD row (line 163): '| Published capacity number (validated N/M) | **TBD** — fill in from the first `LOAD-REPORT.md` |'
- EVIDENCE: /home/aytek/repo/ams-pulse/docs/compatibility.md lines 12 and 163
- DOC IMPACT: Marketplace docs must state AMS 3.0.3 Enterprise (build 20260504_1443) is live-validated at 46/50 scenarios, and that load-validated capacity (publishers/viewers) is TBD pending a dedicated-instance load run.

## docs.lims  [CONFIRMED]
- CLAIM CHECKED: docs/known-limitations.md does NOT yet cover the web Users-tab stub nor the firing-orphan FO-1; report the highest LIM number currently present.
- FACT: No entry for a Users-tab stub or FO-1 firing-orphan exists anywhere in the file. The highest LIM number present is LIM-24 (PDF export not implemented, added D-094/S32). Changelog confirms: 'count 23 → 24' at D-094.
- EVIDENCE: /home/aytek/repo/ams-pulse/docs/known-limitations.md — full file read; changelog line 542: 'D-094 (S32, 2026-07-14) | Added LIM-24 ... count 23 → 24'
- DOC IMPACT: External documentation must note that the Users-tab stub (web UI) and any firing-orphan scenario (FO-1) are not yet documented as known limitations; the current highest catalogued limitation is LIM-24.

## docs.security-stale  [CORRECTED]
- CLAIM CHECKED: SECURITY.md supported-versions table lists only v0.1.x and a closing paragraph claims no LICENSE file has been added yet.
- FACT: The supported-versions table (lines 14–17) lists: '| v0.4.x | Yes |' and '| < v0.4.0 | No — upgrade to the latest v0.4.x release |'. There is no v0.1.x entry. The closing section (lines 129–135) is a License paragraph that references the existing PolyForm Noncommercial 1.0.0 LICENSE file; it makes no claim that a LICENSE file is absent.
- EVIDENCE: /home/aytek/repo/ams-pulse/SECURITY.md lines 14–17 and 129–135
- DOC IMPACT: External documentation must reflect that SECURITY.md already lists v0.4.x as the supported version (not v0.1.x), and that the LICENSE file exists under PolyForm Noncommercial 1.0.0.

## docs.readme-stale  [CORRECTED]
- CLAIM CHECKED: README.md feature-status section says 'Last updated v0.2.0 GA (2026-07-09...)'.
- FACT: README.md line 81 reads: 'Last updated: **2026-07-22 (D-161)** — all 10 PRD features shipped; prod live at **v0.4.0** behind TLS against a real AMS 3.0.3 Enterprise.' There is no v0.2.0 reference in the feature-status section.
- EVIDENCE: /home/aytek/repo/ams-pulse/README.md line 81
- DOC IMPACT: External documentation must state that the README feature-status section was last updated 2026-07-22 (D-161) for v0.4.0, not v0.2.0.

## docs.helm-readme  [CORRECTED]
- CLAIM CHECKED: deploy/helm/README.md still says 'Helm is not started' while the chart exists.
- FACT: The file's full content is: '# Pulse Helm chart\n\nKubernetes deployment for clustered AMS installs (PRD §7.10). The chart lives in [`deploy/helm/pulse/`](pulse/) — see [`pulse/README.md`](pulse/README.md) for the values table, secrets setup, HA deployment, and resource sizing.\n\n**Status: experimental.** The chart is authored and CI-verified by `helm lint` + `helm template` golden-file tests (default, Postgres+S3, and external-ClickHouse value sets), but a real-cluster `helm install` has not yet been validated (D-002). Docker Compose remains the supported production path — see [`docs/runbooks/install.md`](../../docs/runbooks/install.md).' It does not say 'Helm is not started'; it says the chart is experimental and CI-verified but cluster install unvalidated.
- EVIDENCE: /home/aytek/repo/ams-pulse/deploy/helm/README.md lines 1–11 (full file)
- DOC IMPACT: External documentation must state that the Helm chart is authored and CI-verified (helm lint + helm template) but a real-cluster helm install has not yet been validated; Docker Compose is the supported production path.

## config.count  [CORRECTED]
- CLAIM CHECKED: There are roughly 60 PULSE_-prefixed environment variables read by the server.
- FACT: There are 69 distinct non-test PULSE_ env var names found in server/ Go files. The complete scan (grep -oP '"PULSE_[A-Z0-9_]+"' across server/**/*.go, including the S3 vars that the earlier alpha-only pattern missed) yields 75 unique quoted strings. Subtracting one prefix fragment ("PULSE_AMS_" used in string concatenation, not a standalone var) and five test-only names (PULSE_TEST_CMD_HELPER_ZZXYZZY, PULSE_TEST_ENVBOOL, PULSE_TEST_SECRET_GETX, PULSE_TEST_VERBOSE_CH, PULSE_META_TEST_PG_DSN) leaves 69 production-configurable names. The claim of 'roughly 60' undercounts by approximately 9; a correct approximation is 'roughly 70'.
- EVIDENCE: server/cmd/pulse/config.go (full loadEnvConfig function); server/internal/config/config.go (applyEnv function); server/cmd/pulse/serve.go lines 316, 348, 451. Grep command: grep -rn PULSE_ server/ --include='*.go' | grep -oP '"PULSE_[A-Z0-9_]+"' | sort -u | wc -l → 75 total; 6 removed (1 prefix + 5 test) = 69.
- DOC IMPACT: Documentation must state that the server reads 69 distinct PULSE_-prefixed environment variables (not ~60).

## config.defaults  [CORRECTED]
- CLAIM CHECKED: These defaults hold: PULSE_POLL_INTERVAL=5s, PULSE_RETENTION_DAYS=90, PULSE_ROLLUP_TTL_DAYS=395, PULSE_SESSION_IDLE_TIMEOUT=5m, PULSE_CLUSTER_DISCOVERY_INTERVAL=30s, PULSE_BEACON_SAMPLE_RATE=1.0, PULSE_ANOMALY_TICK_S=60, PULSE_REPORT_ARTIFACT_RETENTION_DAYS=90, PULSE_LISTEN_ADDR=:8090, PULSE_WEBHOOK_TIMESTAMP_SKEW=5m.
- FACT: Eight of the ten defaults are confirmed in code. Two are wrong: (1) PULSE_ANOMALY_TICK_S — the env config loader (loadEnvConfig, config.go:388-393) stores 0 when the var is unset; the 60-second default is applied inside the anomaly Detector constructor (anomaly/anomaly.go:232-233: 'if cfg.TickInterval == 0 { cfg.TickInterval = 60 * time.Second }'). The effective behavior is 60 s but the config-layer default is 0, not 60. (2) PULSE_WEBHOOK_TIMESTAMP_SKEW — the env config loader (config.go:257-263) stores 0 (zero duration) when the var is unset; the 5-minute default is applied inside the webhook Handler constructor (collector/webhook/webhook.go:83-84: 'if cfg.TimestampSkew <= 0 { cfg.TimestampSkew = 5 * time.Minute }'). The remaining eight defaults are confirmed: PULSE_POLL_INTERVAL=5s (config.go:283), PULSE_RETENTION_DAYS=90 (config.go:272), PULSE_ROLLUP_TTL_DAYS=395 (config.go:273), PULSE_SESSION_IDLE_TIMEOUT=5m (config.go:319-321), PULSE_CLUSTER_DISCOVERY_INTERVAL=30s (config.go:332-334), PULSE_BEACON_SAMPLE_RATE=1.0 (internal/config/config.go:200), PULSE_REPORT_ARTIFACT_RETENTION_DAYS=90 (config.go:360), PULSE_LISTEN_ADDR=:8090 (config.go:226).
- EVIDENCE: server/cmd/pulse/config.go:226,272,273,283,319-321,332-334,360,388-393; server/internal/config/config.go:200; server/internal/anomaly/anomaly.go:232-233; server/internal/collector/webhook/webhook.go:83-84.
- DOC IMPACT: Documentation must clarify that PULSE_ANOMALY_TICK_S and PULSE_WEBHOOK_TIMESTAMP_SKEW have no config-loader default (unset = 0); the 60 s and 5 m values are subsystem-level fallbacks applied inside the anomaly Detector and webhook Handler respectively, not env var defaults.

## config.drift  [CONFIRMED]
- CLAIM CHECKED: deploy/.env.example and deploy/config/pulse.example.yaml cover only a subset of the config surface. Note how many vars appear in code but neither example file.
- FACT: Of the 69 production PULSE_ env vars in code, 30 are mentioned in deploy/.env.example (the yaml file adds nothing new beyond a comment mentioning the dynamic PULSE_AMS_MAIN_TOKEN pattern and PULSE_POSTGRES_DSN already in the .env.example). That leaves 39 vars that appear in code but neither example file. Additionally, deploy/.env.example lists four S3 variable names with wrong names (PULSE_S3_EXPORT_KEY_ID, PULSE_S3_EXPORT_SECRET_KEY, PULSE_S3_EXPORT_BUCKET, PULSE_S3_EXPORT_REGION) that do not match the actual code names (PULSE_S3_ACCESS_KEY_ID, PULSE_S3_SECRET_ACCESS_KEY, PULSE_S3_BUCKET, PULSE_S3_REGION), so those four are undocumented in practice despite appearing commented in the example. Ten example undocumented names: PULSE_CLICKHOUSE_DSN, PULSE_CLICKHOUSE_ADDR, PULSE_RETENTION_DAYS, PULSE_ROLLUP_TTL_DAYS, PULSE_POLL_INTERVAL, PULSE_SESSION_IDLE_TIMEOUT, PULSE_CLUSTER_DISCOVERY_INTERVAL, PULSE_KAFKA_BROKERS, PULSE_INGEST_LISTEN_ADDR, PULSE_WEBHOOK_TIMESTAMP_SKEW.
- EVIDENCE: deploy/.env.example: grep -oP 'PULSE_[A-Z0-9_]+' lists 35 strings, of which 30 are real server env vars (PULSE_DOMAIN is Docker Compose only; four PULSE_S3_EXPORT_* names do not match code). deploy/config/pulse.example.yaml: only references PULSE_POSTGRES_DSN and a dynamic PULSE_AMS_MAIN_TOKEN pattern comment. Production var list from server/ Go files: 69 names. 69 - 30 = 39 undocumented.
- DOC IMPACT: Documentation must either expand the example files to cover all 69 production env vars or provide a separate configuration reference listing them all; it must also correct the four stale PULSE_S3_EXPORT_* names in deploy/.env.example to match the actual PULSE_S3_* names read by the server.
