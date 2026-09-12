# Designer review — round 2

**Date:** 2026-09-12T22:13:00+08:00
**Artifact:** `web/src/pages/Accounts.tsx`, `web/src/pages/AccountDetail.tsx`, `web/src/components/Overlay.tsx`, `web/src/components/SelectMenu.tsx`, `web/src/pages/SharePage.tsx`, `web/src/index.css`
**DESIGN.md:** `DESIGN.md`
**DESIGN.md read at:** 2026-09-12T22:13:00+08:00
**Viewport:** both
**Prior report:** `.reviews/designer-review-round-1.md`

## Summary

- BLOCK: 0
- WARN: 0
- FYI: 1

## Resolved findings and evidence

- Account identity now prefers the real sign-in address over the iCloud IMAP mailbox in both the matrix and detail header (`web/src/pages/Accounts.tsx:90`, `web/src/pages/AccountDetail.tsx:51`). Browser evidence reads `liuyuquan1101@gmail.com` in both places.
- Account alias load now resolves from the live alias collection rather than the stale persisted `0 / 0` summary (`web/src/pages/Accounts.tsx:35-52`). Browser evidence reads `77 / 97 个活跃`.
- The desktop detail action is a horizontal 95×44px control with `white-space: nowrap`, and the 1440px page has no horizontal overflow (`web/src/index.css:204-205`).
- Side panels have transparent presentation layers, use 180ms enter/exit motion, and dismiss from a document-level outside pointer event (`web/src/components/Overlay.tsx:36-54`, `web/src/index.css:10-30`). Browser evidence observed the `.closing` state immediately and removal after 180ms.
- Searchable custom listboxes provide a centered CSS chevron, filtering, click-outside dismissal, focus, and keyboard controls (`web/src/components/SelectMenu.tsx:5-13`, `web/src/index.css:90-132`). The group editor exposes the `搜索分组` field.
- The alias metadata editor keeps group and note values in a local draft and commits page metadata only after the local API succeeds (`web/src/pages/AccountDetail.tsx:18`, `web/src/pages/AccountDetail.tsx:39-40`, `web/src/pages/AccountDetail.tsx:61`). Cancel no longer leaks unsaved group state into the list.
- Private and public mail rows share a flat, divider-based hierarchy; native button side borders are explicitly removed (`web/src/index.css:184-185`, `web/src/index.css:211-228`).
- Static scans report zero user-visible `Relay` copy, native `<select>`, gradient, backdrop-filter, and dark overlay declarations under `web/src` and `web/index.html`.

## Issues

### [FYI] Legacy minified selectors remain in the base stylesheet

- **Location:** `web/src/index.css:1`
- **Rule:** Keep the maintained visual layer easy to audit.
- **Evidence:** Some non-rendered legacy class names remain in the original minified baseline.
- **Fix suggestion:** Reformat and prune the baseline in a separate maintenance change; this does not alter the current rendered surface.

## Visual verification

- Final desktop account matrix: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/accounts-final-desktop.png`
- Final desktop inbox: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/inbox-final-desktop.png`
- Final mobile account/detail checks: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/accounts-final.png`, `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/detail-final.png`

## Verdict

**PASS** — BLOCK=0 and WARN=0 across the corrected account, group, drawer, mail, and share surfaces.
