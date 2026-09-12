# Designer review — round 3

**Date:** 2026-09-13
**Artifact:** `web/src/pages/AccountDetail.tsx`, `web/src/components/PageLayout.tsx`, `web/src/components/Overlay.tsx`, `web/src/index.css`
**DESIGN.md:** `DESIGN.md` (re-read 2026-09-13)
**Viewport:** both

## Summary

- BLOCK: 2
- WARN: 2
- FYI: 1

## Issues

### [BLOCK] Custom dialogs do not manage focus
- **Location:** `web/src/components/Overlay.tsx:24-54`
- **Rule:** Component states and keyboard accessibility
- **Evidence:** Dialogs support Escape and outside-click closing, but do not move focus into the opened surface, contain keyboard focus, or restore focus to the invoking control.
- **Fix suggestion:** Add initial-focus and focus-return behavior plus a focus loop that does not require a visual backdrop.

### [BLOCK] Two interactive inputs are shorter than the 44px touch target
- **Location:** `web/src/index.css:131`, `web/src/index.css:180`
- **Rule:** Mobile responsiveness — interactive controls must be at least 44px high
- **Evidence:** The searchable select input is 38px high and the pagination jump input is 40px high.
- **Fix suggestion:** Raise both controls to `min-height: 44px` without adding visual decoration.

### [WARN] Alias rows expose too many equal-weight actions
- **Location:** `web/src/pages/AccountDetail.tsx:55`
- **Rule:** Spacing/layout and action hierarchy
- **Evidence:** An active alias row shows five full buttons; the group wraps below 1100px and can dominate the mailbox identity.
- **Fix suggestion:** Keep Inbox as the primary row action and move lower-frequency actions into a clearly labeled compact menu, while preserving direct access to Disable.

### [WARN] Component overrides use values outside the documented token scale
- **Location:** `web/src/index.css:18`, `web/src/index.css:52-53`, `web/src/index.css:127-128`, `web/src/index.css:165-167`
- **Rule:** Color, radius, and spacing token consistency
- **Evidence:** Several literal shadow colors and 8px/5px radii bypass the DESIGN.md scale.
- **Fix suggestion:** Introduce semantic elevation and radius tokens, then replace the literals.

### [FYI] DESIGN.md still describes product elements explicitly removed by the user
- **Location:** `web/src/components/PageLayout.tsx:31-38`
- **Rule:** Brand consistency
- **Evidence:** The artifact uses `iCloud 邮箱管理` and navigation labels without counts, while DESIGN.md still specifies `iCloud Relay` and navigation count badges.
- **Fix suggestion:** Fold the accepted pending preferences into DESIGN.md before the next strict brand review.

## Verdict

- **BLOCK** — keyboard focus management and undersized inputs should be fixed before treating the UI as fully polished.
