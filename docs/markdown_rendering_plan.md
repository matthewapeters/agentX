# Markdown Rendering for Output Panel — Design & Implementation Plan

**Created:** 2026-03-29
**Status:** Draft
**Target:** AgentX `0.18.x` series

---

## How to Use This Document

- `[ ]` Not started
- `[/]` In progress — mark this **before** beginning a step
- `[X]` Failed / blocked — append a note on the same line explaining why
- `[✓]` Complete — replace `[ ]` or `[/]` when the step is done and tested

Work through phases top-to-bottom within a session. At session start, scan this
file for the first `[ ]` item and mark it `[/]` before touching any code.
At session end, confirm every touched step is either `[✓]` or `[X]`.

---

## Motivation

The Output panel currently renders all content — headers, emphasis, tables, images,
code blocks — as plain monospace text inside `tk.Text` widgets. LLM responses that
include markdown structure (common when answering questions about code architecture,
generating documentation, or returning tabular data) are significantly harder to read
as raw markdown text than as rendered HTML.

**Goal:** Replace the plain-text `tk.Text` detail widget with a rendered HTML view
for completed turns, while preserving the existing streaming experience and all
current UI behaviours (collapse/expand, copy, resize, history replay, theming).

---

## Approach: Hybrid Stream → Render

Streaming requires a writable `tk.Text` widget because chunks arrive token-by-token.
HTML rendering works best on a complete document. These two facts motivate a two-phase
approach:

```
Turn in progress  →  tk.Text   (current streaming mechanism — unchanged)
Turn completed    →  HtmlFrame (tkinterweb renders full markdown as styled HTML)
```

At the natural end of a turn (`display_spacing()` is called), completed entries for
roles where markdown is meaningful (Agent, Thinking, User) are **finalised**: their
`tk.Text` is destroyed and replaced with an `HtmlFrame` populated with HTML converted
from the accumulated `full_text`.

Roles that must stay as plain text (Tool, Error, Classification) are never finalised.

---

## Technology Stack

| Library | Role | PyPI |
|---------|------|------|
| `tkinterweb` | `HtmlFrame` — a `tk.Frame` subclass that renders HTML/CSS using the bundled `tkhtml3` Tcl extension. Zero GTK/event-loop conflict with Tkinter on Linux. | `tkinterweb` |
| `markdown` | Converts markdown text to HTML. Extensions: `tables`, `fenced_code`, `nl2br`. | `Markdown` |

Both are **soft dependencies**: the app degrades gracefully to plain `tk.Text` if they
are unavailable (`ImportError` is caught at module load).

---

## Architecture Overview

### New module

```
src/agentx/gui/markdown_renderer.py
```

Houses two pure functions:

```
build_markdown_css(config: GUIConfig) -> str
    Maps GUIConfig color tokens → a self-contained CSS stylesheet string.
    Dark Mode and Light Mode both supported via the config palette.

markdown_to_html(text: str, css: str) -> str
    Converts markdown text → full HTML document with embedded CSS.
    Uses markdown.markdown() with tables + fenced_code + nl2br extensions.
```

### Changes to `GUIManager`

The only Tkinter-touching code lives in `GUIManager`:

```
_create_output_entry()          ← stores toggle_btn in state dict (currently missing)
_finalize_entry_markdown()      ← swaps tk.Text → HtmlFrame for one entry
_finalize_current_turn_markdown() ← iterates current_turn_entries, finalises eligible ones
display_spacing()               ← calls _finalize_current_turn_markdown() before reset
```

### State dict additions (per entry)

```python
state["toggle_btn"]   = toggle_btn        # NEW: reference for rebinding after finalization
state["html_frame"]   = None              # NEW: set after finalization
state["is_finalized"] = False             # NEW: guard against double-finalization
```

### Finalization signal path

```
session.py: _safe_root_after(self.gui.display_spacing)
  → GUIManager.display_spacing()
      → _finalize_current_turn_markdown()      ← NEW
          → _finalize_entry_markdown(entry)    ← NEW (for each eligible entry)
      → _current_turn_entries = {}             ← existing cleanup
```

---

## Configuration

New key in `agentx.toml` under `[agentx]`:

```toml
markdown_render_enabled = true
```

Exposed in `GUIConfig`:

```python
markdown_render_enabled: bool = True
```

Markdown rendering is enabled by default. The Settings tab provides a live toggle.

---

## Markdown Detection Heuristic

Not every response contains meaningful markdown. Rendering plain prose through
`HtmlFrame` adds widget overhead with no visual benefit. A lightweight heuristic detects
likely-markdown before finalising:

```python
MARKDOWN_PATTERNS = re.compile(
    r"(^#{1,6}\s)"          # ATX heading
    r"|(^\s*[-*+]\s)"       # unordered list
    r"|(^\s*\d+\.\s)"       # ordered list
    r"|(\*\*|__)"           # bold
    r"|(\*[^*]|_[^_])"     # italic
    r"|(`{1,3})"            # inline or fenced code
    r"|(\|.+\|)"            # table row
    r"|(^\s*>)"             # blockquote
    r"|(!\[.+\]\(.+\))",    # image
    re.MULTILINE,
)
```

If `MARKDOWN_PATTERNS.search(text)` returns `None`, the entry stays as `tk.Text`.
This is a best-effort heuristic — false negatives leave entries as plain text,
which is always acceptable.

---

## HtmlFrame Height Behaviour

`HtmlFrame` does not auto-size to content out of the box. After `set_content()` is
called, the rendered document height is obtained via:

```python
html_frame.html.yview()   # or frame.winfo_reqheight() after update_idletasks()
```

The height update is scheduled with `after_idle()` (same pattern as
`_schedule_detail_text_height_update`) and calls `html_frame.configure(height=...)`.
The `<Configure>` event on the HtmlFrame canvas triggers a re-check on resize
(matching the pattern already used for `tk.Text`).

---

## Toggle Closure Migration

The `_toggle()` closure inside `_create_output_entry()` captures `detail_text` by name.
After finalization the toggle must show/hide `html_frame` instead. The cleanest migration
is:

1. Store `toggle_btn` in `state["toggle_btn"]`
2. After finalization, write a new `def _html_toggle()` closure that references
   `entry["html_frame"]` and packs/packs_forgets it
3. Call `state["toggle_btn"].config(command=_html_toggle)`

This re-binds the existing button with zero widget-graph changes.

---

## Scope Boundaries

| In scope | Out of scope |
|----------|-------------|
| `assistant` (Agent) entry finalization | Input field markdown preview |
| `thinking` entry finalization | Markdown syntax in the User entry (usually redundant) |
| `user` entry finalization | Plan tree / tool entries (intentionally plain) |
| Tables, headers, bold, italic, inline code, fenced code, images, blockquotes | WebGL, JavaScript, `<script>` tags (stripped by tkhtml3) |
| Dark Mode and Light Mode CSS themes | Runtime theme-switch re-rendering of already-finalized entries |
| Graceful degradation when tkinterweb absent | Upgrading tkinterweb in CI automatically |
| Settings tab toggle | Per-role markdown on/off granularity |
| History replay (entries restored from context display plain text; finalization runs again if turn is re-displayed) | |

---

## Implementation Phases

---

### Phase 1 — Dependencies & Soft-Import Guard

**Goal:** New packages declared; graceful fallback when unavailable.

- [ ] Add `tkinterweb>=3.24` and `Markdown>=3.7` to `dependencies` in `pyproject.toml`
- [ ] Add `markdown_render_enabled = true` under `[agentx]` in `agentx.toml`
- [ ] Add `markdown_render_enabled: bool = True` field to `GUIConfig` dataclass in `src/agentx/gui/gui_config.py`
- [ ] Wire `markdown_render_enabled` into `GUIConfig.from_dict()` (reads from `agentx` section)
- [ ] Create `src/agentx/gui/markdown_renderer.py` with soft-import guard at the top:

  ```python
  try:
      from tkinterweb import HtmlFrame
      TKINTERWEB_AVAILABLE = True
  except ImportError:
      HtmlFrame = None
      TKINTERWEB_AVAILABLE = False
  try:
      import markdown as _md_lib
      MARKDOWN_AVAILABLE = True
  except ImportError:
      MARKDOWN_AVAILABLE = False
  ```

- [ ] Expose `TKINTERWEB_AVAILABLE` and `MARKDOWN_AVAILABLE` as public constants from the module
- [ ] Write unit test: importing `markdown_renderer` with neither package installed (mock `ImportError`) sets both flags to `False` and does not raise
- [ ] Run `uv sync` to install new packages and confirm no version conflicts

---

### Phase 2 — CSS Theme Generator

**Goal:** `GUIConfig` colors map cleanly to a CSS stylesheet embedded in every rendered HTML document.

- [ ] Implement `build_markdown_css(config: "GUIConfig") -> str` in `markdown_renderer.py`; covers:
  - `body` — `background-color`, `color`, `font-family: monospace`, `font-size`, `margin`, `padding`
  - `h1`–`h6` — sizing scale (h1: 1.5em → h6: 0.85em), `color` from `config.ui_fg`, `border-bottom` on h1/h2
  - `code` — inline code: `background-color` slightly lighter than `output_bg`, `border-radius`, `padding`
  - `pre > code` — fenced code blocks: dark background (`#1e1e1e` dark / `#f3f4f6` light), monospace, left border accent
  - `table` — `border-collapse: collapse`, full width
  - `th`, `td` — `border: 1px solid`, `padding`, `th` with header background from `status_bg`
  - `tr:nth-child(even)` — subtle alternate-row shading
  - `blockquote` — left border accent, muted foreground
  - `a` — `color: #4a9eff` (dark) or `#1d4ed8` (light), no underline by default
  - `img` — `max-width: 100%`, `height: auto`
  - `hr` — `border-color` muted
- [ ] Implement `markdown_to_html(text: str, css: str) -> str`:

  ```python
  body = _md_lib.markdown(
      text,
      extensions=["tables", "fenced_code", "nl2br", "sane_lists"],
  )
  return f"<html><head><style>{css}</style></head><body>{body}</body></html>"
  ```

- [ ] Write unit tests for `build_markdown_css`:
  - Dark Mode config produces CSS body containing `#222222`
  - Light Mode config produces CSS body containing `#ffffff`
  - CSS string is non-empty and contains `table`, `pre`, `h1`
- [ ] Write unit tests for `markdown_to_html`:
  - `# Hello` → HTML contains `<h1>Hello</h1>`
  - `**bold**` → HTML contains `<strong>bold</strong>`
  - Table markdown `| a | b |\n|---|---|\n| 1 | 2 |` → HTML contains `<table>`
  - Triple-backtick block → HTML contains `<code>`
  - Plain text (no markdown markers) passed through without breaking

---

### Phase 3 — Detection Heuristic

**Goal:** Short plain-text responses are not replaced with `HtmlFrame`s unnecessarily.

- [ ] Implement `has_markdown(text: str) -> bool` in `markdown_renderer.py` using `MARKDOWN_PATTERNS` regex (full pattern documented in Architecture Overview above)
- [ ] Expose `MARKDOWN_PATTERNS` as a module-level compiled `re.Pattern`
- [ ] Write unit tests for `has_markdown`:
  - `"# Hello"` → `True`
  - `"**bold text**"` → `True`
  - `"| col1 | col2 |"` → `True`
  - `` "`code`" `` → `True`
  - `"Here is a plain sentence."` → `False`
  - `"1. First item"` → `True`
  - `"> blockquote"` → `True`
  - `"![alt](url)"` → `True`
  - Empty string → `False`
  - `"2+2 = 4"` → `False` (does not trigger italic heuristic)

---

### Phase 4 — GUIManager Entry State & Toggle Refactor

**Goal:** Each output entry state dict carries the new fields needed for post-stream finalization; the toggle button can be rebound without structural changes.

- [ ] In `_create_output_entry()` in `gui_manager.py`, add `toggle_btn` to the `state` dict immediately after `toggle_btn` is constructed:

  ```python
  state["toggle_btn"] = toggle_btn
  state["html_frame"] = None
  state["is_finalized"] = False
  ```

- [ ] Verify that no existing code reads `state["toggle_btn"]` (there should be none — this is a new key)
- [ ] Write unit test: `_create_output_entry()` returns a state dict containing keys `toggle_btn`, `html_frame`, `is_finalized`
- [ ] Write unit test: calling `state["toggle_btn"].invoke()` still toggles `state["expanded"]` correctly

---

### Phase 5 — `_finalize_entry_markdown()` Implementation

**Goal:** A completed entry's `tk.Text` is destroyed and replaced with a correctly sized, themed `HtmlFrame`.

- [ ] Implement `_finalize_entry_markdown(self, entry: dict[str, Any]) -> None` in `GUIManager`:

  ```
  Guard 1: return if not TKINTERWEB_AVAILABLE or not MARKDOWN_AVAILABLE
  Guard 2: return if not self.config.markdown_render_enabled
  Guard 3: return if entry["is_finalized"]
  Guard 4: return if entry["role_label"] in {"Tool", "Error", "Classification"}
  Guard 5: return if not has_markdown(entry["full_text"])
  
  1. Build CSS: css = build_markdown_css(self.config)
  2. Build HTML: html = markdown_to_html(entry["full_text"], css)
  3. Determine parent: parent = entry["detail_text"].master
  4. Get pack options: entry["detail_text"] was packed with fill=tk.X, anchor="w", padx=(24, 0)
  5. Remove detail_text from _output_detail_text_widgets list (prune by identity)
  6. Destroy entry["detail_text"]
  7. entry["detail_text"] = None  (guard against stale refs)
  8. Create html_frame = HtmlFrame(parent, messages_enabled=False)
  9. html_frame.load_html(html)
  10. Schedule height update: html_frame.after_idle(_update_html_frame_height)
  11. Store: entry["html_frame"] = html_frame, entry["is_finalized"] = True
  12. Pack or skip based on entry["expanded"]:
      if entry["expanded"]: html_frame.pack(fill=tk.X, anchor="w", padx=(24, 0))
  13. Rebind toggle button:
      toggle_btn = entry["toggle_btn"]
      def _html_toggle(e=entry):
          e["expanded"] = not e["expanded"]
          toggle_btn.config(text=self.EXPAND_COLLAPSE_ICONS[e["expanded"]])
          if e["expanded"]:
              e["html_frame"].pack(fill=tk.X, anchor="w", padx=(24, 0))
              _schedule_html_height_update(e["html_frame"])
          else:
              e["html_frame"].pack_forget()
      toggle_btn.config(command=_html_toggle)
      entry["toggle"] = _html_toggle
  ```

- [ ] Implement `_update_html_frame_height(self, html_frame: "HtmlFrame") -> None`:
  - Calls `html_frame.update_idletasks()`, reads `html_frame.winfo_reqheight()`, configures height
  - Schedules scroll-region update on `output_entries_canvas`
- [ ] Implement `_schedule_html_height_update(self, html_frame: "HtmlFrame") -> None` using `after_idle`
- [ ] Write unit tests for `_finalize_entry_markdown`:
  - With real Tkinter root: create an entry with markdown text, finalize, verify `entry["is_finalized"] == True` and `entry["html_frame"] is not None` and `entry["detail_text"] is None`
  - Entry with no markdown markers → heuristic returns `False` → `is_finalized` stays `False`
  - Entry with `role_label="Tool"` → skipped, `is_finalized` stays `False`
  - Guard 1 activated (TKINTERWEB_AVAILABLE=False, monkeypatched) → method returns without error
  - Guard 2 activated (markdown_render_enabled=False) → skipped
  - Double-finalization call → second call is no-op (Guard 3)
  - Expanded entry → `html_frame.winfo_manager() == "pack"` after finalization
  - Collapsed entry → `html_frame.winfo_manager() == ""` (not packed) after finalization

---

### Phase 6 — `_finalize_current_turn_markdown()` & `display_spacing()` Hook

**Goal:** Finalization is triggered automatically at the natural end of every turn.

- [ ] Implement `_finalize_current_turn_markdown(self) -> None`:

  ```python
  for key, entry in self._current_turn_entries.items():
      if entry is not None:
          self._finalize_entry_markdown(entry)
  ```

- [ ] Call `self._finalize_current_turn_markdown()` at the **start** of `display_spacing()`, before `_current_turn_entries = {}`
- [ ] Ensure `display_spacing()` still resets all state fields as before (no regression)
- [ ] Handle bootstrap turn: `display_bootstrap_agent_response()` does not create `_current_turn_entries`; `_finalize_current_turn_markdown()` handles empty dict safely (no-op)
- [ ] Write unit tests:
  - Call `display_user_message()` + `display_agent_response("# Title\n\n**bold** text")` + `display_spacing()` → `_current_turn_entries["assistant"]["is_finalized"] == True`
  - Same flow with plain text response → `is_finalized == False`
  - `_current_turn_entries` is reset to `{}` after `display_spacing()` regardless of finalization
  - Two turns in sequence: both turns' entries finalized at their respective `display_spacing()` calls; no cross-turn interference

---

### Phase 7 — Resize Handling for HtmlFrame

**Goal:** HtmlFrame height stays correct after the output panel is resized.

- [ ] In `_update_output_wraplength()`, after the existing `tk.Text` resize loop, add a second loop over a new `self._output_html_frames: list["HtmlFrame"]` list:

  ```python
  active_html_frames = []
  for hf in self._output_html_frames:
      try:
          self._update_html_frame_height(hf)
          active_html_frames.append(hf)
      except tk.TclError:
          continue  # widget was destroyed; auto-prune
  self._output_html_frames = active_html_frames
  ```

- [ ] In `_finalize_entry_markdown()`, append the new `html_frame` to `self._output_html_frames`
- [ ] Initialise `self._output_html_frames: list = []` in `GUIManager.__init__()`
- [ ] Write unit test: after finalization, `html_frame` is present in `_output_html_frames`; calling `_update_output_wraplength(800)` does not raise
- [ ] Write unit test: destroyed `HtmlFrame` is pruned from `_output_html_frames` during next resize event

---

### Phase 8 — `IGUIManager` Protocol Update

**Goal:** The `IGUIManager` protocol reflects the new public capability surface; business logic can call `finalize_current_turn_markdown()` directly if needed.

- [ ] Add `finalize_current_turn_markdown(self) -> None` to `IGUIManager` in `src/agentx/igui_manager.py` with docstring:

  ```
  Finalise all completed entries in the current turn by replacing streaming
  tk.Text widgets with rendered HtmlFrame widgets where markdown is detected.
  Called automatically by display_spacing(); exposed here for callers that need
  explicit control (e.g. bootstrap turn, plan-tab finalization).
  ```

- [ ] Verify `GUIManager` satisfies the updated protocol (run `mypy src/` — no new errors)
- [ ] Write unit test: `GUIManager` is an instance of `IGUIManager` (protocol runtime check via `isinstance(..., IGUIManager)` or structural check)

---

### Phase 9 — Settings Tab Toggle

**Goal:** The user can enable/disable markdown rendering without restarting the application.

- [ ] Add a "Render Markdown" `tk.BooleanVar` checkbox to `src/agentx/gui/settings_tab.py` in the "Output" or "Appearance" section
- [ ] On toggle: update `self.config.markdown_render_enabled`; if `tkinterweb` is not installed, show an inline "(requires tkinterweb)" label and keep the checkbox disabled
- [ ] Persist the new value to `agentx.toml` using the existing settings-save mechanism
- [ ] Write unit test: checkbox is disabled (greyed) when `TKINTERWEB_AVAILABLE == False`
- [ ] Write unit test: toggling the checkbox updates `gui.config.markdown_render_enabled`

---

### Phase 10 — Integration Smoke Test

**Goal:** End-to-end flow works with real Tkinter; rendered HTML is visible and correct.

- [ ] Write an integration test (`tests/test_markdown_rendering.py`) tagged `@pytest.mark.live` (requires display) OR skipped if `DISPLAY` env var is unset:

  ```
  Setup: create GUIManager with create_layout()
  Step 1: display_user_message("Show me a markdown table")
  Step 2: stream display_agent_response() with a table: "| A | B |\n|---|---|\n| 1 | 2 |"
  Step 3: call display_spacing()
  Assert: entry["is_finalized"] == True
  Assert: entry["html_frame"] is not None
  Assert: entry["html_frame"].winfo_manager() == "pack"  (entry was expanded=True)
  Assert: entry["detail_text"] is None
  Assert: "html_frame" in entry and entry["html_frame"].winfo_exists()
  ```

- [ ] Write a headless unit test that exercises the full path with `HtmlFrame` mocked:

  ```python
  with patch("agentx.gui.markdown_renderer.HtmlFrame", FakeHtmlFrame):
      # same assertions as above but no display required
  ```

- [ ] Confirm `python -m pytest tests/test_markdown_rendering.py -m "not live"` passes in CI

---

### Phase 11 — Version Bump & Dependencies Lock

**Goal:** Package version reflects the new feature; lockfile is updated.

- [ ] Increment version in `pyproject.toml` from `0.17.0` → `0.18.0` (new feature, backward-compatible: minor version bump, patch reset to zero)
- [ ] Run `uv sync` and confirm `uv.lock` updated with `tkinterweb` and `Markdown` entries
- [ ] Run `python -m pytest -m "not live"` — full test suite passes
- [ ] Run `mypy src/` — no new type errors
- [ ] Run `flake8 src/ tests/` — no new lint errors
- [ ] Run `black src/ tests/ --line-length=120 --check` — no formatting violations
- [ ] Run `isort src/ tests/ --profile=black --line-length=120 --check` — no import-order violations

---

## Key Files Affected

| File | Nature of Change |
|------|-----------------|
| `pyproject.toml` | Add `tkinterweb>=3.24`, `Markdown>=3.7`; bump version `0.17.0 → 0.18.0` |
| `agentx.toml` | Add `markdown_render_enabled = true` |
| `src/agentx/gui/gui_config.py` | Add `markdown_render_enabled: bool = True`; wire in `from_dict()` |
| `src/agentx/gui/markdown_renderer.py` | **NEW** — `build_markdown_css`, `markdown_to_html`, `has_markdown`, soft-import guards |
| `src/agentx/gui/gui_manager.py` | Add `toggle_btn`/`html_frame`/`is_finalized` to state dict; add `_finalize_entry_markdown`, `_finalize_current_turn_markdown`, `_update_html_frame_height`, `_schedule_html_height_update`; hook `display_spacing()`; extend `_update_output_wraplength` |
| `src/agentx/gui/settings_tab.py` | Add "Render Markdown" checkbox |
| `src/agentx/igui_manager.py` | Add `finalize_current_turn_markdown()` to `IGUIManager` protocol |
| `tests/test_markdown_rendering.py` | **NEW** — all markdown rendering tests |

---

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| `tkhtml3` Tcl extension not available in headless CI/CD environments | Soft-import guard + `@pytest.mark.live` or skip-if-no-DISPLAY; headless tests use `HtmlFrame` mock |
| `HtmlFrame` height calculation is async / takes multiple frames to settle | Use `after_idle` chain with a retry (up to 3 frames) before accepting height; fall back to `winfo_reqheight()` |
| Already-destroyed `tk.Text` refs after finalization cause `TclError` in `_update_output_wraplength` | Existing `try/except TclError` guard already handles this; widget is auto-pruned on next resize |
| Dark/Light theme toggle after finalization produces mismatched colors in existing `HtmlFrame`s | Documented known limitation; HtmlFrames retain the CSS from the theme active at finalization time. Future work: iterate `_output_html_frames` on theme change and call `load_html()` with rebuilt CSS |
| `HtmlFrame.load_html()` may not support all `markdown` extension output (e.g. definition lists) | Restrict to `tables`, `fenced_code`, `nl2br`, `sane_lists` — all well-supported by tkhtml3 |
| Double entries: detail_text is also in `_output_detail_text_widgets`; destroying it while it is referenced causes `TclError` on next height update | `_update_detail_text_height` already guarded by `try/except TclError`; widget pruned automatically |

---

## Non-Goals (Explicit)

- **No pywebview integration.** pywebview creates its own top-level OS window; it cannot be embedded inside a `tk.Frame`. On Linux it runs a GTK event loop that conflicts with Tkinter's Tcl/Tk mainloop. `tkinterweb` uses the bundled `tkhtml3` Tcl extension and has no event-loop conflict.
- **No JavaScript execution.** `tkhtml3` does not execute JS. This is a feature, not a limitation — it eliminates XSS via a rogue LLM response containing `<script>` tags.
- **No live markdown preview in the input box.** In-scope only for the output panel.
- **No per-message font size controls.** CSS sets a reasonable default; not configurable per message.
