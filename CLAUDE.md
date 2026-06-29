# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Repository Is

AgentX is a local-first AI agent application. It is a **client-server** app: a core
orchestration **server** is the hub, and **surfaces** are separate client processes
that attach to it over an HTTP/SSE transport. Surfaces are built with
`charm.land/bubbletea/v2`. The server routes LLM inference to a local
[Ollama](https://ollama.com) instance.

Running `agentx` boots the **server and the human-agent chat surface together**.
Other surfaces (files, config, context, context-history, context-visualizer, and
future surfaces) are launched as separate client processes from other terminals — so
the user arranges them with a multiplexer (tmux/screen/zellij) or separate windows to
fit their own workflow. A server-only launch mode is planned but out of scope for now.

**This branch (`bubbletea`)**: Go-only fresh start. The Python Tkinter GUI and Agentix
middleware from prior branches are not present here. The active entrypoint is
`cmd/agentx/`.

## Commands

### Go (active)

```bash
go build ./...                     # build all packages
go test ./...                      # run Go + Godog tests
go mod tidy && go mod vendor       # after dependency changes
```

### Make

```bash
make all                           # clean + build (canonical gate before merge)
make help                          # full target reference
```

## Architecture

```
cmd/agentx/        — single runtime entrypoint (boots server + chat surface)
internal/          — runtime packages (target layout in docs/implementation/08_go_module_layout.md;
                     mostly unbuilt — M1 fills app/runtime/cli/transport/surfaces/session/tools/llm/prompting)
tests/features/    — Godog Gherkin feature files (by domain)
tests/steps/       — Godog step implementations (importable, by domain)
tests/suites/      — Godog suite runners (tag-scoped: @unit, @integration, @functional, @e2e)
vendor/            — Vendored Go dependencies
bubbletea/         — Git submodule (charmbracelet/bubbletea local fork; wired in M1)
```

**Client-server, multi-surface model.** The core server holds canonical session and
event state and exposes it over HTTP/SSE (`docs/implementation/02_surface_orchestration_http.md`).
Each surface is a separate client process that attaches with an ephemeral token:

- **Chat surface** (launched by `agentx`) — two panels: an **output** panel
  (streaming chat, tool results, plan visualization) and an **input** panel (prompt
  entry, attachment chips, send/stop).
- **System surfaces** — what used to be one tabbed "system" panel is now a set of
  independent, separately launchable surfaces (files, config, context,
  context-history, context-visualizer). The user arranges them via their multiplexer.
- The **surface registry is open-ended** — new surface kinds (log/trace, plan/DAG
  visualizer, etc.) attach without changing existing surfaces.

LLM routing goes through the server's Ollama adapter. Tool execution is gated by the
command policy layer (blacklist / session + global whitelist, audit log). Session
events are persisted as append-only JSON under `~/.config/agentx/sessions/`.

> **Two architectures live in `docs/`.** The near-term build (this model) is "Family A."
> A future multi-expert DAG orchestrator ("Family B") is the server's later
> orchestration brain, behind the surface/transport boundary. See
> `docs/architecture/00_ARCHITECTURE_RECONCILIATION.md`.

> **UX docs migration in progress.** `docs/ux/*` still describe the prior
> single-window split-pane GUI (PD-01…PD-17, 112 affordances). They are being
> migrated to the 2-panel chat surface + independent system surfaces during M2; treat
> their surface geometry as legacy until then.

## Configuration

Runtime config: `agentx.toml` at the project root. Key fields:

| Key | Notes |
|-----|-------|
| `ollama_model` | Must match an installed Ollama model |
| `chat_runtime` | Should be `go` on this branch |

## Architecture Docs

| Document | Purpose |
|----------|---------|
| `docs/implementation/01_runtime_blueprint.md` | Go runtime lifecycle |
| `docs/implementation/08_go_module_layout.md` | Package layout and import rules |
| `docs/architecture/adr/` | Architecture decision records |
| `docs/ux/03_PANEL_DETAILS.md` | Per-surface affordance specs and Gherkin contracts |
| `docs/ux/UX_LIFECYCLE.md` | Affordance lifecycle, traceability matrix |

## Key Invariants

- `make all` must pass before any merge.
- Every touched function requires a GIVEN/WHEN/THEN behavior doc before implementation.
- Dependency changes require `go mod tidy && go mod vendor`.
- Semantic version changes require a `CHANGELOG.md` update.
- All Godog tests use the `@unit`, `@integration`, `@functional`, `@e2e` tag scheme.
