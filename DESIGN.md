---
omd: 0.1
brand: iCloud Relay
bootstrapped_from: linear.app
bootstrapped_at: 2026-09-12T11:45:00Z
---

# Design System Inspiration of Linear

## 1. Visual Theme & Atmosphere

iCloud Relay is a night-shift operations console for people who manage a fleet of iCloud Hide My Email accounts. The visual direction keeps Linear's quiet, near-black workbench and turns the product's memorable moment into a “signal rail”: every account row exposes its operational signal, alias load, and last validation at a glance. No gradient is permitted anywhere in the product UI.

The tone is precise, calm, and slightly technical. It should feel like a control surface that respects the operator's attention. No marketing hero, no decorative illustration, no decorative dots or ornamental marks, no glassmorphism. Data density is intentional; empty space separates decisions.

## 2. Color Palette & Roles

### Background Surfaces

| Token | Value | Use |
|---|---|---|
| `ink-950` | `#080b0d` | App canvas and login backdrop |
| `ink-900` | `#0d1215` | Sidebar, top bar |
| `ink-850` | `#12191d` | Primary panels |
| `ink-800` | `#172126` | Hover and selected surfaces |
| `ink-750` | `#1c292f` | Elevated row and modal surface |

### Text & Content

| Token | Value | Use |
|---|---|---|
| `text-primary` | `#ecf5f1` | Headings, primary labels |
| `text-secondary` | `#b0c0ba` | Supporting copy |
| `text-tertiary` | `#788c84` | Metadata and captions; AA-safe on panels |
| `text-faint` | `#788981` | Quiet hints; AA-safe on panels |

### Brand & Accent

| Token | Value | Use |
|---|---|---|
| `relay-mint` | `#7de2b2` | Primary action, healthy signal, focus |
| `relay-mint-strong` | `#b1f4d0` | Hover text and high-contrast accent |
| `relay-mint-wash` | `rgba(125, 226, 178, 0.12)` | Selected rows and soft status backgrounds |

### Theme Modes

The interface supports two explicit, global display modes: `深色` and `浅色`. The choice is text-labeled, persisted in browser `localStorage`, and applied before the first React render so login, the operator shell, and public share use the same mode. Both modes use opaque, solid surfaces only.

| Mode | Canvas / panel surfaces | Primary text | Ready | Pending | Error |
|---|---|---|---|---|---|
| `深色` | `#080b0d` / `#12191d` | `#ecf5f1` | `#7de2b2` | `#efc276` | `#ec8d86` |
| `浅色` | `#f2f6f4` / `#ffffff` | `#17211c` | `#176b4a` | `#956011` | `#ab3e39` |

The light-mode values are chosen for readable contrast on their intended surfaces. Theme selection changes presentation tokens only; ready, pending, error, and disabled statuses remain separate semantic roles.

### Status Colors

| Token | Value | Use |
|---|---|---|
| `status-ready` | `#7de2b2` | Active and validated |
| `status-pending` | `#efc276` | Pending authorization or review |
| `status-error` | `#ec8d86` | Error and destructive action |
| `status-disabled` | `#788981` | Disabled account and inactive alias |

### Border & Divider

Borders use `rgba(222, 244, 236, 0.10)` by default and `rgba(222, 244, 236, 0.16)` for focused or selected controls. Dividers are `rgba(222, 244, 236, 0.07)`. Avoid solid white borders.

### Light Mode Neutrals (for light theme contexts)

The product remains dark-first. Public share and future light contexts may use `#f3f7f5`, `#ffffff`, `#14211c`, and `#63736c`; they must retain the same mint signal and status semantics.

### Overlay

Modal backdrops use `rgba(3, 7, 8, 0.84)`. Panels remain opaque enough for text clarity; glassmorphism and backdrop blur are not used.

## 3. Typography Rules

### Font Family

Use Space Grotesk for interface text and headings. It brings a distinct geometric rhythm without resembling a default system dashboard. Use the browser monospace stack only for email addresses, account IDs, tokens, and technical metadata.

### Hierarchy

| Level | Size / Weight | Use |
|---|---|---|
| Display | `clamp(32px, 5vw, 52px)` / 600 | Login and section statement |
| Page heading | `28px` / 600 | Main page title |
| Panel heading | `15px` / 600 | Panel title and account name |
| Body | `14px` / 400 | Descriptions and controls |
| Meta | `12px` / 500 | IDs, timestamps, labels |
| Mono | `12px` / 500 | Addresses and machine data |

### Principles

Headings use tight tracking (`-0.04em`). Body copy stays compact and concrete. Never use all-caps for long labels; use a small uppercase eyebrow only for status or section context.

## 4. Component Patterns

### Actions

Primary buttons use `relay-mint` on `ink-950` text with a 6px radius. Secondary actions use an `ink-800` surface and a translucent border. Destructive actions use coral text and a coral wash, never an alarming full-red fill. Every action has a minimum 44px hit area.

### Navigation

The shell uses a 228px left rail on desktop. The wordmark is text-only: `iCloud Relay`. Navigation items combine a quiet glyph, label, and count badge. The active route uses a mint left rule and mint-wash background. On narrow screens, the rail becomes an off-canvas drawer with a visible menu button.

### Forms

Inputs are dark, high-contrast, and full-width. Labels sit above fields. Focus uses a two-layer mint ring. Validation is field-adjacent and blameless. Setup and add-account flows explain what the next credential enables.

### Data display

Tables are rows, not boxed spreadsheets. Each account row has a signal rail, account identity, alias load, validation age, and explicit actions. Selection is independent from navigation: the checkbox stops row navigation. Address text uses monospace to preserve scanability.

### Overlays

Use a centered dialog for irreversible confirmation and a right-side drawer for add/setup forms. Keep overlay motion to 180ms with no bounce. Escape closes non-destructive overlays.

### Feedback & Status

Prefer inline result summaries for batch operations. Toasts are reserved for short copy confirmations such as “Share link copied”. Errors state the actual failed operation and suggest the next action.

## 5. Layout Principles

### Spacing System

Use a 4px base: `4 / 8 / 12 / 16 / 20 / 24 / 32 / 40 / 48 / 64`. Page gutters are 24px desktop and 16px mobile.

### Grid & Container

The shell content uses `max-width: 1440px`, `margin: 0 auto`, and a consistent 24px desktop gutter. Dashboard summaries use a 4-column grid; the account matrix uses a single list with strong row rhythm. Detail pages use a two-column overview only when the viewport allows it.

### Whitespace Philosophy

Whitespace is reserved around decisions, not between every field. Keep rows dense enough for operators to compare accounts without scrolling through card chrome.

### Border Radius Scale

Use 6px for controls and rows, 10px for panels, 14px for login frame. Avoid pills except for status chips and counts.

## 6. Depth & Elevation

Depth comes from background luminance stepping: `ink-900 → ink-850 → ink-800 → ink-750`. Avoid large shadows on dark surfaces. Dialogs may use `0 24px 80px rgba(0,0,0,0.42)` plus the backdrop. A selected row is brighter, not floating.

## 7. Do's and Don'ts

### Do

- Make account state visible before the operator opens a detail page.
- Keep destructive actions explicit, grouped, and confirmable.
- Use real addresses and timestamps as the visual material of the product.
- Keep keyboard focus visible and modal escape behavior predictable.
- Use mint as a signal, not decoration.

### Don't

- Do not use purple gradients, generic glass cards, or brand-logo illustrations.
- Do not hide disabled accounts in a filter-only state.
- Do not use a spinner as the only loading communication.
- Do not turn every row into a rounded card with a shadow.
- Do not add glowing dots, ornamental marks, background patterns, or other elements without a product function.
- Do not copy Linear marketing copy or identity assets.

## 8. Responsive Behavior

### Breakpoints

| Name | Width | Key Changes |
|---|---|---|
| Mobile | `< 640px` | One-column summaries, compact table rows, off-canvas rail |
| Tablet | `640–1024px` | Collapsed rail, two-column summary grid |
| Desktop | `> 1024px` | Full rail, four-column summary grid, detail side panels |

### Touch Targets

Interactive controls are at least 44px high and icon buttons are 44px square. Row selection remains usable without relying on hover.

### Collapsing Strategy

The account matrix keeps identity and status visible; low-priority metadata compresses below 760px. Batch controls wrap and remain sticky at the bottom of the viewport on mobile. Detail tabs become horizontally scrollable.

### Image Behavior

No decorative images are used in the product UI. Icons are compact inline interface glyphs; they never carry meaning without a text label or accessible name.

## 9. Agent Prompt Guide

### Quick Color Reference

- Canvas: `#080b0d`
- Rail: `#0d1215`
- Panel: `#12191d`
- Selected surface: `#172126`
- Text: `#ecf5f1`
- Muted: `#788c84`
- Primary: `#7de2b2`
- Pending: `#efc276`
- Error: `#ec8d86`
- Border: `rgba(222,244,236,0.10)`

### Example Component Prompts

Create an account row on `#12191d` with a 3px mint signal rail, a monospace iCloud address, a compact alias load meter, a status chip, and a 44px action target. Use `#172126` on hover and no shadow.

### Iteration Guide

First validate signal hierarchy and action reachability. Then tune density, row separators, and empty states. If a control needs explanation, improve the label before adding decoration.

## 10. Voice & Tone

iCloud Relay speaks like a careful operator: direct, concrete, and quietly opinionated. Prefer “3 accounts need attention” over “Your accounts are in trouble”. Prefer “Disable 4 accounts” over “Take action”. Error text names the operation, not an imaginary failure.

## 11. Brand Narrative

<!-- omd:limitation Reference §11 requires project-specific facts. Replace before shipping; do not fabricate. -->

[FILL IN: project origin, founding context, and the specific operational problem iCloud Relay exists to solve.]

## 12. Principles

<!-- omd:limitation Reference §12 requires project-specific facts. Replace before shipping; do not fabricate. -->

[FILL IN: product principles for managing iCloud Hide My Email accounts, credential safety, and operator trust.]

## 13. Personas

<!-- omd:limitation Reference §13 requires project-specific facts. Replace before shipping; do not fabricate. -->

[FILL IN: 2–4 target operator segments and the job each needs to complete.]

## 14. States

| State | Treatment |
|---|---|
| Empty active accounts | Keep the canvas quiet. Explain that the first account can be connected from the primary action. |
| Empty disabled accounts | Explain that restored accounts return to the active matrix. Keep a restore-oriented action nearby when possible. |
| Loading | Use row-shaped skeletons with low-contrast shimmer. No blocking spinner for the whole shell. |
| Error | Use an inline banner with the failed operation and a retry action. Never say only “Something went wrong”. |
| Batch result | Show requested, completed, skipped, and failed counts in a persistent result panel until dismissed. |
| Keyboard focus | Use a visible mint outline with a darker outer keyline. |

## 15. Motion & Easing

Use `100ms` for hover, selection feedback, and button press; `180ms` for drawer/dialog and route transitions; and `280ms` only for first-paint section reveal and list stagger. Easing is `cubic-bezier(0.25, 0.1, 0.25, 1)`. No spring, bounce, overshoot, floating decorative animation, or gradient animation. Under `prefers-reduced-motion: reduce`, transitions and stagger collapse to `0ms` and all content is immediately visible.
