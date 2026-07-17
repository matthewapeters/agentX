# Behavior — Continuous Scheduler Wires Value/Error onto Terminal Transitions (ADR 0012 Phase 5, amendment)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's 2026-07-17 amendment's
"Continuous-engine refactor" section and Phased Build Plan (amendment) step 5. Proves
the `Value`/`Error` population discipline against the smaller, better-understood
continuous engine before wavefront (step 7) depends on the same pattern.

Built exactly as scoped below. Tests: `internal/runtime/scheduler/value_error_test.go`
(new) covers all five scenarios plus the Denied-scoping choice explicitly, including
both previously-silent `applyDecompose` failure branches — the `graph.Update` cycle
case needed a deliberately constructed scenario (a child whose own stored `Deps`
points back at its not-yet-updated parent, so the cycle only closes once the parent
is updated to depend on it) to actually exercise, not just the simpler dangling-dep
`graph.Add` case. All pre-existing scheduler tests (`guard_test.go`, `node_test.go`,
`observer_test.go`) pass unchanged, confirming this phase is additive at the
observable-behavior level as designed. Full suite and `-race` on the scheduler
package both clean.

## Problem

Every terminal transition in `internal/runtime/scheduler/scheduler.go` today derives
only a `task.Status`, discarding the real text behind it:

- `Scheduler.execute` maps `executor.Outcome` to a `task.Status` and returns nothing
  else — `Result.Preview` (the resolved value on success) and `Reason` (the failure
  explanation) are both computed and immediately thrown away.
- `workDone`, the internal struct workers report completions through, carries no
  error/value payload at all. `doneError` is dispatched with nothing but the node
  `id` — the real error from `s.decomposer.Decompose` is dropped in `s.work`'s
  Kind-switch `default` branch too, and the ErrNoProgress-fallback execute path
  never had anywhere to put its outcome text either.
- `applyDecompose`'s two failure branches (`graph.Add`/`graph.Update` returning an
  error) both call `s.setStatus(wd.id, task.Failed)` with the real error
  (`ErrDuplicateID`/`ErrDanglingDep`/`ErrCycle`/etc.) checked and discarded in the
  same breath.

None of this reaches the persisted graph. A failed node's `plans/<root-id>.json`
entry today shows *that* it failed, never *why* — the reason is only ever visible
transiently, via `capturingExec`'s separate bookkeeping, if a caller happens to be
looking at the right moment.

## Design

### 1. `execute` returns the resolved value/error alongside the status

```go
func (s *Scheduler) execute(ctx context.Context, rec task.Record) (status task.Status, value, errText string)
```

- `executor.Executed` → `task.Done`, `value = out.Result.Preview`, `errText = ""`.
- `executor.Denied`/`NeedsApproval` → `task.Denied`, both strings empty. **Scoped
  deliberately**: `Record.Error` is documented as "set once Status becomes Failed"
  (ADR 0012 amendment §2) — a denial is a policy/user decision, not a failure
  (TOOL-7's Denied/Failed split), and its reason already reaches the user via the
  approval UI and `capturingExec`. Extending `Error` to cover Denied too is a
  reasonable future widening, not decided here.
- everything else (`Phantom`/`NoTool`/`Failed`) → `task.Failed`, `errText = out.Reason`.

### 2. `workDone` carries the payload through the channel

```go
type workDone struct {
	id      string
	kind    workKind
	status  task.Status   // for doneExecute
	value   string        // for doneExecute (Done)
	errText string        // for doneExecute (Failed) and doneError
	result  branch.Result // for doneDecompose
}
```

Every site that constructs a `workDone` for `doneExecute` or `doneError` now
populates `value`/`errText` from what it actually has — `s.execute`'s three return
values for `doneExecute` (both the normal Task path and the ErrNoProgress fallback);
`err.Error()` for `doneError`, on both the real decompose-error branch and the
invalid-Kind `default` branch (previously silent on both).

### 3. `setStatus` writes `Value`/`Error` onto the record, not just `Status`

```go
func (s *Scheduler) setStatus(id string, st task.Status, value, errText string)
```

Every existing call site is updated to pass what it has — the join-complete `Done`
transition (`decomposed[id]` branch in `Run`) and `doneAsk`'s `Abstained` transition
pass empty strings for both, honestly: neither has a resolved value or a failure to
report. `applyDecompose`'s two failure branches now pass the real `err.Error()`
instead of discarding it.

```
GIVEN a Task node executes successfully
WHEN  its terminal status is set
THEN  Record.Value holds the executor's result preview and Record.Error is empty.

GIVEN a Task node's execution fails (Phantom/NoTool/Failed from the executor)
WHEN  its terminal status is set
THEN  Record.Error holds the executor's Reason text and Record.Value is empty.

GIVEN a Step's Decompose call returns a real error (not ErrNoProgress)
WHEN  its terminal status is set to Failed
THEN  Record.Error holds that error's text.

GIVEN applyDecompose's graph.Add or graph.Update call fails while merging a
      Decompose result
WHEN  the parent node's terminal status is set to Failed
THEN  Record.Error holds the graph integrity error's text (e.g. "task: dependency
      cycle: ...", not a generic "failed").

GIVEN a Step node completes as a parent-as-join (all children Done) or is marked
      Abstained (bounded-recursion clarify)
WHEN  its terminal status is set
THEN  Record.Value and Record.Error are both empty — neither engine has anything to
      report for these transitions yet, and setStatus does not invent placeholder
      text.
```

## Tests

- `internal/runtime/scheduler/scheduler_test.go` (extended) — stub `Executor`/
  `Decomposer` implementations already exist for this package's tests; extend them
  (or add new stubs) to return a distinguishable Preview/Reason and assert the
  resulting `task.Graph` node carries the expected `Value`/`Error` for each of the
  five scenarios above, including the two previously-silent `applyDecompose` failure
  paths and the invalid-Kind `default` branch.
- Full existing `scheduler_test.go`/`guard_test.go`/`node_test.go`/`observer_test.go`
  suite must pass unchanged — this phase is additive at the observable-`Status`
  level; no existing assertion about dispatch order, `Status` values, or
  `ErrStalled`/`ErrNoProgress` behavior should need to change.
