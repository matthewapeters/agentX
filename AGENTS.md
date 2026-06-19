# AgentX — AGENTS.md

## Quick start

```bash
uv sync                                    # install deps
python agentx_diagnostics.py              # health check (Ollama, Agentix, deps)
uv run python main.py                      # launch GUI
```

Runtime config is `agentx.toml` at the repo root. Edit it before any run.

## Architecture at a glance

- **Python GUI**: `src/agentx/` (Tkinter), entry `main.py`
- **Agentix middleware bridge**: `src/agentix/bridge/bridge.py` (prompt classification + tool loop)
- **Shared models**: `src/shared/models/` (Context, Message, Tools, ResponseChunk, WorkingMemory)
- **Go core**: `cmd/agentx-core/` — builds to `bin/agentx`, handles TUI/multiplexer runtime
- **Config**: top-level `agentx.toml`; Go core uses `--layout ./cmd/agentx-core/layouts/default-layout.yaml`

### Multiplexer Backend Selection

AgentX Go core abstracts terminal multiplexer selection via the `MultiplexerDriver` interface.

**Supported backends**:

- `"tmux"` (default) — well-established, widely available
- `"zellij"` (modern alternative) — Rust-based, improved UX

**Configuration**:

```toml
[agentx]
multiplexer_backend = "tmux"    # default; set to "zellij" to switch
```

**Backend resolution** (`cmd/agentx-core/multiplexer_driver.go`):

```go
func resolveMultiplexerBackend(projectDir string) string {
    // 1. Reads agentx.toml [agentx] section
    // 2. Returns configured value or "tmux" (default)
    // 3. Case-insensitive and whitespace-trimmed
}
```

**Factory pattern** (`cmd/agentx-core/multiplexer_driver.go`):

```go
func newMultiplexerDriverFromConfig(projectDir string) (MultiplexerDriver, error) {
    backend := resolveMultiplexerBackend(projectDir)
    switch backend {
    case "", "tmux":
        return NewTmuxMultiplexerDriver(), nil
    case "zellij":
        return NewZellijMultiplexerDriver(), nil
    default:
        return nil, fmt.Errorf("unsupported multiplexer backend: %s", backend)
    }
}
```

**Interface contract** (`cmd/agentx-core/multiplexer_driver.go`):

```go
type MultiplexerDriver interface {
    BackendName() string                              // Returns "tmux" or "zellij"
    Run(ctx context.Context, args ...string) error   // Execute backend command
    RunCombined(ctx context.Context, args ...string) (string, error)
    Capture(ctx context.Context, args ...string) (string, error)
    AttachSession(ctx context.Context, sessionName string, stdin, stdout, stderr) error
}
```

**Adding new backends**: Implement `MultiplexerDriver` interface, add factory case, add config parsing.

## Go core

```bash
make build-core                              # compile Go binary to bin/agentx
make run                                     # build + run Go core
make run-attached                            # build + run + attach tmux client
cd cmd/agentx-core && go test ./...          # Go tests (no Make)
```

## Python tests

```bash
uv run pytest -m "not live"                  # fastest: unit tests only
uv run pytest                                # full suite (requires Ollama + Agentix)
uv run pytest tests/test_foo.py -v           # single file
AGENTIX_BENCH_RUN=1 uv run pytest -m live tests/integration/  # live integration tests
```

Markers: `live`, `unit`, `functional`, `integration`. Headless shell scripts live in `tests/`.

## Lint / format / typecheck

```bash
uv run black src/ tests/ --line-length=120
uv run isort src/ tests/ --profile=black --line-length=120
uv run flake8 src/ tests/
uv run mypy src/
```

pyproject.toml: `[tool.pytest]` adds `--cov=src --cov-report=term-missing` by default.

## Go + Python combined

```bash
make test-all                                # go-test + python-test
make demo-smoke                              # headless DemoMode smoke gate (builds Go core first)
```

## Key gotchas

- **Python 3.12.x required** — pyproject.toml enforces `>=3.12`; 3.13 not supported.
- **Ollama must be running** before any launch or test. `ollama pull llama3.2` (or whichever model you use).
- **Multiplexer backend required** — either tmux (default) or zellij. Install one before launching core.
- **tmuxp required for tmux backend** — used for layout composition in tmux.
- **`agentx.toml` is the runtime config** — settings here control Ollama host, model, multiplexer backend, layouts, and tool registry paths.
- **Go core `make target` aliases `cmd/agentx-core`** — all Make commands with `core` or `tmux` or `demo` or `hybrid` or `layout` or `verify` implicitly build Go core first unless the target doesn't state otherwise.
- **GoDog test suites**: `@unit`, `@integration`, `@functional`, `@e2e`. Use `make go-test-unit` etc. to target individual suites.
- **Tool system gap**: `src/agentix/bridge/bridge.py:315` `_stream_tool_response()` is a stub ("Tool execution coming soon…"). Multi-turn tool use with LLM feedback loop is the top open gap (see `00_START_HERE.md`).

## Testing with Different Backends

To test with a specific backend:

### Option 1: Set in agentx.toml

```toml
[agentx]
multiplexer_backend = "zellij"    # or "tmux"
```

Then run tests normally:

```bash
make go-test
uv run pytest
```

### Option 2: Environment variable (Go only)

Note: Python/pytest tests read config from `agentx.toml`; environment override is Go-only.

### Verify backend in use

Check startup log message:

```bash
./bin/agentx --project-dir . --user "$USER" 2>&1 | grep "session initialized"
```

Expected:

- tmux: `[AgentX Core] ✓ tmux session initialized`
- zellij: `[AgentX Core] ✓ zellij session initialized`

### Testing layout files

Both tmux and zellij have separate layout files auto-generated at startup:

- Tmux: `.agentx/layouts/default-layout.yaml`
- Zellij: `.agentx/layouts/zellij-layout.kdl`

To regenerate:

```bash
rm .agentx/layouts/*
./bin/agentx --project-dir . --user "$USER" --attach
```

To inspect generated layouts:

```bash
cat .agentx/layouts/default-layout.yaml    # tmux
cat .agentx/layouts/zellij-layout.kdl      # zellij
```
