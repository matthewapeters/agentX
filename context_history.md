# CONTEXT HISTORY APPLET

This guide is to provide design concepts for the AgentX TUI (Go) implementation.

- Preserve the GUI's direct, nested feel in keyboard form: stacked boxes, scrollable viewports, emoji state markers, and pointer glyphs are part of the intended look/feel.
- The applet should attempt to manage available viewable space by using viewport scrolling within the active area
- Collapse/Expansion state is not persisted between sessions
- Within any given level of the applet, only one element may have focus at a time.  Focus and drill down within an element, but not down across multiple sibling elements.  Thus, there cannot be a scenario of mixed-focus state.

Example of Widgets in Context History applet. NOTE: widgets are shown
expanded for detail; default shipped state starts with collapsed history and
working-memory sections, with current-context expanded at section level.

```txt
 ↳ 🗄️ CONTEXT HISTORY
   ┌─────────────────────────────────────────────────────┐
   │ ↳ 📂 mpeters                                        │
   │   ┌───────────────────────────────────────────────┐ │
   │   │↳ 📑 2026-06-25 11:12:24                       │ │
   │   │  ┌────────────────────────────────────────┐ # │ │
   │   │  │   💾 Working memory ...              # │ ░ │ │
   │   │  │   🧠 System Prompts ...              ░ │ ░ │ │
   │   │  │   👤 Why are all of the items ...    ░ │ ░ │ │
   │   │  │   🤔 User wants to know why all ...  ░ │ ░ │ │
   │   │  │   🔧 8 tools called ...              ░ │ ░ │ │
   │   │  │ ▶ 🤖 The items are all this way ...  ░ │ ░ │ │
   │   │  └────────────────────────────────────────┘ ░ │ │
   │   │                                             ░ │ │
   │   │  2026-06-24 11:12:24                        ░ │ │
   │   │  ┌────────────────────────────────────────┐ ░ │ │
   │   │  └────────────────────────────────────────┘ ░ │ │
   │   │                                             ░ │ │
   │   │  2026-05-06 09:10:11                        ░ │ │
   │   │  ┌────────────────────────────────────────┐ ░ │ │
   │   │  └────────────────────────────────────────┘ ░ │ │
   │   │                                             ░ │ │
   └─────────────────────────────────────────────────────┘

↳  💾 WORKING MEMORY
   ┌─────────────────────────────────────────────────────────────┐
   │ KEY                       VALUE                             │
   │ ┌───────────────────────┐ ┌───────────────────────┐ ┌─┐     │
   │ │                       │ │                       │ │↳│     │
   │ └───────────────────────┘ └───────────────────────┘ └─┘     │
   │   ✔️ user: mpeters                                        # │
   │   ✔️ current working directory: ~/projects/agentX         ░ │
   │   ⬜ timezone: US/Pacific                                 ░ │
   └─────────────────────────────────────────────────────────────┘

↳ 📑 CURRENT CONTEXT
  ┌──────────────────────────────────────────┐
  │    ✔ 👤 I have to ask why all our ...    │
  │    ✔ 📎 2 attachments ...                │
  │   ⬜ 🤔 Classified as complex task       │
  │   ⬜ 💭 The user wants me to look at ... │
  │   ⬜ 🔧 3 tools used ...                 │
  │ ▶  ✔ 🤖 response: [collapsed] [enabled]  │
  └──────────────────────────────────────────┘
```

## Context Persistence

AgentX uses the `.sessions` folder for persisting user sessions and their contexts. The existence of a local `./.sessions` folder takes precedence over the user's home `.sessions` folder.

The folder structure is:

```txt
.sessions/
  <user_name>/
    session_YYYY-MM-DD_HH-MM-SS/
      context/
        {epoch}_{message_id}.json
        {epoch}_{message_id}.json
        ...
        working_memory.json
      session.log
```

## BORDERS

### Collapsed Boxes

Boxes are still shown, but have no height

Example:

```txt
   │   │  2026-06-24 11:12:24                         │ │
   │   │  ┌────────────────────────────────────────┐  │ │
   │   │  └────────────────────────────────────────┘  │ │
```

## Pointers and Emojis

| Symbol | Meaning |
| ---- | ---- |
| ▶ | Active section pointer in header navigation |
| ↳ | Active inline affordance marker (for example, action cell) |
| 🗄️ | Context History Widget |
| 📂 | User Context History Widget |
| 📑 | Context Widget |
| 💾 | Working Memory Widget |
| 👤 | User Prompt Widget |
| 📎 | Attachment Widget |
| 🤔 | Prompt Classification Widget |
| 💭 | Thinking Widget |
| 🔧 | Tools Widget |
| 🤖 | Agent Response Widget |
| 🧠 | System Prompts Widget |
| ⬜ | Disabled Element |
| ✔️ | Enabled Element |

## Navigation Controls

- The three context-feedback sections render in this fixed order: `CONTEXT HISTORY` -> `WORKING MEMORY` -> `CURRENT CONTEXT`.
- Outside section focus (`insideSection=false`):
  - `Up/Down` moves the active section header cursor.
  - `Space` toggles expand/collapse for the active section.
  - `Tab` drills into the active section by one level, expanding the target section if needed and placing focus on that expanded target.
  - `Enter` is action-only and does not perform drill-in or node expand/collapse.
- Inside section focus (`insideSection=true`):
  - `Up/Down` moves row selection within the active section.
  - `Left/Right` moves horizontal siblings where available (for example prompt <-> response, session <-> first turn).
  - `Space` performs peek/expand on the focused section or node: it toggles the focused node branch visibility without moving focus.
  - `Enter` performs action only on the focused element/cell:
    - enable/disable a focused element,
    - commit a focused cell value and advance focus to the next cell,
    - save a Working Memory key/value pair when focus is on the Save cell.
  - `PgUp/PgDn` scrolls wrapped text content when the active row is an expanded `current-context` text entry; otherwise it pages the row cursor by 5.
  - Section-specific note: in `context-history` and `working-memory`, `PgUp/PgDn` always pages row selection by 5 rows (no separate textbox scroll mode).

### Toggle Options

Expand/Collapse and selection are context specific. The `Space` key only toggles the currently focused target.

#### Expansion / Collapse

To toggle expansion or collapse without entering a section, use `Space` while outside section focus. This preserves section-level navigation and does not enable row-level navigation until the section is entered.

#### Enable / Disable

To toggle an element enabled/disabled, use `Enter` while inside section focus on `current-context` and `working-memory`. In `context-history`, `Space` is reserved for node peek/expand.

### Enter Exit Expandable Areas (Focus)

Drill In / Enter area: `Tab`. `Tab` drills in one level; if the target section/node is collapsed, it is expanded and focus is moved to that expanded target.

Drill Out / Exit area: `Shift-Tab`. `Shift-Tab` backs out one level and collapses the exited node.

For deep context-history focus paths (user -> session -> turn), each `Shift-Tab` backs out exactly one level and collapses the node being exited (turn -> session, session -> user, user -> section). When backing out from the section root, section focus exits and the section remains collapsed.

## Layout and Default State (Shipped)

- Header clutter lines are intentionally omitted from the context-feedback render (no persistent `Controls:` or `Status:` banner rows).
- `CONTEXT HISTORY` starts collapsed and renders summary metadata when collapsed (users/sessions/sort summary).
- `WORKING MEMORY` starts collapsed and renders summary metadata when collapsed (facts summary).
- `CURRENT CONTEXT` starts expanded at section level, with prompt/response rows collapsed by default.
- When collapsed, section content remains discoverable via visible box stubs and summary metadata.
- Collapsed sections still render visible box stubs (`┌ ... └`) instead of disappearing completely.
- Section headers reserve pointer column width for stable alignment when not active.

## Status Vocabulary (Shipped)

Keyboard-driven transitions use normalized status text. Representative statuses:

- `Entered section: <section>.`
- `Exited section: <section>.`
- `Selection moved.`
- `Selection at first row.`
- `Selection at last row.`
- `Viewport moved down: context history.`
- `Viewport moved up: context history.`
- `Viewport moved down: working memory.`
- `Viewport moved up: working memory.`
- `History node expanded.`
- `History node collapsed.`

## Context History Widget

- users are listed in alphabetical order; a system may support more than one user, their sessions are segregated in the `.sessions/` folder by user name.
- For each user, session ordering is presentation-only and deterministic.  The default sort is `Ascending` to match the current loader; `Descending` may be selected when users want most-recent-first browsing.  The active order is controlled by `agentx.context_history_session_sort` in `agentx.toml`.
  Context-history nodes (`user`, `session`, `turn`) are applet-owned model nodes with explicit IDs/parent interfaces and are expanded/collapsed with `Space` while inside section focus.
- Facelift details for the Go TUI variant:
  - section headers are iconized for quick scanning (`🗄️ Context History`, `💾 Working Memory`, `📑 Current Context`)
  - expanded context history surfaces a compact summary line (`sessions`, active `sort`, and keyboard hint)
  - each session card includes absolute timestamp plus relative age (`Xd/Xh/Xm ago`) for faster recency parsing
  - state labels are color-coded (`collapsed` vs `expanded`) to reduce visual ambiguity
- Focus is singular (one active row at a time). Expansion state may be multi-open per row/section unless a section-specific behavior explicitly constrains it.
- A context must be in section focus to enable row navigation and viewport scrolling.
- In context-history row focus:
  - `Space` performs peek/expand: it toggles the focused history node branch visibility without moving focus.
  - This yields deterministic toggle behavior for user/session nodes based on current focus path.
  - Pressing `Down` on a section with only one user row is a no-op and does not implicitly descend into sessions.

## Working Memory Widget

- Working memory should be persisted in a JSON file within the current context folder in the file system, along with enabled/disabled/deleted state.
- existing working keys/values are read-only displays
  - Working Memory (scrollable view port with scrollbar & thumbnail)
- if the Working Memory exceeds the expanded window's area, show scrollbar and thumbnail. Up/Down arrows move row selection, and `PgUp/PgDn` pages row selection by 5 through these KV pairs.
- User may enable or disable working memory elements by using `Enter` on the focused element. Enabled elements will be used in the next prompt context. Enabled and disabled state are remembered for the duration of the session and the user can change them between prompts.
- To remove a KV pair from working memory, the user selects the pair and hits the "DELETE" key. This will not delete the pair from the file persistence, but will change the state to "deleted" and will not be displayed on [re]rendering.  NOTE: if a user want to un-delete a working memory KV pair, they must edit the persistence file and modify it there.  Disable is the better option for "soft delete"

### Context History Widget Working Memory Version

- The working memory widget within the Context History widget does not allow users to change values and remains read-only in this surface.
  This surface supports review only; editing and save actions belong to the applet-level Working Memory editor.
- there are four affordances in the area:
  - Key entry cell (user enters key name)
  - Value entry area (user enters value to map to key)
  - Save "button" (activated by Enter)
  - the scrollable KV Pair list
- The Key, Value, and Save cells do not scroll.

### Applet-Level Working Memory Widget

The applet-level Working Memory Widget supports editing of items for the current context.

- When the area is in focus, the Key cell has first focus, the Value cell as second focus, and the Save cell as third focus. Finally, the scrollable Working Memory viewport has fourth focus. Focus wraps back and forth. User can use Left and Right arrow keys to move between these areas. Active cell and title are bright-colored, inactive are muted.
- `Enter` in Key/Value cells commits the current cell value and advances focus to the next cell.
- `Enter` on the Save cell saves the Working Memory key/value pair.

## Context Widget

The context widget shows each of the elements of an agentX session in the order they occur. Prompt and response rows are collapsed by default. When expanded, full wrapped text content is shown and can be paged with `PgUp/PgDn`.

### Applet-Level Context Widget

The applet-level version of the widget displays only the current context. The current context should not appear in the Context History Widget.  Turns in the current AgentX session are persisted as they occur, and the widget is then updated.

- Enabled elements are used in the next post context.  Disabled elements are excluded from the context.  The user may change these at any time before submitting the next prompt to the agent.
