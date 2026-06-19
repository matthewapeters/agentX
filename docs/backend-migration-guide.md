# Backend Migration Guide

## Overview

AgentX supports two terminal multiplexer backends:

| Backend | Notes |
|---------|-------|
| **tmux** | Default; well-established, widely available |
| **zellij** | Modern alternative; Rust-based, improved UX |

### Why Switch Backends?

**Switch to zellij if**:

- You prefer Rust-based tools
- You want modern multiplexer features (layout composition, mouse support)
- You're experimenting with AgentX extensibility

**Keep tmux if**:

- You already have tmux muscle memory
- You need maximum compatibility with existing tools
- You prefer the "classic" multiplexer approach

### What Stays the Same

Switching backends does **not** affect:

- Configuration structure (`agentx.toml` format)
- Pane layout intent (same panes appear in same locations)
- Chat functionality or model behavior
- Working memory or context management
- Session persistence on disk

### What's Different

| Aspect | tmux | zellij |
|--------|------|--------|
| **Keybindings** | Prefix-based (`Ctrl+b`) | Alt-based (`Alt+...`) |
| **Navigation** | `Ctrl+b arrow` | `Alt+arrow` |
| **Scroll** | `Ctrl+b [` then arrows | `Alt+PageUp/PageDown` |
| **Error output** | Typically brief | More detailed |
| **Daemon** | Optional | Required |
| **Learning curve** | Medium | Low (for new users) |

---

## Pre-Migration Checklist

Before switching backends, verify:

- [ ] Target backend is installed: `which <backend>` (tmux or zellij)
- [ ] Backup current `agentx.toml`
- [ ] Understand your current config settings (save a copy)
- [ ] Note any custom layout files in `.agentx/layouts/`
- [ ] Plan a time when you can test the new backend

---

## Migration Steps: Tmux → Zellij

### 1. Verify zellij is installed

```bash
which zellij
zellij --version
```

If not installed:

```bash
# macOS
brew install zellij

# Linux (Rust/cargo)
cargo install zellij

# Linux (package manager)
sudo apt install zellij    # Debian/Ubuntu
sudo dnf install zellij    # Fedora/RHEL
sudo pacman -S zellij      # Arch
```

### 2. Update agentx.toml

```toml
[agentx]
multiplexer_backend = "zellij"
```

### 3. Backup and kill tmux sessions

```bash
# List current tmux sessions (save names if needed)
tmux list-sessions

# Kill all tmux sessions
tmux kill-server
```

### 4. Restart AgentX

```bash
./bin/agentx --project-dir . --user "$USER" --attach
```

### 5. Test functionality

Check that:

- Zellij starts without errors
- All panes appear (chat, input, logs, context, files)
- Panes are responsive to input
- Navigation works with `Alt+arrow` keys
- Detach/reattach works: `Alt+q` then `zellij attach agentx_<username>_<time>`

### 6. Rollback (if needed)

If issues occur:

```bash
# Kill zellij sessions
pkill zellij

# Restore agentx.toml to use tmux
# Edit agentx.toml and set: multiplexer_backend = "tmux"

# Restart with tmux
./bin/agentx --project-dir . --user "$USER" --attach
```

---

## Migration Steps: Zellij → Tmux

### 1. Update agentx.toml

Option A: Set explicitly

```toml
[agentx]
multiplexer_backend = "tmux"
```

Option B: Remove the line (defaults to tmux)

```toml
# Remove: multiplexer_backend = "zellij"
```

### 2. Kill zellij sessions

```bash
# Option 1: Kill specific sessions
zellij list-sessions                           # List active sessions
zellij action kill-session --session-name <name>

# Option 2: Forcefully kill all zellij processes
pkill zellij
```

### 3. Restart AgentX

```bash
./bin/agentx --project-dir . --user "$USER" --attach
```

### 4. Test functionality

Check that:

- Tmux starts without errors
- All panes appear in correct layout
- Panes respond to input
- Navigation works with `Ctrl+b arrow` keys
- Detach/reattach works: `Ctrl+b d` then `tmux attach-session`

### 5. Rollback (if needed)

If issues occur:

```bash
# Kill tmux sessions
tmux kill-server

# Restore agentx.toml to use zellij
# Edit agentx.toml and set: multiplexer_backend = "zellij"

# Restart with zellij
./bin/agentx --project-dir . --user "$USER" --attach
```

---

## Verification Steps

After migration, verify the switch was successful:

**1. Check startup message**:

```bash
./bin/agentx --project-dir . --user "$USER" --attach 2>&1 | grep "session initialized"
```

Expected output:

- For zellij: `[AgentX Core] ✓ zellij session initialized`
- For tmux: `[AgentX Core] ✓ tmux session initialized`

**2. Verify backend is running**:

```bash
# For zellij
zellij list-sessions

# For tmux
tmux list-sessions
```

**3. Test pane interaction**:

- Click in each pane
- Type a test message
- Verify input appears
- Use navigation keys (Alt+arrow for zellij, Ctrl+b+arrow for tmux)

**4. Test detach/reattach**:

For zellij:

```bash
# Inside zellij: Alt+q to detach
# From terminal:
zellij attach agentx_<username>_<time>
```

For tmux:

```bash
# Inside tmux: Ctrl+b d to detach
# From terminal:
tmux attach-session -t agentx_<username>_<time>
```

**5. Check log locations**:

```bash
ls -la .agentx/
# Should contain: logs, layouts, sessions directories
```

---

## Performance Comparison

### First Run (New Session)

| Backend | Typical Time | Notes |
|---------|--------------|-------|
| tmux | ~100-200ms | Direct session creation |
| zellij | ~200-400ms | Includes daemon startup |

### Subsequent Runs (Daemon Active)

| Backend | Typical Time | Notes |
|---------|--------------|-------|
| tmux | ~100-200ms | Direct session creation |
| zellij | ~150-250ms | Reuses running daemon |

### Pane Response

| Backend | Latency | Notes |
|---------|---------|-------|
| tmux | ~0-10ms | Direct |
| zellij | ~0-10ms | Via daemon |

**Summary**: After first session, performance is essentially equivalent.

---

## Troubleshooting Migration

### Session creation fails

See [Troubleshooting Guide](./troubleshooting.md#session-creation-issues).

### Panes don't appear

See [Troubleshooting Guide](./troubleshooting.md#panes-invisible-or-not-created).

### Keybindings don't work

- For zellij: Alt-based keys (`Alt+arrow`, `Alt+z` for zoom)
- For tmux: Prefix-based (`Ctrl+b` then arrow keys)

See [Troubleshooting Guide](./troubleshooting.md#pane-navigation-and-display-issues) for details.

### Backend binary not found

Install the backend binary. See [Troubleshooting Guide](./troubleshooting.md#zellij-installation--setup) for installation steps.

---

## Rollback Procedure (Quick Reference)

If you need to switch back quickly:

```bash
# Kill current backend
pkill <backend>          # zellij or tmux

# Edit agentx.toml to switch backend
# Then restart
./bin/agentx --project-dir . --user "$USER" --attach
```

The entire process typically takes <1 minute.

---

## Extensibility

AgentX's multiplexer architecture supports future backends. To add a new backend:

1. Implement the `MultiplexerDriver` interface in Go
2. Add factory case in `newMultiplexerDriverFromConfig()`
3. Add config parsing in `config.go`
4. Add tests for new backend
5. Document in this guide

See `cmd/agentx-core/multiplexer_driver.go` for interface details.

---

## Support

If migration issues persist:

1. **Check prerequisites**: Run `python agentx_diagnostics.py`
2. **Review logs**: See `.agentx/logs/` for detailed output
3. **Consult troubleshooting**: See [Troubleshooting Guide](./troubleshooting.md)
4. **File issue**: Include backend version, config, and error output

---

**Quick Links**:

- [Main README](../README.md)
- [Troubleshooting Guide](./troubleshooting.md)
- [AGENTS.md Development Guide](../AGENTS.md)
