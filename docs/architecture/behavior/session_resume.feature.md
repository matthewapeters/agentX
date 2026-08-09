# Behavior — Session Resume

Status: Design — not yet implemented.

## Problem

`Orchestrator.Start()` (`internal/runtime/orchestrator.go:275`) unconditionally
calls `o.store.Create()`: a brand-new session ID, a fresh directory, fresh
`seedWorkingMemory()` (bootstrap facts only), and an empty in-memory
conversation history, on every single `agentx` launch. All existing "resume"
machinery in this codebase (`internal/surfaces/client`,
`docs/build-plan/06_system_surfaces_backlog.md`) means a *surface* reattaching
to an *already-running* orchestrator's live event stream by ordinal cursor — a
different, already-working capability. Quitting `agentx` and relaunching it
loses the conversation entirely; the session's persisted event log, working
memory, and artifacts sit on disk, unread by the new process.

This doc specifies resuming into the **same, already-established session
directory** — not forking a new session seeded from an old one's history —
per the explicit product decision. That choice pulls in two correctness
concerns a fork-based design would not have needed: ordinal continuity (the
new process's first published event must not collide with an ordinal already
on disk) and concurrent-writer safety (nothing today stops two `agentx`
processes from writing into the same `events/` dir at once). Both are
addressed below.

## Design

### 1. Locking: `flock(2)` + PID, mirroring `config.LockConfig`

`internal/config/config.go`'s `LockConfig`/`Unlocker` already establishes the
exact pattern needed here — open a lock file, `syscall.Flock` it, write the
holder's PID and a timestamp into its content for diagnostics. `Store.Lock`
(new, `internal/session/lock.go`) adapts it for sessions with one behavioral
difference: `LockConfig` uses a **blocking** `LOCK_EX` (config writers should
politely wait their turn); a session lock must be **non-blocking**
(`LOCK_EX|LOCK_NB`) — a resume attempt against a still-running session must
fail immediately with a clear message, not hang until the other process
exits.

```go
func (s *Store) Lock(id string) (*Unlocker, error) {
    f, err := os.OpenFile(filepath.Join(s.Dir(id), "session.lock"),
        os.O_CREATE|os.O_RDWR, 0o644)
    ...
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
        if errors.Is(err, syscall.EWOULDBLOCK) {
            holder, _ := io.ReadAll(f) // best-effort: PID + timestamp already there
            return nil, &SessionLockedError{Holder: string(holder)}
        }
        return nil, err
    }
    _, _ = f.Write([]byte(fmt.Sprintf("%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))))
    return &Unlocker{f: f}, nil
}
```

**Why `flock`, not a bare PID liveness check**: a recorded PID alone is
unsound for this — PIDs are recycled by the OS, so "is PID N alive" can
answer *yes* for a completely unrelated process once enough time/process
churn has passed since the original `agentx` exited (a real risk on a
long-uptime desktop, not exotic). It's also racy: "check PID, decide it's
dead, then start writing" has a gap two concurrent resume attempts could both
slip through. `flock` is atomic (kernel-arbitrated, not app logic — no race
window) and self-releasing (tied to the kernel's own open-file-descriptor
table, so a crash, `kill -9`, or power loss releases it automatically with no
cleanup code required to run correctly). The PID is still recorded in the
lock file's content, but purely for a legible error message
("session already running: PID 48213, since 2026-08-08T14:03:11Z") — the
*enforcement* is the flock, not the PID value.

**Every session locks, not just resumed ones.** If only the resume path
acquired the lock, a plain fresh `agentx` (no `--resume`) would never hold
it, and a later resume attempt against that session would see "unlocked" and
collide with it anyway. `Orchestrator.Start()` acquires the lock right after
`Create()`/`Load()` succeeds (for a fresh session this can't practically
contend, since `Create()` always allocates a new, unique directory; the lock
is acquired uniformly regardless, so the code path — and the guarantee — is
the same either way) and releases it in `Shutdown()`.

**Platform note**: `syscall.Flock` is POSIX (Linux/macOS/BSD); Windows has no
equivalent and would need a different primitive (`LockFileEx`) or a small
cross-platform dependency (e.g. `github.com/gofrs/flock`). Out of scope here,
same posture as the filesystem tools' cross-platform notes: the Linux-native
path ships now; a Windows implementation is future/contributor work, not
silently pretended to be handled.

### 2. Resuming without a target: list prior sessions, showing name + last prompt

`agentx --resume` with no value shows a picker; `agentx --resume <name-or-id>`
resumes that one directly (matching the existing `--session`/`-s`
value-or-absent parsing shape already in `internal/cli/cli.go`'s
`nextValue`). `agentx --resume last` resumes the most recent session without
listing anything — an ergonomic special case, not a separate mechanism (just
index 0 of the same sorted list).

`Store.ListResumable(limit int) ([]ResumeCandidate, error)` (new,
`internal/session`) enumerates `SessionRoot()`'s subdirectories, reads each
one's `session.json` for its name, and finds the most recent non-ephemeral
`user_prompt` event for the display line. A session with no user_prompt at
all (started, bootstrap greeting shown, immediately quit) is excluded — there
is nothing to show, and nothing meaningfully to resume. `Ephemeral` events
(the bootstrap exchange) are skipped, the same exclusion rule
`internal/surfaces/context/context.go`'s `Apply` already applies for the same
reason.

```go
type ResumeCandidate struct {
    ID         string
    Name       string
    LastPrompt string    // truncated for display; the full text is not needed here
    LastPromptAt time.Time
}
```

Finding the last user_prompt does **not** require loading a session's full
event log. `Recorder.Write`'s filenames are epoch-prefixed and zero-padded
(`<epoch>_<seq>_<content_type>.json`), so they sort chronologically by name;
reading a directory's entries in reverse and stopping at the first
`user_prompt`-typed file (rather than `Recorder.Load`'s current
read-everything-then-filter) keeps a listing across many sessions cheap. This
machine already has 200+ session directories on disk — reading every event of
every one just to build a picker would be genuinely wasteful, not a
theoretical concern.

The list is capped (a constant, not a flag — this CLI has no precedent for
that level of configurability and doesn't need one here) at the most recent
N by last-activity time, printed to stdout as a plain numbered list (this
happens before the TUI/Bubble Tea program starts, so it's an ordinary
stdin/stdout prompt, not a widget):

```
 1. eager-enchanting-mango   "implement the prompt in ./tidal_two.md"
 2. soft-alarming-nurse      "review the tidal_one.md implementation"
 ...
Resume which session? [1-N]:
```

An invalid/unmatched name or ID given directly (`--resume nonexistent-name`)
is a clear error and non-zero exit — never a silent fallback to creating a
new session under that name, which would be a much worse failure mode than
just refusing.

### 3. The resume load path

`Orchestrator.Start()` branches on whether a resume target was resolved
(`Settings.ResumeSessionID`, set by the CLI layer after the picker/lookup
above, or empty for the existing fresh-session path):

- `Store.Load(id)` (new, mirrors `Store.Create`) reads `session.json` back
  into an `Identity`, instead of minting a new one.
- `LoadWorkingMemory(id)` (already exists, already used elsewhere) replaces
  `seedWorkingMemory()` — no fresh bootstrap facts overwrite what the session
  already had.
- **`o.history` reconstruction** (new — `historyFromEvents`,
  `internal/runtime/core_context.go`, alongside `turnMsg`/`recordTurn`): walks
  `Recorder(id).Load()`'s full event list in order, skipping `Ephemeral`
  events, and appends one `turnMsg` per `user_prompt`/`agent_response`/
  `tool_call`/`tool_result` event. Unlike `recordTurn`'s forward-direction
  construction (which hardcodes `enabled: true` for fresh user/assistant
  turns), reconstruction reads `enabled: ev.Enabled` uniformly for every
  entry — confirmed via `Orchestrator.SetEventEnabled`
  (`orchestrator.go:969`) that a toggle already rewrites the specific
  persisted event file (`Recorder.SetEnabled`), so the on-disk log is
  already the correct, up-to-date source of truth for this, exactly as
  `06_system_surfaces_backlog.md` states ("the durable append-only log ...
  is the source of truth, including each event's `enabled` state"). No new
  persistence work is needed for this half of the problem — only the
  reverse-direction read.
- **Bus ordinal continuity** (new — a `state.NewBusFrom(startOrdinal uint64)`
  constructor alongside `state.NewBus()`): `state.Bus.ordinal` is an
  in-process `atomic.Uint64`, always zeroed by `NewBus()` today. On resume it
  must be seeded from the max `Ordinal` found in the just-loaded event list,
  or the new process's first published event collides with ordinal 1 already
  on disk — a real bug, not a style concern: ordinal is the exact key
  `SetEventEnabled` matches against and the cursor a reattaching surface
  seeds its live-stream boundary from (`Bus().CurrentOrdinal()`,
  `internal/transport/http/server.go`'s `handleEvents`). A collision means a
  toggle or a surface's resume cursor can silently target the wrong event.
- Attach token / `transport.json`: **no new work** — `Start()` already
  unconditionally mints a fresh token and writes `transport.json` for every
  session, resumed or not; a resumed session's new process gets its own
  correct, current endpoint/token for free from existing code.

### 4. Triggering resume from inside a running `ax` session

A user mid-conversation (or just having launched a fresh, empty `ax`) may
want to switch to resuming a *different*, prior session without quitting to
a shell first. This needs an in-TUI trigger and a selector — but critically,
**not** a live in-process rebuild of the running `Orchestrator`'s state.
`Start()` is explicitly one-shot (`if o.started { return error }`), and none
of its internal state (bus, history, working memory, model, session
identity) was built to be safely re-initialized while surfaces stay
attached; doing that live would be a materially larger, riskier piece of
work than everything above.

Instead, this reuses the entire launch-time resume path unchanged, just
triggered from a keystroke instead of a shell argument — because the
bundled `ax` launch (`internal/app.RunChat`, per `cmd/agentx/main.go`'s own
header: "agentx boots the core orchestration server and the human-agent chat
surface together") is **one OS process**, not client+server as two. `RunChat`
holds a direct Go reference to `*runtime.Orchestrator` (`orc`) and talks to
it through a plain in-process `chat.Bridge` of closures — never
`transporthttp.Client` — confirmed by reading `app.go` directly, not
assumed.

**The picker needs no new server API at all.** `session.NewStore(root)` is a
trivially-constructible, stateless path wrapper (`&Store{root: root}`, no
shared mutable state) — the escape-key handler in `app.RunChat` can
construct a second, throwaway `Store` pointed at `orc.Settings().SessionRoot`
and call `ListResumable` on it directly. This is the exact same function
section 2 already specifies for the CLI picker, called from a different
place, not a second implementation.

**Triggering the switch**: on selection, `app.RunChat`'s handler runs the
*same* clean-quit path an ordinary `ctrl+q` already takes (restoring the
terminal from Bubble Tea's alt-screen — this must happen *before* anything
else, or the resumed session's fresh TUI can inherit a stale terminal mode),
then calls `orc.Shutdown(ctx)` (releasing the `session.lock` flock, same as
any other clean exit), then `syscall.Exec` — replacing the current process
image in place with `agentx --resume <chosen-id>`, same PID, same
controlling terminal, no new process to hand off to or orphan. This is the
same Unix-only caveat already noted for `flock`: `syscall.Exec` has no
Windows equivalent, so this trigger is Linux/macOS/BSD-only for now, same
posture as everything else platform-specific in this doc.

**The abandoned (outgoing) session**: at the moment of triggering the
switch, the session being left behind is checked against the *exact same*
"zero non-ephemeral `user_prompt` events" predicate `ListResumable` already
uses to exclude empty sessions from the picker (one canonical definition,
not two that could drift apart) — reused here to decide whether to delete
its now-abandoned directory. If it has zero prompts (a fresh `ax` the user
never typed into before deciding to resume something else), its directory is
removed as part of the same shutdown — this is `Orchestrator`-internal
housekeeping, the same category of deterministic infrastructure as `Start()`
calling `os.MkdirAll`, never a model tool call, so it never touches the
approval system. If it has even one real prompt — including a session
being abandoned *mid-conversation* — it is never deleted, full stop,
regardless of how the user got there. The check runs on the outgoing
session's own final state at the moment of the switch; nothing about it
depends on the session being resumed.

### 5. Surface continuity across the switch

Section 4's `Shutdown()` call, unmodified, would terminate every already-
attached external surface (files, config, context, context-history,
context-visualizer): `Shutdown()` (`orchestrator.go:444`) stops the HTTP
server (closing every open SSE stream), calls `o.surfaceReg.StopAll()`, and
removes the attach token from disk — confirmed by reading the function
directly, not assumed. `internal/surfaces/client/client.go` has no
reconnect/retry logic today (zero hits for reconnect/retry/backoff), so a
disconnected surface would otherwise just sit there, requiring the user to
manually relaunch each one. This section replaces that outcome with a
clean handoff, using four pieces, three of which are existing mechanisms
reused rather than new ones invented from scratch.

**A new signal event, mirroring the existing `ContentConfigChanged`
precedent.** `internal/state/event.go` already documents exactly this shape:
"emitted by the orchestrator's config-file watcher whenever agentx.toml is
modified externally. Attaching config surfaces consume it to reload the
tree; other surfaces ignore it." A new `ContentSessionSwitching` content
type (payload: the new session's ID and name) follows the same pattern —
`DefaultEnabled` returns `false` for it, same as `ContentConfigChanged`: it
is a surface notification, never conversation context.

**Publish-and-flush must happen before the server stops, not as part of the
same teardown.** A new `Orchestrator.ShutdownForResume(ctx, newSessionID
string) error` (alongside, not replacing, the existing `Shutdown`) publishes
`ContentSessionSwitching` to the bus, waits a short bounded grace period
(long enough to flush one small SSE frame over a loopback connection — not
tuned to accommodate a stuck or slow surface, which must never block the
whole resume), then calls the existing `Shutdown` unchanged. `app.RunChat`'s
mid-session trigger (section 4) calls this variant instead of plain
`Shutdown`; the launch-time `--resume` path (section 3, nothing attached to
notify) keeps calling plain `Shutdown` as today.

**Port continuity via explicit handoff, not the coincidence that
`Allocate`'s deterministic ascending scan usually produces anyway.**
`syscall.Exec` passes the environment through cleanly, so before exec'ing,
`app.RunChat`'s trigger sets an env var carrying the currently-bound port;
`Start()` checks for it and attempts that exact port first, falling back to
the normal `Allocate` range-scan if something else claimed it in the gap
(rare, but not architecturally excluded — the reconnect design below stays
correct even then, it just costs the affected surfaces a few extra poll
ticks).

**Surfaces reconnect by re-running their own existing attach bootstrap, not
new bespoke reconnect code.** Every surface already knows how to "resolve
endpoint+token for session X from disk, clear local state, seed-then-
subscribe from scratch" — that's what `agentx surface launch` already does
on every ordinary launch. `internal/surfaces/client.Host`/`SurfaceModel`'s
lifecycle is restructured so that bootstrap is a re-invokable operation, not
a one-time startup path, triggered by: (a) receiving
`ContentSessionSwitching` — tear down the current SSE subscription and
re-run the bootstrap against the *named* new session, or (b) a bare
connection drop with no prior signal (a crash, not a deliberate switch) —
re-run the same bootstrap against the *same* session ID it already had, a
useful independent side effect (surviving a crash-and-manual-restart no
longer requires relaunching every attached surface by hand either).
Reusing the existing bootstrap is what makes this solve section 4's "don't
leave the abandoned session visible" concern for external surfaces the same
way `syscall.Exec` already solves it for free for the bundled chat surface —
clearing old widgets is a natural consequence of re-running a fresh attach,
not a special case to remember.

**No token hand-off mechanism is needed.** Because reconnection re-runs the
*existing* SS-5 disk-auto-resolve bootstrap (`agentx surface launch <kind>
--session <id>` already needs no token flag), and because `Start()` already
unconditionally mints a fresh token for whatever session it becomes, the
reconnecting surface simply picks up the new, correct token the same way a
manual relaunch already would. This is why the design does not try to pass
the old token through the switch — there is nothing to pass; the existing
mechanism already resolves the current, correct value on every attempt.

**The poll/retry loop re-resolves from disk on every attempt, not once,
cached.** This is what makes the residual "port didn't come back identical"
risk noted above a non-issue rather than a hard failure: a surface's retry
doesn't trust a remembered endpoint from the moment it received the
switching signal — it re-reads `transport.json`/the attach token fresh each
attempt, the same disk-resolution `ShortLaunchCommand` already relies on.
If the port changed, the next process's `transport.json` reflects that, and
the next poll tick picks it up. Two layers of recovery compound here: the
explicit signal says *when* and *which session* to look for; the poll loop's
fresh disk reads make it robust to *where* that session actually ended up
listening.

```
GIVEN multiple external surfaces attached to a running session
WHEN  a mid-session resume switch is triggered
THEN  each surface receives ContentSessionSwitching before its connection
      is closed, tears down its current subscription, and reconnects to the
      new session with freshly cleared local state — no widgets from the
      abandoned session remain visible

GIVEN a surface that is slow to consume the switching signal
WHEN  the grace period before Shutdown proceeds elapses
THEN  Shutdown proceeds regardless — a single slow or stuck surface must
      never block the resuming session from switching

GIVEN the new process's port handoff fails (something else claimed the
      port in the gap between the old listener closing and the new one
      binding)
WHEN  a surface's reconnect attempt fails against the remembered port
THEN  its next poll attempt re-resolves the endpoint from transport.json on
      disk rather than retrying the same remembered value, and succeeds once
      the new process's actual (possibly different) endpoint is visible
      there

GIVEN a surface loses its connection with no ContentSessionSwitching having
      been received first (the process crashed, not a deliberate switch)
WHEN  the surface detects the drop
THEN  it retries against the *same* session ID it already had, not the
      switching behavior — recovering automatically from a crash without
      requiring the user to manually relaunch every attached surface
```

### Explicit non-goals

- **In-flight plan/task state** (`o.planTrees`) is not reconstructed on
  resume — abandoned, same as it already is on any ordinary process restart
  today. Resurrecting a mid-drain plan is a separate, materially harder
  problem not addressed here.
- **Cross-provider resume** (a session started under Ollama, resumed with
  `provider = "llamacpp"` in `agentx.toml`, or vice versa) is not validated —
  `session.json` records no provider/model field today, so there is nothing
  to check against. The reconstructed history is provider-agnostic text, so
  this will generally still work, but is explicitly unverified, not silently
  guaranteed.

```
GIVEN a session directory with persisted events, working memory, and no
      active flock on session.lock
WHEN  `agentx --resume <name>` runs
THEN  it acquires the lock, loads working memory from disk instead of
      bootstrapping fresh facts, reconstructs o.history from the persisted
      event log (respecting each event's persisted enabled state), seeds the
      new Bus's ordinal past the highest one already on disk, and proceeds
      into the normal chat loop as if the process had never stopped

GIVEN a session directory whose session.lock is currently held (flock
      contended) by another live agentx process
WHEN  `agentx --resume <name>` runs
THEN  it fails immediately (non-blocking) with a clear "session already
      running: PID N, since T" message, sourced from the lock file's own
      recorded content, and does not touch working memory, events, or start
      any runtime state

GIVEN a session directory whose session.lock exists on disk but is not
      currently flocked by any live process (the prior agentx crashed or was
      killed without a clean Shutdown)
WHEN  `agentx --resume <name>` runs
THEN  the flock acquires successfully — the OS released it automatically
      when the prior process's file descriptors were torn down — and resume
      proceeds normally; no stale-lock cleanup code is needed for this case

GIVEN `agentx --resume` with no target
WHEN  more than one resumable session exists
THEN  a numbered list is printed (name + most recent non-ephemeral
      user_prompt per session, most-recent-first, capped at a fixed count)
      and the user is prompted to choose one

GIVEN a session that was created and immediately quit with no user_prompt
      ever recorded
WHEN  the resume list is built
THEN  that session is excluded — there is no last-prompt line to show, and
      nothing meaningful to resume

GIVEN `agentx --resume nonexistent-name`
WHEN  no session matches that name or ID
THEN  it fails with a clear error and a non-zero exit — never falls back to
      silently creating a new session under that name

GIVEN a session where the user toggled a tool_result off (disabled) before
      the process stopped
WHEN  that session is resumed
THEN  the reconstructed o.history entry for that tool_result is disabled,
      matching the persisted event's Enabled field — the toggle survives
      the restart, it is not reset to whatever a fresh turn would default to

GIVEN a running ax session with at least one recorded user_prompt
WHEN  the user triggers resume-to-a-different-session mid-conversation
THEN  the current session's directory is left fully intact — its content is
      never deleted just because the user chose to switch away from it

GIVEN a running ax session that has just launched with no user_prompt
      recorded yet (only the bootstrap exchange)
WHEN  the user triggers resume to a different session before typing anything
THEN  the outgoing session's directory is removed as part of the same
      shutdown, using the identical "zero non-ephemeral user_prompt events"
      check ListResumable already applies — no separate definition of
      "empty" to keep in sync

GIVEN a resume trigger fired from inside the TUI (not from a shell arg)
WHEN  the switch proceeds
THEN  the terminal is restored from Bubble Tea's alt-screen (the same
      clean-quit path ctrl+q already takes) before Shutdown/exec runs, not
      after — a resumed session's fresh TUI must not inherit a stale
      terminal mode
```

## Tests

- `internal/session/lock_test.go` (new): `Lock` succeeds when unheld and
  writes PID+timestamp into the lock file; a second `Lock` call on the same
  ID while the first is still held fails immediately (non-blocking) with a
  `SessionLockedError` carrying the first holder's recorded content;
  `Unlock` releases it and a subsequent `Lock` succeeds; closing the file
  descriptor without calling `Unlock` (simulating a crash) still leaves the
  lock acquirable by a fresh `Lock` call, proving the self-release property
  without any explicit cleanup code.
- `internal/session/resume_test.go` (new): `ListResumable` excludes a
  session with no user_prompt, returns the correct last-prompt text and
  timestamp, sorts most-recent-first, respects the cap; `Store.Load`
  round-trips an `Identity` written by `Store.Create`.
- `internal/runtime/core_context_test.go` (new tests alongside existing
  `turnMsg`/`recordTurn` coverage): `historyFromEvents` reconstructs
  user/assistant/tool entries in order, skips ephemeral events, and — the
  scenario most likely to be gotten wrong silently — preserves each event's
  persisted `Enabled` state rather than defaulting it.
- `internal/state/bus_test.go`: `NewBusFrom(n)`'s first `Publish` call
  stamps an ordinal strictly greater than `n`.
- An end-to-end style test (mirroring this session's own
  `TestRequestApprovalGlobalDecisionReusesScopeWithoutReprompting` pattern):
  start a real `Orchestrator`, publish several events, `Shutdown()`, start a
  **second** `Orchestrator` instance resuming the same session ID, publish
  one more event, and assert its ordinal is strictly greater than every
  ordinal from the first instance — this is the specific class of bug
  (ordinal/ID collisions) that has slipped past code-review-only review
  earlier in this project's history and needs a real running-process-level
  test, not just unit coverage of the pieces in isolation.
- `internal/session/resume_test.go`: the same "zero non-ephemeral
  user_prompt" predicate applied to deciding cleanup — a session with one
  prompt is never a deletion candidate even when explicitly checked
  mid-conversation; a session with none is. One shared predicate function
  tested once, not two call sites independently asserting the same rule.
- `internal/app` (new test alongside `RunChat`'s existing coverage, using a
  fake/stubbed process-replace instead of a real `syscall.Exec` — a unit
  test cannot safely exec-replace the test binary itself): the resume
  trigger calls the terminal-restore/clean-quit path strictly before the
  shutdown-and-exec sequence, not after, and the outgoing session's
  emptiness check runs against its state *at the moment of the switch*, not
  some cached earlier snapshot.
- `internal/runtime/orchestrator_test.go`: `ShutdownForResume` publishes
  `ContentSessionSwitching` (asserting the payload carries the new
  session's ID/name) before the underlying `Shutdown` runs — ordering, not
  just presence; a slow bus subscriber does not block `Shutdown` past the
  grace period.
- `internal/state/event_test.go`: `DefaultEnabled(ContentSessionSwitching)`
  is `false`, matching `ContentConfigChanged`'s existing precedent — it is a
  surface notification, never conversation context.
- `internal/surfaces/client` (new tests on `client.Host`'s restructured
  lifecycle): receiving `ContentSessionSwitching` tears down the current
  subscription and re-runs the attach bootstrap against the *new* session
  (local state cleared, not merged with the old session's); a bare
  connection drop with no prior signal re-runs the bootstrap against the
  *same* session instead; a reconnect attempt re-resolves
  endpoint/token from disk on every retry rather than reusing a value
  cached from the original signal or the first failed attempt.
- An end-to-end style test extending the ordinal-continuity test above:
  start an `Orchestrator` with a stub surface attached (a real subscriber
  on the bus, not a mock that just records calls), trigger
  `ShutdownForResume`, and assert the stub actually receives
  `ContentSessionSwitching` before its connection observably ends — the
  ordering guarantee is the entire point of this section, and it is exactly
  the kind of cross-component timing behavior that survives a code review
  and fails in practice, per this project's own history.
- `make all` must pass throughout.
