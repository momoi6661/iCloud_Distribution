---
schema: omd.preferences/v1
design_md_hash_at_creation: 41e30d9190f050dfd8a3c42a10b06f2e95ae099b426607b0be43188041cb416b
---

# Preference Log

## 2026-09-12T11:52:49.613Z — never-use-gradients

```omd-meta
id: pref_mtybsunh_567d7c14
timestamp: 2026-09-12T11:52:49.613Z
scope: color
signal: user-correction
confidence: explicit
status: pending
source_agent: codex
source_context: "full product UI rewrite"
```

Never use gradients; build a premium palette with solid layered surfaces and restrained highlights.

## 2026-09-12T11:52:49.613Z — purposeful-premium-motion

```omd-meta
id: pref_mtybsuqa_6c34ca43
timestamp: 2026-09-12T11:52:49.613Z
scope: motion
signal: user-statement
confidence: explicit
status: pending
source_agent: codex
source_context: "full product UI rewrite"
```

Use polished, purposeful motion for state transitions and interaction feedback without decorative floating or bounce.

## 2026-09-12T13:17:55.298Z — overlays-never-use-backdrops

```omd-meta
id: pref_mtyeuaao_4ce7e05b
timestamp: 2026-09-12T13:17:55.298Z
scope: components.dialog
signal: user-correction
confidence: explicit
status: pending
source_agent: codex
source_context: "web/src overlay and drawer components"
```

Dialogs, drawers, and confirmation panels never use a dimming or black backdrop; keep background content visible and use restrained enter and exit motion.

## 2026-09-12T13:21:16.663Z — readable-medium-weight-typography

```omd-meta
id: pref_mtyeyloc_9ad4bc9d
timestamp: 2026-09-12T13:21:16.663Z
scope: typography
signal: user-correction
confidence: explicit
status: pending
source_agent: codex
source_context: "web/src global interface typography"
```

Use a Windows-friendly Chinese font stack with medium-weight body text and stronger headings; do not use thin or low-contrast typography to create hierarchy.

## 2026-09-12T16:26:19.218Z — active-rows-avoid-redundant-status

```omd-meta
id: pref_mtylkka1_e32a5799
timestamp: 2026-09-12T16:26:19.218Z
scope: components.table
signal: user-correction
confidence: explicit
status: pending
source_agent: codex
source_context: "web/src/pages/AccountDetail.tsx"
```

Active mailbox rows do not repeat an enabled status label when the active list and signal rail already communicate that state; retain an explicit label only for disabled rows.

## 2026-09-12T16:28:49.845Z — centered-pagination-and-honest-loading

```omd-meta
id: pref_mtylnsi0_3f595def
timestamp: 2026-09-12T16:28:49.845Z
scope: components.table
signal: user-correction
confidence: explicit
status: pending
source_agent: codex
source_context: "web/src list pagination and alias loading states"
```

Data-list pagination is centered and supports direct numeric page entry; lists show a dedicated loading state instead of briefly presenting unloaded data as empty.

## 2026-09-12T16:29:00.862Z — navigation-avoids-duplicate-counts

```omd-meta
id: pref_mtylo109_a05a5ffc
timestamp: 2026-09-12T16:29:00.862Z
scope: components.navigation
signal: user-correction
confidence: explicit
status: pending
source_agent: codex
source_context: "web/src/components/PageLayout.tsx"
```

Sidebar navigation does not repeat account counts that already appear in the destination page content.
