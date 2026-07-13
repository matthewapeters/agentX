# AgentX — Implementation Documentation

This folder contains implementation-specific documentation that reconciles UX and architecture contracts into build guidance.

> **Note**: Distinction from [../](../) architecture documentation:
>
> - [../](../) contains **implementation-agnostic** UX and system specifications
> - This folder (`implementation/`) contains **implementation-specific** guidance for Go runtime, HTTP backend, deployment, and technology choices

## Start Here

- [00_index.md](00_index.md) is the authoritative entry point for implementation planning.

## Folder Contents

### Runtime Implementation

- **Go core runtime** — orchestrator lifecycle, session lifecycle, event loop
- **HTTP backend** — REST API design, endpoint patterns, server startup
- **Channel/event routing** — event coordination and processing-state distribution

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
- Implementing surface orchestration and transport contracts

**When to reference parent `docs/`:**

- Understanding UX requirements and surface behaviors
- Designing new features
- Writing tests that validate acceptance criteria
- Reviewing architecture and system boundaries

## Adding to This Documentation

When you implement a feature or fix, place **implementation-specific** details here:

- Go code patterns and project structure
- HTTP endpoint definitions
- Deployment procedures
- Docker/build tooling

Keep **architecture and UX specifications** in the parent `docs/` folder.

---

## See Also

- [Parent Documentation](../../README.md) — Architecture, UX, and system design
- [AGENTS.md](../../AGENTS.md) — Runtime configuration and launch notes
- [Makefile](../../Makefile) — Build and test commands
