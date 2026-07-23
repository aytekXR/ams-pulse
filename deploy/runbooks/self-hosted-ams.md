# Runbook — self-hosted Ant Media Server for Pulse (D-034)

Stand up an operator-owned AMS Enterprise on the Pulse VPS and point Pulse at it. This removes the dependency
on the shared, IP-blocked `test.antmedia.io` (RESUME-PROMPT U1) and lets the full ingest/QoE/webhook pipeline
be exercised on an AMS you fully control.

> Secrets (AMS admin creds, license key) live in `oguz-testing.md` and `deploy/.env` — both **gitignored**.
> Never commit them. The license is a single-instance trial (expires 2026-07-12); don't run two AMS
> containers with the same key.

## 0. Prereqs (this VPS, already true)
- Docker present; user in `docker` group → prefix `sg docker -c "…"`.
- AMS ports free: 5080/5443/1935/5000 (host nginx owns 80/443; `brier-db` owns 5432).
- Public IP `161.97.172.146` directly on `eth0` → WebRTC ICE advertises it (no PUBLIC_IP env needed).

## 1. Run AMS (Docker, host network)
```bash
sg docker -c "docker volume create antmedia_data"
sg docker -c "docker pull antmedia/enterprise:3.0.3"     # matches the version Pulse models
sg docker -c "docker run -d --name antmedia --restart unless-stopped --network host \
  --mount source=antmedia_data,target=/usr/local/antmedia antmedia/enterprise:3.0.3"
```
Wait for HTTP 200: `curl --retry 30 --retry-delay 3 --retry-connrefused -s -o /dev/null -w '%{http_code}\n' http://localhost:5080/`

## 2. Activate the license  (the image does NOT read `-e LICENSE_KEY`)
```bash
KEY=<your-trial-key>
sg docker -c "docker exec antmedia sed -i 's|^server.licence_key=.*|server.licence_key=$KEY|' \
  /usr/local/antmedia/conf/red5.properties"
sg docker -c "docker restart antmedia"
```
Verify (after creating the admin in step 3): authed `GET /rest/v2/version` → `Enterprise Edition`. Real proof
= a stream muxes/transcodes (step 5). Activation needs outbound 443 to Ant Media's license endpoint.

## 3. Create the admin user (headless) + log in
```bash
EMAIL=admin@beyondkaira.com ; PW=$(openssl rand -hex 20)
curl -s -X POST http://localhost:5080/rest/v2/users/initial \
  -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}"
curl -s -c /tmp/ams.cookies -X POST http://localhost:5080/rest/v2/users/authenticate \
  -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}"   # → system/ADMIN
```
Save `$EMAIL`/`$PW` to `oguz-testing.md`.

## 4. Open per-app REST to the Pulse container  (THE key gotcha)
Each app's `remoteAllowedCIDR` defaults to `127.0.0.1`, so the Pulse container is 403'd "Not allowed IP".
For each app (`LiveApp WebRTCAppEE live …`): GET settings, set `remoteAllowedCIDR`, POST the full object back.
- `0.0.0.0/0` — also lets the browser panel's per-app live view load from anywhere (matches "expose fully").
- `172.16.0.0/12` — Pulse works but app REST stays host-private (panel per-app view won't load from a browser).
```bash
for app in LiveApp WebRTCAppEE live; do
  curl -s -b /tmp/ams.cookies http://localhost:5080/rest/v2/applications/settings/$app -o /tmp/$app.json
  python3 -c "import json;d=json.load(open('/tmp/$app.json'));d['remoteAllowedCIDR']='0.0.0.0/0';json.dump(d,open('/tmp/$app.new.json','w'))"
  curl -s -b /tmp/ams.cookies -X POST http://localhost:5080/rest/v2/applications/settings/$app \
    -H 'Content-Type: application/json' --data-binary @/tmp/$app.new.json
done
```

## 5. (Optional) synthetic test publisher
```bash
sg docker -c "docker run -d --name ams-teststream --network host jrottenberg/ffmpeg:4.1-alpine \
  -re -f lavfi -i testsrc2=size=1280x720:rate=30 -f lavfi -i sine=frequency=1000 \
  -c:v libx264 -preset veryfast -tune zerolatency -b:v 2000k -pix_fmt yuv420p -g 60 \
  -c:a aac -f flv rtmp://localhost:1935/LiveApp/teststream"
# remove when real streams flow:  docker rm -f ams-teststream
```

## 6. Point Pulse at the new AMS
In `deploy/.env`: `PULSE_AMS_URL=http://161.97.172.146:5080`, `PULSE_AMS_LOGIN_EMAIL/PASSWORD` = AMS admin
creds, `PULSE_AMS_NODE_ID=beyondkaira-ams`, `PULSE_AMS_APPLICATIONS=` (empty = auto), `PULSE_AMS_AUTH_TOKEN=`,
`PULSE_INGEST_TARGET_BITRATE_KBPS=2000`. Always **staging-verify on an isolated `-p` project first** (base +
hardened + real-ams; curl via `docker exec`), then redeploy prod:
```bash
DC="-p pulse-prod -f deploy/docker-compose.prod.yml -f deploy/docker-compose.real-ams.yml --env-file deploy/.env"
sg docker -c "docker compose $DC config -q" && sg docker -c "docker compose $DC up -d"   # never -v
```
Smoke: `/healthz` 200; `/api/v1/live/overview` (`Authorization: Bearer <admin-token>`) `total_publishers≥1`;
prod logs show no 403/"Not allowed IP"/decode/login errors.

## 7. Rollback (Pulse → test.antmedia.io)
Restore the prior `deploy/.env` (backup in the session scratchpad as `prod.env.bak-pre-ams`) and re-run the
prod `up -d`. To stop AMS entirely: `docker rm -f antmedia ams-teststream` (keep `antmedia_data` to avoid
license re-activation; the trial is one-instance).
