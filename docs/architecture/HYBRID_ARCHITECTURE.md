# AgentX Hybrid Architecture: Go Core + Python Applets

_Last updated: 2026-05-21 (v0.62.0.post1)_

## Overview

AgentX is migrating from a pure-Python TUI to a **hybrid Go-core + Python-applets** architecture. This document outlines the design, IPC protocol, and migration strategy.

### Architecture Layers

1. **Go Core** (`cmd/agentx-core/main.go`)
   - Single binary, static executable
   - Manages tmux session/pane lifecycle
   - Supervises Python applet processes
   - Routes IPC (FIFOs, environment variables)
   - Exposes HTTP health/status endpoint
   - Owns session state and context persistence

2. **Python Applets** (`applets/`)
   - Pluggable, independent processes
   - Run in dedicated tmux panes
   - Full access to Python LLM ecosystem (ollama, transformers, etc.)
   - Communicate with Go core via standard IPC
   - Can be restarted independently without affecting core

3. **tmux Session**
   - TUI-first layout: chat, logs, input, context visualizer, system
   - Each pane runs a Python applet or built-in command
   - User navigates panes with tmux keybindings (Ctrl-b + arrow keys)
   - Pane layout defined in Go core startup

## IPC Protocol

### Environment Variables

Each applet receives these env vars on startup:

```bash
AGENTX_APPLET_NAME=chat           # Applet name
AGENTX_SESSION_ID=agentx_...      # Session ID
AGENTX_IPC_INPUT=/tmp/agentx_...  # Input FIFO path (core → applet)
AGENTX_IPC_OUTPUT=/tmp/agentx_...  # Output FIFO path (applet → core)
AGENTX_PROJECT_DIR=/path/to/project
AGENTX_SESSION_DIR=/path/to/sessions/...
```

### Startup Signal

Applet sends a JSON-formatted `READY` message on stdout:

```json
{"type": "ready", "applet": "chat", "session": "agentx_...", "timestamp": 1715974800000}
```

Go core waits for this signal and logs readiness. If not received within timeout, applet is considered failed.

### Message Format

Messages between core and applets use JSON:

```json
{
  "type": "output|input|control|error",
  "applet": "chat",
  "payload": { "text": "...", "..." }
}
```

### Shutdown Protocol

When user quits or core receives SIGTERM:

1. Go core cancels `context.Context`
2. All goroutines receive `<-ctx.Done()` signal
3. Core sends SIGTERM to each applet process
4. Applets clean up and exit
5. Core kills tmux session
6. Core exits (return to shell)

## First Iteration MVP

**Goal:** Establish core + applet infrastructure with placeholder panes.

### Pane Layout

```
┌─────────────────────────────────────────────────────────┐
│ Chat/Output (70% height)                                │
│                                                         │
├─────────────────────────────────────────────────────────┤
│ Logs (30% width)       │ Context Visualizer (20% width) │
├─────────────────────────────────────────────────────────┤
│ Input (bottom, 20% height)                              │
└─────────────────────────────────────────────────────────┘
```

### Current Runtime Behavior (Hybrid Migration Branch)

The current Go-core runtime does not yet spawn real Python applet processes in panes.

### Go Core Features

 ✅ Python chat applet bridge path exists for prompt/response handoff (`template.py --bridge-chat-server`)

```bash
# Build Go core
cd cmd/agentx-core
go build -o ../../bin/agentx

# Run
../../bin/agentx --project-dir /path/to/agentx --user $USER

# Expected output:
# [AgentX Core] ✓ tmux session initialized
# [AgentX Core] ✓ Applet supervisor started
# [AgentX Core] ✓ Health endpoint started
# (user attaches to tmux session manually)
```

## Migration Path

### Phase 1: Hybrid Foundation (Current)

- ✅ Go core orchestrates tmux and tracked pane handlers
- ✅ Deterministic prompt/input/context persistence path is implemented
- ✅ Placeholder applet process model remains migration-in-progress

### Phase 2: LLM Integration

- Migrate chat applet to use existing AgentixBridge
- Wire up agent logic, tool execution
- Implement context visualizer (text-based)

### Phase 3: Input/Output

- Implement input pane with prompt_toolkit
- Wire user prompts to chat applet
- Implement log streaming to logs pane

### Phase 4: GUI as Optional Applet

- Tkinter GUI runs as optional applet (separate process)
- Can be launched/closed from input pane
- Shares session context with core

### Phase 5: Backward Compatibility

- Keep existing Python codebase as reference
- Support both GUI and TUI modes during transition
- Deprecate pure-Python mode in v1.0.0

## Configuration & Session State

### Session Directory Structure

```
sessions/
├── mpeters/
│   └── agentx_1715974800/
│       ├── context/
│       │   ├── messages.jsonl
│       │   └── tools.json
│       └── logs/
│           ├── agentx.log
│           └── applets.log
```

### Context Manager

Go core owns `sessions/<username>/<session_id>/context/`. Python applets query/update context via IPC or HTTP endpoint.

Context structure (JSON):

```json
{
  "session_id": "agentx_...",
  "username": "mpeters",
  "created_at": 1715974800,
  "messages": [ { "role": "user", "content": "...", "id": "..." } ],
  "tools": [ { "name": "...", "description": "..." } ],
  "state": { "active_pane": "chat", "model": "llama2" }
}
```

## Health Endpoint

HTTP server on 127.0.0.1:9876 (configurable).

### Endpoints

```
GET /health
-> { "status": "ok", "session_id": "...", "uptime_seconds": 42, "pane_count": 4, "applet_count": 1 }

GET /panes
-> { "session_id": "...", "panes": [ { "name": "chat", "applet": "chat", "status": "ready" } ] }

GET /applets
-> { "session_id": "...", "applets": [ { "name": "chat", "pane": "chat", "status": "running", "crash_count": 0 } ] }

GET /context
-> { "session_id": "...", "turn_count": 1, "turns": [ { "prompt": "...", "response": "...", "created_at": 1715974800000 } ] }

POST /request-focus?pane=chat
-> { "status": "ok" }
```

## Testing Strategy

- Unit tests for core components (tmux manager, IPC router, context manager)
- Integration tests: core + mock applets
- End-to-end tests: full session lifecycle
- Applet tests: Python template + real applets

## Merge Readiness Gate

Hybrid default-branch promotion is enforced through a single gate command:

```bash
make hybrid-merge-gate
```

This gate runs required checks for startup, behavior, and layout invariants:

- `make go-test`
- `make verify-tmux-layout`
- `make build-core`

## References

- Go context package: <https://golang.org/pkg/context/>
- tmux manual: `man tmux`
- Existing Python code: `src/agentx/`, `src/agentix/` (reference during migration)
