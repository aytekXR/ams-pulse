# iOS TestFlight Distribution Runbook

Last updated: 2026-07-28 — created in D-186 (S118), reconciled against the workflow it documents

This runbook walks the operator through every step required to distribute the
Pulse iOS app via TestFlight. It is written for someone who has never shipped
an iOS app before.

**Scope:** This document covers what needs to happen outside the codebase. The
app code, CI workflow, and project configuration are already in place. This is
the human-account-holder work.

---

## The Operator's Critical Path

Complete these steps in order. Each step unlocks the next.

1. Enroll in the Apple Developer Program (USD 99/year).
2. Register the bundle identifier `com.beyondkaira.pulse`.
3. Create the App Store Connect app record.
4. Generate an App Store Connect API key and download the `.p8` file.
5. Add the three secrets to GitHub repository settings (see section 6).
6. Push a tag matching `ios-v*` (e.g., `ios-v0.1.0`) to trigger CI.
7. In App Store Connect, add internal testers to the uploaded build.
8. Testers install TestFlight and accept the invitation.

**What happens automatically once each step is done:**

| After step | Automation |
|------------|------------|
| 5 (secrets added) | CI can archive and upload builds to TestFlight |
| 6 (push/tag) | Workflow builds, signs, and uploads to App Store Connect |
| 7 (testers added) | Apple sends email invitations; build is immediately available |
| 8 (tester accepts) | App appears in TestFlight; updates arrive automatically |

---

## Contents

1. [Overview](#1-overview)
2. [Apple Developer Program enrollment](#2-apple-developer-program-enrollment)
3. [App ID and bundle identifier](#3-app-id-and-bundle-identifier)
4. [App Store Connect app record](#4-app-store-connect-app-record)
5. [App Store Connect API key](#5-app-store-connect-api-key)
6. [GitHub Actions secrets](#6-github-actions-secrets)
7. [Xcode 26 and iOS 26 SDK requirement](#7-xcode-26-and-ios-26-sdk-requirement)
8. [Export compliance](#8-export-compliance)
9. [Privacy manifest](#9-privacy-manifest)
10. [Privacy policy and support URLs](#10-privacy-policy-and-support-urls)
11. [TestFlight distribution](#11-testflight-distribution)
12. [Common rejection causes](#12-common-rejection-causes)
13. [Triggering a release](#13-triggering-a-release)
14. [What the app does](#14-what-the-app-does)

---

## 1. Overview

### What the app is

The Pulse iOS app is a **read-only monitoring companion** for Pulse — the
self-hosted analytics and QoE platform for Ant Media Server. It connects to a
running Pulse server and displays:

- Live dashboard overview (viewers, publishers, protocol mix, stream count)
- Active stream list with per-stream health scores
- Alert history with severity and state
- Server health and component status

### Architecture

The iOS codebase is split into two layers:

| Package | Location | Builds on Linux | Purpose |
|---------|----------|-----------------|---------|
| **PulseKit** | `ios/PulseKit` | Yes | Foundation-only models, API client, formatters. No UIKit/SwiftUI. |
| **PulseApp** | `ios/` (XcodeGen) | No | SwiftUI views, AVKit integration, platform glue. Requires Xcode. |

PulseKit builds and tests on Linux via `swift test`. This is the verification
layer — if PulseKit tests pass on Linux, the API contract and business logic
are sound.

### What CI can do

The `.github/workflows/ios.yml` workflow:

- Validates PulseKit on Linux (fast, catches logic bugs)
- Builds PulseApp on macOS (proves the UI layer compiles)
- Archives for App Store distribution (when TestFlight job lands)
- Uploads to TestFlight automatically (when secrets are configured)

### What only a human can do

- Enroll in the Apple Developer Program
- Create the App ID and App Store Connect app record
- Generate the App Store Connect API key
- Configure GitHub secrets
- Answer export compliance questions
- Add testers and manage groups in TestFlight
- Respond to Beta App Review rejections

---

## 2. Apple Developer Program enrollment

### Individual vs. organization enrollment

**Cost:** USD 99/year for both enrollment types.

| Factor | Individual | Organization |
|--------|-----------|--------------|
| **Seller name on App Store** | Your personal legal name | Your organization's legal name |
| **Verification** | Apple ID with 2FA; may require ID scan | D-U-N-S number required; verification takes longer |
| **Timeline** | 24-48 hours typical | 5 business days for D-U-N-S + 2 business days for Apple sync + enrollment processing |
| **Team management** | Single account holder | Multiple team members with roles |

**Recommendation:**

- **Organization enrollment** displays your company name as the seller. This
  looks more professional for a B2B product like Pulse. It requires a D-U-N-S
  number.

- **Individual enrollment** displays your personal name. It is faster to set
  up. You can convert to organization later, but conversion requires a new
  enrollment and creates friction.

**The tradeoff is professional appearance vs. time to first build.** If you
already have a D-U-N-S number, organization enrollment adds about one week. If
you need to request a D-U-N-S number, budget two weeks.

### Enrollment steps

1. Go to [developer.apple.com/programs/enroll](https://developer.apple.com/programs/enroll/).
2. Sign in with your Apple ID (or create one with two-factor authentication).
3. For organizations:
   - Enter your organization's D-U-N-S number.
   - Confirm you have legal binding authority (owner, founder, executive, or
     authorized employee).
   - Apple verifies against Dun & Bradstreet records.
4. Complete payment (USD 99 or local equivalent).
5. Wait for enrollment to process. Your membership shows "Active" when complete.

**Source:** [Enrollment overview](https://developer.apple.com/help/account/membership/program-enrollment/) |
[D-U-N-S requirements](https://developer.apple.com/help/account/membership/D-U-N-S)

---

## 3. App ID and bundle identifier

### Creating the App ID

1. Go to [Certificates, Identifiers & Profiles](https://developer.apple.com/account/resources/identifiers/list).
2. Click **+** to register a new identifier.
3. Select **App IDs** then **App**.
4. Enter:
   - **Description:** `Pulse iOS App`
   - **Bundle ID:** Select **Explicit**, enter `com.beyondkaira.pulse`
5. Under Capabilities, no special entitlements are required for the current
   feature set. Skip to **Continue** then **Register**.

The bundle identifier `com.beyondkaira.pulse` is hardcoded in the XcodeGen
`project.yml` file. If you use a different identifier, update:

- `ios/project.yml` (the `PRODUCT_BUNDLE_IDENTIFIER` setting)
- The provisioning profile and signing settings in CI

---

## 4. App Store Connect app record

### App name uniqueness

The App Store app name must be **globally unique** across all apps on the store.
"Pulse" is almost certainly taken. You will need to choose an alternative.

**Suggestions (verify availability in App Store Connect):**

- `Pulse for AMS`
- `Pulse Analytics`
- `AMS Pulse`

The name can be changed later (with Apple review), but the initial name must be
available at creation time.

### Creating the app record

1. Go to [App Store Connect](https://appstoreconnect.apple.com/).
2. Click **My Apps** > **+** > **New App**.
3. Fill in:
   - **Platforms:** iOS
   - **Name:** Your chosen unique name
   - **Primary Language:** English (US) or your preference
   - **Bundle ID:** Select `com.beyondkaira.pulse` (from the dropdown)
   - **SKU:** Any unique string, e.g., `pulse-ios-001`
   - **User Access:** Full Access (or Limited if you have multiple apps)
4. Click **Create**.

The app record now exists. No content is visible to the public until you submit
a build and it passes App Review.

---

## 5. App Store Connect API key

The API key allows CI to authenticate with App Store Connect for automated
uploads. This replaces manual Xcode uploads.

### Creating the key

1. In App Store Connect, go to **Users and Access** > **Integrations**.
2. Ensure **App Store Connect API** is selected (it is by default).
3. Click **Team Keys** > **Generate API Key** (or **+**).
4. Enter:
   - **Name:** `GitHub Actions`
   - **Access:** `App Manager`
5. Click **Generate**.
6. **Download the `.p8` file immediately.** Apple shows this file exactly once.
   If you lose it, you must revoke and regenerate.

### Values CI needs

After generating the key, note three values:

| Value | Where to find it | Example format |
|-------|------------------|----------------|
| **Issuer ID** | Top of the Team Keys page, labeled "Issuer ID" | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` (UUID) |
| **Key ID** | Listed in the key row under "KEY ID" | `XXXXXXXXXX` (10 alphanumeric characters) |
| **Private key** | The `.p8` file you downloaded | Begins with `-----BEGIN PRIVATE KEY-----` |

**Source:** [Creating API Keys for App Store Connect API](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api)

---

## 6. GitHub Actions secrets

The `.github/workflows/ios.yml` workflow requires the following secrets for
TestFlight upload.

**Secret names (verified from ios.yml):**

| Secret name | Value | Source |
|-------------|-------|--------|
| `APP_STORE_CONNECT_ISSUER_ID` | The Issuer ID (UUID) | Users and Access > Integrations > Team Keys |
| `APP_STORE_CONNECT_KEY_ID` | The Key ID (10 chars) | Same page, listed in the key row |
| `APP_STORE_CONNECT_PRIVATE_KEY` | The `.p8` contents, **base64-encoded** | See encoding instructions below |

The workflow uses `-allowProvisioningUpdates` with the API key for automatic
signing, so no separate `APPLE_TEAM_ID` or provisioning profile secrets are
needed.

### Encoding the private key

The `.p8` file must be base64-encoded before adding as a secret:

```bash
base64 -i AuthKey_XXXXXXXXXX.p8 | pbcopy   # macOS: copies to clipboard
# or
base64 AuthKey_XXXXXXXXXX.p8               # Linux: prints to stdout
```

Paste the entire base64 string (no newlines) as the secret value.

### Adding secrets to GitHub

1. Go to your repository on GitHub.
2. Navigate to **Settings** > **Secrets and variables** > **Actions**.
3. Click **New repository secret**.
4. Enter the name exactly as shown above, paste the value, and click **Add secret**.
5. Repeat for all required secrets.

### Finding your Team ID

1. Go to [developer.apple.com/account](https://developer.apple.com/account/).
2. Under Membership, find **Team ID** (10 alphanumeric characters).

### What happens if secrets are missing

The `ios-testflight` job checks for all required secrets before attempting
upload. If any are missing, the job prints a notice listing the missing
secrets and skips all subsequent steps. The job succeeds (to avoid blocking
unrelated PRs) but logs:

```
SKIPPING TestFlight upload — missing secrets: APP_STORE_CONNECT_ISSUER_ID ...
See docs/mobile/ios-testflight.md for setup instructions.
```

This allows the repository to run CI without an Apple account configured while
making the gap obvious.

---

## 7. Xcode 26 and iOS 26 SDK requirement

**Apple requirement (in force since 2026-04-28):**

> Apps uploaded to App Store Connect must be built with Xcode 26 or later using
> an SDK for iOS 26, iPadOS 26, tvOS 26, visionOS 26, or watchOS 26.

**Source:** [developer.apple.com/news/upcoming-requirements](https://developer.apple.com/news/upcoming-requirements/)

### What this means for CI

The default Xcode on `macos-15` runners is **Xcode 16.4**. A build using the
default produces an artifact that **App Store Connect rejects at upload** —
long after CI has gone green.

The workflow **must** select Xcode 26.x explicitly before any archive or upload
step. The runner has Xcode 26.0 through 26.3.0 installed (see
`docs/mobile/ci-runner-facts.md` for the full list).

### Current CI configuration

The `ios.yml` workflow has three jobs:

1. **ios-kit** — Linux, Swift 6.1 container, builds and tests PulseKit
2. **ios-app** — macOS 15, Xcode 26.2.0, builds and tests PulseApp on iOS 26.2 simulator
3. **ios-testflight** — macOS 15, Xcode 26.2.0, archives and uploads to TestFlight

Both `ios-app` and `ios-testflight` pin Xcode 26.2.0 and assert the major
version is 26 before proceeding. The assertion pattern:

```yaml
- name: Select and assert Xcode 26.2
  run: |
    sudo xcode-select -s /Applications/Xcode_26.2.0.app/Contents/Developer
    XCODE_VERSION=$(xcodebuild -version | head -1)
    MAJOR=$(echo "$XCODE_VERSION" | grep -oE '^Xcode [0-9]+' | grep -oE '[0-9]+')
    if [ "$MAJOR" != "26" ]; then
      echo "::error::Xcode major version is $MAJOR, expected 26."
      exit 1
    fi
```

The `ios-testflight` job only runs on `ios-v*` tags and `workflow_dispatch`.

---

## 8. Export compliance

App Store Connect asks: _"Does your app use encryption?"_

### The correct answer for Pulse

**Yes, but it is exempt.**

The Pulse app uses HTTPS (TLS) for all network communication. HTTPS is
specifically exempted from export compliance documentation requirements because
it uses encryption built into the operating system via `URLSession`.

### How this is declared in the app

The `Info.plist` file includes:

```xml
<key>ITSAppUsesNonExemptEncryption</key>
<false/>
```

This tells App Store Connect that the app does not use non-exempt encryption,
which is true: HTTPS-only apps qualify for the exemption.

**Source:** [ITSAppUsesNonExemptEncryption](https://developer.apple.com/documentation/bundleresources/information-property-list/itsappusesnonexemptencryption) |
[Complying with Encryption Export Regulations](https://developer.apple.com/documentation/security/complying-with-encryption-export-regulations)

### If you add custom encryption

If you add encryption beyond HTTPS (e.g., local data encryption with custom
algorithms, certificate pinning with bundled keys), you must update the
`Info.plist` and potentially submit export compliance documentation.

---

## 9. Privacy manifest

Apple requires a privacy manifest (`PrivacyInfo.xcprivacy`) for apps that use
certain APIs categorized as "required reason APIs." These APIs include file
timestamps, user defaults, disk space, and system boot time.

### Does Pulse need a privacy manifest?

The PulseKit and PulseApp layers do not use required-reason APIs directly:

- No `NSFileManager` file modification/creation date access
- No `UserDefaults` (token is stored in Keychain, not UserDefaults)
- No `NSProcessInfo.systemUptime`
- No disk space APIs

**Current state:** No privacy manifest is required for the current feature set.

If future versions add UserDefaults or other required-reason APIs, a
`PrivacyInfo.xcprivacy` file must be added to the Xcode project.

**Source:** [Privacy manifest files](https://developer.apple.com/documentation/bundleresources/privacy-manifest-files) |
[TN3183: Adding required reason API entries](https://developer.apple.com/documentation/technotes/tn3183-adding-required-reason-api-entries-to-your-privacy-manifest)

---

## 10. Privacy policy and support URLs

App Store Connect requires two URLs for all apps:

### Privacy policy URL (required)

A publicly accessible page explaining what data the app collects (or that it
collects none).

**Suggested hosting locations:**

- Your company website (e.g., `https://pulse.example.com/privacy`)
- GitHub-rendered markdown: `https://github.com/<user>/<repo>/blob/main/docs/privacy-policy.md`

**Content for Pulse (minimal):**

> **Privacy Policy**
>
> The Pulse iOS app does not collect, store, or transmit personal data to any
> third party. The app connects only to servers you configure. Your API token
> is stored locally in the iOS Keychain and is never transmitted except to your
> own Pulse server. No analytics, advertising, or tracking SDKs are included.

### Support URL (required)

A page where users can get help. This can be:

- The GitHub repository issues page
- Your company support email or contact form
- A dedicated support page

**Source:** [Manage app privacy](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy/)

---

## 11. TestFlight distribution

### Internal testers (fast, no review)

**Limit:** Up to 100 App Store Connect users.

**Review:** None. Builds are available immediately after processing.

**How to add:**

1. In App Store Connect, open your app.
2. Go to **TestFlight** > click **+** next to Internal Testing to create a group.
3. Add testers by their Apple ID email (they must be App Store Connect users).
4. Once a build is uploaded and processed (~15 minutes), enable it for the group.
5. Testers receive an email to install TestFlight and the app.

**Use internal testers first.** This validates the upload pipeline and basic
functionality without waiting for Apple review.

### External testers (public beta, requires review)

**Limit:** Up to 10,000 people.

**Review:** Beta App Review is required for the **first build** submitted to an
external group. Subsequent builds on the same version may skip review.

**How to add:**

1. In App Store Connect, go to **TestFlight** > **External Testing** > **+**.
2. Create a group (e.g., "Public Beta").
3. Before adding testers, fill in required test information:
   - **Beta App Description:** What the app does and how to test it
   - **Feedback Email:** Where testers send feedback
   - **Privacy Policy URL:** (see section 10)
4. Add a build to the group. It is automatically submitted for Beta App Review.
5. Once approved (typically 24-48 hours), add testers by email or enable the
   public link.

**Source:** [TestFlight overview](https://developer.apple.com/help/app-store-connect/test-a-beta-version/testflight-overview/) |
[Add internal testers](https://developer.apple.com/help/app-store-connect/test-a-beta-version/add-internal-testers/) |
[Invite external testers](https://developer.apple.com/help/app-store-connect/test-a-beta-version/invite-external-testers/)

---

## 12. Common rejection causes

Based on Apple's Beta App Review patterns, these are the most likely rejection
causes for an app like Pulse:

### 1. Missing demo credentials

**The most common rejection.** The Pulse app requires a server URL and API token.
Apple reviewers cannot test without them.

**How to avoid:**

1. Provision a Pulse server accessible from the public internet.
2. Populate it with sample data (a few streams, alerts).
3. Create a read-only API token.
4. In App Store Connect, under **App Review Information** > **Notes for Reviewer**,
   provide:

   ```
   This app connects to a self-hosted Pulse server.

   Server URL: https://pulse-demo.example.com
   API Token: plt_abc123...

   On the connection screen, enter the Server URL, then enter the API Token.
   Tap Connect. The dashboard shows live monitoring data.
   ```

5. Keep the demo server running until review completes.

### 2. Missing privacy manifest

If the app uses required-reason APIs (file timestamps, user defaults, etc.)
without a privacy manifest, the build may be rejected.

**How to avoid:** Audit for required-reason APIs and add a privacy manifest if
needed (see section 9).

### 3. Beta description does not explain how to test

Apple rejects apps where the beta description says things like "internal test"
or "for developers only" without explaining what the app does.

**How to avoid:** Write a clear beta description:

> Pulse is a monitoring companion for Ant Media Server. It displays live viewer
> counts, stream health, alerts, and server status. To test, you need access
> to a Pulse server. Contact the developer for demo credentials.

### 4. Incomplete or placeholder content

TestFlight builds with obvious placeholder text, broken features, or incomplete
flows may be rejected.

**How to avoid:** Test internally first. Fix obvious bugs before submitting for
external testing.

**Source:** [Tips for preventing common review issues](https://developer.apple.com/videos/play/tech-talks/10885/)

---

## 13. Triggering a release

### Via git tag

Push a tag matching `ios-v*` to trigger a TestFlight upload:

```bash
git tag ios-v0.1.0
git push origin ios-v0.1.0
```

The workflow extracts the version from the tag (e.g., `ios-v0.1.0` becomes
marketing version `0.1.0`). The build number is the GitHub Actions run number
(automatically incrementing and guaranteed unique).

### Via workflow dispatch

1. Go to **Actions** > **ios** workflow.
2. Click **Run workflow**.
3. Select the branch.
4. Click **Run workflow**.

This triggers the same `ios-testflight` job as a tag push. Use this for ad-hoc
test builds.

### Manual upload (fallback)

If CI is unavailable:

1. On a Mac, run `xcodebuild archive` with your signing credentials.
2. Export the archive as an App Store IPA.
3. Use Transporter.app to upload to App Store Connect.

### Verification

After upload (manual or automated):

1. Go to App Store Connect > your app > **TestFlight**.
2. The build appears under **Builds** within ~15 minutes (processing time).
3. Enable it for your tester group.
4. Testers receive a notification in TestFlight.

---

## 14. What the app does

### Prerequisites for users

| Requirement | Details |
|-------------|---------|
| **A reachable Pulse server** | The server must be accessible from the device (public internet or same network). HTTPS is strongly recommended. |
| **An API token** | Obtained from the Pulse web UI at **Settings > API Tokens**. The token must have `kind: api` and at least `read` scope. |

### Getting an API token

1. Open your Pulse deployment in a web browser.
2. Sign in with your admin credentials.
3. Navigate to **Settings** > **API Tokens**.
4. Click **Create Token**, select `api` kind and `read` scope.
5. Copy the token (`plt_...`) immediately — it is shown only once.

The app stores this token in the iOS Keychain and sends it as a bearer token
(`Authorization: Bearer plt_...`) on every API request.

### App screens

| Tab | Shows |
|-----|-------|
| **Live** | Viewers, publishers, stream count, protocol mix; active stream list with health scores; tap stream to open player |
| **Alerts** | Alert history with severity (info/warning/critical), state (firing/resolved), breach details |
| **Settings** | Server URL, health status, component status, app version, sign out |

### Connection flow

1. Launch the app.
2. Enter your Pulse server URL (e.g., `https://pulse.example.com`).
3. Enter your API token.
4. Tap **Connect**.

The app validates the token against the server. If successful, it navigates to
the Live dashboard.

---

## Appendix: Runner facts

The following values are measured from actual GitHub Actions runner runs. They
are documented in `docs/mobile/ci-runner-facts.md` and should be re-verified
after GitHub updates the `macos-15` image.

| Fact | Measured value |
|------|----------------|
| Runner label | `macos-15` |
| Image | `macos-15-arm64`, release `20260715.0234` |
| OS | macOS 15.7.7 (arm64) |
| Default Xcode | **16.4** (not sufficient for App Store upload) |
| Installed Xcodes | 16.0-16.4, 26.0, 26.0.1, 26.1, 26.1.1, 26.2.0, 26.3.0 |
| iOS simulator runtimes | 18.5, 18.6, 26.0, 26.1, 26.2 (no iOS 17) |
| `brew install xcodegen` | Works; XcodeGen 2.45.4; ~4.5s |

---

## Appendix: Verified vs. UNVERIFIED claims

| Claim | Status | Source |
|-------|--------|--------|
| Enrollment costs USD 99/year | Verified | [Apple Developer enrollment](https://developer.apple.com/programs/enroll/) |
| D-U-N-S takes 5 business days | Verified | [D-U-N-S requirements](https://developer.apple.com/help/account/membership/D-U-N-S) |
| Internal testers: up to 100 | Verified | [TestFlight overview](https://developer.apple.com/help/app-store-connect/test-a-beta-version/testflight-overview/) |
| External testers: up to 10,000 | Verified | Same source |
| First external build requires review | Verified | Same source |
| HTTPS-only apps are export exempt | Verified | [Encryption export regulations](https://developer.apple.com/documentation/security/complying-with-encryption-export-regulations) |
| Xcode 26 required since 2026-04-28 | Verified | [Upcoming requirements](https://developer.apple.com/news/upcoming-requirements/) |
| API key roles (App Manager) | Verified | [Creating API keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api) |
| GitHub secret names | Verified | Confirmed in `.github/workflows/ios.yml` as of 2026-07-28 |

All claims marked "Verified" have been checked against Apple's official
documentation as of 2026-07-28.
