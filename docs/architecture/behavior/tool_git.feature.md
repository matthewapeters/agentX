# Behavior — git: Run Any Git Subcommand

Status: In progress.

## Problem

The only git-aware tool in the registry was `git_status`, a fixed, read-only
`git status` call. Nothing could stage or commit changes, create or delete
branches, push, pull, tag, rebase, or run any other git subcommand — a live
session driving a real multi-hour implementation task hit this directly: the
model repeatedly reported the work "complete" and tried to "commit," but its
only reachable git-related tool was `git_status`, which just prints the same
"use `git add <file>...`" hint back without changing anything. Over 50 tool
calls looped through `git_status`/`run_checks`/file reads with no way to
actually reach a commit — not a reasoning failure, a missing capability.

## Design

`git` is a Go builtin (`Builtin: "git"`) that execs the real `git` binary via
`os/exec` (no shell) with `-C {path}` plus a caller-supplied argv tail —
**no subcommand allowlist or denylist**: `add`, `commit`, `push`, `pull`,
`reset`, `branch -D`, `rebase`, `tag`, anything git itself accepts, is
reachable. This is a deliberate product decision (unlike every other tool in
this registry, which is scoped to one fixed operation) — blast radius is
gated the same way every other mutating tool here is gated, by
`Risk: RiskWrite` + `RequiresApproval: true`, not by restricting which git
functionality is exposed.

The subcommand + arguments arrive as `args`, a **JSON array of argv tokens**
(e.g. `["commit", "-m", "a message with spaces"]`), not a single shell-style
string. `Descriptor.BuildArgv`'s `"{name}"` template substitutes exactly one
argv slot per named placeholder, which cannot represent a variable-length
tail — a multi-word commit message would either need a shell (reintroducing
an injection surface this codebase's whole argv-vector execution model
exists to avoid) or naive whitespace-splitting (breaks on any quoted or
multi-word token). A JSON array names each argv token explicitly and decodes
straight into `[]string`, so it execs exactly as `git`'s own argv, with no
shell in between.

Execution is refactored to share the same subprocess path every
`Command`/`Argv`-backed tool already uses: `Executor.runProcess` is extracted
from `Executor.Run`'s non-builtin branch (timeout, stdout/stderr capture,
exit-code classification, truncation, artifact persistence — previously
inline in `Run`, now a shared helper) so the `git` builtin's exec path never
drifts from every other subprocess tool's behavior. This is also why
`Executor.runBuiltin` now takes `ctx context.Context` — every other builtin
is a fast, synchronous filesystem op that never needed one; `git` is the
first builtin that execs a real subprocess and needs the timeout/cancellation
`TimeoutSeconds` already governs for `run_checks`/`apply_patch`/etc.

```
GIVEN a git repository and args = ["add", "hello.txt"] followed by a second
      call with args = ["commit", "-m", "add hello"]
WHEN  the git tool runs both calls in sequence
THEN  the file is staged and committed — a real mutation of git state, not
      just a read

GIVEN args naming a subcommand no other AgentX git tool exposes (e.g.
      ["branch", "-D", "does-not-exist"])
WHEN  the git tool runs
THEN  it reaches real git and fails with git's own error — no AgentX-side
      subcommand allowlist stands between the model and the requested
      functionality

GIVEN a subcommand that itself exits nonzero (e.g. ["log"] on a repository
      with no commits yet)
WHEN  the git tool runs
THEN  the result reports status "error" with git's real exit code and
      stderr — the tool reports git's outcome faithfully, it does not
      swallow or reinterpret it

GIVEN args that is not valid JSON, or decodes to an empty array
WHEN  the git tool runs
THEN  it fails cleanly with a descriptive error instead of passing a garbage
      token straight to argv or invoking bare `git -C {path}` with no
      subcommand

GIVEN a commit message containing spaces, quotes, and punctuation, passed as
      one JSON array element
WHEN  the git tool commits
THEN  the full message survives intact in history — the JSON-array argv
      shape carries a multi-word argument without shell-splitting or
      quoting corruption
```

## Explicitly out of scope

- No subcommand-level restriction, confirmation text, or scoped-approval
  description beyond the existing `RequiresApproval` gate — this mirrors
  every other `RiskWrite` tool's approval tier, not a bespoke one. A
  scope-aware approval prompt (as `write_file`/`edit_file`/`apply_patch`
  have via `Descriptor.ScopeArgs`) is a separate decision, not added here.
- No network credential handling beyond whatever the host's ambient git/SSH
  config already provides — `push`/`pull`/`fetch` run exactly as they would
  from a shell in the same working directory.

## Tests

- `internal/tools/executor_test.go`: stage-then-commit creates real history;
  an unrestricted subcommand (`branch -D` on a nonexistent branch) reaches
  git and fails with git's own error; a nonzero-exit subcommand is reported
  faithfully; invalid JSON and an empty array both fail cleanly; a
  multi-word, punctuated commit message survives intact through the
  JSON-array argv path.
- `internal/tools/descriptors_test.go`: `git` registers as
  `Builtin: "git"`, `Risk: RiskWrite`, `RequiresApproval: true`, with exactly
  `path`/`args` as required arguments; `git_status` remains unchanged
  (`RiskRead`, no approval) alongside it.
