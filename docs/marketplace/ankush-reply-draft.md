# DRAFT — reply to Ankush Banyal (developer-meeting request)

> Loop-drafted 2026-07-25 (D-169/S103). **The operator sends this from their own account — do NOT
> send on their behalf.** Fill the [bracketed] blanks. Content is derived from
> `docs/marketplace/developer-meeting-brief.md`.

**To:** Ankush Banyal [ankush@antmedia.io — CONFIRM the correct address]
**Subject:** Pulse is ready for the marketplace — requesting the developer meeting

---

Hi Ankush,

Following up on your offer to set up a developer meeting once Pulse was fully ready — it is, and
the documentation package is complete, so I'd like to take you up on it.

**Where things stand:**

- **Pulse v0.4.3 is released and publicly installable.** The image is published and public on GHCR
  (`ghcr.io/aytekxr/ams-pulse:0.4.4`, multi-arch, cosign-signed); a one-command Docker Compose
  quickstart brings up the full stack in ~15 minutes. I verified the anonymous install path
  end-to-end this week — a fresh pull with no credentials reaches a live dashboard.
- **Integration is read-only.** Pulse polls AMS REST v2 (plus optional webhook/Kafka), never modifies
  AMS, and keeps all customer data on the customer's own infrastructure — no SaaS backend, no
  phone-home. It's validated live against AMS 3.0.3 Enterprise (46/50 scenarios).
- **The full documentation set is ready to share** — evaluator overview, install guide, user &
  administrator guides, API reference, compatibility matrix, known limitations, and security policy.

**What Pulse does, in one line:** self-hosted analytics, QoE monitoring and alerting for Ant Media
Server — a live ops dashboard, 13-month historical analytics, alerting to Slack/email/Telegram/
PagerDuty/webhook, a lightweight player beacon for real viewer-side QoE, usage/billing reports,
cluster fleet view, synthetic probes, anomaly detection, and a Prometheus endpoint.

**For the meeting**, I'd love to (1) give you a deeper live demo than the one you saw, (2) close a
couple of technical compatibility questions on my side, and (3) walk through the marketplace
qualification and listing process — packaging/format expectations, the review flow, and business
terms (revenue share, API-stability commitment). I have a 60-minute agenda prepared and can send it
ahead of time.

Would [next week / a specific window — FILL IN] work for a call? Happy to fit your calendar.

Thanks,
[Operator name]
[Beyond Kaira — title, contact]

---

*Attachments to offer (do not attach until the operator decides what is externally shareable — the
internal assessment/process docs stay internal): the evaluator overview, the one-page product brief
(`developer-meeting-brief.md` §"One-page product brief"), and the public licensing page.*
