# AgentX Hybrid Migration Feature Branch

**Branch:** `feat/hybrid-go-core-tui-migration`  
**Status:** 🚀 Foundation Scaffolded (Phase 1 Ready)  
**Created:** 2026-05-17  
**Target Merge:** After Phase 2 completion (LLM integration stable)

## What's New

This feature branch introduces the hybrid Go-core + Python-applets architecture for AgentX.

### Files Added

#### Go Core (cmd/agentx-core/)

- `go.mod` - Go module definition (chi, fatih/color, mattn/go-isatty)
- `main.go` - Entry point with signal handling and component startup
- `config.go` - Configuration structures, pane layout, session management
- `core.go` - AgentXCore orchestrator (tmux, applets, shutdown)
- `ipc.go` - IPC infrastructure (FIFOs, message format)

#### Python Applets (applets/)

- `template.py` - Template for Python applets (READY signal, env vars, shutdown)

#### Documentation (docs/)

- `HYBRID_ARCHITECTURE.md` - Complete architecture spec, IPC protocol, migration path
- `HYBRID_MIGRATION_PLAN.md` - Phase checklist, design decisions, risks, build instructions

#### Build System

- `build_core.sh` - Build script for Go core
- `Makefile.hybrid` - Makefile targets for build, test, run

#### Architecture Diagrams (docs/architecture/)

- `agentx_tui_hybrid_architecture.md` - Mermaid diagram of Go/Python/tmux structure

### Key Features (Phase 1 MVP)

- ✅ Go core compiles and creates tmux session with 5 panes
- ✅ Python applet template sends `READY` signal
- ✅ Graceful shutdown with `context.Context`
- ✅ Health endpoint (HTTP on 127.0.0.1:9876)
- ✅ IPC infrastructure (FIFOs, environment variables)
- ✅ Comprehensive documentation

### Getting Started

#### Build

```bash
cd /Projects/agentX
make -f Makefile.hybrid build
```

or

```bash
./build_core.sh
```

#### Run (after build)

```bash
./bin/agentx --project-dir . --user $USER
```

Expected output:

```
[AgentX Core] ✓ tmux session initialized
[AgentX Core] ✓ Applet supervisor started
[AgentX Core] ✓ Health endpoint started
```

Then manually attach to the tmux session:

```bash
tmux attach -t agentx_<username>_<timestamp>
```

#### Health Check

While running:

```bash
curl http://127.0.0.1:9876/health
```

### Next Steps (Phase 2: LLM Integration)

1. Migrate chat applet to use OllamaClient
2. Wire agent logic and tool execution
3. Implement context visualization (text-based)
4. Test multi-turn agentic workflows
5. Verify context persistence

### Architecture Overview

```
┌─ Go Binary (agentx-core) ─────────────────────┐
│                                                │
│  tmux Session Manager                          │
│  ├── Creates tmux session                      │
│  ├── Manages pane layout                       │
│  └── Attaches user to session                  │
│                                                │
│  Applet Supervisor (ctx.Context-based)        │
│  ├── Launches Python applets                  │
│  ├── Listens for READY signals                │
│  └── Monitors process health                  │
│                                                │
│  IPC Router                                    │
│  ├── Creates FIFOs for each applet            │
│  └── Manages message format                   │
│                                                │
│  Health Endpoint (HTTP 127.0.0.1:9876)        │
│  └── Exposes /health, /panes, /applets        │
│                                                │
└────────────────────────────────────────────────┘
         ↓
┌─ tmux Session ────────────────────────────────┐
│                                                │
│  Pane 0: Chat/Output (Python applet)          │
│  Pane 1: Logs (Python applet)                 │
│  Pane 2: Input (Python applet)                │
│  Pane 3: Context (Python applet)              │
│  Pane 4: System (Python applet)               │
│                                                │
└────────────────────────────────────────────────┘
         ↓
┌─ Python Applets ──────────────────────────────┐
│                                                │
│  template.py (instantiated for each pane)    │
│  ├── Reads AGENTX_* env vars                 │
│  ├── Sends READY signal                      │
│  ├── Handles SIGTERM                         │
│  └── Exits cleanly                           │
│                                                │
└────────────────────────────────────────────────┘
```

### Documentation

- **Architecture:** [docs/architecture/HYBRID_ARCHITECTURE.md](docs/architecture/HYBRID_ARCHITECTURE.md)
- **Migration Plan:** [docs/HYBRID_MIGRATION_PLAN.md](docs/HYBRID_MIGRATION_PLAN.md)
- **UX Design:** [docs/ux/06_TUI_MIRROR.md](docs/ux/06_TUI_MIRROR.md)
- **Applet Template:** [applets/template.py](applets/template.py)

### Branch Status

| Phase | Status | Tests | Notes |
|-------|--------|-------|-------|
| P1: Foundation | 🚀 Scaffolded | To-do | Compile-ready, needs manual testing |
| P2: LLM Integration | ❌ Not Started | | Next phase after foundation validation |
| P3: Input/Output | ❌ Not Started | | Depends on P2 |
| P4: GUI as Applet | ❌ Not Started | | Depends on P3 |
| P5: Cleanup | ❌ Not Started | | Final polish before v1.0 |

### Known Issues / TODO

- [ ] Implement health endpoint HTTP server (currently stubbed)
- [ ] Test tmux session creation on multiple platforms (Linux, macOS)
- [ ] Implement applet READY signal waiting
- [ ] Add tests for Go core components
- [ ] Create example applets for each pane
- [ ] Document env var contract for applets

### How to Contribute to This Branch

1. Review architecture and ask questions (docs/HYBRID_ARCHITECTURE.md)
2. Test Phase 1 foundation (build, run, verify placeholders)
3. Implement Phase 2 (chat applet with LLM)
4. Add tests as each component stabilizes
5. Iterate on migration plan based on learnings

### Merge Criteria

Before merging to `main`:

- [ ] Phase 1 fully working with unit tests
- [ ] Phase 2 LLM integration stable
- [ ] No regressions vs. current TUI
- [ ] Documentation complete
- [ ] Performance acceptable

---

**Created by:** Agent  
**Date:** 2026-05-17  
**Version:** v0.50.0+hybrid-draft
