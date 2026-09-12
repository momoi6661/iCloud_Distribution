# Final QA — round 2

**Date:** 2026-09-12T22:13:00+08:00
**Artifacts:** `web/index.html`, `web/src/**`, `internal/account/**`, `internal/server/**`
**DESIGN.md read at:** 2026-09-12T22:13:00+08:00
**Prior reviews:** `.reviews/designer-review-round-1.md`, `.reviews/designer-review-round-2.md`
**Voice preset:** concise Simplified Chinese operator UI

## Rubric

| # | Item | Verdict | Evidence |
|---|---|---|---|
| 1 | Brand consistency | PASS | Rendered surfaces use solid dark/light tokens and restrained borders; current static scans find no gradient, backdrop filter, dark overlay, visible `Relay` text, or native select. Explicit pending user preferences override the stale backdrop and Space Grotesk statements in `DESIGN.md`. |
| 2 | Typography hierarchy | PASS | The account matrix and compact detail header use one clear page identity, body controls compute at medium weight, and headings use the stronger Windows-friendly stack in `web/src/index.css:4-8`. |
| 3 | Voice register | PASS | Visible labels are concise Simplified Chinese; local grouping copy explicitly says it does not modify or sync to iCloud in `web/src/pages/AccountDetail.tsx:53` and `web/src/pages/AccountDetail.tsx:61`. |
| 4 | Image / figure | PASS | Product surfaces contain no decorative images; functional icons are paired with text or accessible button names. |
| 5 | Cross-locale parity | PASS | Simplified Chinese is the sole product locale and `web/index.html` declares `zh-CN`; there is no secondary locale artifact to diverge. |
| 6 | Accessibility | PASS | Controls have visible focus, listboxes support keyboard navigation, panel Escape/outside dismissal works, desktop action height is 44px, and measured desktop/mobile pages have no horizontal overflow. |
| 7 | Performance | PASS | Production build emits no image/webfont payload. Alias metadata save no longer performs the 2–3 second full alias reload; it updates local state after the local request succeeds (`web/src/pages/AccountDetail.tsx:40`). |
| 8 | Links | PASS | Each active alias exposes a text-labeled share action; created links are normalized to an absolute URL and public inbox UI uses the same mail-row hierarchy (`web/src/pages/AccountDetail.tsx:44-45`, `web/src/pages/SharePage.tsx`). |

## Failed items detail

None.

## Acceptance evidence

- Production frontend build: PASS
- `go test ./...`: PASS
- `go vet ./...`: PASS
- Desktop 1440×900: Gmail main address, live `77 / 97`, 95×44px horizontal detail action, no horizontal overflow
- Mobile 390×844: no horizontal overflow; low-priority columns/actions collapse as designed
- Drawer: transparent presentation layer; outside click produces closing state and removes the panel after 180ms
- Group selector: searchable listbox with two current options and CSS chevron
- Inbox: seven rows rendered in live Web API acceptance before refresh; flat row UI and full-row click target verified
- Static UI scan: PASS

## External acceptance boundary

The current inbox is operating in Web API summary mode. Full IMAP body reading remains unverified because no valid App-specific password is stored; the user must enter a valid password once before real iCloud IMAP acceptance can be claimed.

## Verdict

**PASS** — all eight closed checks pass for the rebuilt local UI and code paths. The only unverified item is external IMAP body retrieval, which requires a valid credential and is not represented as completed.
