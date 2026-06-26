# AgentX — AGENTS.md

See [CLAUDE.md](CLAUDE.md) for development guidance on this branch.

## Active entrypoint

```
cmd/agentx/    — Go TUI binary (charm.land/bubbletea/v2)
```

## Quick commands

```bash
cd cmd/agentx && go build ./...    # build
cd cmd/agentx && go test ./...     # test
make all                           # clean + build (canonical gate)
```

Runtime config: `agentx.toml` at the repo root.
LLM backend: Ollama — must be running locally before launching.
