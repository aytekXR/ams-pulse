# SESSION-111 — 2026-07-27 — marketplace-integration readiness: submission pack audited against the published artifacts

**Decision:** D-178. **Operator directive:** *"get ready for the marketplace integration."*

Every remaining item in the submission queue is an operator **action** (rotate the credential,
submit the listing, set up billing, reply to Ankush). So this session took the engineering half:
**verify that nothing on our side can block or embarrass the submission**, by testing the
published `v0.4.3` artifacts the way an Ant Media qualification reviewer will — anonymously,
from outside the repo — rather than re-reading our own documentation.

That distinction mattered. Six defects were found, one of them the kind that fails a security
review while the underlying artifact is perfectly sound.

## What was verified against the published release (all green)

| Check | Evidence |
|---|---|
| Anonymous image pull | `0.4.3` and `latest` → same digest `sha256:75a76c67…727b4`, HTTP 200 with no auth |
| Anonymous Helm OCI pull | `helm pull oci://ghcr.io/aytekxr/charts/pulse --version 0.3.1` → digest `sha256:f589d3f4…`, chart `0.3.1` / appVersion `0.4.3` |
| **Clean-room evaluator install of v0.4.3** | `curl … install.sh` (from `main`, which pins `PULSE_REF=v0.4.3`) → stack healthy → all three `/healthz` components `ok` → `/fleet/nodes` returned the **real AMS 3.0.3 node** (`version 3.0.3`, Linux/amd64, java 17, 6 CPUs). Pulled digest matched the anonymously-resolved manifest. Torn down with `down -v`; no residue. |
| `install.sh` integrity | `raw.githubusercontent.com/…/v0.4.3/…/install.sh` is byte-identical to the tagged tree |
| Release binaries | `sha256sum -c SHA256SUMS` → both OK; `./pulse-linux-amd64 version` → `v0.4.3 (commit 669952ed)` |
| Signature (with the right client) | cosign **v3.0.2** verify PASSES — claims, Rekor inclusion, Fulcio chain all validated |
| Docs link integrity | 96 relative links across the marketplace + public doc set — zero broken |
| Listing copy hygiene | zero internal identifiers (`D-0NN`, `LIM-NN`, session/agent IDs), zero placeholders; title 42/60 and short description 240/250 chars — both stated counts exact |
| Listing screenshots | all 6 + light variant present; `ss1-dashboard.png` opened and read panel-by-panel |

## Findings and fixes

| ID | Sev | Finding | Fix |
|---|---|---|---|
| **M-01** | **HIGH** | **The `cosign verify` command we publish fails.** A cosign **v2** client returns `Error: no signatures found` against a correctly signed image. From **`v0.3.0`** on, the signature is published as an **OCI 1.1 referrer** instead of the legacy `sha256-<digest>.sig` tag. Boundary established empirically: cosign **v2.4.3 fails** on `0.3.0`…`0.4.3` and **passes on `0.2.0`** (Rekor logIndex 2128354996 — the same one D-070 recorded); cosign **v3.0.2 passes** on `0.4.3`. A marketplace security reviewer running our own documented command concludes the image is unsigned. | Client-version requirement documented at every published location: `README.md`, `release.yml` (header + signing step), `values.yaml`, `submission-package.md` — each stating the symptom, the cause, and the verified evidence. |
| **M-02** | MED | `listing.md` told Ant Media the screenshots were *"captured from a live deployment."* They are route-mocked captures (`capture-live-screenshots.mjs` → vite preview, **no backend**). The internal `listing-draft.md` was honest; the copy actually being submitted was not. | Reworded to "the shipping Pulse web UI running representative demo data (not a customer's production instance)" — accurate, and no weaker. `submission-package.md` row corrected to match. |
| **M-03** | MED | The OCI Helm chart is **published on every release and anonymously pullable** — and documented **nowhere** user-facing. `listing.md` actively undersold it as "chart available in-repo; install from a local chart path." | `helm install pulse oci://ghcr.io/aytekxr/charts/pulse --version 0.3.1` documented in `install.md`, the chart README and `listing.md`, with the chart-semver-vs-appVersion distinction spelled out. |
| **M-04** | LOW | `submission-package.md` named chart `0.3.0`; the v0.4.3 release ships `0.3.1`. | Corrected to `0.3.1`. |
| **M-05** | LOW | `install.md` clean-install note still said "re-verified against `0.4.2`". | Now records the `0.4.3` verification, including the digest match. |
| **M-06** | LOW | The capture script's header said its output directory is "gitignored — safe to write PNGs there". The PNGs are **committed** (S105/D-172), so a re-run silently dirties tracked files. | Header corrected, and it now states the mock-provenance rule so the M-02 wording cannot silently regress. |

M-01 is the one worth remembering. The artifact was never defective — signing succeeded, the
signature is valid and transparency-logged. What was defective was **the instruction we hand a
reviewer.** No amount of re-reading our own docs would have surfaced it; only running the
published command with a client version we had not thought about did.

## Deliberate non-actions

- **No release cut.** Every fix is documentation or comment; none changes the image. D-177
  ruled that round-5 copy/hardening items "ride the next release," and nothing found here
  overturns that. `v0.4.3` remains the submission target. The `v0.4.3`→`main` doc delta is
  recorded in the handoff so it is a known state, not a surprise.
- **No change to how releases are signed.** The OCI 1.1 referrer layout is the standards-track
  direction; reverting to legacy tag mode to satisfy old clients would silently change what
  every downstream verifier must look for. Documented instead, with a comment in `release.yml`
  warning against "fixing" it reflexively.
- **No credential rotation, no prod roll.** Operator-gated. The ClickHouse password was
  re-checked (silently) and is **still un-rotated** — the live value's 32-hex prefix still
  appears in 2 commits of public history.

## Also shipped

**`docs/assessment/EXTERNAL-REVIEW-PROMPT.md`** — a standing, reusable brief for external
reviewers (operator request). It enforces **black box → documentation → code** ordering, with
an explicit rule that the source tree stays closed until the black-box phase is written up,
because reading the code first destroys that evidence for the round. It mandates a **single**
output file whose schema the maintenance loop consumes directly: falsifiable claims, reproduction
commands, `file → symbol` citations instead of drift-prone line numbers, an empty
**Disposition** column the maintainer fills, and a prior-round re-audit table — so the completed
file becomes the input for the next round. It also fences off the standing, already-disclosed
items (LIM-10, provisional capacity, operator-gated actions) so rounds stop re-litigating them.
