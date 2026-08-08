# Behavior — grep_files: Content Search Across Files

Status: In progress.

## Problem

The read-only tool surface (`read_file`, `list_dir`, `tree`, `find_path`) has no
way to search *inside* file contents for a pattern. `find_path` only matches
filenames. A question like "where is `RequestApproval` called from" or "find
every reference to `ContentToolCall`" has no primitive today except reading
files one at a time and hoping — the same class of hallucinated-path risk
`tree`'s own doc comment names as the reason it was added ("the repeated cause
of hallucinated paths this session: package.json/tsconfig guessed instead of
observed", `internal/tools/descriptors.go`). A live test of the native
tool-calling loop against `tidal_two.md` (2026-08-08) surfaced the sibling gap
on the write side (`write_file` couldn't create a missing directory, fixed
separately) — this doc addresses the analogous, higher-priority gap on the
read side.

## Design

`grep_files` is a Go builtin (`Builtin: "grep_files"`, no `Command`/`Argv`),
not a wrapper around the host's `grep`/`ripgrep` binary — deliberately, for
two reasons: (1) it removes a dependency on any specific external tool being
installed at all, and (2) it means the tool's behavior is identical on every
OS Go itself supports. `os.ReadDir`/`filepath.WalkDir`/`bufio.Scanner`/
`regexp` are already cross-platform in the Go standard library, so — unlike a
`Command: "grep"`/`Command: "rg"` Argv template, which would need an
OS-specific binary name and flag set (`findstr` on Windows has an entirely
different flag surface) — this tool needs no build tags, no per-OS Argv
variants, and no contributor work to "port" later; portability is a property
of the implementation choice, not a future task.

Args: `path` (file or directory root to search — a directory is walked
recursively), `pattern` (a Go `regexp` pattern, RE2 syntax — not PCRE; no
lookahead/lookbehind), `max_results` (optional, caps the number of matching
lines returned so a broad pattern over a large tree can't produce an
unbounded result). `Risk: RiskRead` — no approval required, matching
`read_file`/`list_dir`/`tree`/`find_path`; it only reads, never writes.

Walking a directory root reuses `tree`'s exact exclusion list
(`node_modules|.git|vendor|__pycache__|.venv|dist|build|.next|target`) so a
repo-root search doesn't wander into generated/vendored trees a human
wouldn't want scanned either — this list already exists as a literal in
`descriptors.go`'s `tree` entry; `grep_files` reuses the same set (as a real
Go slice both descriptors reference, not a second hand-copied string) rather
than defining a second, driftable copy.

A file is skipped without error if it looks binary (a `\x00` byte in the
first 8KB read) — matching common grep/ripgrep practice — since a binary
match is never useful output and risks emitting non-UTF8 bytes into the
model's context. A per-file read error (permission denied, a symlink cycle,
etc.) is likewise skipped, not fatal to the whole search: one unreadable file
in a large tree must not blank out every match found in the rest of it.

Output format per match: `path:line: text` (1-indexed line number), one line
per match, same shape a human would get from `grep -n`. `Preview` is the
joined match lines (capped by `max_results`); `Lines`/`Bytes` report the
actual returned size, same convention every other builtin already follows.

```
GIVEN a directory tree containing files with lines matching a regex pattern
WHEN  grep_files runs with path=<dir>, pattern=<regex>
THEN  it returns one "path:line: text" entry per matching line, walking the
      directory recursively and skipping the same generated/vendored
      directories `tree` already excludes

GIVEN a single file path (not a directory) as the target
WHEN  grep_files runs
THEN  it searches only that file, without walking anything else

GIVEN a pattern that matches more lines than max_results
WHEN  grep_files runs with max_results=N
THEN  exactly N matches are returned, not the full match set — the result is
      not silently truncated without the caller having asked for a bound

GIVEN a binary file within the search tree (contains a NUL byte in its first
8KB)
WHEN  grep_files walks over it
THEN  that file is skipped without error and without appearing in the results
      — it does not abort the search or emit non-text bytes into the result

GIVEN a file within the search tree the process cannot read (permission
denied)
WHEN  grep_files walks over it
THEN  that file is skipped without error; matches from every other readable
      file in the tree are still returned

GIVEN a pattern that is not valid RE2 regexp syntax
WHEN  grep_files runs
THEN  it fails with the regexp compile error as the result's failure reason,
      not a panic

GIVEN a path that does not exist
WHEN  grep_files runs
THEN  it fails with a clear "no such file or directory"-class error
```

## Tests

- `internal/tools/executor_test.go`: unique match in a single file, multiple
  matches across a directory tree, `max_results` truncation, binary-file
  skip, unreadable-file skip (permission-denied fixture, matching
  `edit_file`'s existing `os.Geteuid()==0` skip convention for a
  privilege-dependent test), invalid-regexp failure, nonexistent-path
  failure, vendored-directory exclusion.
