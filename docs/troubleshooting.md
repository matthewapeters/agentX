# AgentX Troubleshooting Guide

## Multiplexer Backend Issues

### Zellij Installation & Setup

#### Error: `zellij: command not found`

**Cause**: Zellij binary is not installed or not in PATH.

**Solution**:

1. Install zellij:

   ```bash
   # macOS
   brew install zellij
   
   # Linux (using cargo)
   cargo install zellij
   
   # Linux (using package manager)
   # Debian/Ubuntu
   sudo apt-get install zellij
   
   # Fedora/RHEL
   sudo dnf install zellij
   
   # Arch
   sudo pacman -S zellij
   ```

2. Verify installation:

   ```bash
   which zellij
   zellij --version
   ```

3. Restart AgentX after installation:

   ```bash
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

---

### Session Creation Issues

#### Error: `error creating zellij session: <error>`

**Possible Causes**:

- Zellij daemon is not running
- Invalid layout file path or syntax
- Permission denied on session directory
- Session name already exists

**Troubleshooting Steps**:

1. Check if zellij daemon is running:

   ```bash
   zellij list-sessions
   ```

   If no sessions appear, the daemon is not active.

2. Verify layout file exists:

   ```bash
   ls -la .agentx/layouts/
   ```

   Expected: `default-layout.yaml` (for tmux) or `zellij-layout.kdl` (for zellij)

3. Check directory permissions:

   ```bash
   ls -ld .agentx/
   # Should show 'drwx...' with your user as owner
   ```

4. Try manual restart of zellij daemon:

   ```bash
   pkill zellij
   sleep 1
   zellij
   # Then in another terminal:
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

5. Check system logs for more details:

   ```bash
   # Review recent zellij errors
   zellij --debug 2>&1 | tail -50
   ```

---

### Pane Navigation and Display Issues

#### Panes not responding or misaligned

**Cause**: Zellij keybindings differ from tmux; terminal size may be too small for layout.

**Troubleshooting**:

1. **Verify terminal size**:

   ```bash
   echo "Width: $COLUMNS, Height: $LINES"
   ```

   Recommended: Terminal should be at least 120x40 characters.

2. **Check layout file for syntax errors**:

   ```bash
   cat .agentx/layouts/zellij-layout.kdl
   ```

   Look for mismatched braces, missing semicolons, or invalid pane names.

3. **Try detaching and reattaching**:

   ```bash
   zellij action kill-session --session-name agentx_default
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

4. **Review default zellij keybindings**:

   ```bash
   zellij --print-default-config
   ```

   Common navigation:
   - Focus pane: `Alt+←` `Alt+→` `Alt+↑` `Alt+↓`
   - Zoom pane: `Alt+z`
   - Scroll up/down: `Alt+PageUp` / `Alt+PageDown`

5. **Verify pane response with a test command**:

   ```bash
   # From within zellij, try typing in each pane
   # If input doesn't appear, the pane may not be focused
   ```

---

### Session Attach/Detach Issues

#### Error: `session not found` or cannot attach to session

**Cause**: Session was killed, crashed, or multiplexer daemon died.

**Solution**:

1. List active sessions:

   ```bash
   zellij list-sessions
   ```

2. If session exists but won't attach:

   ```bash
   # Kill the problematic session
   zellij action kill-session --session-name agentx_default
   
   # Wait and restart AgentX
   sleep 1
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

3. If no sessions exist or daemon won't start:

   ```bash
   # Restart zellij daemon completely
   pkill -9 zellij
   sleep 2
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

---

## Backend Configuration Issues

### Invalid backend configuration

#### Error: `unsupported multiplexer backend: <value>`

**Cause**: `agentx.toml` has a typo or unsupported value in `multiplexer_backend`.

**Solution**:

1. Check `agentx.toml` for exact value:

   ```bash
   grep -i "multiplexer" agentx.toml
   ```

2. Valid values are:
   - `"tmux"` (default if unset)
   - `"zellij"`

3. Correct config example:

   ```toml
   [agentx]
   multiplexer_backend = "zellij"
   ```

4. Reload AgentX:

   ```bash
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

---

## Switching Between Backends

### Switching from tmux to zellij

**Steps**:

1. Verify zellij is installed:

   ```bash
   which zellij && zellij --version
   ```

2. Edit `agentx.toml`:

   ```toml
   [agentx]
   multiplexer_backend = "zellij"
   ```

3. Kill existing tmux sessions:

   ```bash
   tmux list-sessions    # List active sessions
   tmux kill-server      # Kill all sessions
   ```

4. Restart AgentX:

   ```bash
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

5. Verify in startup logs:

   ```
   [AgentX Core] ✓ zellij session initialized
   ```

---

### Switching from zellij back to tmux

**Steps**:

1. Edit `agentx.toml`:

   ```toml
   [agentx]
   multiplexer_backend = "tmux"
   ```

   Or simply remove the line (defaults to tmux).

2. Kill existing zellij sessions:

   ```bash
   zellij list-sessions                           # List active sessions
   zellij action kill-session --session-name <name>
   # Or forcefully:
   pkill zellij
   ```

3. Restart AgentX:

   ```bash
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

4. Verify in startup logs:

   ```
   [AgentX Core] ✓ tmux session initialized
   ```

---

## Performance Issues

### Zellij feels slower than tmux

**Cause**: First-time daemon startup, system load, or layout complexity.

**Analysis**:

- **First session creation**: Slightly slower due to daemon initialization (normal)
- **Subsequent sessions**: Daemon is running; pane response typically faster
- **Pane interaction**: Generally equivalent or faster than tmux

**Troubleshooting**:

1. Verify zellij daemon is running:

   ```bash
   zellij list-sessions
   ```

   If sessions appear, daemon is active and ready.

2. Check system load:

   ```bash
   top -bn1 | head -5
   htop
   ```

   If CPU/memory is high, system is under load; this may affect any multiplexer.

3. Monitor specific startup:

   ```bash
   time ./bin/agentx --project-dir . --user "$USER" --attach
   ```

   Compare startup time before and after switching backends.

4. Try restarting zellij daemon:

   ```bash
   pkill zellij
   sleep 1
   ./bin/agentx --project-dir . --user "$USER" --attach
   ```

---

## Layout and Display Issues

### Layout file not found or not applied

#### Error: `Failed to resolve layout file for backend`

**Cause**: Layout file path is incorrect or file doesn't exist.

**Solution**:

1. Check expected layout file location:

   ```bash
   # For zellij
   ls -la .agentx/layouts/zellij-layout.kdl
   
   # For tmux
   ls -la .agentx/layouts/default-layout.yaml
   ```

2. If file is missing, regenerate it:

   ```bash
   ./bin/agentx --dump-default-layout .agentx/layouts/default-layout.yaml
   ```

3. Verify layout file is readable:

   ```bash
   head -20 .agentx/layouts/default-layout.yaml
   # or
   head -20 .agentx/layouts/zellij-layout.kdl
   ```

4. Check for syntax errors:
   - YAML files: Check for indentation and colons
   - KDL files: Check for matching braces and semicolons

---

### Panes invisible or not created

**Cause**: Terminal too small, layout syntax error, or pane command failed.

**Troubleshooting**:

1. Increase terminal size:

   ```bash
   # Try resizing terminal to at least 120x40
   echo "Terminal size: $(tput cols)x$(tput lines)"
   ```

2. Check layout for pane definitions:

   ```bash
   # For zellij-layout.kdl
   grep -E "panes|name" .agentx/layouts/zellij-layout.kdl | head -20
   ```

3. Try with explicit layout flag:

   ```bash
   ./bin/agentx --project-dir . --layout .agentx/layouts/zellij-layout.kdl --attach
   ```

4. Check logs for pane errors:

   ```bash
   # Look for error messages during startup
   ./bin/agentx --project-dir . --user "$USER" --attach 2>&1 | grep -i "error\|fail"
   ```

---

## Common Zellij-Specific Behaviors

### Session name format

Zellij session names in AgentX follow the pattern: `agentx_{username}_{timestamp}`

Example: `agentx_mpeters_20260615`

To list your sessions:

```bash
zellij list-sessions | grep agentx
```

---

### Daemon background process

Zellij runs a background daemon (typically invisible).

To monitor:

```bash
ps aux | grep zellij
```

Expected: One `zellij` process managing the daemon.

To forcefully reset:

```bash
pkill -9 zellij
```

---

## Getting Help

If issues persist:

1. **Collect diagnostic information**:

   ```bash
   python agentx_diagnostics.py
   zellij --debug 2>&1 | head -50
   cat .agentx/layouts/zellij-layout.kdl 2>/dev/null || echo "Layout not found"
   ```

2. **Check backend requirements**: See [Backend Migration Guide](./backend-migration-guide.md)

3. **Report issue** with:
   - Backend name and version: `zellij --version`
   - Terminal size: `echo "$COLUMNS x $LINES"`
   - Error message (full output)
   - Steps to reproduce

---

**Need more help?** See [Backend Migration Guide](./backend-migration-guide.md) or consult the [main README](../README.md).
