# AgentX

A local-first AI agent application. AgentX is a **client-server** app: a core
orchestration **server** routes LLM inference to a local
[Ollama](https://ollama.com) instance, and **surfaces** — built with
[`charm.land/bubbletea/v2`](https://charm.land) — attach to it over HTTP/SSE
as separate client processes. Running `agentx` boots the server and the chat
surface together; other surfaces (files, config, context, context-history,
context-visualizer, working-memory) are launched separately and arranged with
a terminal multiplexer to fit your own workflow.

![AgentX chat surface](docs/agentx_tui_screenshot.png)

---

## Features

| Feature | Description |
| ---- | ---- |
| 💬 **Streaming chat** | token-by-token response display, with markdown rendered as terminal styling (bold, headers, lists, blockquotes, GFM tables, syntax-highlighted code blocks) |
| 🔧 **Tool execution** | file read/write/search and shell commands, gated by a command policy (blacklist + session/global approval whitelist) |
| ✅ **Interactive approvals** | a navigable-list widget for any decision the runtime needs from you (tool approval, stated-continuation follow-ups) — one consistent interaction regardless of what's being asked, with a persisted, gray one-line audit record of what you decided |
| 🧠 **Working memory** | a persistent fact store injected into every conversation turn |
| 🗂️ **Context management** | enable/disable individual turns, attachments, and tool usage from what's sent to the model — curate context without lossy summarization |
| 🗺️ **Task decomposition** | a compound goal is broken into a DAG of investigative steps, executed (with real read/write tools) before the model answers, so responses are grounded in what was actually found |
| 📋 **Session history** | conversations persist to disk as an append-only event log; prior sessions are resumable and browsable via the context-history surface |
| 🖥️ **Multi-surface architecture** | independently launchable surfaces (files, config, context, context-history, context-visualizer, working-memory) attach to the same running session over HTTP/SSE — arrange them with tmux/zellij/screen as you like |

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| [Go](https://go.dev) | version matching `go.mod` (currently 1.26+) |
| [Ollama](https://ollama.com) | must be running locally, with at least one model pulled |
| A terminal multiplexer (optional) | tmux, zellij, or screen — only needed if you want to run additional surfaces alongside the chat window |

Install Ollama and pull a model before first run:

```bash
# Install Ollama (Linux/macOS)
curl -fsSL https://ollama.com/install.sh | sh

# Pull a model
ollama pull llama3.2
```

---

## Installation

```bash
git clone https://github.com/matthewapeters/agentX.git
cd agentX
make install     # build + seed config into ~/.config/agentx + install the binary
```

`make install` builds the binary, seeds baseline config/prompt files into
`~/.config/agentx/` (without overwriting anything already customized there),
installs `agentx` onto your `PATH`, and runs a health check (`make doctor`).

To just build without installing:

```bash
make build       # compiles to bin/agentx
```

---

## Configuration

Runtime settings live in **`agentx.toml`** at the project root (and are seeded
to `~/.config/agentx/agentx.toml` on install). Key fields:

```toml
[agentx]
chat_backend = "ollama"

[agentx.ollama]
host  = "localhost:11434"
model = "llama3.2"          # must match an installed Ollama model

[agentx.tools]
enabled   = true
read_only = true            # write/network tools are denied outright when true

[agentx.thinking]
enabled = true

[agentx.transport]
enabled = true               # HTTP/SSE endpoint other surfaces attach to
```

See `docs/implementation/03_configuration_and_storage.md` for the full schema.

---

## Running

```bash
# Launch the server + chat surface
agentx

# Name the session explicitly (useful for scripted multiplexer layouts)
agentx --session my-session
```

Launch additional surfaces from other terminals, attached to the same
session:

```bash
agentx surface launch files --session my-session
agentx surface launch config --session my-session
agentx surface launch context --session my-session
agentx surface launch context-history --session my-session
agentx surface launch context-visualizer --session my-session
agentx surface launch working-memory --session my-session
```

Arrange these with tmux, zellij, or screen to build your own layout — a
server-only launch mode (no bundled chat surface) is planned but not yet
available.

---

## Development

```bash
go build ./...          # build all packages
go test ./...            # run Go + Godog tests
make all                 # clean + build (canonical gate before merge)
make help                # full target reference
```

`make all` is the required gate before any merge. Godog behavior tests use
the `@unit`, `@integration`, `@functional`, `@e2e` tag scheme; see
`docs/implementation/07_test_and_documentation_contract.md`.

---

## Project Structure

```
agentX/
├── cmd/agentx/           # runtime entrypoint (boots server + chat surface)
├── internal/             # runtime packages: app, runtime, cli, transport,
│                         # surfaces, session, tools, llm, prompting, config
├── tests/
│   ├── features/         # Godog Gherkin feature files, by domain
│   ├── steps/            # Godog step implementations, by domain
│   └── suites/           # tag-scoped Godog suite runners
├── docs/                 # architecture, implementation, UX, and build-plan docs
├── system_prompts/       # default prompt templates
├── config/seed/          # baseline config/prompt files installed to ~/.config/agentx
├── bubbletea/             # git submodule: charmbracelet/bubbletea local fork
└── agentx.toml           # runtime configuration
```

---

## Architecture Overview

```
┌─────────────────────────────┐        HTTP/SSE        ┌──────────────────┐
│  agentx (server + chat)     │◄───────────────────────►│  other surfaces  │
│                              │                         │  (files, config, │
│  ┌────────────────────────┐  │                         │  context, ...)   │
│  │ Orchestrator           │  │                         └──────────────────┘
│  │  classify → route:     │  │
│  │   respond_directly     │  │
│  │   single_tool          │──┼──► tool proposer → command policy →
│  │   invoke_planner       │  │      approval gate (if required) → executor
│  └───────────┬─────────────┘  │
│              ▼                │
│         Ollama (local)        │
└─────────────────────────────┘
```

Every prompt is classified into a route: answer directly, run one tool, or
decompose into a DAG of investigative steps (task decomposition) that execute
before the model synthesizes an answer. Tool calls requiring approval go
through a single shared decision gate — the same interactive-approval widget
handles every kind of decision the runtime can ask about, serialized one at a
time. Session events (prompts, responses, tool calls/results, approvals) are
persisted as an append-only JSON log under `~/.config/agentx/sessions/`.

See `docs/architecture/adr/` for the full set of architecture decision
records, and `docs/architecture/00_ARCHITECTURE_RECONCILIATION.md` for the
near-term ("Family A") vs. future ("Family B") distinction.

---

## Documentation

| Document | Description |
|----------|-------------|
| [`00_START_HERE.md`](00_START_HERE.md) | Orientation and reading path into `docs/` |
| [`CLAUDE.md`](CLAUDE.md) | Canonical project guide: architecture, commands, key invariants |
| [`docs/implementation/`](docs/implementation/) | Go runtime lifecycle, module layout, security/approvals, transport |
| [`docs/ux/`](docs/ux/) | Per-surface affordance specs and behavior contracts |
| [`docs/architecture/adr/`](docs/architecture/adr/) | Architecture decision records |
| [`docs/build-plan/`](docs/build-plan/) | Active delivery plan and per-domain backlogs |
| [`nits.md`](nits.md) | Informal active backlog of small fixes and feature requests |

---

## License

No `LICENSE` file has been published for this repository yet.
