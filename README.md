# AgentX

A local-first AI agent desktop application with a **Tkinter GUI**, streaming LLM responses, tool execution, and persistent conversation history. AgentX connects to a local [Ollama](https://ollama.com) instance for inference and optionally to **Agentix** middleware for prompt classification and advanced tool orchestration.

---

## Features

- 💬 **Streaming chat** — token-by-token response display with interrupt support
- 🔧 **Tool execution** — file read/write/search (client-side) and code analysis via CST/AST (server-side)
- 🧠 **Working memory** — persistent fact store injected into every conversation turn
- 🗂️ **File explorer** — browse and attach local files as context
- 📋 **Session history** — conversations persisted to disk; prior sessions are browsable in the sidebar
- 🏷️ **Prompt classification** — optional Agentix middleware classifies each prompt to choose the best response strategy
- ⚙️ **Model selector** — switch Ollama models at runtime without restarting

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Python | 3.12.x | Enforced by `pyproject.toml`; 3.13 not yet supported |
| [uv](https://docs.astral.sh/uv/) | latest | Package and venv manager |
| [Ollama](https://ollama.com) | latest | Must be running locally before launching AgentX |
| Agentix | optional | Provides prompt classification and server-side tools |

Install Ollama and pull a model before first run:

```bash
# Install Ollama (Linux/macOS)
curl -fsSL https://ollama.com/install.sh | sh

# Pull the default model (or any model you prefer)
ollama pull llama3.2
```

---

## Installation

```bash
# Clone the repository
git clone https://github.com/matthewapeters/agentX.git
cd agentX

# Install all dependencies into a managed virtual environment
uv sync

# Verify services are reachable
python agentx_diagnostics.py
```

---

## Configuration

All runtime settings live in **`agentx.toml`** at the project root. Edit this file before starting the application.

```toml
[agentx]
ollama_host = "localhost:11434"   # host:port of your Ollama instance
ollama_model = "llama3.2"         # must match an installed model name
ollama_initial_load_timeout_seconds = 120
screen_side = "left"              # "left" or "right" — which monitor edge the window anchors to

[agentix]
host = "localhost:8000"           # Agentix middleware host:port (optional)
classify_prompts = true           # route prompts through Agentix classification
classification_backend = "ollama" # "ollama" or "torch"
agentix_bench_classification_model = "phi4-mini:3.8b"
available_tools = ["cst"]         # code analysis tools: "cst" and/or "ast"
debug = false

[agentx.working_memory]
enabled = true          # persist facts across turns
inject_into_context = true
max_facts = 50

[agentix.classification_display]
enabled = true          # show classification metadata in the GUI
show_intent = true
show_reasoning = true
show_clarification = true
show_next_step = true
```

> **Tip:** If you are not running Agentix, set `classify_prompts = false` and AgentX will talk directly to Ollama.

---

## Running

```bash
# Launch the GUI
uv run python main.py

# Alternative module invocation
uv run python -m agentx
```

The application window docks to the side of the screen specified by `screen_side`. The left panel shows the file explorer and session history; the right panel contains the chat interface.

---

## Health Check

Run the built-in diagnostics to confirm Ollama and Agentix are reachable and all Python dependencies are present:

```bash
python agentx_diagnostics.py
```

The script checks Ollama connectivity, lists available models, verifies optional dependencies (`libcst`, `agentix`, etc.), and reports any configuration problems.

---

## Development

### Run tests

```bash
# All unit tests (no live services required)
uv run pytest -m "not live"

# Full suite including integration tests (requires Ollama + Agentix)
uv run pytest

# Single test file
uv run pytest tests/test_active_model.py -v
```

Integration tests that require live services are marked `@pytest.mark.live`. Run them with:

```bash
AGENTIX_BENCH_RUN=1 uv run pytest -m live tests/integration/
```

### Lint and format

```bash
uv run black src/ tests/ --line-length=120
uv run isort src/ tests/ --profile=black --line-length=120
uv run flake8 src/ tests/
uv run mypy src/
```

---

## Project Structure

```
agentX/
├── main.py                        # Entry point
├── agentx.toml                    # Runtime configuration
├── agentx_diagnostics.py          # Service health-check CLI
│
├── src/
│   ├── agentx/                    # GUI application
│   │   ├── session.py             # Central orchestrator
│   │   ├── service_manager.py     # Ollama + Agentix lifecycle
│   │   ├── config.py              # Config load/save
│   │   ├── gui/                   # Tkinter widgets
│   │   ├── integration/           # Bridge adapters and tool executors
│   │   ├── file_explorer.py       # File navigation panel
│   │   └── history.py             # Session history loader
│   │
│   ├── agentix/                   # Agent middleware (Agentix bridge)
│   │   ├── bridge/bridge.py       # LLM + tool loop orchestration
│   │   ├── tools/                 # CST/AST code analysis tools
│   │   └── context/               # Agentix session context helpers
│   │
│   └── shared/                    # Models shared between agentx and agentix
│       └── models/                # Message, Context, ResponseChunk, Tools
│
├── sessions/                      # Conversation history (created at runtime)
├── system_prompts/                # Markdown prompt files loaded at runtime
├── tests/                         # Pytest test suite
└── docs/                          # Architecture and integration documentation
    ├── architecture.md
    ├── tool_usage_plan.md
    └── integration/               # Phased integration plan and design docs
```

---

## Architecture Overview

```
User Input (GUI)
      │
      ▼
AgentXSession  ──► ServiceManager (Ollama health, Agentix health)
      │
      ├── classify_prompts = true
      │         │
      │         ▼
      │   AgentixBridgeAdapter ──► Agentix middleware
      │         │                  (classification + tool routing)
      │         ▼
      │   ResponseHandler (streams chunks to GUI)
      │
      └── classify_prompts = false
                │
                ▼
          Ollama (direct streaming)

Tool execution:
  ClientToolExecutor  — file read/write/search (runs in AgentX process)
  ServerToolExecutor  — CST/AST code analysis (runs via Agentix)
```

Conversations are stored under `sessions/<session_id>/context/` as JSON. The `Context` model (`src/shared/models/context.py`) is the single source of truth and handles both in-memory state and disk persistence.

---

## Documentation

| Document | Description |
|----------|-------------|
| [`docs/architecture.md`](docs/architecture.md) | Module index and architecture overview |
| [`docs/tool_usage_plan.md`](docs/tool_usage_plan.md) | Phased implementation plan for the tool pipeline |
| [`docs/integration/`](docs/integration/) | AgentX ↔ Agentix integration design and decisions |

---

## License

See [LICENSE](LICENSE) for details.
