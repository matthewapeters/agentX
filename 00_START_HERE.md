# AgentX — Start Here

## What This Is

AgentX is a local-first AI agent application: a Go **client-server** app where a
core orchestration **server** routes LLM inference to a local
[Ollama](https://ollama.com) instance, and **surfaces** (built with
`charm.land/bubbletea/v2`) attach to it over HTTP/SSE as separate client
processes. Running `agentx` boots the server and the chat surface together;
other surfaces (files, config, context, context-history, context-visualizer)
are launched separately and arranged with a terminal multiplexer.

This repository's active branch (`bubbletea`) is a **Go-only rewrite**. An
earlier Python/Tkinter implementation with an "Agentix" middleware layer
existed on prior branches; it is not present here, and any file, path, or
architecture description that mentions `src/agentix/`, `bridge.py`, or a
Tkinter GUI describes that retired implementation, not this one.

## Where to Start

1. **[`CLAUDE.md`](CLAUDE.md)** — the canonical project guide: architecture
   overview, commands (`go build`, `go test`, `make all`), key invariants, and
   links into `docs/`. Read this first.
2. **[`docs/implementation/00_index.md`](docs/implementation/00_index.md)** —
   implementation guidance translating UX/architecture contracts into Go
   package-level detail (runtime lifecycle, module layout, security/approvals,
   transport).
3. **[`docs/ux/00_INDEX.md`](docs/ux/00_INDEX.md)** — per-surface affordance
   specs and the chat surface's Gherkin behavior contracts. Note its own
   migration-in-progress banner: older per-panel specs there describe a prior
   single-window layout, superseded by the current two-panel chat surface +
   independent system surfaces.
4. **[`docs/build-plan/00_index.md`](docs/build-plan/00_index.md)** — the
   active delivery plan and per-domain backlogs.
5. **[`docs/architecture/00_ARCHITECTURE_RECONCILIATION.md`](docs/architecture/00_ARCHITECTURE_RECONCILIATION.md)** —
   orientation note distinguishing the near-term build ("Family A", what
   `docs/implementation/` targets) from a future multi-expert DAG orchestrator
   ("Family B", parked).
6. **[`docs/architecture/adr/`](docs/architecture/adr/)** — architecture
   decision records (0001–0009), covering the orchestrator's control/execution
   planes, recursive task decomposition, the DAG scheduler, and plan/tool
   execution visibility.
7. **[`nits.md`](nits.md)** — the active, informal backlog of small fixes and
   feature requests being worked through directly.

## Build & Test

```bash
go build ./...       # build all packages
go test ./...         # run Go + Godog tests
make all              # clean + build (canonical gate before merge)
make help             # full target reference
```

Every touched function requires a GIVEN/WHEN/THEN behavior doc before
implementation; Godog tests use the `@unit`, `@integration`, `@functional`,
`@e2e` tag scheme (see `docs/implementation/07_test_and_documentation_contract.md`).
