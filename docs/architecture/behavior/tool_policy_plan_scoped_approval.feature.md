# Behavior — Plan-Scoped Tool Approval

Status: **Implemented** (2026-08-02).

## Problem

`tools.Policy` offers two approval scopes today: `ScopeSession` (this
conversation only, in-memory) and `ScopeGlobal` (every session, persisted to
disk). A multi-step plan (`runPlanPhase`/`runWavefrontPhase`,
`internal/runtime/plan_cycle.go`/`wavefront_cycle.go`) routinely issues the
same write/network call several times across its leaves — e.g. `write_file`
to the same scratch path from three different steps, or `http_get` against
the same host. Approving "for this session" is broader than the user may
want (it survives long after this specific investigation ends, silently
whitelisting the call for whatever comes next in the conversation);
approving "for all sessions" is broader still. Denying and re-approving every
single occurrence is the only alternative today, which is exactly the
repeated-interruption cost item 5 (of the original six observations this
approval-loop effort addresses) called out. A third scope — valid only for
the plan currently draining, gone once it finishes — closes that gap without
widening the blast radius of either existing scope.

## Design

`tools.Policy` gains a third approval set, keyed by plan root id (plans are
already required to carry a session-unique root id — see `runPlanPhase`'s
doc comment on `rootID`):

```go
type Policy struct {
    blacklist []Rule
    global    map[string]ApprovalEntry
    session   map[string]bool
    plan      map[string]map[string]bool // root -> approvalKey -> true
}

func (p *Policy) ApprovePlan(root string, d Descriptor, args map[string]string)
func (p *Policy) PlanApproved(root string, d Descriptor, args map[string]string) bool
func (p *Policy) ExpirePlan(root string)
```

`ApprovePlan`/`PlanApproved` are no-ops for `root == ""` (a call outside any
plan — the single-tool cycle, `ConversationCore.runNativeToolCall` — never
has a root, so the plan option is never offered there in the first place;
see below). Deliberately not folded into `Policy.Evaluate`, whose signature
(`Evaluate(d, args)`) has no root parameter and is called from many
plan-agnostic sites (`ConversationCore`, `executor.Gate`, existing tests) —
widening it everywhere for one caller's need is out of scope. Instead the one
call site that has a root (the plan executor's approver, below) checks
`PlanApproved` itself, before ever reaching `RequestDecision`, to skip
re-prompting for a call already approved within this plan.

**Threading the root to where it's needed.** `executor.Approver.Approve`
gains the task record being executed (it already had no way to identify
which plan a leaf belongs to — that mapping lives in
`internal/runtime/plan_tree.go`'s `planTreeRegistry.ownerOf`, not in the
executor package):

```go
type Approver interface {
    Approve(ctx context.Context, d tools.Descriptor, args map[string]string, rec task.Record, reason string) bool
}
```

`planTreeRegistry` gains a read accessor for that mapping:

```go
func (r *planTreeRegistry) rootOf(id string) (root string, ok bool)
```

`buildTaskExecutor`'s approver closure (`classifier_pipeline.go`) resolves
the root, short-circuits on a standing plan approval, and otherwise prompts
with the root threaded through:

```go
approver := executor.ApproverFunc(func(ctx context.Context, d tools.Descriptor, args map[string]string, rec task.Record, _ string) bool {
    root, _ := o.planTrees.rootOf(rec.ID)
    if o.policy.PlanApproved(root, d, args) {
        return true
    }
    v, err := o.RequestApproval(ctx, d, args, o.policy, root)
    return err == nil && v.Decision == tools.Allow
})
```

`ApprovalSeeker.RequestApproval` (the interface `ConversationCore` also
depends on) and `Orchestrator.RequestApproval` both gain the same `root
string` parameter. `ConversationCore.runNativeToolCall` — the single-tool
cycle, never inside a plan — passes `""`, so it never sees the plan option;
behavior there is unchanged. `RequestApproval` picks the option set based on
whether a root is present:

```go
var toolApprovalOptions = []state.ApprovalOption{
    {Label: "Approve for this session", Decision: "session"},
    {Label: "Approve for all sessions", Decision: "global"},
    {Label: "Deny", Decision: "deny"},
}

var toolApprovalOptionsPlan = []state.ApprovalOption{
    {Label: "Approve for this session", Decision: "session"},
    {Label: "Approve for this plan", Decision: "plan"},
    {Label: "Approve for all sessions", Decision: "global"},
    {Label: "Deny", Decision: "deny"},
}

func toolApprovalOptionsFor(root string) []state.ApprovalOption {
    if root == "" {
        return toolApprovalOptions
    }
    return toolApprovalOptionsPlan
}
```

and handles the new decision string identically to the other two scopes:

```go
case "plan":
    pol.ApprovePlan(root, d, args)
    return tools.Verdict{Decision: tools.Allow}, nil
```

**Expiry.** Both plan-running entry points (`runPlanPhase`,
`runWavefrontPhase`) construct their `root` task record once, at the top —
`defer o.policy.ExpirePlan(root.ID)` right after that construction covers
every return path (normal completion, `ctx.Err()` cancellation, or an error
mid-drain) uniformly, so a plan-scoped approval never outlives the plan it
was scoped to, regardless of how that plan ends. Guarded by `o.policy != nil`
(mirroring the same nil check `toolsReady()` already does) — some tests
inject `taskExec`/`taskDecomp` directly via `WithTaskExecutor`/
`WithDecomposition`, bypassing `buildTaskExecutor` (the only path that ever
wires a policy in), so `o.policy` is legitimately nil there and plan-scoped
approval is simply inapplicable, not an error.

```
GIVEN a plan leaf proposes a write_file call and the user chooses
      "Approve for this plan"
WHEN  a later leaf in the SAME plan proposes the identical tool+args
THEN  it executes without a new approval prompt (PlanApproved short-circuits
      the executor's approver before RequestDecision is reached).

GIVEN the same tool+args was approved "for this plan" in an EARLIER,
      already-finished plan (a different root id)
WHEN  a NEW plan (or the single-tool cycle) proposes the identical call
THEN  it still requires approval — a plan-scoped approval never survives
      past ExpirePlan(root), and the single-tool cycle has no root at all.

GIVEN the single-tool cycle (ConversationCore.runNativeToolCall, root == "")
      proposes a call needing approval
WHEN  the approval panel renders
THEN  it offers exactly the original three options (session/global/deny) —
      "Approve for this plan" never appears outside a plan, since there is
      no plan to scope it to.

GIVEN a plan-scoped approval was granted mid-plan
WHEN  that plan finishes draining (success, cancellation, or error)
THEN  ExpirePlan(root) discards it — a later, unrelated plan reusing the same
      tool+args (with a new, different root id) is not affected either way,
      since plan-scoped approvals are already keyed per-root; ExpirePlan's
      purpose is bounding Policy.plan's memory growth over a long session,
      not correctness of a fresh root.
```

## Tests

- `internal/tools/policy_test.go` (extended): `ApprovePlan`/`PlanApproved`
  round-trip for a given root; a no-op for `root == ""`; `PlanApproved`
  false for a different root with the identical tool+args; `ExpirePlan`
  clears exactly that root's entries and leaves others untouched.
- `internal/runtime/approval_test.go` (extended): `toolApprovalOptionsFor("")`
  omits the plan option, `toolApprovalOptionsFor("root-1")` includes it;
  `RequestApproval`'s `"plan"` decision calls `pol.ApprovePlan` with the
  given root and returns `Allow`.
- `internal/runtime/plan_tree_test.go` (extended): `rootOf` returns the
  registered root for a dispatched leaf id, and `("", false)` for an unknown
  id.
- Full existing suite / `make all` passes unchanged.
