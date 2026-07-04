# ADR 0007: Output-Panel Markdown Rendering (Dual Renderer)

Status: Accepted
Date: 2026-07-04
Deciders: AgentX architecture owners

> Scope note: ADRs 0001–0006 govern the Family B orchestrator. This ADR governs a
> Family A surface concern — how the chat surface's output panel renders model
> markdown. It sits behind the surface/transport boundary and does not touch
> orchestration ownership.

## Context

The output panel renders assistant answers as bordered, collapsible widgets. Model
output is markdown, and legibility is a usability force-multiplier: humans visually
review everything they read as they read it, so a richly-styled, well-aligned answer is
materially easier to consume than raw markdown source.

Rendering happens in two very different regimes:

1. **While streaming** — `agent_delta` chunks arrive incrementally, including
   mid-word/mid-construct fragments. The panel already renders these live with a
   lightweight, per-line ANSI scanner (`styleMarkdown`/`styleLine`, ADR-less Tier 1/2
   of nits.md #6): `**bold**`, `` `code` ``, ATX headers, lists, blockquotes. This
   scanner is width-agnostic — it styles logical lines and the panel wraps afterward
   with ANSI-aware math (`ansi.Wrap`/`ansi.StringWidth`), which keeps borders,
   padding, the height cap, the proportional scrollbar, and collapsed previews exact.

2. **After the answer completes** — a single, well-formed markdown document is
   available. This is where a full renderer (charmbracelet **glamour**: goldmark AST +
   chroma highlighting) can produce tables, syntax-highlighted code blocks, and
   polished list/quote layout that a per-line scanner cannot.

Glamour is a *document* renderer, not a span styler. It owns word-wrapping, margins,
and inter-block spacing, and it needs a complete, valid document. Handing it partial
streaming markdown re-renders the whole doc every chunk and makes half-formed
constructs (an unterminated fence, a half-typed table) flicker or shift layout. It also
pulls goldmark + chroma into a deliberately tight vendored tree (4 direct deps + a
bubbletea submodule fork).

## Decision

Adopt a **dual renderer** split by lifecycle, not a replacement:

- **Streaming path keeps the scanner.** `agent_delta` chunks continue to render with
  `styleMarkdown` — incremental, width-agnostic, graceful on fragments. This is the
  "readable immediately" layer and is never removed.
- **On finalize, upgrade to glamour.** When the complete `agent_response` lands
  (`finalizeAssistant`), the assistant widget's markdown source is rendered by glamour
  to a width the panel dictates, and the rendered block replaces the scanner output in
  that widget. The scanner render stays on screen until the glamour block is ready, so
  there is never a blank frame.
- **The scanner is the fallback, not throwaway.** Any time a glamour render is
  unavailable or stale (see resize), the widget silently falls back to the scanner
  render. Correct-but-plainer always beats broken.

### Width contract (load-bearing)

The panel owns the dimensions it renders into, but they change, so the contract is
explicit:

- Glamour is always rendered to **`innerW - 1`** (the box inner width minus one
  column), reserving the per-widget vertical-scrollbar gutter **unconditionally**, even
  when no scrollbar is currently shown.
- Rationale: the per-widget in-place scrollbar only appears once a body exceeds
  `max_widget_lines`, at which point `renderBody` shrinks content to
  `bodyW = innerW - 1`. A table laid out at the full `innerW` would be one column too
  wide the instant the scrollbar appears → horizontal overflow. Reserving the column up
  front means a table that grows tall enough to trigger vertical scroll still fits
  horizontally with zero reflow.
- **Only vertical scroll is ever acceptable in the output panel; horizontal fidelity
  outranks vertical.** A horizontal scroll breaks the read flow far worse than a taller
  vertical scroll. Reserving the gutter is the mechanism that guarantees this for the
  worst case (wide tables).
- Because glamour output is pre-wrapped to a fixed width, every rendered block is
  tagged with the width it was rendered at (`renderedWidth`). On resize, a width
  mismatch **invalidates** the cached block: the widget reverts to the scanner render
  and re-renders glamour at the new width. A resize therefore briefly de-beautifies,
  then re-beautifies — never a broken frame.

### Glamour configuration

- Zero document margins and set word-wrap to the reserved width so glamour never adds
  horizontal indentation the box did not budget for.
- Force the color profile explicitly rather than letting termenv autodetect — the panel
  composes to a string, not directly to a TTY.
- Trim trailing margin/blank lines so the height cap and collapsed first-line preview
  stay clean.

### Rollout: behind a flag

- A config flag `[agentx.output] markdown_renderer` selects `"scanner"` (default) or
  `"glamour"`. The scanner remains the default until the glamour path is proven; then
  the default flips in a follow-up.
- Because the finalized glamour block is fed back through the *existing* `renderBody`
  pipeline (windowing, cap, scrollbar, padding all operate on the pre-wrapped lines),
  the swap is confined to the render seam — no change to widget navigation, collapse,
  selection, or persistence.

### Sync now, async later

The spike renders synchronously (a single answer renders in a few ms). The panel
already exposes `Update(tea.Msg) tea.Cmd`, so the future upgrade — render off the event
loop via a `tea.Cmd` yielding an `mdRenderedMsg{ordinal,width,lines}` that `Update`
swaps in by ordinal — is available without new plumbing style, and matters mainly for
large syntax-highlighted code blocks.

## Consequences

- **Unlocks Tier 3** of nits.md #6: real tables and syntax-highlighted code blocks —
  the "HUGE bonus" a per-line scanner structurally cannot do.
- **New dependencies vendored**: glamour + goldmark + chroma, via
  `go mod tidy && go mod vendor` with a reviewed vendor diff. This is the cost the
  dual-renderer split does *not* eliminate; it is justified only because the payoff
  (tables/highlighting) now matches it, which it did not for bold/headers alone.
- **Two renderers to keep coherent.** The scanner must stay a faithful-enough preview
  of the finalized glamour so the finalize swap is not visually jarring. Both are gated
  by the same per-widget `markdown` flag, so non-model content (user prompts, tool
  output) is never touched by either.
- **Testing strategy shifts for glamour.** The scanner's exact-SGR-span assertions are
  stable; glamour's ANSI is theme/version-dependent and brittle to pin exactly. Tier-3
  scenarios assert *structural* invariants instead — source markers consumed, **no line
  wider than the reserved width** (the horizontal-scroll guarantee), tables render
  aligned box-drawing, headers present.
- **Resize cost.** Cached blocks invalidate on width change; with the sync spike a
  resize re-renders affected assistant widgets on the next frame. Acceptable for the
  spike; the async path bounds it further.

## Companions

- Implementation: `internal/surfaces/output/output.go` (`finalizeAssistant`, the
  render seam in `renderWidget`, `renderBody`).
- Config: `internal/config/config.go` (`[agentx.output] markdown_renderer`).
- Behavior: `tests/features/surfaces/output_widget.feature` (UC-WIDGET-MARKDOWN).
- Preference of record: output panel scrolls vertically only; size block renderers to
  `innerW - 1`.
