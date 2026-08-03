# Behavior — Approval-Request Scrollback Record Collapses by Default

Status: **Implemented** (2026-08-03).

## Problem

Reported from a real session: the `❓ approval needed` scrollback widget
(`internal/surfaces/output.Model`'s record of every `ContentApprovalRequest`
event — "a lightweight scrollback record of what was asked," separate from
the live, interactive approval widget chat.go swaps into the input panel)
rendered fully expanded by default, taking up significant screen space —
sometimes many rows, for a `write_file`/`edit_file` proposal with substantial
content. Its sibling `📋 result` widget (`kindToolResult`) already defaults
to `collapsed: true`; the approval-request widget never got the same
treatment, so once a decision is made (recorded separately, right below it,
as a `kindApprovalDecision` audit line) the full proposal text just sits
there permanently expanded with no ongoing reason to.

## Design

One-line fix: add `collapsed: true` to the widget constructed for
`ContentApprovalRequest`, matching `ContentToolResult`'s existing widget
exactly:

```go
case state.ContentApprovalRequest:
    p, _ := ev.Payload.(map[string]any)
    m.add(&widget{kind: kindApprovalRequest, title: "❓ approval needed",
        body: str(p["prompt"]), collapsible: true, collapsed: true, previewWhenCollapsed: true})
```

`previewWhenCollapsed: true` was already set, so this widget already had
the machinery for a one-line collapsed preview
(`collapsedPreview`/`renderWidget`'s `case w.collapsed` branch) — it just
never used it. No other change needed: the widget is still `collapsible`,
so Enter/click still expands it on demand, same as any other collapsible
scrollback entry (tool calls, tool results, plan steps).

This widget is added to the transcript at REQUEST time, before the decision
is known (the live, interactive copy is what the user actually reads and
decides from, in the input panel) — collapsing it by default from the
start, rather than only after resolution, is intentional: the scrollback
copy is redundant with the live widget for as long as the decision is
pending anyway, so there's no reason for it to be expanded then either.

```
GIVEN an approval-request event applied to the output panel
WHEN  the resulting widget renders
THEN  it shows a single-line collapsed preview (not the full multi-line
      prompt) — matching the tool-result widget's default collapsed state.

GIVEN a collapsed approval-request widget
WHEN  the user expands it (Enter/ToggleCollapse)
THEN  the full prompt renders, same as any other collapsible widget —
      nothing about the collapse-by-default change removes the ability to
      see the full text on demand.
```

## Tests

- `internal/surfaces/output/output_test.go` (extended): applying a
  `ContentApprovalRequest` event produces a widget whose rendered View()
  output is a single line (collapsed), not the full multi-line prompt;
  toggling it expands to the full body.
- Full existing suite / `make all` passes unchanged.
