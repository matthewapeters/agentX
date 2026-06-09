# Nested tmux Navigation Architecture and Delivery Plan

## 1. Objective

Design and implement a seamless, keyboard-first navigation experience across many applet/application screens while preserving the current outer AgentX tmux contract.

Primary goals:

- Keep the existing three-pane user layout as the stable outer shell.
- Support nested tmux sessions inside host panes for richer screen navigation.
- Make navigation programmatic from Input Control (no manual prefix choreography required).
- Drive screen assignment and topology from tmuxp-compatible configuration.
- Support menu-driven navigation definitions from configuration.
- Enable parallel delivery streams and complete navigation test coverage.

## 2. Current Contract Constraints

The implementation must preserve these runtime invariants:

- Outer session/window/pane contract in [docs/ux/06_TUI_MIRROR.md](../ux/06_TUI_MIRROR.md).
- Required pane titles validated in [cmd/agentx-core/pane_titles.go](../../cmd/agentx-core/pane_titles.go).
- Existing layout overlay and reconciliation behavior in [cmd/agentx-core/core.go](../../cmd/agentx-core/core.go) and [cmd/agentx-core/core_layout_overlay_test.go](../../cmd/agentx-core/core_layout_overlay_test.go).
- Existing default tmuxp template behavior in [cmd/agentx-core/layout_template.go](../../cmd/agentx-core/layout_template.go).

Required outer compatibility guarantees:

- Primary window remains `tui-chat`.
- Logs window remains `logs`.
- Primary pane titles remain `output`, `system`, `input`.
- Startup keeps interactive focus behavior compatible with current attached flows.

## 3. Architecture Decision

## 3.1 Decision

Adopt a meta-configuration source of truth that is transpiled into:

- tmuxp layout YAML (topology projection)
- Navigation manifest (runtime routing and focus semantics)

Why not tmuxp-only:

- tmuxp models topology well, but not semantic routing, focus state machines, fallback policy, or nested recovery behavior.
- Runtime still needs deterministic post-load reconciliation and health-aware control.

## 3.2 High-Level Components

1. Layout Compiler

- Parses and validates meta-config.
- Enforces invariant contracts (outer windows/titles cannot drift).

1. Layout Transpiler

- Emits runtime tmuxp files for outer and nested layers.
- Emits navigation manifest for runtime focus/routing.

1. Nested Session Manager

- Owns nested tmux bootstrap, readiness checks, attach/detach rebind, cleanup.

1. Focus Coordinator

- Resolves and executes focus transitions across outer and inner layers.

1. Navigation Router

- Maps logical targets from Input Control to concrete outer/inner tmux actions.

1. Input Control Adapter

- Captures Tab/arrow/Enter/Esc navigation intents.
- Calls core navigation APIs.

## 3.3 Focus Model

Focus state is hierarchical:

- Outer focus: `output|system|input|logs`
- Inner focus (optional): window and pane inside host nested session

Canonical logical target format:

- `<host>/<screen>`
- Examples: `system/context-history`, `system/context-visualizer`, `output/tools-sequence`

Canonical ID and alias policy:

- Runtime canonical IDs must align with existing system tab vocabulary where already defined.
- Aliases are accepted at config boundary and normalized by compiler.

Required canonical mappings:

| Alias input | Canonical ID |
|---|---|
| `all` | `full` |
| `context` | `context` |
| `files` | `files` |
| `working_memory` | `working-memory` |
| `history` | `context-history` |
| `visualizer` | `context-visualizer` |
| `context-visualization` | `context-visualizer` |
| `system-configuration` | `configuration` |
| `ctx-history` | `context-history` |

Compiler behavior:

- Reject unknown IDs.
- Reject duplicate IDs after alias normalization.
- Emit normalized canonical IDs only in navigation manifest.

Persisted state migration behavior:

1. On startup, previously persisted tab/screen values are normalized through the same alias map.
2. If normalized value exists in active config, restore it.
3. If normalized value is valid runtime canonical but not in active config, route to host default screen.
4. If value is unknown after normalization, route to `system/full` and emit migration warning event.

## 4. Configuration Strategy

## 4.1 Meta-config Shape (Proposed)

```yaml
version: 1
outer:
  required_windows:
    primary: tui-chat
    logs: logs
  required_panes:
    - output
    - system
    - input

hosts:
  system:
    nested: true
    windows:
      - id: context-visualizer
        title: Context Visualization
      - id: context-history
        title: Context History
      - id: configuration
        title: System Configuration
      - id: command-session
        title: Command Session
  output:
    nested: true
    windows:
      - id: output
        title: Output
      - id: file-edit
        title: File Edit
      - id: tools-sequence
        title: Tools Sequence

menu:
  mode: input-control
  bindings:
    enter_mode: Tab
    next_host: Right
    prev_host: Left
    next_screen: Down
    prev_screen: Up
    activate: Enter
    cancel: Esc

fallback:
  inner_unavailable: outer_only
  on_error_focus: input
```

## 4.2 Generated Outputs

Compiler/transpiler outputs:

- `.agentx/layouts/runtime/<session>.outer.yaml`
- `.agentx/layouts/runtime/<session>.inner.system.yaml`
- `.agentx/layouts/runtime/<session>.inner.output.yaml`
- `.agentx/layouts/runtime/<session>.navigation.json`

Design note:

- User-provided tmuxp remains supported.
- Runtime manifest remains authoritative for semantic navigation.

Compatibility and precedence with existing runtime:

1. Effective outer layout source precedence remains:
1. explicit `--layout` (or compatibility alias)
1. implicit default `.agentx/layouts/default-layout.yaml`
1. Meta-config compilation produces runtime artifacts, but startup still consumes one effective outer layout path.
1. Existing overlay failure semantics remain unchanged:
1. explicit custom layout load failure is non-fatal and falls back
1. implicit/default layout load failure is fatal
1. In `visible-windows` startup mode, overlay remains disabled unless explicitly revised by a future ADR.

## 5. POSIX and tmux Control Model

Programmatic control rules:

- Core executes tmux actions through explicit outer/inner wrappers.
- Never rely on user prefix sequences for navigation.
- Resolve outer pane targets by pane title, not fixed pane index.

Operational command contract (normative):

1. Every tmux command must target an explicit server.
2. Every nested action must clear inherited tmux context (`TMUX=`) before command execution.
3. Wrapper types:
4. outer wrapper: targets outer server only
5. inner wrapper: targets per-host nested server only
6. Inner server targeting policy must use exactly one strategy per implementation:
7. named socket (`-L`) or
8. path socket (`-S`)
9. Mixed `-L` and `-S` usage for the same session is prohibited.

Implementation default selection:

- Nested runtime uses path sockets (`-S`) rooted under agent-owned runtime directory.
- Outer and inner wrappers must use `-S` consistently.

Required activation sequence (idempotent):

1. Resolve outer host pane by title.
2. Select outer host window and pane.
3. Ensure inner session exists and target window exists.
4. Select inner target window and optional pane.
5. Restore desired outer focus target.
6. Publish status event.

Timeout and retry contract:

- Step 1-2: retry up to 2s total, 100ms interval.
- Step 3-4: retry up to 3s total, 100ms interval.
- Step 5: single retry allowed.
- If step 3 or 4 fails, fallback to outer-only focus and emit degraded event.
- If step 5 fails, force recovery target to input pane and emit warning event.

Per-command execution timeout:

- Each tmux invocation must run with a hard timeout of 1s.
- Timeout class `command_timeout` is retriable only within the per-step retry budget.
- On timeout budget exhaustion, emit terminal event and apply configured degraded fallback.

Canonical action flow for target activation:

1. Select outer window and host pane.
2. Ensure inner nested session is healthy for host.
3. Select inner target window/pane.
4. Re-assert desired outer focus.
5. Emit status to Input Control.

Lifecycle safeguards:

- Deterministic socket naming per session and host role.
- Stale socket/session detection and safe cleanup under agent-owned path prefix only.
- Ordered shutdown: nested sessions first, outer session last.
- Attach/detach recovery with pane-title rediscovery.

Lifecycle ownership and cleanup rules:

1. Agent-owned socket root is the only location eligible for stale cleanup.
2. Cleanup must verify naming prefix before delete.
3. Cleanup must verify artifact type and same-UID ownership before delete.
4. Cleanup must prove staleness via server probe failure before delete.
5. Startup sweep order:
6. probe stale inner sockets
7. remove only verified stale artifacts
8. initialize outer layout
9. initialize nested sessions
10. Shutdown order:
11. stop nested sessions
12. stop nested clients
13. stop outer session
14. Crash recovery must re-discover pane IDs by title before accepting navigation commands.

## 6. Input Control UX Contract (Keyboard-First)

Input Control introduces explicit Nav Mode:

1. `Tab` enters Nav Mode.
2. `Left/Right` cycles hosts.
3. `Up/Down` cycles screens within selected host.
4. `Enter` activates selected target.
5. `Esc` exits Nav Mode back to normal input editing.

Behavioral constraints:

- If input buffer is non-empty, editing semantics take precedence unless Nav Mode is explicitly active.
- On failed inner focus, degrade gracefully to outer host focus and return actionable status.
- Emergency recovery action always available to focus Input.

## 7. Parallel Delivery Plan

## 7.1 Milestones

### M0: Contract Freeze (2-3 days)

- Finalize meta-config schema and invariants.
- Finalize focus and navigation vocabulary.
- Freeze complete navigation test matrix.

### M1: Parallel Core Build (1 sprint)

- Stream A: schema, compiler, transpiler.
- Stream B: runtime navigation engine and focus coordinator.
- Stream C: feature flags, telemetry, kill switch.
- Stream D: unit and integration test harness.
- Stream E: operator docs and troubleshooting runbook.

### M2: Integration and Hardening (0.5-1 sprint)

- Merge streams.
- Run full matrix including degraded and recovery paths.
- Validate no regressions on existing layout contract tests.

### M3: Flagged Rollout (1 sprint)

- CI dark launch.
- Dev opt-in.
- Canary enablement.
- Default-on after soak and rollback rehearsal.

## 7.2 Workstream Ownership

- Architecture and contract: application-architect
- POSIX/tmux runtime mechanics: linux-automation-expert
- Delivery sequencing and release governance: tpm-expert
- Test strategy and quality gates: sdet
- Runtime implementation: core engineering team

## 8. Testing and Quality Gates

Navigation coverage must include all command and key-driven transitions.

## 8.0 Mandatory Scenario Matrix

| ID | Scenario | Layer | Expected result |
|---|---|---|---|
| NAV-001 | Enter Nav Mode with empty input | Unit + Integration | Nav Mode active, no text mutation |
| NAV-002 | Tab with non-empty input | Unit | Edit behavior preserved unless explicit Nav Mode toggle |
| NAV-003 | Host cycle right/left | Unit + Integration | Deterministic host selection, wrap-around behavior |
| NAV-004 | Screen cycle up/down | Unit + Integration | Deterministic screen selection within active host |
| NAV-005 | Activate target happy path | Integration + Headless | Outer focus and inner raise succeed, status success emitted |
| NAV-006 | Inner unavailable | Integration + Headless | Outer-only fallback, degraded status emitted, no crash |
| NAV-007 | Restore outer focus failure | Integration | Forced input focus recovery, warning emitted |
| NAV-008 | Detach and reattach | Integration + Headless | Target re-resolution by title, navigation resumes |
| NAV-009 | Stale socket artifact | Integration | Verified stale cleanup only under agent-owned root |
| NAV-010 | Overlay coexistence | Integration + Headless | Navigation works with overlay and preserved outer invariants |
| NAV-011 | Visible-windows mode behavior | Integration | Explicitly unsupported or supported behavior enforced per contract |
| NAV-012 | Emergency recovery command | Unit + Integration | Immediate input focus regardless of prior state |
| NAV-013 | Esc exits Nav Mode | Unit + Integration | Nav Mode exits, edit semantics restored, no activation side effect |
| NAV-014 | Persisted tab alias restore | Unit + Integration | Legacy alias normalizes and restores canonical target |
| NAV-015 | Persisted tab missing in active config | Unit + Integration | Host default target selected, migration status emitted |
| NAV-016 | Persisted tab unknown | Unit + Integration | Fallback to `system/full`, warning status emitted |

## 8.1 Unit Coverage

1. Meta-config parsing and validation.
2. Transpiler determinism and idempotence.
3. Navigation target resolution and alias mapping.
4. Focus transition reducer (outer and inner).
5. Recovery and degraded-policy reducer.
6. Key-intent mapping for Input Control Nav Mode.

## 8.2 Integration Coverage (Fake tmux + Runtime)

1. Command sequence for focus transitions.
2. Inner raise plus outer focus restore.
3. Attach/detach rebind and target re-resolution.
4. Overlay + navigation coexistence.
5. Failure injection: missing inner session, stale socket, command failure.

## 8.3 Headless End-to-End Coverage

1. Nested navigation happy paths.
2. Recovery after detach/reattach.
3. Degraded behavior when nested layer unavailable.
4. Contract regression checks for outer windows/panes/focus.

## 8.4 Required Gates

- No open blocker findings.
- All required navigation tests pass in CI.
- Existing baseline tmux contract tests remain green.
- Rollback path and kill switch validated before default-on.

Measurable gate table:

| Gate | Required suites | Pass criteria |
|---|---|---|
| G1 Contract | Existing layout contract tests + NAV-001..004 unit | 100 percent pass, zero skips |
| G2 Integration | NAV-005..016 integration | 100 percent pass, zero blocker defects |
| G3 Headless | NAV-005, 006, 008, 010 headless | 100 percent pass, deterministic artifact capture on failure |
| G4 Rollout | Rollback + kill switch rehearsal | Successful disable and safe fallback verified with saved command transcript, status event log, and post-fallback focus assertion |

Mandatory baseline suites that must remain green:

- `cmd/agentx-core/core_layout_overlay_test.go`
- `tests/test_tmux_layout_headless.sh`
- `tests/test_tmux_pane_affordances_headless.sh`
- `tests/test_tmux_attached_runtime_layout_headless.sh`
- `tests/test_layout_file_fallback_headless.sh`

## 9. Risk Register

1. Contract drift between meta-config intent and live tmux state.
2. Nested lifecycle complexity (stale sockets/orphans).
3. Keybinding ambiguity between edit mode and Nav Mode.
4. Recovery races during startup/attach.
5. Operational confusion if logs ownership is duplicated.

Logs ownership rule:

- Outer `logs` window remains the authoritative logs surface.
- Any nested logs view is informational mirror only and must be labeled non-authoritative.

Mitigations:

- Strong schema validation and startup contract guard.
- Explicit mode indicator and fallback messages.
- Deterministic wrapper commands and health checks.
- Feature-flagged progressive rollout with telemetry and kill switch.

## 10. Definition of Done

1. Architecture contract approved and documented.
2. Meta-config transpilation path implemented.
3. Programmatic navigation working across configured targets.
4. Unit and integration suites cover all navigation transitions.
5. Headless E2E validates happy, degraded, and recovery paths.
6. Outer tmux compatibility contract remains unchanged.
7. Rollout guardrails and rollback path proven.
