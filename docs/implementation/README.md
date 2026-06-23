# AgentX — Implementation Documentation

This folder contains implementation-specific documentation for the AgentX TUI application runtime.

> **Note**: Distinction from [../](../) architecture documentation:
> - [../](../) contains **implementation-agnostic** UX and system specifications
> - This folder (`implementation/`) contains **implementation-specific** guidance for Go runtime, HTTP backend, deployment, and technology choices

## Folder Contents

### Go Runtime Implementation

- **Go core runtime** — Terminal multiplexer setup, session lifecycle, event loop
- **Multiplexer drivers** — tmux vs. zellij backend selection and configuration
- **HTTP backend** — REST API design, endpoint patterns, server startup
- **Channel/event routing** — Go implementation of event coordination layer

### Frontend Implementation

- **TUI client** — Terminal-based UI rendering and interaction patterns
- **Browser client** — HTTP-based web frontend (future capability)
- **Demo mode** — Headless testing and visualization harness

### Deployment and Operations

- **Building** — Docker, binary compilation, artifact management
- **Configuration** — Environment variables, config file formats (agentx.toml)
- **Debugging** — Logs, profiling, troubleshooting guides

## How to Use This Documentation

**When to reference this folder:**
- Building/compiling the runtime
- Debugging runtime failures
- Understanding Go-specific idioms or tech choices
- Setting up deployment environments
- Configuring multiplexer backend

**When to reference parent `docs/`:**
- Understanding UX requirements and surface behaviors
- Designing new features
- Writing tests that validate acceptance criteria
- Reviewing architecture and system boundaries

## Adding to This Documentation

When you implement a feature or fix, place **implementation-specific** details here:

- Go code patterns and project structure
- Multiplexer backend-specific configuration
- HTTP endpoint definitions
- Deployment procedures
- Docker/build tooling

Keep **architecture and UX specifications** in the parent `docs/` folder.

---

## See Also

- [Parent Documentation](../README.md) — Architecture, UX, and system design
- [AGENTS.md](../../AGENTS.md) — Runtime configuration and multiplexer backend selection
- [Makefile](../../Makefile) — Build and test commands
