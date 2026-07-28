# PulseApp resources — what is here, and what deliberately is not

Last updated: 2026-07-28

This file used to be a to-do list ("create these nine icon sizes in Xcode", "create
LaunchScreen.storyboard"). None of that is true any more, and a stale checklist is worse than no
checklist — someone follows it and adds work the project does not need. What follows is what the
app actually ships.

## App icon — one file per appearance, not nine sizes

`../Assets.xcassets/AppIcon.appiconset/` holds exactly two PNGs, both 1024×1024:

| File | Source | Used for |
|---|---|---|
| `app-icon-light.png` | `brandkit/icons/png/app-icon-ios-light-1024.png` | default appearance |
| `app-icon-dark.png` | `brandkit/icons/png/app-icon-ios-dark-1024.png` | dark appearance |

Xcode 14 and later derive every other size from the single 1024 asset, so the old
20/29/40/60@2x/@3x matrix is obsolete. The PNGs are **copied**, not referenced out of
`brandkit/` — a build must not reach outside the target for an asset it needs.

Do not add transparency: iOS app icons must be opaque, and the App Store rejects alpha.

## Launch screen — none, on purpose

`Info.plist` declares `UILaunchScreen` as an **empty dictionary**. That is a complete
declaration, not a placeholder: the system composes a launch screen from the app's background and
safe areas. There is no storyboard and none is needed.

The previous version of this file asked for a `LaunchScreen.storyboard`, and `Info.plist` named
one that was never created. An app that *declares* a launch screen it does not have is treated as
having none at all, and iOS then runs it letterboxed at a compatibility size — a defect that
shows up only in a screenshot on a real device.

## Fonts — system stack

`brandkit/design-system/tokens.json` specifies IBM Plex Sans and IBM Plex Mono. They are **not**
bundled: no OFL font files are committed anywhere in this repository (the web UI falls back the
same way). The app therefore renders in the system stack, and the token file's declared fallbacks
are what actually apply.

If they are ever bundled, they are self-hosted only — never a CDN — per `docs/ARCHITECTURE.md` §3:
drop the `.otf` files here, list them under `UIAppFonts` in `Info.plist`, and ship the OFL text.

## Accent colour

`../Assets.xcassets/AccentColor.colorset/` carries the brand signal colour in both appearances —
`#2CE5A7` dark, `#087A59` light — read from `tokens.json`. Those two values are not
interchangeable: the light-theme signal is darkened specifically to pass WCAG AA on a light
background, and that contrast table in `brandkit/documentation/design-rationale.md` §2 is binding.
