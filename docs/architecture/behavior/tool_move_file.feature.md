# Behavior — move_file: Move or Rename a Single File

Status: In progress.

## Problem

There is no way to rename or relocate a file — a common refactor operation
("rename `foo.go` to `bar.go`", move a file into a new package directory).
Today the only available path is `read_file` + `write_file` to the new
location, with no way to remove the original (`delete_file` closes that half,
but the pair is a two-call, non-atomic imitation of a rename, not a real one)
— every "move" left the original behind as debris, the same class of gap
`delete_file`'s doc describes.

## Design

`move_file` is a Go builtin (`Builtin: "move_file"`), using `os.Lstat` +
`os.MkdirAll` + `os.Rename` — cross-platform via the Go standard library,
same rationale as `grep_files`/`delete_file`.

Args: `from` (existing path), `to` (destination path). Deliberately narrow,
mirroring `delete_file`: **refuses to move a directory** (classified via
`Lstat` on `from`, not `Stat`, for the same symlink-target reasoning
`delete_file`'s doc describes), and **refuses to overwrite an existing
destination** rather than silently clobbering it — no `force`/`overwrite`
flag is added, matching this codebase's existing non-clobbering conventions
(`make seed`'s baseline-config install, the approval env-file seed) rather
than introducing a flag whose only job is to opt back into a footgun. If
`to`'s parent directory doesn't exist yet, it is created first —
`os.MkdirAll(filepath.Dir(to), 0o755)`, the exact fix `write_file` just
received for the same reason (a live test run needed to create a new
package's first file; a move into a new package directory is the same case).

`os.Rename` can fail with an `EXDEV`-class error ("invalid cross-device
link") when `from` and `to` are on different filesystems or mount points —
this is a real `os.Rename` limitation on every OS Go supports, not a
Linux-specific one, and no copy+delete fallback is implemented for it here;
it surfaces as an ordinary tool failure. A same-filesystem move (the common
case: renaming or relocating a file within one project checkout) always
succeeds.

`Risk: RiskWrite`, `RequiresApproval: true`, same tier as `delete_file`.
Approval scoping is the one place this tool differs from the other mutating
file tools: it names *two* paths, not one, so `ScopeArgs` returns a scope
entry for each of `from` and `to` (deduplicated if they'd resolve to the same
scope) — mirroring `apply_patch`'s existing multi-path handling
(`internal/tools/pathscope.go`) rather than inventing a new pattern.
Approving a move requires — and then covers — the scope of both its source
and destination.

```
GIVEN an existing regular file at `from` and no existing entry at `to`,
      whose parent directory already exists
WHEN  move_file runs
THEN  the file is renamed to `to` and no longer exists at `from`

GIVEN `to`'s parent directory does not exist yet
WHEN  move_file runs
THEN  that directory tree is created first, then the file is moved into it —
      the same missing-parent-directory fix write_file received

GIVEN an existing entry already at `to`
WHEN  move_file runs
THEN  it fails without moving anything — the existing destination is left
      untouched, never silently overwritten

GIVEN `from` names a directory, not a regular file
WHEN  move_file runs
THEN  it fails without moving anything — directories are out of scope for
      this tool, same as delete_file

GIVEN `from` does not exist
WHEN  move_file runs
THEN  it fails with a clear "no such file or directory"-class error

GIVEN a call whose from/to resolve to scopes requiring approval
WHEN  the policy evaluates it
THEN  approval must cover both the from-scope and the to-scope; approving
      only one is not sufficient (mirrors apply_patch's "approving a patch
      approves every file it touched" completeness rule, applied to a
      two-path call instead of an N-file one)
```

## Tests

- `internal/tools/executor_test.go`: moves a file to a new name in the same
  directory, moves a file into a new (not-yet-existing) subdirectory, refuses
  when the destination already exists (asserting both the source and the
  pre-existing destination are unchanged afterward), refuses to move a
  directory, fails on a nonexistent source.
- `internal/tools/pathscope_test.go` / `policy_test.go`: `move_file`'s
  `ScopeArgs` returns one entry per distinct from/to scope; approving a move
  requires both scopes, matching `apply_patch`'s multi-file completeness
  test already established this session.
