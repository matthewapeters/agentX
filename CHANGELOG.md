# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased] - 2026-07-10

### Added

- **A response that states an intent to keep investigating ("Let me examine the
  source code...", "Should I check the config?") now actually continues instead of
  silently ending the turn** (sessions clever-raven-3, amber-quartz: the model
  correctly recognized its own investigation was incomplete, said so, and the turn
  just completed anyway — `finishCycle`'s only criterion is `err == nil`, with zero
  inspection of what the response text says). Detection is a deliberate two-step,
  non-LLM process (matching this codebase's existing preference for cheap structural
  checks over another model round-trip — `SimilarGoals`, the retry-then-degrade
  guard): a regex finds `"let me"`/`"should i"`/`"shall i"` in the response's last
  sentence and captures the verb, then the verb is checked against a new pair of
  externalized, human-editable lists (`agentx-continuation-verbs-allowed.md` /
  `-denied.md`, seeded with common investigative verbs and a few pre-seeded benign
  closers like "know"/"explain" that would otherwise trigger on every "let me know if
  you have questions"). A recognized-allowed verb runs one bounded follow-up
  decomposition round — reusing composed findings so it's grounded in what the first
  round already found, not just its own, directly building on the grounding fix below
  — before re-synthesizing the final answer; an unrecognized verb pauses the turn and
  asks the user (allow once / allow always / not this time / never), the same
  per-request-queue interaction shape as tool-call approval, generalized: the
  tool-approval gate (`vivid-raven`'s fix) is now `gate[Req, Resp]`, a small Go
  generic, with tool approval and verb-continuation approval as two independent
  instantiations sharing the identical proven queueing mechanics rather than
  duplicating them. "Always" persists the verb to the corresponding list so the
  system learns without needing every possible verb predicted in advance. New
  `state.PhaseVerb` / `ContentVerbPrompt`, a chat-surface keymap (o/a/n/x) and status
  hint distinct from tool approval's (a/g/d), and a symmetric `POST /verb/approval`
  HTTP endpoint alongside the existing `/tool/approval`. Live-verified against the
  real `ornith:latest` model: a hand-crafted "Let me examine..." response correctly
  triggered a full continuation round (grounded sub-goals, not redundant/hallucinated
  ones) via the real decomposer/executor: `internal/prompting/continuation` handles
  detection and list I/O; `internal/runtime/gate.go`, `continuation.go` handle the
  gate and orchestration.
- **Decomposition and tool-call resolution are now grounded in this plan's own
  accumulated findings, not just the parent goal text** — the recurring
  sibling/dependency-context gap (mellow-meadow, lively-raven, quiet-cove, confirmed
  starkly in session `amber-quartz`: 17 tool calls across two plans, zero of which
  ever read a line of source code — `ls -la` on the project root ran 3 times,
  `package.json` was searched for twice in a Go project, and `cat src/main.py`/
  `cat src/utils.py` both failed on hallucinated Python paths moments after a
  successful `tree` call in the SAME plan had already shown the real Go layout). New
  `internal/planfindings` package threads a plan's live, growing findings (via
  `capturingExec`'s already-existing, mutex-protected accumulator — it was
  accumulating in real time during the drain the whole time, just never read until
  after) through `context.Context` to both `decompose.Decomposer.Decompose` (folded
  into the planner's context text) and `tools.Proposer.Propose` (folded in as its own
  system message, new `prompting.PlanFindingsMessage`, deliberately separate from
  working-memory grounding — different context shape, different source, different
  lifetime). Context-scoped rather than a struct field because the decomposer and
  proposer are shared, session-lifetime singletons that can serve concurrent plan
  drains (`runDecomposition` is deliberately backgrounded so a turn is never
  blocked) — a shared mutable field would race or cross-contaminate between two
  plans. Live-verified against the real `ornith:latest` model (the same one
  `amber-quartz` used): given an identical vague goal ("Read the main entry point..."),
  the proposer resolved to a generic `list_dir .` without grounding and to the exact
  real path `cmd/agentx/main.go` with it; decomposing the same root goal produced one
  generic child ungrounded vs. four children all correctly targeting real
  already-discovered paths when grounded.
  **Known residual gap, found live-verifying this fix**: a Step that decomposes
  several children in ONE planner call, where a later child depends on an earlier
  sibling in the SAME batch (`deps: ["sibling-id"]`), can still have that later
  child's tool call pre-resolved by the planner (`Params["tool"]/["args"]` baked in at
  decompose time) using a GUESS rather than the sibling's real result — because a
  pre-resolved Task record skips the proposer entirely (`resolvedProposal` short-
  circuits `Execute`), the grounding this fix adds to `Proposer.Propose` never runs
  for it. The scheduler's own dep-ordering correctly delays *dispatch* until the
  sibling completes, but the *args* were already frozen before that sibling ever ran.
  Observed live: `cat main.py`/`cat package.json` guessed and failed in a Go project
  despite an explicit `deps` edge on the sibling that had already listed the real
  layout. Distinct from — and not fixed by — the two grounding paths above; would need
  the scheduler to force a fresh proposer resolution for a Task whose `Params` were
  set against an unmet dependency, once that dependency actually completes.

### Fixed

- **A finished plan permanently collapsed to just its root line, with no way to see
  the individual steps** (session `brave-fjord-2`). The liveness-propagating
  auto-collapse rule (previous entry) gated a node's children on it or a descendant
  *currently* running — but a real fast tool call (`ls`, `tree`, `grep`) dispatches
  and completes in single-digit milliseconds, far under one terminal frame, so the
  "live" window was never actually observed: by the time any redraw happened, the
  step had already finished and the whole group had already collapsed back down, with
  no manual per-node expand to bring it back. Liveness now only bounds clutter *while*
  a plan is running; once `phase: "ended"` lands, the plan's full structure always
  renders, unconditionally — matching the widget's own documented job of being "the
  record of what ran." New `TestEndedPlanShowsFullStructure` reproduces the exact
  brave-fjord-2 timing (same-epoch dispatch→completion) and asserts both the
  before-ended collapse (unchanged) and the after-ended full expansion.
- **A zero-value `spinner.TickMsg{}` forwarded from the chat surface to the output
  panel hung the full `@functional` godog suite** (never in an isolated single-scenario
  run — only under the full suite's concurrent scenario execution). `chat.go`'s
  `spinner.TickMsg` handler now only forwards to `output.Update` when `msg.ID > 0` — a
  real `spinner.Tick()` always carries a positive, globally-unique ID; a hand-built
  zero-value fixture (as the existing "spinner animates" godog scenario constructs) is
  not owned by anything and stays with chat's own spinner branch. `output.Update` also
  gained its own `msg.ID <= 0` guard, independent of the caller, since it has no other
  guarantee to lean on. New regression coverage in
  `internal/surfaces/output/plan_test.go` (`TestPerNodeSpinnerLifecycle`'s zero-ID
  case) locks in the defensive side; the pre-existing godog scenario now exercises the
  routing side on every `make all`.
- **The plan widget's `anySlice` helper silently dropped every element of a
  server-native payload.** `plan_cycle.go` builds `task_plan`/`task_node` payloads as
  literal `[]map[string]any`/`[]string`, and the bundled chat surface's event delivery
  never crosses a JSON boundary (a direct in-process channel of `state.Event`) — so a
  plain `v.([]any)` type assertion (what `anySlice` did) never matched, and
  `task_plan`'s bulk node list and `task_node`'s decomposed-children list were empty in
  the live surface for as long as ADR 0009 §9a/9c-v1 has existed. Masked because
  individual `dispatched`/`completed` deltas (which never touch `anySlice`) kept
  populating nodes one at a time regardless — a standalone HTTP/SSE surface, whose
  payloads *do* cross a JSON boundary, would have seen `[]any` and never hit this.
  `anySlice` now tolerates any concrete slice type via `reflect`, with
  `TestDecomposedChildrenSurviveNativePayload` reproducing the exact native shape.
- **Concurrent tool approvals deadlocked the session (`vivid-raven`).**
  `approvalGate` was a singleton — one shared response channel, safe only
  when the single-tool cycle called `RequestApproval` one at a time. Once
  the scheduler could dispatch concurrent plan leaves, a second concurrent
  approval request's `arm()` silently overwrote the first's channel,
  permanently orphaning it: blocked forever on a channel nothing would ever
  write to again, holding a scheduler slot hostage so the plan (and the
  whole turn) could never finish — a lost wakeup, not a timeout, so nothing
  ever recovered it. `RequestApproval` now owns a private response channel
  per request, and `approvalGate` serializes concurrent requests into a FIFO
  queue, showing exactly one at a time and advancing to the next when the
  shown one resolves or is canceled while still waiting. New
  `internal/runtime/approval_test.go` reproduces the exact broken mechanism
  deterministically (no goroutine timing) and proves it now resolves
  correctly; all 4 existing approval scenarios pass unchanged;
  `go test -race` clean.

### Added

- **The plan widget renders as a recursive nested Step/Task DAG, not a flat indented
  list** (ADR 0009 §9c redesign). Separate boxes per node, color-differentiated by
  Kind (Step blue, Task tan, running amber overriding both); a Task's resolved tool
  command always renders in reverse video inside its own box, even collapsed; a
  Step's decomposition renders as its children fully nested inside it, recursively, to
  whatever depth the plan actually reaches; each currently-running node animates its
  own independent spinner (not a shared/lockstep one); a liveness-propagating
  auto-collapse rule keeps exactly the currently-active parts of the plan expanded,
  collapsing a fully-quiet branch back to one line — even mid-plan, not only at the
  end. Drawn recursively at *view* time (not baked into the widget body at event time),
  so a terminal resize is always correct. The old flat nested tool-call/tool-result
  widget mechanism (§9c-v1) is retired: a tagged `tool_call`/`tool_result` now folds
  directly into the owning plan node's own `command`/`resultText` fields instead of
  spawning a separate widget. The full nested tree is also persisted to
  `sessions/<id>/plans/<root-id>.json` on every mutation (ADR 0009 §9b, previously
  unbuilt) — new `internal/session/plans.go`, deliberately write-only and
  resumability-friendly in shape without building any reconstruction logic (a named,
  explicitly deferred follow-on). See `docs/ux/06_OUTPUT_WIDGET.md`'s new "Plan widget"
  section for the full behavior contract.
- **Plan synthesis now sees a step's real findings, not its UI preview.** Session
  `lively-raven`: `tree` ran successfully (538 lines) on this repo, but the
  model's final answer called agentX "a Python-based AI agent framework" — the
  plan cycle was folding `Result.Preview` (the executor's 20-line UI-display
  cap) into synthesis, so `go.mod`/`cmd/`/`internal/` never made it in; what
  did was a stray `agentx.egg-info` and a separately-guessed, failed
  `src/main.py` read. The UI preview and the model's one shot at seeing a
  result are different audiences with different needs (Context Curation) —
  sharing one small cap discarded exactly the evidence that mattered.
  `capturingExec` now reads back up to 200 lines from the artifact store
  (which already persists every result's full output, independent of the tiny
  preview) for synthesis, while the collapsed tool widget is untouched. New
  `internal/runtime/plan_cycle_test.go` (the package's first unit test).
  Live-verified against a real `tree` run on this repo: the old preview
  genuinely lacked `go.mod`/`cmd/`; the widened read has both. Honest limit:
  200 lines is a bounded budget, not "everything" — `internal/` still fell
  just past it on this repo's actual output.
- **Context Curation** is now stated in `CLAUDE.md` as the project's core design
  motto: every LLM call gets deliberately curated context for *that specific
  call*, never a default one-size-fits-all assembly. Written down after tracing
  session `clever-raven-3`'s stall to exactly this — see below.
- **Indirect imperatives now classify as actions.** `agentx-classification.md`
  (+ `classify.DefaultPrompt` fallback) gained a rule: a question about whether
  something was already done ("did you try X?", "have you considered Y?") is an
  indirect request to do it now, not conversation. Previously "did you try
  `tree .`?" classified `respond_directly`, the model narrated "let me check...
  and run it now" with no tool phase on that route to act on it, and the turn
  just stopped. Live-verified: the exact text now classifies `single_tool`.
- **`Ask` folds into `Decompose` when the turn's own abstain leans actionable.**
  `reconcile.TurnSignal` gained `LeansActionable` — the same spread-shape
  discriminator `responseSignal` already used for the response classifier's
  abstain (`LeansProduced`), now applied to the *action* classifier's abstain
  too (structural: everything except `"none"` counts toward action, so it's
  robust to the existing out-of-enum label-pollution issue rather than needing
  to special-case it). An abstain that scattered across actionable labels is not
  genuinely ambiguous about *whether* it's an action, only *which* — reify a
  plan instead of silently punting.
- **The background Decompose route synthesizes a follow-up answer.**
  `runDecomposition` previously drained a plan and stopped — no synthesized
  response, unlike the foreground plan cycle. It now folds findings into a real
  answer via the same `streamResponse`/`recordTurn`/`finishCycle` machinery
  `runPlanPhase` already uses, with `setProcessing` bookending it so the surface
  shows "working" during the background investigation instead of going silent.
  Together with the two entries above, this closes the clever-raven-3 stall
  end to end: misroute fixed at the gate, and even a residual abstain-but-
  actionable turn now investigates and answers instead of dead-ending.
- **`tree` joins the tool catalog** (depth-capped at 3, common generated/
  vendored dirs excluded) — a bounded structural overview in one call instead
  of one `list_dir` per directory (each an LLM-authored guess at what to look
  at next, the repeated cause of hallucinated paths this session).
- **The tool proposer is grounded in working-memory facts.** Live-testing
  `tree` surfaced a second bug: `single_tool` resolution had *no* cwd/project
  context at all — asked to use `tree .`, it proposed `read_file` at `/app` (a
  hallucinated container-convention path). Unlike the conversational path,
  this doesn't fold in full history (a tool resolution is a narrow job, not a
  conversation — Context Curation, not "just reuse `withContext`"): `Proposer`
  gained a `Facts func() []prompting.Fact` field (mirroring
  `decompose.Decomposer.Facts`'s existing pattern), folded in as its own
  system message. One wiring point in `buildTools()` fixes two call sites at
  once — the `single_tool` route and the executor's own Redispatch/Reify
  fallback proposal — since both share the same `*Proposer` instance. New
  `internal/tools/proposal_test.go` (the package's first unit test file).
  Live-verified: the exact miss now resolves to `tree, path:.`.
- **Typed DAG nodes — Step vs. Task (ADR 0008 amendment).** The scheduler's LLM
  "atomicity oracle" (a vote on every dispatch) is retired: the planner now declares
  each node's kind — `Step` (decomposes, ≤5 children) or `Task` (a leaf, executes
  once, never decomposes) — at generation time, under Ollama JSON-schema-constrained
  decoding (`Format`, already used by the fan-group classifiers, newly wired to the
  planner). A tool-call loop is now structurally impossible rather than merely
  guarded against, and this retires `decompose.Oracle`/`ForceRoot`/`HeuristicOneStep`
  and the clause-verb lexicon from the entries above outright — one declared field
  replaces three inference-time patches. New `task.Kind`/`KindStep`/`KindTask`;
  `scheduler.Node`/`Step`/`Task` interfaces (+ adapters) for code that needs to hold
  a typed node; `planner.PlanSchema()` (the `oneOf(task,step)` wire shape — a node
  carries exactly one of `{"task":{"tool","args","explanation"}}` or
  `{"step":{"description","deliverable"}}`, discriminated by key presence, not a
  flat tag, so a task cannot structurally acquire step-shaped fields or vice versa).
  A `Task` node's tool call is pre-resolved by the planner itself and the executor
  skips the separate proposer LLM call for it (`executor.resolvedProposal`) — one
  fewer model round-trip per leaf. A decomposition violation (>5 children, invalid/
  missing kind, or a child echoing its parent — the one thing schema-constraint
  can't catch, still `SimilarGoals`) gets one retry with the problem named, then
  degrades to executing as a Task rather than recursing (reuses the existing
  `scheduler.ErrNoProgress` fallback). The planner's system prompt moved out of a
  hardcoded Go constant into `config/seed/agentx-planner.md` (mirrors
  `agentx-classification.md`'s pattern) so the Step/Task rules can be tuned without
  a rebuild. Retiring the Oracle also removed `buildDecomposition`'s only real
  dependency on the experimental prompt corpus, so `invoke_planner` now works even
  with no `prompts.toml` configured — a deliberate behavior broadening, not just a
  refactor. Live-verified end to end against Ollama and the real filesystem (real
  classifier → real schema-constrained planner → real scheduler → real executor,
  including the skip-propose path): `oneOf`/`maxItems:5`/`minLength` all honored,
  correct Step/Task mix, bounded recursion, zero spiral on the exact prompts that
  previously broke (`tidy-cove`, `nimble-otter`). That run also surfaced a real,
  pre-existing (not introduced by this change) gap worth a follow-up: a step's
  decomposition only sees session working-memory facts, not its own siblings'
  tool findings, so a later step can still guess a plausible-but-wrong path (e.g.
  assuming `package.json` in a Go project) instead of using what was already
  discovered earlier in the same plan.
- **Pre-response plan execution (`invoke_planner` goes live).** The prompt classifier
  was rewritten as a request-type gate — it classifies the *kind* of request by verb
  and scope and is forbidden from judging missing specifics ("which project") or
  punting to conversation. An `invoke_planner` verdict now runs a **plan cycle**
  before the model answers: the goal is decomposed (ADR 0008 scheduler), leaves
  execute real read tools, and the findings are folded into the prompt so the answer
  is grounded in what was actually inspected. Previously `invoke_planner` was a dead
  branch that fell through to a free-form narrated answer.
- **Streamed plan events (ADR 0009 §9a).** All tool/plan execution is user-visible
  *while it runs* — a hard requirement from the tidy-cove RCA (123 s of silent
  planning). New `scheduler.Observer` seam (`WithObserver`); the plan cycle publishes
  an initial `task_plan` snapshot before any work, live `task_node` deltas
  (dispatched / decomposed / completed) per transition, and a final snapshot with an
  executed count. A plan that executed nothing reports a loud "plan blocked" error.
  New `task_node` content type and `planning` processing phase. Batch-emit at
  completion is retired as a documented invariant.
- **Decomposition spiral guard.** Four layers, from the tidy-cove RCA (ten recursive
  re-plannings of `ls -la`): planner prompt rules (one verb+object action per step;
  result plumbing like "and capture the output" explicitly forbidden — the tool
  returns results automatically; no shell syntax; never restate the goal as a step);
  a hardened one-step heuristic (plumbing stripped before judging, noun "and" is not
  clause chaining); a non-progress guard (`scheduler.ErrNoProgress` — a child that
  echoes its parent executes instead of recursing, with stopword/verb-synonym
  tolerant similarity); and `decompose.DefaultMaxDepth = 3` (was 10).
- **Plan widget in the Output surface (ADR 0009 §9c).** Each plan renders as one live
  widget ("🗺 plan") created the moment planning starts — before anything executes —
  and mutated in place by the streamed deltas: ⏳ running steps with elapsed time,
  ⑂ decomposition with children indented under the parent, ✅/❌/⊘ completion glyphs
  with per-step durations (from server event epochs), an "N running ∥" parallel cue
  in the title, and 🚫 blocked marks plus an ⚠ error line on an incomplete plan. The
  widget persists in the transcript as the record of what ran.

- **Nested tool widgets under the plan (ADR 0009 §9c.2).** The executor gained a
  `CallObserver` seam that announces every resolved call *before* it runs and reports
  its terminal outcome (including denied attempts). Plan-leaf calls publish
  `tool_call`/`tool_result` events tagged with their step's task id; the Output
  surface nests them as indented boxes under the 🗺 plan — "🔧 <tool> · <step>" with
  the rendered command, and a collapsed "📋 result · <outcome>" that expands to the
  height-capped, scrollable body. Untagged (single_tool cycle) events render flat as
  before.
- **`internal/jsonx`** — one tolerant first-JSON-object extractor shared by the
  classifier, tool proposer, and planner, so a model reply wrapped in ```json fences
  parses everywhere (the calm-pebble planner failure); the two hand-rolled duplicate
  extractors were removed.
- **ADR 0009 — Plan & Tool Execution Visibility and Control** (+ behavior doc):
  pre-execution announcement, approval, abort, result sub-widgets, decompose/parallel
  cues, ✅/❌ status, plan JSON persistence, Context-surface representation; phases
  9a–9e with 9a built.
- **`make seed`** installs the baseline config files (`config/seed/*`) into the
  user's config dir (`$XDG_CONFIG_HOME/agentx`, else `~/.config/agentx`) — the
  packaging step previously noted as future work. Seeding now **overwrites** the
  deployed files on every install (customization retention is not yet implemented),
  so an upgrade always picks up prompt/config changes; the seed model default is
  `ornith:latest`. The zellij harness layout now lives in the seed set as
  **`config/seed/agentx.kdl`** (deployed by `make seed`, consumed by `ax`, never read
  by the agentx runtime). Its per-pane `sleep` hacks are removed now that
  `surface launch` waits out the server-start race. Vendored the zellij layout and
  options docs under `docs/reference/zellij/` (for zellij 0.44.3) — recording that
  layouts cannot embed keyboard/mouse config, so the harness *documents* the
  recommended `config.kdl` settings (`default_mode "locked"`,
  `support_kitty_keyboard_protocol true`, `mouse_mode` on with Shift+drag for native
  selection) rather than overriding the user's global config.
- **`agentx session new-name`** prints one session name in AgentX's own
  adjective-noun style and exits — a side-effect-free helper (no server, no session
  created) so a scripted launcher can pre-mint a name in the same vocabulary the app
  uses, instead of rolling its own. It prefers a name not already used by a session
  on disk, falling back to a plain generated name if the session root cannot be
  resolved. New `session.GenerateName`, `session.Store.UniqueName`, `cli.NewSessionName`,
  and `Command.GenSessionName`. The `ax` launcher now uses it in place of a random
  base64 string (which could contain `/`, `+`, `=`).

### Fixed

- **Read tools no longer phantom (the mellow-meadow first-leaf kill).** `FSVerifier`
  stat-ed the "path" arg as a file target, so every successful `list_dir` — whose
  path is a directory — failed verification, was reported Phantom, and (in a plan)
  marked Failed, stranding every dependent step. Read-class tools now verify on
  clean exit + output present (the output *is* the effect); write semantics are
  unchanged.
- **Incomplete plans report loudly.** The final `task_plan` snapshot now derives a
  "plan incomplete: N failed, M abstained, K never ran" error from node statuses;
  previously a plan with one failed leaf and five stranded nodes reported no error
  (the blocked diagnostic only fired when *nothing* executed).
- **The plan root always decomposes (`decompose.ForceRoot`).** The request classifier
  already judged an `invoke_planner` turn multi-step; re-litigating that with the
  one-step heuristic let compound goals run as a single leaf on lexicon gaps
  (nimble-otter: "review X **and suggest** Y" executed one `list_dir` and answered
  from a top-level listing). The heuristic now only arbitrates children, and its
  clause-verb lexicon was broadened (suggest, describe, explain, improve, …).
- **Thinking restored on the plan path.** The plan-handled response hard-coded the
  no-thinking variant, so a successful plan never showed a 💭 widget even with
  `thinking.routes.invoke_planner = true`; the synthesis over findings now gets the
  same route-aware thinking pass as a direct answer.

### Changed

- **Surface launch waits out the server-start race instead of relying on a
  `sleep`.** Auto-discovery (`agentx surface launch <kind>` with no `--connect`)
  now polls for a published, answering session until a bounded deadline
  (`LaunchArgs.ConnectTimeout`, default 10s) rather than failing the first look —
  so a surface launched concurrently with `agentx` (e.g. every pane of a
  multiplexer layout starting at once) attaches instead of dying. The wait covers
  both "transport not published yet" and "published but not answering"; terminal
  outcomes (unreadable session root, genuine multi-session ambiguity) still fail
  immediately. The single retry loop lives in `cli.resolveConnection`, so it
  cascades to every current and future surface. Layouts can drop their per-pane
  `sleep` hack.
- **Surface titles omit the session name by default.** A launched surface's title
  shows just its label (e.g. `context`, `working memory`) unless the operator
  passes `--session-in-title`, which restores the `<label> · <session>` form for
  standalone launches that need to tell peers apart. Under a display harness the
  surrounding pane labels already name the session, so the default stays
  uncluttered; the chat's "attach surfaces" widget continues to name the session
  regardless, to guide operators launching by hand. The toggle is gated once in
  `cli.RunSurface` (`LaunchTitleSession`) and cascades to every surface. New
  `LaunchArgs.SessionInTitle`.

- The **classification widget renders flat** — `⚙ classification · <intent →
  route>` on a single line, no box — instead of a three-row bordered widget, since
  its payload is always one line of metadata. Frees two transcript rows per turn
  and matches the output-widget spec's "single greyed line" intent. Still
  selectable.
- **A streaming widget's body follows the incoming tail.** While an `agent_delta`
  (or thinking) response streams past the `max_widget_lines` cap, its in-place
  scroll window now tracks the growing tail so the newest text stays on screen
  without a manual scroll — the reader watches the answer arrive instead of a frozen
  head. Scrolling up detaches from the tail so an earlier passage can be held in
  view; scrolling back to the bottom re-attaches. A complete, non-streamed body
  still anchors at its top. New per-widget `followTail`, honored in
  `output.renderBody` and toggled by `appendAssistant`/`appendThinking`/`ScrollSelected`.
  Closes nits.md #2 (UC-WIDGET-STREAM-FOLLOW).
- **Assistant responses render lightweight markdown as terminal styling.** Model
  bodies now show `**bold**`, `` `code` `` (reverse-video), and level 1–3 ATX headers
  (`#`/`##`/`###`, styled bold/underline by level) as ANSI SGR, with the source
  markers consumed — so LLM markdown reads richly in the TUI without a heavyweight
  renderer (no new dependency). Styling is applied before wrapping so the existing
  ANSI-aware wrap/pad/scrollbar math stays exact, and is scoped to the assistant
  kind (via a per-widget `markdown` flag) so user prompts and tool output keep their
  text literal. The SGR constants are the seed of a future emphasis/header theme.
  New `output.styleMarkdown`. Tier 1 of nits.md #6 (UC-WIDGET-MARKDOWN); lists,
  block quotes, and tables are follow-on tiers.
- **Assistant markdown now renders lists and blockquotes (Tier 2).** Unordered list
  markers (`-`/`*`/`+`) fold to a single bold bullet glyph (`•`), ordered markers
  (`1.`) keep their number with the marker emboldened, and blockquotes (`> `) render
  dim behind a gutter glyph (`▎`) — source markers consumed, leading indentation
  preserved, item/quote text still getting inline emphasis. Line-level styling extends
  `output.styleLine` (new `output.orderedMarker`) so the ANSI-aware wrap math stays
  exact; wrapped blockquote continuation lines fall back to plain text. Tier 2 of
  nits.md #6 (UC-WIDGET-MARKDOWN); tables remain the Tier 3 follow-on.
- **Optional glamour rendering upgrades finalized assistant answers (Tier 3).** With
  `[agentx.output] markdown_renderer = "glamour"`, a completed `agent_response` is
  re-rendered by charmbracelet **glamour** (goldmark + chroma) — GFM tables draw
  aligned box-drawing columns and fenced code blocks are syntax-highlighted, the
  "HUGE bonus" a per-line scanner cannot do. The streaming path is untouched: deltas
  still render live with the lightweight scanner, and glamour swaps in only on
  finalize, so partial markdown never flickers. Glamour is now the **default**
  renderer (`markdown_renderer = "glamour"`); an explicit `"scanner"` opts out. The
  scanner remains the streaming renderer and the always-available fallback whenever a
  glamour render is unavailable or stale. Glamour is rendered to `innerW - 1`, reserving the per-widget
  vertical-scrollbar gutter unconditionally, which guarantees a table that grows tall
  enough to scroll still fits horizontally — the output panel never forces a
  horizontal scroll. Renders are cached per width and invalidated on resize. New
  dependencies vendored: `charm.land/glamour/v2`, goldmark, chroma, bluemonday
  (lipgloss bumped v2.0.3 → v2.0.4). New config `output.MarkdownRenderer`,
  `output.SetMarkdownRenderer`. Design of record: ADR 0007. Tier 3 of nits.md #6.
- **`native` markdown renderer draws bordered, zebra-striped tables (ADR 0007 spike).**
  A third `markdown_renderer` mode, `"native"`, styles prose with the per-line scanner
  and renders GFM table blocks directly with `lipgloss/table` — with inter-row rules
  and alternating row backgrounds glamour cannot express (its table renderer discards
  the row index and never enables `BorderRow`). A row's wrapped continuation lines
  share its stripe, so long cells stay legible. Bounded to `innerW - 1` like glamour
  (no horizontal scroll); parses column alignment from the delimiter row. Trade vs.
  glamour: `native` loses chroma code-block highlighting. Glamour remains the default;
  `native` is the lower-dependency fallback and the demonstrated "drop glamour" path.
  New `output.renderNative` / `output.renderMarkdownTable`.
- **`native` renderer now syntax-highlights fenced code (chroma), reaching parity with
  glamour.** Fenced code blocks in `native` mode are tokenized and formatted to
  256-color ANSI via `chroma` (already vendored through glamour — no new dependency),
  with tabs expanded and each line SGR-reset so color never bleeds into the widget
  border, hard-wrapped to `innerW - 1` (no horizontal scroll). `native` now has both
  code highlighting *and* the table clarity (row rules + zebra) glamour cannot express;
  per ADR 0007 the standing recommendation is to make `native` the default and retire
  glamour (keeping `chroma` + `lipgloss/table`). New `output.renderCodeBlock` /
  `output.chromaHighlight`; `chroma` promoted to a direct dependency.
- **`native` is now the default markdown renderer; glamour retired.** Executing the ADR
  0007 recommendation: `markdown_renderer` now defaults to `"native"` and accepts only
  `"native"` or `"scanner"` (a retired `"glamour"` value resolves to native). The
  `"glamour"` mode, `output.glamourBody`/`renderGlamour`, and the
  `charm.land/glamour/v2` + goldmark + goldmark-emoji + bluemonday + douceur +
  gorilla/css dependencies were removed from `go.mod` and the vendor tree — `chroma`
  and `lipgloss/table` are the only rendering deps now. Net: clearer tables (row rules
  + zebra) and highlighted code on a smaller dependency footprint.
- **Agent responses are now streamed and stored as two distinct kinds.** The live
  answer streams as transient `agent_delta` chunks (in-process bus, chat window's
  typing effect only — never persisted, never sent to external surfaces); when it
  finishes, the complete answer is published once as a durable `agent_response`.
  The recorder and every external surface (context viewer, context-visualizer) now
  deal in one complete event per conversation element rather than a fragmented
  delta stream, giving each element a single durable identity (its ordinal) for
  enable/disable. New `state.ContentAgentDelta`; `state.Bus.Publish` returns the
  stamped ordinal. Documented in docs/implementation/03 (Streaming vs. durable).
- The chat input now has a **terminal-agnostic soft-newline**: `Alt+Enter` (and
  `Ctrl+J`) insert a newline on any terminal, alongside `Shift+Enter` which only
  works where the terminal disambiguates modified keys (Kitty protocol /
  modifyOtherKeys — already requested by default). The empty input shows a dim
  placeholder hint (`alt+enter for newline`) that clears once you type; when the
  terminal reports key-disambiguation support (`tea.KeyboardEnhancementsMsg`) the
  chat upgrades the hint to `shift+enter for newline`. Fixes Shift+Enter
  submitting instead of inserting a newline on terminals (e.g. VTE) that collapse
  it to Enter. New `input.Model.SetNewlineKey`.
- The chat input panel now **word-wraps** long text to the panel width instead of
  running off the terminal edge, and **grows vertically** with its content up to a
  configurable cap (`input_max_lines`, default 8); beyond the cap it windows the text
  around the cursor with a right-gutter scrollbar. The chat layout is now dynamic —
  the output panel takes whatever height the input does not. An empty input is a
  single row. New `input.Model.DesiredHeight`/`SetMaxHeight`; `[agentx.output]
  input_max_lines`.
- The context viewer now omits the startup **bootstrap exchange**, which engages the
  session but is not part of the user's conversation. Events from the bootstrap cycle
  are marked `ephemeral` on the envelope (`state.Event.Ephemeral`); the chat surface
  still shows them, the read-only context viewer skips them (on both seed and live).
- Output widgets now render the kind label (emoji + type) **in the top border**
  (`┌─ 🤖 AgentX ───┐`) instead of on the first inner row, so every visible row is
  content. Collapse behaviour is now kind-aware: narrative boxes (user, assistant,
  tool call) collapse to the titled border plus a one-line content preview (with `…`
  when there is more), while noise boxes (thinking, tool result) collapse to the
  titled border only. Both the chat and context surfaces inherit this. Documented in
  `docs/ux/06_OUTPUT_WIDGET.md` (Anatomy, collapse behaviour).

### Added

- `agentx --session <name>` (or `-s`) names the booted session instead of the
  generated adjective-noun, so scripted multiplexer layouts (zellij/tmux) get
  predictable session names to launch peer surfaces against. A collision is
  disambiguated with a numeric suffix (`<name>-2`, …). Absent the flag, a name is
  generated as before. New `cli.Command.SessionName`, `app.Options.SessionName`,
  `runtime.Settings.SessionName`.
- The **context surface** is now the context-management surface: selecting a
  user-prompt or agent-response element and pressing **space** toggles whether it
  participates in the agent's upcoming context. The orchestrator applies the toggle
  in memory (effective next prompt) and persists it in the element's event file, so
  a re-attaching surface seeds the correct state. Each toggleable element shows an
  **enabled checkbox** left of its emoji (`[x]` in context / `[ ]` disabled),
  independent of the selection border; every element renders **collapsed by
  default** (a navigable summary — expand with Enter). Non-conversation elements
  (thinking/tool) are not toggleable. New `POST /events/{ordinal}/enabled`, `Orchestrator.SetEventEnabled`,
  `Recorder.SetEnabled`. Re-authors PD-CTX; see docs/implementation/03 (Enabled
  Semantics). `SurfaceModel.Key` now returns a `tea.Cmd`.
- Added the **context-visualizer** surface (SS-7): a read-only budget meter that
  polls the orchestrator's assembled context composition and renders one bar per
  content class (working memory 🧠, instructions 📜, user 👤, attachments 📎,
  thinking 💭, assistant 🤖, tools 🔧) plus a remaining-capacity band, measured
  against the model's context window. Launch with `agentx surface launch
  context-visualizer`. It performs no writes — the enable/disable management
  affordance lives on the context pane; the meter only hints at what to prune.
  Token figures are a `chars ÷ 4` estimate. New `GET /context` endpoint,
  `session.ContextReport`/`ContextComponent`, `Orchestrator.ContextBreakdown`,
  `internal/surfaces/contextviz`. Re-authors legacy PD-10 (ContextMeterWidget).
- The Ollama adapter now reads a model's **maximum context window** from
  `POST /api/show` (`ollama.Client.ContextLength`, cached per model) and sets
  `options.num_ctx` on every chat to that window, so the model uses its full
  context instead of Ollama's small server default — and the visualizer measures
  the budget against the window actually enforced. A lookup failure falls back to
  the default window (num_ctx unset).
- Added flagless surface launch with on-disk token discovery (SS-5), making
  attach-over-SSH first-class without any clipboard. The orchestrator now publishes the
  raw attach token to a `0600` `attach-token` file beside `transport.json` (removed on
  shutdown); since loopback-only peers run on the same machine, `agentx surface launch
  <kind>` with no `--connect/--token/--session` auto-resolves the endpoint and token
  from the newest reachable session on disk (explicit flags still override). The
  launch-info widget now advertises the short `agentx surface launch <kind>` command,
  which is short enough to type or cleanly select over SSH in any terminal — fixing the
  wrapped/bordered-command scrape problem; digit-copy via OSC 52 remains as
  terminal-dependent convenience. New `session.Store` token/discovery methods
  (`WriteAttachToken`/`ReadAttachToken`/`RemoveAttachToken`/`DiscoverTransports`) and
  `transporthttp.ShortLaunchCommand`. Documented in
  `docs/implementation/02_surface_orchestration_http.md` (Flagless launch & token
  discovery) and `docs/ux/06_OUTPUT_WIDGET.md`.
- Added live peer-connection status to the launch-info widget (SS-4): each surface row
  now shows 🟢 when at least one surface of that kind is attached and 🔴 otherwise.
  "Connected" is defined by an active event stream, not a registration — `GET
  /events?surface_id=<id>` marks the surface live on stream open and dead on close
  (via `defer`, so a crash/kill is caught when the connection drops). The registry
  tracks a per-surface live-stream count and exposes `ConnectedKinds()`; the chat
  surface polls it on a ~1s tick (`Bridge.Connected`) and re-renders the row emojis.
  Documented in `docs/implementation/02_surface_orchestration_http.md` (Connection
  liveness) and `docs/ux/06_OUTPUT_WIDGET.md`.
- Added an in-session launch-info widget so the surface-attach commands survive the
  alternate screen: the chat boot path now installs a collapsed, scrollable
  launch-info widget as the first output widget (after the banner, before the
  bootstrap response) instead of printing a startup hint that the alt-screen wiped.
  Expanding it lists the launchable surfaces by name; with the widget selected,
  pressing a digit `1..N` copies that surface's full `agentx surface launch <kind>`
  command to the clipboard via OSC 52 (`tea.SetClipboard`) and confirms by name. The
  attach command (and token) is never rendered — it only ever reaches the clipboard,
  so the token stays off-screen and no mouse capture is needed (native text selection
  is preserved). The widget is surface-local — never a session event — so it is not
  persisted and never appears on attached peer surfaces; it is omitted when the
  transport is disabled. New `output.Model.SetLaunchInfo`/`CopyCommand`; documented in
  `docs/ux/06_OUTPUT_WIDGET.md` (Launch-info widget). Replaces the dead `LaunchHint`
  stdout print.
- Added the context viewer surface (SS-3, completing M2's first peer surface): a new
  read-only `internal/surfaces/context` surface launched with
  `agentx surface launch context`. It projects the session event stream into the
  shared collapsible output renderer (reused from the chat output), intercepts
  processing-state events for a one-line status indicator, and exposes scroll/select
  navigation only (no prompt input). Registered in the launch dispatch
  (`surfaceModelFor`). Specced as `docs/ux/03_PANEL_DETAILS.md` PD-CTX (superseding
  the legacy GUI PD-03/PD-08 context affordances for the TUI). An end-to-end test
  attaches a context surface to a live orchestrator, seeds the prior exchange from
  the durable log, and renders live events streamed thereafter.
- Added the shared surface-client framework (SS-2, M2): a new
  `internal/surfaces/client` package with a `SurfaceModel` contract
  (`Apply`/`SetSize`/`Key`/`View`) and a Bubble Tea `Host` that drives the attach
  lifecycle — apply the durable seed before any live event, listen on the
  cursor-resumed stream, resize, and quit (invoking `POST /surface/{id}/shutdown`)
  on a quit key or a closed stream; non-quit keys forward to the surface.
  `client.Run` wires seed→subscribe→program, and `cli.RunSurface` makes
  `agentx surface launch <kind>` dispatch by kind to a TUI (or attach headless when
  a kind has no surface yet), kept in `internal/cli` to honor the import matrix.
  `transport/http.Client` gained `Shutdown`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the disk-seeded, cursor-resumed event stream for attaching surfaces (SS-1,
  first slice of M2): the event bus now stamps a per-session monotonic `ordinal` on
  every event at publish time (carried on the envelope so the live event and its
  persisted copy share one identity — a versioned addition to
  `event-envelope.schema.json`). A new `GET /sessions/current/events` returns the
  durable log as the seed (incl. each event's `enabled`), and `GET /events?after=<ordinal>`
  resumes the live stream after a cursor: the handler captures a boundary ordinal at
  subscribe time, serves `(after, boundary]` from the durable log and `(boundary, ∞)`
  live, so the seed→live handover has no gap and no duplicate with no client-side
  de-dup. `transport/http.Client` gained `Seed` and `Subscribe(after)`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Wired the transport into the live runtime, completing M1 external-surface
  enablement (TRN-6): when `[agentx.transport] enabled` (default), the orchestrator
  allocates a loopback port and serves the HTTP/SSE server as part of `Start` (after
  the bus/processing/registry/recorder are live, before accepting prompts) and stops
  it on `Shutdown` (server first, then attached surfaces marked `stopped`, then the
  recorder drains); `enabled = false` keeps the pure in-process mode. The chat boot
  path prints the attach hint (endpoint + raw token + launchable kinds) for use in
  other terminals. The SSE handler now also returns on server shutdown so a
  long-lived stream cannot block the graceful drain. `*runtime.Orchestrator` carries
  a compile-time assertion that it satisfies the transport `Provider`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the `agentx surface launch` CLI (TRN-5): the canonical
  `agentx surface launch <kind> --session <s> --connect <ep> --token <t>` plus the
  `-l/--launch -s/--session -p/--port` compatibility alias (port mapped to a
  loopback endpoint; alias also accepts `-t/--token` since v1 requires one). It
  validates in order — known surface kind, non-empty session selector, local-safe
  (loopback) endpoint, endpoint reachability, session-selector match, then token —
  mapping each failure to a deterministic category (validation | auth | transport |
  conflict) and a non-zero exit. A new `transport/http.Client` performs the attach
  (`GET /sessions/current` + `POST /surface/register`), and `surfaces.KnownKind`
  gates the launchable surface set. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added transport port allocation and endpoint publication (TRN-4): a new
  `[agentx.transport]` config table (`enabled`, `host`, `port_start`, `port_end`,
  default loopback `127.0.0.1:8420-8460`); `transport/http.Allocate` binds the
  lowest free TCP port in the range ascending (the bind is the availability check,
  so concurrent agentx instances fall through and there is no TOCTOU gap) and
  returns the bound listener, failing with a range-exhausted error when the range
  is occupied; the resolved endpoint is published to session metadata as
  `sessions/<id>/transport.json` (`session.Store.WriteTransport`/`ReadTransport`),
  never carrying the raw attach token. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the HTTP transport write endpoints (TRN-3): `POST /surface/register`
  (authorized by the attach token in the body; reason category maps to status —
  auth→401, validation→400, conflict→409), `POST /prompt` (bearer-authorized,
  gated by the orchestrator accepting state, runs the cycle async with events
  flowing back over SSE), `POST /tool/approval` (forwards the decision to the
  approval gate), `POST /surface/{id}/shutdown`, plus `POST /surface/{id}/command`
  and `POST /model/switch` reserved as `501 not_implemented` in v1. Non-register
  writes require an `Authorization: Bearer <attach-token>` header validated against
  the session token (`Registry.ValidateToken`). The `Provider` seam widened with
  `Submit`/`Resolve`/`Accepting`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the loopback HTTP/SSE transport read + streaming endpoints (TRN-2): a new
  `internal/transport/http` server adapts the orchestrator's canonical state behind a
  local `Provider` interface (so transport never imports runtime). `GET /health`,
  `GET /processing-state`, `GET /surfaces`, and `GET /sessions/current` return JSON
  snapshots; `GET /events` streams the event bus as Server-Sent Events, one
  independent bus subscription per connection so a slow surface never blocks others.
  Mechanics + GIVEN/WHEN/THEN contracts in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the surface registry and ephemeral attach token (TRN-1, first slice of the
  M1 external-surface enablement backlog): the orchestrator mints one per-session
  attach token at startup (raw value held in memory only) and owns an
  `internal/surfaces` registry that validates the token at registration, records the
  frozen `surface-registration` payload, manages surface lifecycle (ready/stopped),
  rejects bad tokens (`auth`) and id conflicts (`conflict`), and exposes only the
  non-secret token fingerprint. Registration mechanics documented in
  `docs/implementation/02_surface_orchestration_http.md`; backlog in
  `docs/build-plan/05_transport_backlog.md`.
- Added the `classification` event content type to the frozen event-envelope
  contract (`docs/architecture/runtime_contracts/event-envelope.schema.json` and
  `internal/state`) for the Stage-1 prompt classification cycle (CHT-D4).
- Added panel focus model, ESC chord keymap, and themed focus borders to the chat
  surface, with new `[agentx.theme]` config (CHT-D5).
- Added thinking pass-through: the respond phase streams model reasoning as
  `thinking` events into a collapsed `💭` widget, gated by new `[agentx.thinking]`
  config (default on) (CHT-D6).
- Added thinking sweet-spot tuning: route-aware depth (`[agentx.thinking.routes]`),
  a tunable `agentx-thinking.md` guidance prompt, and a wall-clock
  `time_budget_seconds` (default 180) that falls back to a direct answer on expiry
  (CHT-D7).

- Added `internal/tools` command policy and curated descriptors (TOOL-1): argument
  schema validation plus blacklist → global → session → approval evaluation with
  reason codes, and the built-in curated toolset registry.
- Added the tool executor and session artifact store (TOOL-2): argv/no-shell
  execution with stdin, timeout, stdout/stderr/exit capture and an output cap;
  `read_file`/`write_file`/`read_output` built-ins; full output persisted to
  `sessions/<id>/artifacts/` with a compact preview + ref and line-windowed read-back.
- Added the tool approval round-trip (TOOL-3): new `awaiting_input` processing
  state, an orchestrator approval gate (`RequestApproval`/`Resolve`) that pauses the
  cycle and persists the approved scope, and a chat affordance mapping a/g/d to
  approve-session / approve-global / deny.
- Wired the end-to-end `single_tool` cycle (TOOL-4): a strict-JSON tool proposer
  (`tools.Proposer` + `DefaultCatalog`), `classify → tool → respond` integration
  with policy/approval and read-only gating, `tool_call`/`tool_result` events, and a
  respond turn that carries the result preview + ref (not the full artifact). New
  `[agentx.tools]` config and `agentx-shell-commands.md` catalog loading.
- Persisted the command policy across sessions (TOOL-5): the blacklist loads from
  `agentx-tool-blacklist.toml` and global approvals are written to / reloaded from
  `agentx-tool-approvals.toml` under `~/.config/agentx/`; executor output cap now
  honors `[agentx.tools] output_max_bytes`. New blacklist seed template.
- Added a bootstrap logo banner to the chat output surface: the application logo
  (`logo/agentx.logo`, ANSI-colored text) is embedded into the binary and rendered
  as the first element of the output transcript, pinned above all widgets, as a
  "running" signal while the bootstrap prompt is processed. Each banner line is
  clipped to the panel width so the art survives narrow terminals. The Makefile
  re-syncs the embedded copy (`cmd/agentx/assets/agentx.logo`) from the authored
  source whenever it changes. Documented in `docs/ux/06_OUTPUT_WIDGET.md` and
  `docs/implementation/09_makefile_and_quality_gate_contract.md`.
- Added a text cursor and readline-style line editing to the chat input panel:
  typing, Backspace, and Shift+Enter now act at the cursor, with Left/Right
  (char), Ctrl-A/Ctrl-E (buffer start/end), and Alt-B/Alt-F (word back/forward)
  movement; word motion is also bound to Ctrl-←/Ctrl-→ as a multiplexer-safe
  alias, since zellij intercepts Alt-F for its floating-pane toggle. History
  seeding leaves the cursor at the end of the seeded text. The
  cursor renders as a reverse-video cell while the panel is focused. Documented as
  `docs/ux/03_PANEL_DETAILS.md` PD-02-AF-017…024.
- Added readline-style prompt history seeding to the chat input panel: `↑`/`↓`
  seed the editable buffer with prior prompts submitted during the current run
  (the in-progress draft is stashed and restored at the present line), hitting a
  boundary flashes the input border instead of moving, and the idle Esc,Esc chord
  clears an active seed back to an empty prompt. Seeding copies a prompt for reuse —
  submitting (as-is or edited) always creates a new prompt. History is in-memory and
  current-run only; persisting across session reload is a follow-up. Documented as
  `docs/ux/03_PANEL_DETAILS.md` PD-02-AF-013…016.
- Added conversation context continuity (CTX-1): each turn is now assembled with
  the prior enabled turns folded in (instructions → working memory → enabled
  history → current user prompt), giving the model multi-turn continuity instead of
  the previous single-turn context. User prompts and agent responses are enabled by
  default; thinking and tool events are retained but disabled by default; the
  bootstrap prompt and its response are excluded from context after processing. Adds an
  `enabled` field to the frozen event-envelope contract
  (`docs/architecture/runtime_contracts/event-envelope.schema.json`) with
  per-content-type defaults (`state.DefaultEnabled`) — a versioned schema change.
- Added session working memory (WM-1): a per-session `working_memory.json` of
  user-controlled facts (`internal/session` `WorkingMemory`/`Fact`), bootstrap
  seeding of stable environment facts (`userid`, `cwd`, `project`, `home`, `os`,
  `arch`, and `repo_root` when in a git work tree) as user-owned facts when absent,
  and re-read-each-turn injection of enabled facts as a system message after the
  instruction layer. The TUI management surface is deferred.
- Captured the tool-runtime design (first built-in command-line tool) ahead of
  implementation: `docs/build-plan/04_tool_runtime_backlog.md` (TOOL-1…5 for M3b),
  the `single_tool` cycle + output-artifact/context-shaping notes in
  `docs/implementation/04`/`05`, `[agentx.tools]` config, and the
  `config/seed/agentx-shell-commands.md` tool catalog seed.

### Changed

- Made output-widget collapse uniform: user, thinking, tool, and assistant widgets
  are all collapsible (Enter/^o toggles the selection) with bodies bounded by
  `max_widget_lines`; the assistant answer and user prompts gained label headers.
- Added a synthesized remediation brief artifact capturing the documentation triad review outcomes under `.subutai/runs/2026-06-23-doc-review-triad/`.
- Added ADR-0006 and indexed it in the ADR navigation so architecture decisions referenced by implementation docs are discoverable.

### Changed

- Documented branch triad independent reviews (Go, architect, and SDET) and aligned downstream documentation updates with the consolidated findings.
- Corrected documentation link/path references to match current repository structure and avoid stale or non-resolvable paths.
- Aligned reason-code and determinism contract documentation language across planning, execution, and validation references.
- Clarified event-broker gate criteria documentation so promotion/acceptance checks are explicit and testable.
- Clarified consistency-audit historical disposition language to distinguish prior findings from currently active remediation items.
