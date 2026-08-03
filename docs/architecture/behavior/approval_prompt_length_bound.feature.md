# Behavior — Bounding Approval-Prompt Length and the Approval Panel's Height

Status: **Implemented** (2026-08-02).

## Problem

A real session's very first approval request pushed the input widget off
the bottom of the screen — the exact failure Phase A's `clampRowWidth`
(`docs/architecture/behavior/chat_width_overflow_clamp.feature.md`) was
built to prevent, but Phase A only clamps row *width*; nothing bounds a
single widget's row *count*. Root cause: `proposalText`
(`internal/runtime/approval.go`), which renders a proposed tool call for
both the approval prompt and every `tool_call` audit/log line, only
truncates argument values on its fallback (`k=v`) rendering path. Its other
path — used whenever the descriptor has an `Argv` template — substitutes
raw, untruncated argument values via `d.BuildArgv(args)` and joins them with
no cap at all. `edit_file` (`sed -i -e {script} -- {path}`) is exactly this
shape: `script` is free-form text with no length limit, so a model
proposing a large in-place edit put the entire substitution text — easily
hundreds of lines — into the approval prompt verbatim. `approval.Model`'s
`DesiredHeight()` (`internal/surfaces/approval/approval.go`) has no cap of
its own on the wrapped prompt's row count, so `relayout()`'s height budget
(`internal/surfaces/chat/chat.go`) computed an `approvalH` large enough to
consume the whole terminal, leaving nothing for the input panel below it.

## Design

Two independent layers, mirroring Phase A's own two-layer fix (a real cause
fixed, plus a defensive clamp that holds regardless of what any future
caller does):

**1. Root cause — `proposalText` truncates every argument value uniformly,
on both rendering paths.** Previously only the fallback path capped a
value at 60 characters; that cap now applies before either path builds its
display string, so an `Argv`-templated tool's substituted values are exactly
as bounded as the fallback's:

```go
const maxArgPreviewLen = 60

func truncateArg(v string) string {
    if len(v) <= maxArgPreviewLen {
        return v
    }
    return v[:maxArgPreviewLen] + "…"
}

func proposalText(d tools.Descriptor, args map[string]string) string {
    display := make(map[string]string, len(args))
    for k, v := range args {
        display[k] = truncateArg(v)
    }
    if len(d.Argv) > 0 {
        if argv, err := d.BuildArgv(display); err == nil {
            return strings.Join(argv, " ")
        }
    }
    // ...unchanged k=v fallback, now reading from display instead of args
}
```

`proposalText` is purely a display/audit string — `BuildArgv` here never
feeds the actual `Runner.Run` call (that uses the real, unmodified `args`
map directly; see `core_tools.go`'s `runNativeToolCall` and
`executor.Execute`), so truncating its output changes nothing about what
actually executes, only what's shown in the approval prompt and every
`tool_call` log line that reuses the same function.

**2. Defensive backstop — `approval.Model` scrolls the prompt instead of
truncating it, reusing the output panel's own windowing pattern.** A widget
that trusts every caller to have already truncated correctly is exactly the
assumption Phase A's own history argues against (`padLine`/`statusBar`'s
rune-vs-width bug was found independently of the clamp that was added
anyway, per that behavior doc) — but truncating with a bare "…" marker
silently drops content, which is worse than necessary when the app already
has a proven pattern for "cap a body's visible rows, page through the rest":
`internal/surfaces/output.Model.renderBody`'s per-widget wrap/window/
scrollbar mechanics (`w.offset`, `scrollutil.WrapLines`/`ClampInt`/`PadTo`/
`ScrollbarCell`). `approval.Model` reuses those same primitives instead of a
second, drifting implementation — same posture as Phase A's `clampRowWidth`
(bound the rendered quantity directly, by ROW COUNT not character count, so
it holds regardless of panel width):

```go
const maxPromptRows = 10

func (m *Model) promptLines() []string {
    w := max(m.width, 1)
    if m.prompt == "" {
        return nil
    }
    lines := scrollutil.WrapLines(m.prompt, w)
    if len(lines) <= maxPromptRows {
        m.promptOffset = 0
        return lines
    }
    bodyW := max(w-1, 1)
    lines = scrollutil.WrapLines(m.prompt, bodyW)
    total := len(lines)
    maxOffset := total - maxPromptRows
    m.promptOffset = scrollutil.ClampInt(m.promptOffset, 0, maxOffset)
    window := lines[m.promptOffset : m.promptOffset+maxPromptRows]
    out := make([]string, maxPromptRows)
    for i, l := range window {
        out[i] = scrollutil.PadTo(l, bodyW) + scrollutil.ScrollbarCell(i, m.promptOffset, total, maxPromptRows)
    }
    return out
}

func (m *Model) ScrollPrompt(n int) {
    lines := scrollutil.WrapLines(m.prompt, max(m.width, 1))
    maxOffset := max(len(lines)-maxPromptRows, 0)
    m.promptOffset = scrollutil.ClampInt(m.promptOffset+n, 0, maxOffset)
}
```

`DesiredHeight()` already derives from `len(m.promptLines())`, so it stays
capped at `maxPromptRows` automatically — no separate change needed there.
`promptOffset` resets to 0 in `Set`, so every newly-shown request starts
scrolled to its top.

**Wiring the scroll key.** PgUp/PgDn normally jump chat focus into the
output panel (`chat.go`'s `handleKey`) — while a decision is pending,
`handleKey`'s "awaiting an interactive decision" branch now runs BEFORE that
jump-focus check (previously it ran after, so PgUp/PgDn during an approval
never reached the widget at all) and routes them into
`approval.Model.Update`, which pages the prompt by `maxPromptRows` per
press. Up/Down/j/k/Enter are unaffected — they still navigate the option
list and confirm, exactly as before.

```
GIVEN a tool descriptor with an Argv template (e.g. edit_file's sed script)
      and an argument value far longer than 60 characters
WHEN  proposalText renders it
THEN  the value is truncated to 60 characters plus an ellipsis, identically
      to how the k=v fallback path already truncated non-Argv tools —
      neither path can put unbounded text into the prompt.

GIVEN a prompt string that would wrap to more than maxPromptRows rows at
      the panel's current width
WHEN  the approval panel computes DesiredHeight()/renders View()
THEN  the prompt section is windowed to exactly maxPromptRows rows, each
      carrying a scrollbar cell — never unbounded regardless of what
      produced the prompt text, and nothing is silently dropped.

GIVEN an oversized prompt, freshly shown (scrolled to the top)
WHEN  the user presses PgDn while the decision is pending
THEN  the visible window advances instead of chat focus jumping to the
      output panel — PgUp pages back and returns to the original window;
      scrolling clamps at both ends rather than running past the content.

GIVEN a short prompt that fits well within maxPromptRows
WHEN  the panel renders
THEN  it is unchanged from before this fix — the cap is a no-op in the
      common case, same posture as Phase A's width clamp.
```

## Tests

- `internal/runtime/approval_test.go` (extended): `proposalText` with an
  `Argv`-templated descriptor and an oversized argument value truncates it
  identically to the existing non-Argv fallback case.
- `internal/surfaces/approval/approval_test.go` (extended): `promptLines`
  windows to exactly `maxPromptRows` for an oversized prompt; PgDn/PgUp
  actually change (and clamp) the visible window via `ScrollPrompt`, and
  work even before any options are set; an ordinary short prompt is
  unaffected (same output as before this fix).
- `internal/surfaces/chat/chat_test.go` (new): PgDn while
  `StateAwaitingInput` scrolls the approval widget's prompt rather than
  jumping chat focus to the output panel.
- Full existing suite / `make all` passes unchanged.
