# Designer review — round 1

**Date:** 2026-09-12T13:48:28.2552044Z
**Artifact:** `web/src/pages/AccountDetail.tsx`, `web/src/components/Overlay.tsx`, `web/src/components/SelectMenu.tsx`, `web/src/index.css`
**DESIGN.md:** `DESIGN.md`
**DESIGN.md read at:** 2026-09-12T13:48:28.2552044Z
**Viewport:** both

## Summary

- BLOCK: 0
- WARN: 0
- FYI: 1

## Evidence

- Typography uses a Windows-friendly Chinese stack and computed body weight 500; this follows the explicit pending typography preference over the older Space Grotesk rule (`web/src/index.css:4-8`).
- The account header is one compact 72px desktop surface with back navigation, identity, status, real alias count, host, and password action (`web/src/pages/AccountDetail.tsx:49`).
- Dialogs and drawers preserve their DOM during the 180ms exit state, use no backdrop, and expose Escape handling (`web/src/components/Overlay.tsx:4-48`).
- Custom listboxes implement click-outside dismissal plus Enter, Space, Escape, Arrow, Home, and End keys; native `select` elements are absent (`web/src/components/SelectMenu.tsx:5-10`).
- All primary controls inherit visible focus, hover, active, and disabled behavior; tested mobile layout has 390px client and scroll widths with no horizontal overflow.
- Alias, active-account, and disabled-account lists paginate at 20 items per page (`web/src/pages/AccountDetail.tsx:11-26`, `web/src/pages/Accounts.tsx:20-51`, `web/src/pages/DisabledAccounts.tsx:8-18`).

## Issues

### [FYI] Legacy selectors remain in the recovered stylesheet

- **Location:** `web/src/index.css:1`
- **Rule:** Keep the shipped visual surface free of obsolete presentation concepts.
- **Evidence:** The minified baseline still contains unused selectors such as `.workspace-switcher` and `.relay-health`, while the corresponding DOM was removed.
- **Fix suggestion:** Remove these dead selectors during a later stylesheet formatting cleanup; they have no rendered effect and no user-visible text.

## Visual verification

- Desktop dark detail: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/detail-desktop.png`
- Drawer without backdrop: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/drawer-no-mask.png`
- Light custom dropdown: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/light-dropdown.png`
- Fixed toast: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/light-toast.png`
- Mobile detail: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/detail-mobile.png`
- Per-alias share entry: `.omd/runs/run-2026-09-12T11-44-14-938Z-icloud-relay-frontend-rewrite/browser-qa/share-button-panel.png`

## Verdict

**PASS** — no blocking or warning-level visual-system issue remains in the reviewed surfaces.
