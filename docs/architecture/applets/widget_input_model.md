# Widget Input Model (Unified)

Last updated: 2026-06-03

## Goal

Define one logical input contract for all Go-native applets so:

- interactive terminal mode (raw keys) and headless mode (line commands)
  drive the same state transitions,
- command behavior is consistent across applets,
- E2E testing remains deterministic without a TTY dependency.

## Logical Command Schema

The runtime command payload is a normalized command token.

Core tokens (cross-applet baseline):

- `q` / `quit`
- `?` / `help`
- `r` / `refresh`
- `k` / `j` (move up/down)
- `left` / `right`
- `pgup` / `pgdn`
- `top` / `end`
- `enter`
- `tab`
- `space`

Extended tokens (applet-specific):

- Filesystem/settings/context: `a`, `e`, `u`, `b`, `f`, `h`
- Input applet: key events represented as logical input actions and mapped into
  local state transitions (cursor move, viewport pan, focus toggle, submit)
- Output applet (current): line-command parser; planned alignment to shared
  navigation/collapse tokens as collapse widgets are added

## Input Adapters

### 1) Interactive adapter (TTY)

- Reads raw terminal bytes.
- Normalizes escape sequences and aliases into logical command tokens.
- Emits one command at a time into applet state handlers.

### 2) Headless adapter (non-TTY)

- Reads newline-delimited command text from stdin.
- Normalizes aliases into the same logical command tokens.
- Enables deterministic command-script E2E in CI.

## Shared Debug Toggle

Environment variable:

- `AGENTX_WIDGET_KEY_DEBUG`

When enabled (`1|true|yes|on|debug`), widget input adapters emit raw key/escape
input and normalized command mapping to stderr.

Purpose:

- diagnose terminal-emulator escape-sequence differences,
- validate normalization behavior without changing applet logic,
- speed up onboarding of additional key variants.

## Shared Loop Pre-Handler

`handleWidgetLoopControlCommand` in `cmd/agentx-core/widget_input.go` provides a
baseline pre-handler for all applet loops. It evaluates quit/help/refresh intents
before applet-specific command dispatch, via callback-based `widgetLoopControlHandlers`.

All active interactive applets are wired:

- filesystem, settings, context, output loops all use `handleWidgetLoopControlCommand`.
- Per-applet command semantics remain local to each applet's `handleCommand` path.

## Current Adoption Snapshot

- Shared reader/normalizer + shared loop pre-handler:
  - filesystem
  - settings
  - context
- Shared reader with line-assembly adapter + shared loop pre-handler:
  - output
- Custom input reader with shared debug toggle:
  - input
- Passive/no-input:
  - logs

## Headless E2E Strategy

Command-stream tests are independent of real key events. The strategy is
implemented as a shared harness in `cmd/agentx-core/widget_harness_test.go`:

- `runHeadlessCommandScript` — drives a command-reader factory with a scripted input stream.
- `runHeadlessWidgetLoopScript` — executes a full applet loop against scripted input and captures output.
- `runHeadlessWidgetCommandScript` — invokes a command entrypoint with scripted input and captures exit code + output.
- `setWidgetTestEnv` — scoped environment setup using `t.Setenv`.
- `createWidgetTestProjectDir` — temp project directory provisioning for filesystem/context tests.
- `createWidgetTestConfigProject` — temp project directory + `agentx.toml` provisioning for settings tests.

Raw-key behavior remains unit-tested at normalization and state-handler level.

## Migration Plan (Incremental)

1. ✅ Shared adapter for filesystem/settings/context stable and loop pre-handler wired.
2. ✅ Output applet migrated to shared reader; loop pre-handler wired for `:q`/`:help`.
3. Continue input applet custom behavior while sharing debug and key
   normalization policy.
4. Expand output applet command handling to consume navigation/collapse tokens
   directly as output expand/collapse widgets are added.
5. Keep logs applet passive, but reserve `q/help/refresh` compatibility if
   interactive controls are later introduced.

## Contract Rules

- Applet state handlers consume logical commands, never raw bytes.
- Raw key parsing is infrastructure-only.
- Headless and interactive paths must normalize to equivalent command tokens.
