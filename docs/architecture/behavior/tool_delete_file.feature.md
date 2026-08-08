# Behavior — delete_file: Remove a Single File

Status: In progress.

## Problem

The mutating tool surface (`write_file`, `edit_file`, `apply_patch`) can
create and modify files but has no way to remove one. A cleanup or refactor
task that requires deleting an obsolete file has no path today — this was
confirmed directly this session: a stray `b/` directory left behind by an
earlier session's `apply_patch` mishap had to be cleaned up by a human
(via a shell, outside AgentX entirely), because no tool in the registry could
have done it.

## Design

`delete_file` is a Go builtin (`Builtin: "delete_file"`), using `os.Lstat` +
`os.Remove` — both already cross-platform in the Go standard library, same
rationale as `grep_files`: no OS-specific `Command`/`Argv` (`rm` vs `del`),
no build tags, nothing for a future contributor to "port".

Deliberately narrow blast radius: **it refuses to delete a directory**, even
an empty one, even with a hypothetical recursive flag — that is a
categorically more dangerous operation (unbounded, path-tree-wide) than this
tool is meant to authorize, and is not added here. A future
`delete_directory` tool, if ever wanted, is a separate, more carefully
approval-scoped decision, not a flag bolted onto this one.

Uses `Lstat`, not `Stat`, to classify the path entry itself rather than what
it points to: a symlink whose target happens to be a directory is still safe
to remove as a symlink (`os.Remove` on a symlink never follows it into the
target), so classifying by the symlink's *target* would incorrectly refuse a
perfectly safe removal. `Risk: RiskWrite`, `RequiresApproval: true` — same
tier as `write_file`/`edit_file`; a delete is exactly as consequential as an
overwrite. Approval is scoped by `Descriptor.ScopeArgs` the same way
`write_file`/`edit_file` already are (extension + project-boundary,
`internal/tools/pathscope.go`) — approving one `.md` file's deletion inside
the project covers the next one, an outside-project path is scoped per exact
path, same rules established for the other mutating file tools this session.

```
GIVEN a path that names an existing regular file
WHEN  delete_file runs
THEN  the file is removed from disk and the result reports success with the
      deleted path

GIVEN a path that names an existing directory
WHEN  delete_file runs
THEN  it fails without removing anything — directories are categorically out
      of scope for this tool, not a size/emptiness-dependent decision

GIVEN a path that is a symlink whose target is a directory
WHEN  delete_file runs
THEN  the symlink itself is removed (classified by Lstat, not by resolving
      the target) — this is the one case where "is it a directory" and "is
      it safe to remove" diverge, and the tool follows the latter

GIVEN a path that does not exist
WHEN  delete_file runs
THEN  it fails with a clear "no such file or directory"-class error, not a
      silent no-op

GIVEN a path the process lacks permission to remove
WHEN  delete_file runs
THEN  it fails with the permission error as the result's failure reason
```

## Tests

- `internal/tools/executor_test.go`: removes an existing file, refuses an
  existing directory (asserting the directory is still present afterward),
  removes a symlink pointing at a directory (asserting the symlink is gone
  but its target directory is untouched), fails on a nonexistent path, fails
  on a permission-denied removal (skipped under `os.Geteuid()==0`, matching
  `edit_file`'s existing convention).
- `internal/tools/pathscope_test.go` / `policy_test.go`: `delete_file` scopes
  through `ScopeArgs` identically to `write_file` (extension-scoped inside
  the project, exact-path-scoped outside it).
