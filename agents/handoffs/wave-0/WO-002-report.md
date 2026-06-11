# WO-002 Completion Report — SDK-01

**Work order:** WO-002  
**Agent:** SDK-01  
**Date:** 2026-06-11  
**Status:** DONE

## Summary

Fixed the mismatch between the filenames tsup emits and what `package.json` referenced.
tsup derives output names from the entry filename (`src/index.ts` → `index.*`), but
`package.json` pointed to `pulse-beacon.*` everywhere — causing size-limit to abort
with "file not found".

## Changes

`sdk/beacon-js/package.json` only (no SDK source changes):

| Field | Before | After |
|---|---|---|
| `main` | `./dist/pulse-beacon.cjs` | `./dist/index.cjs` |
| `module` | `./dist/pulse-beacon.js` | `./dist/index.js` |
| `size-limit[0].path` | `dist/pulse-beacon.js` | `dist/index.js` |

`types` was already correct (`./dist/index.d.ts`).

## Verification

```
$ cd sdk/beacon-js && npm run build && npm run size

> @pulse/beacon@0.1.0 build
> tsup src/index.ts --format esm,cjs,iife --dts --minify

CLI Building entry: src/index.ts
CLI tsup v8.5.1
CLI Target: es2020
ESM dist/index.js       114.00 B
CJS dist/index.cjs      602.00 B
IIFE dist/index.global.js 113.00 B
DTS dist/index.d.ts     1.55 KB
DTS dist/index.d.cts    1.55 KB

> @pulse/beacon@0.1.0 size
> size-limit

  Size limit: 15 kB
  Size:       88 B  with all dependencies, minified and gzipped
```

Both commands exit 0. Size 88 B << 15 KB gate.

## Acceptance criteria

- [x] `npm run build && npm run size` exits 0
- [x] Size reported (88 B gzipped) is under 15 KB
- [x] `main`, `module`, and `size-limit` all reference files tsup actually emits
- [x] No SDK source files modified

## Gaps (out of scope)

None.
