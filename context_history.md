# CONTEXT HISTORY APPLET

This guide is to provide design concepts for the AgentX TUI (Go) implementation.

- Preserve the GUI's direct, nested feel in keyboard form: stacked boxes, scrollable viewports, emoji state markers, and pointer glyphs are part of the intended look/feel.
- The applet should attempt to manage available viewable space by using viewport scrolling within the active area
- Collapse/Expansion state is not persisted between sessions
- Within any given level of the applet, only one element may have focus at a time.  Focus and drill down within an element, but not down across multiple sibling elements.  Thus, there cannot be a scenario of mixed-focus state.

Example of Widgets in Context History applet. NOTE: widgets are shown
expanded for detail, in practice only one widget can be expanded at a time

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
  - `Enter` and `Tab` both enter the active section.
- Inside section focus (`insideSection=true`):
  - `Up/Down` moves row selection within the active section.
  - `Left/Right` moves horizontal siblings where available (for example prompt <-> response, session <-> first turn).
  - `Space` selects/deselects the active row.
  - `Enter` executes row expansion/collapse action for rows that support it.
  - `PgUp/PgDn` scrolls section viewports for `context-history` and `working-memory`; on expanded current-context rows, it scrolls wrapped text content.

### Toggle Options

Expand/Collapse and selection are context specific. The `Space` key only toggles the currently focused target.

#### Expansion / Collapse

To toggle expansion or collapse without entering a section, use `Space` while outside section focus. This preserves section-level navigation and does not enable row-level navigation until the section is entered.

#### Enable / Disable

To toggle an element as selected or not, use `Space` while inside section focus.

### Enter Exit Expandable Areas (Focus)

Drill In / Enter area: `Tab` (or `Enter` when outside section focus). Entering a section forces that section expanded.

Drill Out / Exit area: `Shift-Tab`. Exiting a section collapses that section.

For deep context-history focus paths (user -> session -> turn), `Shift-Tab` exits section focus, collapses the section, and pops focus-path depth by one node.

## Layout and Default State (Shipped)

- Header clutter lines are intentionally omitted from the context-feedback render (no persistent `Controls:` or `Status:` banner rows).
- `CONTEXT HISTORY` starts collapsed and renders summary metadata when collapsed (users/sessions/sort summary).
- `WORKING MEMORY` starts collapsed and renders summary metadata when collapsed (facts summary).
- `CURRENT CONTEXT` starts expanded at section level, with prompt/response rows collapsed by default.
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

- users are listed in alphabetical order; a system may support more than one user, their sessions are segregated in the `.sessions/` folder by user name.  Only one user session history may be expanded at a time.  Only one user session history may have focus at a time.
- For each user, session ordering is presentation-only and deterministic.  The default sort is `Ascending` to match the current loader; `Descending` may be selected when users want most-recent-first browsing.  The active order is controlled by `agentx.context_history_session_sort` in `agentx.toml`.
  Individual contexts can be expanded and collapsed without entering them (`Space` outside section focus).
- Facelift details for the Go TUI variant:
  - section headers are iconized for quick scanning (`🗄️ Context History`, `💾 Working Memory`, `📑 Current Context`)
  - expanded context history surfaces a compact summary line (`sessions`, active `sort`, and keyboard hint)
  - each session card includes absolute timestamp plus relative age (`Xd/Xh/Xm ago`) for faster recency parsing
  - state labels are color-coded (`collapsed` vs `expanded`) to reduce visual ambiguity
- Only one historical context can be expanded and take focus at a time
- Only one element in the focused context can be expanded at a time
- A context must be in section focus to enable row navigation and viewport scrolling.
- In context-history row focus:
  - `Enter` expands the focused history node when it is not currently the focused leaf path.
  - `Enter` collapses to the parent node when the focused node is already the focused leaf path.
  - This yields deterministic toggle behavior for user/session nodes based on current focus path.

## Working Memory Widget

- Working memory should be persisted in a JSON file within the current context folder in the file system, along with enabled/disabled/deleted state.
- existing working keys/values are read-only displays
  - Working Memory (scrollable view port with scrollbar & thumbnail)
- if the Working Memory exceeds the expanded window's area, show scrollbar and thumbnail. Up/Down arrows and PageUp/PageDown allow for scrolling through these KV pairs.
- User may enable or disable working memory elements by using the Space bar.  Enabled elements will be used in the next prompt context.  Enabled and disabled state are remembered for the duration of the session and the user can change them between prompts
- To remove a KV pair from working memory, the user selects the pair and hits the "DELETE" key. This will not delete the pair from the file persistence, but will change the state to "deleted" and will not be displayed on [re]rendering.  NOTE: if a user want to un-delete a working memory KV pair, they must edit the persistence file and modify it there.  Disable is the better option for "soft delete"

### Context History Widget Working Memory Version

- The working memory widget within the Context History widget does not allow users to change values.  However, users may select items from it and copy them into the current working memory by selecting the pair and hitting ENTER.
  This is a nested import affordance only: historical facts may be mined into the current Working Memory, but they do not redefine the current-session editor.
- there are four affordances in the area:
  - Key entry cell (user enters key name)
  - Value entry area (user enters value to map to key)
  - Save "button" (activated by Enter)
  - the scrollable KV Pair list
- The Key, Value, and Save cells do not scroll.

### Applet-Level Working Memory Widget

The applet-level Working Memory Widget supports editing of items for the current context.

- When the area is in focus, the Key cell has first focus, the Value cell as second focus, and the Enter button as third focus. Finally, the scrollable Working Memory viewport has fourth focus.  Focus wraps back and forth.  User can use Left and Right arrow keys to move between these areas.  Active cell and title are bright-colored, inactive are muted

## Context Widget

The context widget shows each of the elements of an agentX session in the order they occur.  They are collapsed by default, showing only the first few words followed by elipses.  When expanded, the elipses disappear and the full element is shown. Scrollable viewpanes should be used for when the context exceeds more than three lines.

### Applet-Level Context Widget

The applet-level version of the widget displays only the current context. The current context should not appear in the Context History Widget.  Turns in the current AgentX session are persisted as they occur, and the widget is then updated.

- Enabled elements are used in the next post context.  Disabled elements are excluded from the context.  The user may change these at any time before submitting the next prompt to the agent.
