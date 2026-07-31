# Email to Ankush Banyal (Ant Media) — initiate marketplace integration

> **The operator sends this from their own account — never send on their behalf.**
> Rewritten 2026-07-31 (D-191/S123). Content derived from
> [`submission-process.md`](submission-process.md) §1 (the agreed process),
> [`developer-meeting-brief.md`](developer-meeting-brief.md) (agenda + questions), and
> [`submission-package.md`](submission-package.md) (artifact index).
>
> ### ⚠ TWO THINGS BEFORE YOU SEND
>
> 1. **Merge PR #244 first.** Every documentation link below points at `main`, and the public
>    website deploys from `main`. Until #244 merges, the live site still advertises **historical
>    analytics (F2)** and **ingest health (F4)** inside the **Free** plan — and both return
>    `403 LICENSE_REQUIRED` on Free. If Ankush installs a Free instance and clicks either screen,
>    he meets the exact defect we just fixed. Verified against the live site on 2026-07-31.
> 2. **Confirm the address.** `ankush@antmedia.io` is an assumption, never verified. Reply on the
>    original thread if you still have it — that also preserves context.
>
> Fill every `[bracket]`. Nothing else needs editing.

**To:** Ankush Banyal `[ankush@antmedia.io — CONFIRM, or reply on the original thread]`
**Subject:** Pulse is ready — documentation complete, requesting the developer meeting

---

Hi Ankush,

Following up on your offer to arrange a developer meeting once Pulse was fully ready and the
relevant documentation was in place. Both are now done, so I'd like to take you up on it and start
the marketplace qualification process.

Everything below is public and needs no credentials from your side — you or your developer can
install, verify and read all of it without anything from me.

## What Pulse is

Pulse is self-hosted analytics, QoE monitoring and alerting for Ant Media Server. It installs
*next to* AMS — a single Go binary plus ClickHouse via Docker Compose (or Helm) — and answers:
who is watching, where, on what device, at what quality, and is anything broken right now.

It covers the layers AMS doesn't: alerting and notification channels, player-side quality
measurement, long-horizon analytics, usage/billing reports, synthetic probes and anomaly
detection. I want to be direct about positioning, since I know the management panel is being
revamped: **Pulse is complementary, not competitive.** The new panel charts live server metrics;
Pulse adds the alerting, viewer-side QoE, 13-month history, reporting and probing on top of the
same REST surface. I'd rather have that conversation openly than have it surface later in review.

**The integration posture is the part I'd most like your developer to scrutinise:**

- **Read-only.** Pulse polls `/rest/v2` and never writes to AMS. No AMS-side plugin, no JAR, no
  WAR, no configuration change to your server, nothing to uninstall from AMS itself.
- **Self-hosted, zero phone-home.** No SaaS backend. Customer data never leaves customer
  infrastructure. License keys are ed25519-signed and verified offline — no activation callback.
- **Upgrade-tolerant.** Because it touches no AMS state, an AMS upgrade doesn't require a Pulse
  change. Optional webhook and Kafka paths exist but nothing depends on them.

Live-validated against **AMS 3.0.3 Enterprise**: 46 of 50 scenario scripts pass. The four that
don't are documented rather than hidden — see Known Limitations below.

## Try it yourself (~15 minutes, no credentials)

```sh
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh \
  | bash -s -- --ams-url http://YOUR-AMS:5080 --email you@example.com
```

The image is public on GHCR (`ghcr.io/aytekxr/ams-pulse:0.4.5`, multi-arch amd64/arm64). If your
team verifies signatures:

```sh
cosign verify \
  --certificate-identity-regexp '^https://github\.com/aytekXR/ams-pulse/\.github/workflows/release\.yml@refs/tags/v.+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/aytekxr/ams-pulse:0.4.5
```

⚠ One practical note that will save your developer a confusing ten minutes: **this needs a cosign
v3 or newer client.** Our signature is stored as an OCI 1.1 referrer rather than under the legacy
`sha256-<digest>.sig` tag, so a **cosign v2 client reports `Error: no signatures found` against a
correctly signed image.** Verified both ways (v2.4.3 fails, v3.0.2 passes). Kubernetes instead:

```sh
helm install pulse oci://ghcr.io/aytekxr/charts/pulse --version 0.3.3 \
  --set pulse.ams.url=http://your-ams:5080 --set pulse.ams.nodeId=node-01
```

## Documentation set

Everything is public on GitHub. If you need it as PDFs or uploaded into a portal instead, the
markdown converts cleanly — just tell me the format you want.

**Start here**
- **Repository** — https://github.com/aytekXR/ams-pulse
- **Product & architecture overview** (the evaluator's first read, with diagrams) —
  https://github.com/aytekXR/ams-pulse/blob/main/docs/overview.md
- **Release v0.4.5** (binaries, `SHA256SUMS`, beacon SDK tarball, Helm chart) —
  https://github.com/aytekXR/ams-pulse/releases/tag/v0.4.5
- **Public website** — https://aytekxr.github.io/ams-pulse/

**Install & operate**
- Install guide (quickstart / Compose / binary / Helm) —
  https://github.com/aytekXR/ams-pulse/blob/main/docs/runbooks/install.md
- User guide, per screen — https://github.com/aytekXR/ams-pulse/blob/main/docs/user-guide.md
- Administrator guide (full configuration reference) —
  https://github.com/aytekXR/ams-pulse/blob/main/docs/admin-guide.md
- Upgrade & rollback —
  https://github.com/aytekXR/ams-pulse/blob/main/deploy/runbooks/upgrade-rollback.md
- Troubleshooting — https://github.com/aytekXR/ams-pulse/blob/main/docs/troubleshooting.md
- FAQ — https://github.com/aytekXR/ams-pulse/blob/main/docs/faq.md

**Integration surface**
- API guide — https://github.com/aytekXR/ams-pulse/blob/main/docs/api-guide.md
- OpenAPI 3 specification —
  https://github.com/aytekXR/ams-pulse/blob/main/contracts/openapi/pulse-api.yaml
- Player QoE beacon SDK (3.52 KB gzip, **MIT** — embeddable in commercial players with no
  licence obligation to us) — https://github.com/aytekXR/ams-pulse/blob/main/docs/beacon-sdk.md
- **AMS compatibility matrix** — which AMS versions are live-validated vs mock-verified —
  https://github.com/aytekXR/ams-pulse/blob/main/docs/compatibility.md

**Diligence — the documents I'd expect a reviewer to want**
- **Known limitations** — 29 honest disclosures, including where AMS 3.x doesn't expose what we'd
  need — https://github.com/aytekXR/ams-pulse/blob/main/docs/known-limitations.md
- Security policy & disclosure process —
  https://github.com/aytekXR/ams-pulse/blob/main/SECURITY.md
- Licensing, tiers and trial terms —
  https://github.com/aytekXR/ams-pulse/blob/main/docs/licensing-public.md
- Third-party licence attribution — **generated from the dependency tree, not hand-written**, so
  it cannot claim a licence a dependency doesn't carry: 56 Go modules and 64 npm packages, zero
  undetermined — https://github.com/aytekXR/ams-pulse/blob/main/THIRD-PARTY-LICENSES.md
- Support policy and response targets —
  https://github.com/aytekXR/ams-pulse/blob/main/docs/support.md
- Changelog — https://github.com/aytekXR/ams-pulse/blob/main/CHANGELOG.md
- Privacy policy — https://aytekxr.github.io/ams-pulse/privacy/ ·
  Terms — https://aytekxr.github.io/ams-pulse/terms/

**On the Known Limitations document specifically:** I'd rather you find our limits written down
than discover them in review. The significant one is cluster mode — AMS 3.x exposes no node role
or version on the cluster-nodes endpoint, so every node displays as `origin` and edge/origin viewer
deduplication cannot activate. The code path exists and would work unchanged if a future AMS
exposed roles. That's LIM-10, and it's one of the things I'd like to ask your developer about.

**Two licensing points I'd rather raise than have you find:** nothing copyleft is redistributed
inside our artifacts, but the Helm chart uses a stock `busybox` init container to wait for
ClickHouse — that's GPL-2.0-only, pulled by the cluster, never bundled or modified by us, so no
distribution obligation attaches to Pulse. And ClickHouse itself is **Apache-2.0, not SSPL**,
which is worth saying plainly because it's commonly assumed otherwise. Both are stated in the
attribution file rather than left to be discovered.

## How it's verified

- **Supply chain:** cosign-signed multi-arch images, SBOM and SLSA provenance attached, and the
  release pipeline is Trivy-gated on HIGH/CRITICAL. That gate is real, not decorative — it
  **blocked v0.4.5 once**, on a HIGH CVE reaching the binary through an indirect Go dependency.
  The image is pushed to a quarantine tag *before* the scan and promoted only after it passes, so
  no public release tag ever pointed at the vulnerable build. It shipped after the fix.
- **CI on every change:** Go test suite under `-race` with a coverage floor, web unit suite,
  Playwright browser e2e, full-stack Compose e2e (licence mint → beacon event → alert fires), CSP
  e2e, Helm lint and template goldens, SDK size gate, CodeQL, npm advisory gate, shellcheck, and a
  nightly AMS wire-format matrix.
- **Live validation:** 46/50 scenarios against real AMS 3.0.3 Enterprise.
- **Privacy:** viewer IPs are SHA-256 hashed with optional full anonymisation; GeoIP only from an
  operator-supplied MMDB; secrets encrypted at rest (AES-256-GCM); admin writes are audit-logged.

## What I'd like from the meeting

Mainly the qualification steps your development team has been defining — but concretely, these are
the open questions on my side, roughly in priority order:

**1. Listing shape.** Pulse is a standalone self-hosted service, not an artifact that loads inside
AMS. Bitmovin lists as a WAR; GST-Ant Fusion as an external process. **Can Pulse list as-is, or
would you want some AMS-side artifact?** This one shapes everything else, so it's the question I'd
most like answered first.

**2. Qualification steps, and the artifact spec.** Whatever checklist your team produced. Also the
practical specs — screenshot dimensions and count, logo formats, whether a demo video is required
and at what length, and whether linking to our GitHub docs is acceptable or you need uploads.
I've prepared 1920×1080 screenshots, SVG and PNG logos, a 1200×630 banner and a demo video, but
those are our guesses at your requirements rather than anything you published.

**3. Review flow and timeline.** What the review consists of, roughly how long it takes, and
whether the security review is audited by you or self-certified by the vendor.

**4. Load-test evidence.** You pointed me at your load-testing documentation, which I read as
capacity validation being part of qualification. I've built a load lane that drives **your**
official tools (WebRTC Load Test Tool, `hls_players.sh`) and asserts our numbers stay correct
under load. **What evidence format and what thresholds do you want?** I'd rather run it once
against your expectations than produce a number in the wrong shape. Related: my AMS trial expired,
so I'll be running this on a pay-as-you-go instance — happy to do that, just confirm it's the
sanctioned path for continued testing.

**5. AMS version support.** We live-validate 3.0.3 (current stable); older versions are
mock-profile verified only, and that's stated plainly in the compatibility matrix. Is that
acceptable, or do you require N-1/N-2 live validation?

**6. Four technical questions for your developer** — these affect what we can offer users, and
I've pre-researched each from your public `Management-panel-reborn` repository so we don't spend
meeting time on discovery:
   - Do the `/rest/v2` paths and response envelopes survive the panel revamp, or is a v2→v3
     re-versioning planned? (The new panel's only API prefix is `/rest/v2` — I read that as "no",
     but I'd like it confirmed, since our entire integration is REST v2.)
   - Does the revamp change the authentication mechanism? (Login still looks like
     `POST /rest/v2/users/authenticate` plus a session cookie.)
   - In 3.0.3 cluster mode, is `GET /rest/v2/cluster/nodes` a flat array or only the paginated
     form? The new panel calls the paginated form; I need the definitive server behaviour.
   - Are the new `/system-resources/history` and `metrics-history` endpoints going to be public,
     stable REST surface we may consume?

   Plus four smaller data-source questions I'll bring rather than list here: webhook HMAC signing
   plans, `hlsViewerCount` sliding-window semantics, WHEP viewer-count exposure, and whether
   `currentFPS` is returning to the REST surface.

**7. Business terms.** The publicly stated first-year 100%-to-vendor / no-commission arrangement —
I'd like to confirm that, and get the **post-year-one terms in writing**. Also whether the vendor
agreement can include an API-stability or deprecation-notice commitment, given the integration is
entirely REST v2. And I'd like to understand the co-marketing side (blog, newsletter, webinar) and
the timing around listing.

## Being straight about what isn't finished

Two things are deliberately still open, and neither blocks the meeting:

- **The capacity number.** The load lane is built but not yet run against a dedicated instance,
  so the listing carries a provisional figure. That's question 4 above — I'd rather run it once
  to your specification.
- **The demo video.** A rough cut exists; I'm re-recording the final with voiceover.

## Meeting

I have a 60-minute agenda covering the above and can send it ahead so your developer can come
prepared — the technical questions are specific enough that pre-reading would save us time.

Would `[propose two or three concrete windows, with your timezone]` suit? I'll fit your calendar,
and I'm happy to do a deeper live demo than the one you saw — against a real AMS, showing an alert
fire end to end, viewer QoE from the beacon, and the 13-month analytics path.

Thanks for the offer to set this up — looking forward to it.

Best regards,
`[Your name]`
`[Title]`, Beyond Kaira
`[Phone, if you want it in the signature]`
support@beyondkaira.com · https://github.com/aytekXR/ams-pulse

---

## Operator notes — not part of the email

**Deliberately NOT included, and why:**
- **Pricing figures.** The listing has them, but leading with price invites a negotiation before
  they've agreed what Pulse *is*. Let the listing carry it.
- **Internal documents.** `listing-draft.md`, `submission-process.md`,
  `developer-meeting-brief.md`, `docs/assessment/**` and the handoff docs all stay internal. Every
  link in the email is a public, customer-facing document.
- **The old 20–30% revenue-share figure** from the PRD. It was never verified and is superseded
  by the publicly stated first-year 100%/no-commission terms — do not repeat it anywhere.
- **The 84.5% product-completeness score.** It's an internal metric and invites a question you
  gain nothing by answering.

**If he asks for attachments** rather than links, the shareable set is: the overview, the one-page
product brief (`developer-meeting-brief.md` § "One-page product brief"), the compatibility matrix,
and the public licensing page.

**Log every answer** into `docs/operator-expected.md` and close the A1–A10 rows in
[`submission-process.md`](submission-process.md) §2 afterwards — those assumptions are the reason
several docs still carry "⚠ ASSUMPTION — verify at the developer meeting" markers.
