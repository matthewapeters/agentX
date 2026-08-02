# Behavior — Remove Read-Only Mode: Approval Gating Is the Sole Tool-Execution Gate

Status: In progress.

## Problem

`Settings.ToolReadOnly`/`config.Tools.ReadOnly` (`tools.read_only` in `agentx.toml`,
default `true`) added a hard, pre-approval deny for any non-`RiskRead` tool call:
`ConversationCore.runNativeToolCall`'s `case c.toolReadOnly() && d.Risk !=
tools.RiskRead: verdict = Deny, reason: "read_only"`. This ran *before*
`Policy.Evaluate`, so a write/network tool call under read-only mode never
reached the interactive approval path at all — it was silently, instantly
denied, indistinguishable in the plan-completion summary ("N denied (needs
approval)") from a human actually declining a real prompt.

Product decision: remove read-only mode entirely. The existing approval flow
(`Policy.Evaluate` → `RequestApproval` for `RequiresApproval`-tagged tools) is
the sole gate on write/network-risk execution going forward. A pending approval
with no human present to answer it blocks indefinitely — accepted deliberately,
not a bug to work around with an auto-deny fallback.

## Design

Delete the `ToolReadOnly`/`toolReadOnly`/`read_only` concept end to end: the
`ConversationCore` field and its deny branch, `Settings.ToolReadOnly`, `config
.Tools.ReadOnly`/`Config.ToolReadOnly()`, the `agentx.toml` seed default, every
live-reload/config-surface/validation touchpoint in `orchestrator.go`, and every
test that constructs a `Settings`/`Config` referencing it. `Policy.Evaluate`
itself is untouched — every tool call now reaches it directly, the same path a
`RiskRead` tool already always used.

```
GIVEN a tool call whose Descriptor has Risk != RiskRead and RequiresApproval: true
WHEN  runNativeToolCall evaluates it
THEN  it reaches Policy.Evaluate directly (no read-only short-circuit exists
      anywhere in the call path) and, absent a session/global whitelist match,
      produces NeedsApproval — routing to the real interactive RequestApproval
      prompt, which blocks until a human resolves it or the context is
      canceled. It is never auto-denied.

GIVEN a tool call whose Descriptor has Risk == RiskRead
WHEN  runNativeToolCall evaluates it
THEN  behavior is unchanged — read-risk tools were never gated by read-only
      mode in the first place (the deleted branch only fired for Risk !=
      RiskRead), so this scenario has nothing to regress.

GIVEN a session/global approval already on file for a tool+args combination
WHEN  that exact call is proposed again
THEN  Policy.Evaluate's existing whitelist check still short-circuits straight
      to Allow, unaffected by this removal — the deleted code sat before this
      check in the switch, never inside it.

GIVEN Settings/Config as constructed by any existing call site
WHEN  the struct is built
THEN  it compiles with no ToolReadOnly/ReadOnly field anywhere in Settings or
      config.Tools — this is a straightforward field deletion, not a
      behavior-preserving default change.
```

## Tests

- `internal/runtime/core_tools_test.go`:
  `TestConversationCoreRunNativeToolCallDeniesUnderReadOnly` is deleted (the
  behavior it asserted no longer exists) — the remaining tests in that file
  (`ExecutesDirectly`, `RequestsApproval`, `PropagatesApprovalInterrupt`)
  continue to cover the real approval path unchanged.
- `internal/runtime/core_standalone_test.go`,
  `tests/steps/runtime/tool_cycle_steps.go`,
  `tests/steps/runtime/wm_pin_steps.go`,
  `tests/steps/runtime/tool_context_enable_steps.go`,
  `tests/steps/runtime/config_live_reload_steps.go`: `ToolReadOnly`/`read_only`
  references removed from `Settings`/`Config` construction; any step/scenario
  whose entire point was read-only-mode behavior is removed, not adapted, since
  the behavior itself no longer exists.
- Full existing suite (`go test ./...`, `go vet ./...`, `gofmt -l`) and `make
  all` must pass with the concept fully absent — not just unreferenced.
