# AgentX + Agentix Integration Documentation

## Overview

This documentation set provides comprehensive guidance for integrating **AgentX** (GUI frontend) with **Agentix** (agent middleware) to create a unified agentic platform for data analytics teams.

**Current Status (Feb 7, 2026):**
- Phase 4 (Context Unification): **Complete**
- Phase 5 (Testing & Refinement): **In Progress (benchmarks below target)**

## Project Summary

| Project | Purpose | Key Technology |
|---------|---------|----------------|
| **AgentX** | tkinter-based GUI chat application | `ollama` Python library, tkinter |
| **Agentix** | Agent middleware with intent classification | REST API, FastAPI, MCP tools |

> **Design Principle:** AgentX is the better-structured project (designed after Agentix). AgentX owns all client-side state. Agentix server operates statelessly.

## Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Session Storage** | AgentX structure only | AgentX is canonical; Agentix server is stateless |
| **Ollama Communication** | Hybrid | `ollama` library for local streaming UX; REST for remote Agentix |
| **Tool Execution** | Client + Server | File ops on client; DB/API ops on server |
| **Context Ownership** | AgentX client | All state persisted client-side in session folders |

## Integration Goal

> Create a highly-distributable agentic platform where AgentX serves as the user-facing frontend and Agentix provides intelligent prompt analysis, tool orchestration, and middleware capabilities.

---

## Documentation Index

### 1. [Architecture Overview](01_ARCHITECTURE_OVERVIEW.md)
- Current state analysis of both projects
- Target integrated architecture
- Component responsibilities
- Data models and interfaces
- Key design decisions

### 2. [Phased Integration Plan](02_PHASED_INTEGRATION_PLAN.md)
- **Phase 0:** Foundation & Preparation (3-5 days)
- **Phase 1:** Agentix Programmatic API (5-7 days)
- **Phase 2:** AgentX Integration Layer (5-7 days)
- **Phase 3:** Model Selection & Tool Display (4-5 days)
- **Phase 4:** Context Unification (4-5 days) — **Complete**
- **Phase 5:** Testing & Refinement (3-4 days) — **In Progress**

### 3. [Research & Clarifications](03_RESEARCH_AND_CLARIFICATIONS.md)
- Questions requiring user input
- Technical research areas
- Architectural decisions pending
- Prototype experiments needed

### 4. [Improvement Suggestions](04_IMPROVEMENT_SUGGESTIONS.md)
- High-priority improvements (unified client, error handling, token counting)
- Medium-priority enhancements (plugin architecture, session export)
- Nice-to-have features (themes, keyboard shortcuts, search)
- Technical debt to address

---

## Quick Start for Agents

When implementing integration tasks, read documents in this order:

1. **First:** Read `01_ARCHITECTURE_OVERVIEW.md` for context
2. **Second:** Find your phase in `02_PHASED_INTEGRATION_PLAN.md`
3. **Check:** Review `03_RESEARCH_AND_CLARIFICATIONS.md` for blockers
4. **Apply:** Use patterns from `04_IMPROVEMENT_SUGGESTIONS.md`

---

## Key Integration Points Summary

| Integration Point | AgentX Component | Agentix Component | Action |
|-------------------|------------------|-------------------|--------|
| User Prompts | `AgentXSession.stream_ollama_response_worker()` | `agentix()` | Route through classification |
| Context | `Context`, `Message` | `sessions.py`, `Message` | Unify models |
| Models | Config TOML | `get_models()` | Display selector in GUI |
| Tools | Console output | `tools/` module | Render in GUI, add to context |

---

## File Structure (Post-Integration)

```
src/
├── shared/                      # NEW: Shared module
│   ├── models/
│   │   ├── message.py          # Unified Message
│   │   ├── context.py          # Unified Context
│   │   └── response.py         # ResponseChunk
│   ├── config/
│   │   └── unified_config.py   # Combined configuration
│   └── ollama_client/
│       └── interface.py        # Unified Ollama client
│
├── agentx/
│   ├── session.py              # Enhanced with Agentix bridge
│   ├── gui_manager.py          # Enhanced with tool display
│   └── integration/            # NEW: Integration layer
│       ├── agentix_bridge_adapter.py
│       ├── model_selector.py
│       └── tool_panel.py
│
└── agentix/
    ├── bridge.py               # NEW: Programmatic API
    ├── api_client.py           # Enhanced with streaming
    └── context/
        └── sessions.py         # References AgentX sessions
```

---

## Timeline Estimate

| Phase | Duration | Cumulative |
|-------|----------|------------|
| Phase 0 | 3-5 days | 3-5 days |
| Phase 1 | 5-7 days | 8-12 days |
| Phase 2 | 5-7 days | 13-19 days |
| Phase 3 | 4-5 days | 17-24 days |
| Phase 4 | 4-5 days | 21-29 days |
| Phase 5 | 3-4 days | 24-33 days |

**Total:** 4-6 weeks (depending on team size and blockers)

---

## Critical User Decisions ✅ RESOLVED

All critical decisions have been made:

| Decision | Resolution |
|----------|------------|
| **Session Storage** | ✅ AgentX structure - single source of truth |
| **Ollama Communication** | ✅ Hybrid - right tool for each job |
| **Tool Execution** | ✅ Both client-side and server-side supported |

See [Research & Clarifications](03_RESEARCH_AND_CLARIFICATIONS.md) for detailed rationale.

---

## Verification Checkpoints

### Phase 0 Complete When:
- [ ] `from shared.config import UnifiedConfig` works
- [ ] Both projects import shared module successfully
- [ ] Integration test scaffold runs

### Phase 1 Complete When:
- [ ] `AgentixBridge.classify_prompt()` returns valid classification
- [ ] Streaming API client yields response chunks
- [ ] Unit tests pass

### Phase 2 Complete When:
- [ ] User prompts route through Agentix classification
- [ ] GUI displays classification result
- [ ] Existing chat functionality unbroken

### Phase 3 Complete When:
- [ ] Model selector populates from Agentix
- [ ] Tool panel displays available tools
- [ ] Tool calls appear in context

### Phase 4 Complete When:
- [x] Single unified Message/Context model
- [x] Agentix reads AgentX sessions
- [x] All tests pass with new models

**Phase 4 Status (verification):**
- ✅ AgentX and Agentix both re-export shared models (no legacy message/context classes)
- ✅ Agentix session handling prefers AgentX session structure when available
- ✅ GUI rendering accepts `MessageRole` enums
- ✅ Full test suite passes

### Phase 5 Complete When:
- [x] Integration tests pass
- [ ] Performance benchmarks met
- [x] Documentation complete

**Phase 5 Status (verification):**
- ✅ Phase 5 tool execution tests passed: [tests/test_phase5_tool_execution.py](tests/test_phase5_tool_execution.py)
- ✅ Integration tests passed: [tests/integration/](tests/integration/)
- ✅ Full test suite passes (`pytest -q`)
- ⚠️ Performance benchmarks ran; classification latency exceeded target (10.8s > 2.0s)
- ✅ Streaming first chunk benchmark passed (< 8.0s)
- Benchmarks: [tests/integration/test_performance.py](tests/integration/test_performance.py) (run with `AGENTIX_BENCH_RUN=1 pytest -m live tests/integration/test_performance.py`)
- Tip: try `AGENTIX_BENCH_CLASSIFICATION_MODEL=phi4-mini:3.8b` to reduce classification latency

---

## Contact & Resources

- **Repository:** `matthewapeters/agentX`
- **Branch:** `main`
- **Related:** Ollama documentation, MCP specification

---

*Generated: February 7, 2026*
