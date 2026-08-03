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

**2. Defensive clamp — `approval.Model.promptLines()` caps its own row
count, regardless of prompt length.** A widget that trusts every caller to
have already truncated correctly is exactly the assumption Phase A's own
history argues against (`padLine`/`statusBar`'s rune-vs-width bug was found
independently of the clamp that was added anyway, per that behavior doc).
Cap by rendered ROW count, not character count, so the bound holds
regardless of terminal width — a narrow terminal wraps more chars per row,
which a char-count cap alone wouldn't protect against:

```go
const maxPromptRows = 10

func (m *Model) promptLines() []string {
    w := max(m.width, 1)
    if m.prompt == "" {
        return nil
    }
    lines := wrapText(m.prompt, w)
    if len(lines) > maxPromptRows {
        lines = append(lines[:maxPromptRows], "…")
    }
    return lines
}
```

`DesiredHeight()` already derives from `len(m.promptLines())`, so this caps
it automatically — no separate change needed there.

```
GIVEN a tool descriptor with an Argv template (e.g. edit_file's sed script)
      and an argument value far longer than 60 characters
WHEN  proposalText renders it
THEN  the value is truncated to 60 characters plus an ellipsis, identically
      to how the k=v fallback path already truncated non-Argv tools —
      neither path can put unbounded text into the prompt.

GIVEN a prompt string that would wrap to more than maxPromptRows rows at
      the panel's current width (even a truncated proposalText string is
      still ~60 chars per arg times several args, which can exceed this at
      a narrow width)
WHEN  the approval panel computes DesiredHeight()/renders View()
THEN  the prompt section is capped at maxPromptRows rows plus a trailing
      "…" row — never unbounded, regardless of what produced the prompt
      text.

GIVEN a short prompt that fits well within maxPromptRows
WHEN  the panel renders
THEN  it is unchanged from before this fix — the cap is a no-op in the
      common case, same posture as Phase A's width clamp.
```

## Tests

- `internal/runtime/approval_test.go` (extended): `proposalText` with an
  `Argv`-templated descriptor and an oversized argument value truncates it
  identically to the existing non-Argv fallback case.
- `internal/surfaces/approval/approval_test.go` (extended): `promptLines`/
  `DesiredHeight` cap at `maxPromptRows` for an oversized prompt; an
  ordinary short prompt is unaffected (same output as before this fix).
- Full existing suite / `make all` passes unchanged.
