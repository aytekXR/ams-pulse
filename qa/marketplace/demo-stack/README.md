# Marketplace demo stack

Reproducible environment for the Ant Media Marketplace screenshots and walkthrough
evidence (`docs/marketplace/antmedia-submission/`). It runs the **real application**:
the Pulse binary and web UI built from this checkout, a real ClickHouse, and the real
collector, alert evaluator and notification channels. Unlike the older
`qa/marketplace/capture-live-screenshots.mjs` (route-mocked API, no backend), nothing
here is mocked inside Pulse.

## What is simulated (say so wherever the captures are used)

| Input | Source | Why |
|---|---|---|
| Ant Media Server | `qa/mock-ams` — Pulse's AMS simulator speaking the AMS REST v2 wire format, app `LiveApp` | the scenes need dozens of streams and 1,000+ viewers on demand, including a staged incident, which one real AMS cannot produce cheaply; the simulator is the same one CI uses (the real AMS 3.1.0 is tested separately by `qa/realams`) |
| Viewers | `seed-demo.mjs` — scripted player sessions in the beacon SDK wire format, POSTed to the public `/ingest/beacon` endpoint, including a replayed 24-hour history | real players need real media |
| Alert recipients | Mailpit (SMTP sink with web UI) and `webhook_sink.py` | no third-party accounts are used |

Every stream name, viewer count and QoE figure in the captures is demo data. The
simulator reports AMS version `3.0.3 Enterprise` for itself; that string is the
simulator's, not a real server's.

## Run it

```sh
bash qa/marketplace/demo-stack/up.sh                   # build image, start (no license key needed)
node qa/marketplace/demo-stack/seed-demo.mjs setup     # streams, channels, rules, probes
node qa/marketplace/demo-stack/seed-demo.mjs history   # replay ~24 h of sessions (≈1–2 min)
node qa/marketplace/demo-stack/seed-demo.mjs live &    # live traffic; keep it running
node qa/marketplace/demo-stack/seed-demo.mjs degrade   # start the incident (alerts fire)
node qa/marketplace/demo-stack/seed-demo.mjs recover   # end it
bash qa/marketplace/demo-stack/down.sh                 # remove everything
```

- Pulse UI/API: `http://127.0.0.1:18190` (sign in with the token in `.state/admin-token`)
- Mailpit UI: `http://127.0.0.1:18125` (the alert e-mails Pulse actually sent)
- Simulator control API: `http://127.0.0.1:19190/control/*`

All ports bind to `127.0.0.1`. The compose project is fixed to `pulse-mktdemo`, so it
never collides with `pulse-prod`, `pulse-realams` or `pulse-quickstart`.

## Secrets and licensing

`up.sh` writes `.env` (a fresh `PULSE_SECRET_KEY`) and `.state/admin-token`; both are
gitignored. From v0.5.0 every feature is free, so no license key is minted by default
(`DEMO_LICENSE_TIER=none`). For an older image, `DEMO_LICENSE_TIER=business` (or another tier)
mints a demo license with `qa/licensegen` and a **throwaway key pair generated on the spot** —
the official license signing key is never involved and the license is useless outside this
stack.

## Accuracy check

`node seed-demo.mjs truth` sends one session with a known ground truth (10 minutes
watched, one 6-second stall, 1.0 % rebuffer ratio) on a stream nobody else uses and
prints what `/qoe/summary` and `/analytics/audience` report for it. The findings of
the 2026-10-01 run are recorded in
`docs/marketplace/antmedia-submission/marketplace/submission-notes.md`.
