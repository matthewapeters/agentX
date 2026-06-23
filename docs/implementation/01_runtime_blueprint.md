# Runtime Blueprint

## Summary

AgentX is implemented as a Go module and launched through a single command, agentx.
The runtime orchestrator coordinates:

- LLM interaction (default Ollama)
- Tool invocation with explicit user approvals
- Processing state publication
- Session persistence
- Surface lifecycle and transport endpoints

## High-Level Process Model

Single binary:

- agentx starts orchestrator
- orchestrator initializes config, storage, model adapter, tool registry
- orchestrator allocates surface ports and starts transport listeners
- orchestrator starts event coordination layer and processing-state publisher

Suggested process shape for v1:

- One orchestrator OS process (agentx)
- Multiple surface OS processes launched and supervised by orchestrator
- Bi-directional HTTP communication between orchestrator and surfaces
- Internal orchestrator goroutines for:
  - surface transport listeners and routing
  - event fan-out
  - tool execution workers
  - persistence writer
  - model streaming client

Operational requirement:

- A user can launch child surfaces from additional terminal sessions (for example via multiplexer workflows), and orchestrator must register/manage their lifecycle.

## Go Module Strategy

Repository root remains primary go module.

Proposed module additions:

- internal/runtime (orchestrator, lifecycle)
- internal/surfaces (surface contracts and host)
- internal/transport/http (surface transport)
- internal/state (processing state and event channels)
- internal/session (session store and JSON schema)
- internal/tools (tool registry, approval workflow, execution)
- internal/prompting (prompt assembly and procedural prompts)
- internal/llm/ollama (default model adapter)

## Bubble Tea Adoption

Adopt Charmbracelet packages for rich TUI implementation.

Recommended dependencies:

- github.com/charmbracelet/bubbletea
- github.com/charmbracelet/lipgloss
- github.com/charmbracelet/bubbles
- github.com/charmbracelet/glamour (optional markdown rendering)

Integration pattern:

- Bubble Tea program is one first-class surface endpoint in the orchestrator
- surface updates consume canonical processing state and event channels
- no business logic lives exclusively in UI components

Dependency strategy:

- Use Go module dependencies for Charmbracelet packages in v1.
- Add repository submodule reference for charmbracelet/bubbletea per project requirement.
- Keep go mod as the build dependency mechanism; submodule is maintained as pinned source mirror and reference.

## Runtime Lifecycle

Startup:

1. Resolve config path precedence
2. Seed default config and prompt/tool files if missing
3. Create session identity set:
   - stable session_id (internal)
   - human-readable session_name (adjective-noun style)
4. Build runtime dependency graph
5. Allocate/verify ports
6. Start transports
7. Start surface host(s)
8. Load or verify default model
9. Enter serving loop

Session identity policy (v1):

- session_name is user-facing and human-readable.
- session_id is canonical internal identity for routing, persistence, and audit.
- name collisions must be resolved deterministically (for example, suffix increment).

Surface launch policy (v1):

- canonical CLI shape uses subcommands:
  - agentx surface launch <surface-name> --session <session-name-or-id> --connect <endpoint> --token <attach-token>
- backwards-compatible alias form is supported:
  - agentx --launch|-l <surface-name> --session|-s <session-name-or-id> --port|-p <port>
- orchestrator should print ready-to-run launch commands for additional terminal sessions.

Shutdown:

1. Stop accepting new prompt submissions
2. Drain in-flight tool/model tasks
3. Flush session writes
4. Persist final processing state snapshot
5. Send shutdown command to all registered surfaces and verify exit
6. Close listeners and exit

## Decisions Already Anchored

- Single command entrypoint: agentx
- Go implementation with go mod
- Bubble Tea family for TUI
- HTTP transport initially, HTTPS planned later
- Processing state is canonical shared status model
