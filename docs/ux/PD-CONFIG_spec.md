# AgentX — PD-CONFIG: Configuration Surface (TUI)

> **Status:** Spec only — not yet implemented.
> **Surface kind:** `config` (registered in `internal/surfaces/registry.go:49`)
> **Launch command:** `agentx surface launch config`
> **Owner:** Delivery Lead (M2+)

---

## Purpose

Let the user inspect and edit AgentX's runtime configuration (`agentx.toml`) through an interactive TUI that:

1. **Reflects the source of truth.** `agentx.toml` is the canonical config file. The surface reads from it, writes to it, and detects changes from both the file and the TUI.
2. **Pushes changes live to the running orchestrator.** Editing a config key in the TUI (or detecting a file change) updates the running session's state immediately — no restart required for tunable keys.
3. **Validates before accepting.** Host fields are tested against the live provider endpoint before acceptance. Numeric fields are type-validated. Model fields are populated from the provider's API.
4. **Handles complex changes gracefully.** Some changes (context size, provider switch) require special handling — the surface communicates what's needed and guides the user.

---

## Architecture

```
User terminal                          AgentX orchestrator              agentx.toml
  ┌──────────────┐                      ┌──────────────────┐              ┌──────────────┐
  │ config.TUI   │◀────SSE────┐         │                  │              │              │
  │ (Bubble Tea) │            │         │  Provider        │              │              │
  │              │            │         │  (read-only)     │              │              │
  │ ├─ tree      │            ├────────▶│  ├─ GET /config  │              │              │
  │ ├─ editor    │            │         │  ├─ POST /config │              │              │
  │ ├─ status    │            │         │  ├─ GET /provider│              │              │
  │ └─ hint row  │            │         │    /{name}/models│              │              │
  └──────────────┘            │         │  └─ POST /test   │              │              │
                              │         │    /host          │              │              │
                              │         └────────┬─────────┘              │              │
                              │                  │                        │              │
                              └──────────────────┼────────────────────────┘              │
                                                   │    Filesystem watch (in-process)        │
                                                   │                                        │
                                                   └────────────────────────────────────────┘
```

**Key design decisions:**

1. **Separate Bubble Tea v2 client process** — launched by `agentx surface launch config`, attaching over the transport (same pattern as `workmemory`, `contextviz`).
2. **Document-based, not event-stream-based** — config is a single TOML file, not an event log. The surface reads the whole file on attach, polls for changes (file watch + transport push), and writes back on save.
3. **Two-way sync** — detects external file changes (via filesystem watch in the orchestrator) AND TUI changes. When the file changes externally, the surface reloads and highlights the diff. When the TUI changes, it writes to disk AND pushes to the orchestrator.
4. **Per-instance, not per-session** — config applies to the AgentX installation, not to a specific session. The surface attaches to the running orchestrator (which manages one instance), but changes affect all sessions.
5. **Live push for tunable keys** — the orchestrator applies changes immediately for keys that don't require restart (border colors, widget line caps, thinking routes, tool timeouts). For restart-required keys (provider, model, host), the surface notifies the user and offers to restart.

---

## Internal Structure

```text
ConfigSurface
  ├─ TitleBar              — "config" + session name (if --session-in-title)
  ├─ SectionTree           — collapsible tree of config sections ([agentx.ollama], etc.)
  │    ├─ SectionHeader    — collapsible header with section name
  │    └─ KeyList          — list of keys in this section
  │         ├─ KeyRow      — one row per config key
  │         │    ├─ KeyLabel   — key name (e.g. "host", "model")
  │         │    ├─ KeyValue   — current value (text, dropdown, toggle, color)
  │         │    └─ StatusIcon — ✓ unchanged, ↻ pending, 🔁 restart required
  │         └─ ...
  ├─ StatusBar             — one-line processing state (syncing, saved, error)
  ├─ HintRow               — keybindings hint
  └─ DialogOverlay         — modal dialogs (confirm restart, error, model picker)
```

**Component responsibilities:**

| Component | Responsibility |
|-----------|---------------|
| `TitleBar` | Surface title, session name (optional) |
| `SectionTree` | Navigation between config sections, expand/collapse |
| `KeyList` | List keys in a section, selection cursor |
| `KeyRow` | Render one config key with its value editor |
| `StatusBar` | Sync state, error messages, unsaved-changes indicator |
| `HintRow` | Keybindings (j/k scroll, ↑/↓ navigate, ↵ edit, s save, q quit, ? help) |
| `DialogOverlay` | Modal confirmations (restart required), errors, model pickers |

---

## Config Key Taxonomy

Config keys are categorized by their editability and runtime behavior:

### Editable, live-reload (no restart)

| Section | Key | Type | Validation | Live Reload |
|---------|-----|------|------------|-------------|
| `[agentx.classification]` | `retries` | int ≥ 0 | Integer, non-negative | ✅ |
| `[agentx.classification]` | `clarification_options` | int ≥ 1 | Integer, ≥ 1 | ✅ |
| `[agentx.output]` | `max_widget_lines` | int ≥ 1 | Integer, ≥ 1 | ✅ |
| `[agentx.output]` | `input_max_lines` | int ≥ 1 | Integer, ≥ 1 | ✅ |
| `[agentx.output]` | `markdown_renderer` | enum: `native`, `scanner` | Dropdown | ✅ |
| `[agentx.thinking]` | `enabled` | bool | Toggle | ✅ |
| `[agentx.thinking]` | `time_budget_seconds` | int ≥ 0 | Integer, non-negative | ✅ |
| `[agentx.thinking.routes]` | `respond_directly` | bool | Toggle | ✅ |
| `[agentx.thinking.routes]` | `single_tool` | bool | Toggle | ✅ |
| `[agentx.thinking.routes]` | `invoke_planner` | bool | Toggle | ✅ |
| `[agentx.theme]` | `active_border_color` | color | Name/ANSI/hex | ✅ |
| `[agentx.theme]` | `inactive_border_color` | color | Name/ANSI/hex | ✅ |
| `[agentx.tools]` | `enabled` | bool | Toggle | ✅ |
| `[agentx.tools]` | `read_only` | bool | Toggle | ✅ |
| `[agentx.tools]` | `timeout_seconds` | int ≥ 1 | Integer, ≥ 1 | ✅ |
| `[agentx.tools]` | `output_max_bytes` | int ≥ 1024 | Integer, ≥ 1 KiB | ✅ |
| `[agentx.tools]` | `absolute_max_bytes` | int ≥ `output_max_bytes` | Integer, ≥ output_max_bytes | ✅ |
| `[agentx.wavefront]` | `enabled` | bool | Toggle | ✅ |

### Editable, requires restart

| Section | Key | Type | Validation | Restart Required |
|---------|-----|------|------------|------------------|
| `[agentx]` | `provider` | enum: `ollama`, `llamacpp` | Dropdown | ✅ (recreates model adapter) |
| `[agentx.ollama]` | `host` | string | URL/hostname:port, tested against live endpoint | ✅ (reconnects Ollama client) |
| `[agentx.ollama]` | `model` | string | Dropdown (populated from provider API), tested against live endpoint | ✅ (reconnects Ollama client) |
| `[agentx.llamacpp]` | `host` | string | URL/hostname:port, tested against live endpoint | ✅ (reconnects llama.cpp client) |
| `[agentx.llamacpp]` | `model` | string | Dropdown (populated from provider API), tested against live endpoint | ✅ (reconnects llama.cpp client) |
| `[agentx.transport]` | `enabled` | bool | Toggle | ✅ (starts/stops HTTP server) |
| `[agentx.transport]` | `host` | string | Loopback IP only (v1 policy) | ✅ (rebinds listener) |
| `[agentx.transport]` | `port_start` | int 1024–65535 | Integer, valid port range | ✅ (rebinds listener) |
| `[agentx.transport]` | `port_end` | int ≥ `port_start` | Integer, ≥ port_start | ✅ (rebinds listener) |

### Read-only (display only, not editable)

| Section | Key | Source | Display |
|---------|-----|--------|---------|
| Session identity | `session_id`, `session_name` | Orchestrator | Displayed in title/status |
| Ollama context length | (derived) | `POST /api/show` | Displayed in status |
| Connected surfaces | (derived) | `GET /surfaces` | Displayed in status |

### Prompt files (managed separately)

The following prompt files are **not** edited inline in the config surface. They are displayed as file paths with an "open in editor" affordance:

| File | Purpose |
|------|---------|
| `agentx-instructions.md` | Standing user instructions |
| `bootstrap-prompt.md` | Startup auto-submit prompt |
| `agentx-classification.md` | Classification system prompt |
| `agentx-thinking.md` | Thinking guidance |
| `agentx-planner.md` | Decomposition planner prompt |
| `agentx-shell-commands.md` | Tool catalog |
| `agentx-wavefront-classify.md` | Wavefront classify prompt |
| `agentx-wavefront-synthesis.md` | Wavefront synthesis prompt |
| `agentx-wavefront-summary.md` | Wavefront summary prompt |

---

## Behaviour Inventory

### PD-CONFIG-AF-001: Launch and attach

| Attribute | Value |
|-----------|-------|
| **Trigger** | `agentx surface launch config` |
| **Expected behaviour** | Surface attaches to the running orchestrator, reads `agentx.toml`, renders the config tree |
| **Edge cases** | No running orchestrator → clear error message; config file missing → use defaults, mark as "unsaved" |

### PD-CONFIG-AF-002: Navigate config sections

| Attribute | Value |
|-----------|-------|
| **Trigger** | `↑`/`↓` or `j`/`k` while section list has focus |
| **Expected behaviour** | Selection cursor moves between sections; expanding a section reveals its keys |
| **Edge cases** | Empty section (no keys) → section header shows "(empty)"; single key → section auto-expands on selection |

### PD-CONFIG-AF-003: Edit a config key

| Attribute | Value |
|-----------|-------|
| **Trigger** | `↵` (Enter) on a selected key row |
| **Expected behaviour** | Enters edit mode for that key; the value editor matches the key's type (text, dropdown, toggle, color picker) |
| **Edge cases** | Read-only keys → "read-only" indicator, `↵` is a no-op; restart-required keys → edit proceeds, but status shows "requires restart" |

### PD-CONFIG-AF-004: Validate host fields before acceptance

| Attribute | Value |
|-----------|-------|
| **Trigger** | User confirms a host value (e.g., `[agentx.ollama].host`) |
| **Expected behaviour** | Surface sends `POST /test/host` with the proposed host to the orchestrator; orchestrator probes the endpoint (GET /health for Ollama, GET /v1/models for llama.cpp); if 200 OK, accepts the change; otherwise shows error |
| **Edge cases** | Network timeout → "host unreachable, please check the address"; non-200 status → shows the actual status code and response; malformed URL → client-side validation rejects before probe |

### PD-CONFIG-AF-005: Populate model dropdown from provider API

| Attribute | Value |
|-----------|-------|
| **Trigger** | User selects `[agentx.ollama].model` or `[agentx.llamacpp].model` for editing |
| **Expected behaviour** | Surface sends `GET /provider/{name}/models` to the orchestrator; orchestrator probes the provider's API (`GET /api/tags` for Ollama, `GET /v1/models` for llama.cpp); dropdown is populated with available models |
| **Edge cases** | Provider unreachable → dropdown shows "unreachable" with the configured host; empty model list → dropdown shows "(no models available)"; user types a model name not in the dropdown → accepted if it passes the host test (AF-004) |

### PD-CONFIG-AF-006: Type-appropriate validation

| Attribute | Value |
|-----------|-------|
| **Trigger** | User confirms any value edit |
| **Expected behaviour** | Surface validates the value against the key's type rules before sending to the orchestrator |
| **Validation rules** | - **int**: must be a valid integer; leading/trailing whitespace trimmed; negative values rejected unless the key allows them<br>- **float**: not used in current config (all numeric keys are int); reserved for future use<br>- **string**: non-empty after trimming; host fields additionally tested against live endpoint (AF-004)<br>- **bool**: toggle only (no free-text)<br>- **enum**: dropdown selection only<br>- **color**: name (from named palette), ANSI 256 index (0-255), or hex (`#RRGGBB`) |
| **Edge cases** | Empty string on a non-empty-required field → inline error; out-of-range integer → inline error with min/max; invalid hex color → inline error |

### PD-CONFIG-AF-007: Auto-save and apply changes

| Attribute | Value |
|-----------|-------|
| **Trigger** | **Auto-save always ON** — changes are applied to the orchestrator immediately on edit; `s` (Save) is a no-op alias for auto-save |
| **Edge cases** | Write failure → error message, config remains in TUI unsaved; orchestrator rejects → error message, TUI reverts to last known-good state |

### PD-CONFIG-AF-008: Detect external file changes

| Attribute | Value |
|-----------|-------|
| **Trigger** | Filesystem watch detects a modification to `agentx.toml` |
| **Expected behaviour** | Orchestrator sends a `config_changed` event to all attached surfaces; surface reloads the file, highlights changed keys (yellow background), and prompts the user to reload or discard |
| **Edge cases** | User has unsaved changes in TUI → prompts "File changed externally. Discard TUI changes and reload, or keep TUI changes?"; rapid successive changes → debounced (100ms) to avoid flicker |

### PD-CONFIG-AF-009: Confirm restart for restart-required changes

| Attribute | Value |
|-----------|-------|
| **Trigger** | User saves changes that include restart-required keys |
| **Expected behaviour** | Surface shows a confirmation dialog: "The following changes require a restart: [list]. Restart now?" with options "Restart now", "Restart later", "Discard changes" |
| **Edge cases** | User selects "Restart later" → changes are persisted but not applied; a 🔁 indicator appears next to the changed keys until restart; user selects "Discard changes" → TUI reverts to last saved state |

### PD-CONFIG-AF-010: Complex change handling (context size, etc.)

| Attribute | Value |
|-----------|-------|
| **Trigger** | User attempts to shrink the context window (future key, not currently in config) or perform other non-trivial changes |
| **Expected behaviour** | Surface shows a warning: "Shrinking the context window may discard pending context. Continue?" with details about what will be affected |
| **Edge cases** | User confirms → change proceeds; user cancels → change is reverted |

### PD-CONFIG-AF-011: Help and documentation

| Attribute | Value |
|-----------|-------|
| **Trigger** | `?` (help key) or hover on a key (if mouse is enabled) |
| **Expected behaviour** | Surface shows a help overlay with documentation for the selected key, including its purpose, valid values, and whether it requires restart |
| **Edge cases** | No documentation for a key → shows "(no documentation)" |

### PD-CONFIG-AF-012: Quit and cleanup

| Attribute | Value |
|-----------|-------|
| **Trigger** | `q` or `Ctrl-C` |
| **Expected behaviour** | If there are unsaved changes, prompts "Unsaved changes. Quit without saving?"; otherwise exits cleanly, POSTs `/surface/{id}/shutdown` |
| **Edge cases** | User confirms quit with unsaved changes → exits, changes are lost; user cancels quit → returns to surface |

---

## Gherkin Use-Cases

### Scenario: Launch config surface with defaults `[PD-CONFIG-AF-001]`

```
GIVEN no running orchestrator
WHEN  the user runs `agentx surface launch config`
THEN  the surface displays an error: "No running AgentX session found. Start AgentX first."
AND   the process exits with a non-zero status
```

### Scenario: Launch config surface and render config tree `[PD-CONFIG-AF-001, AF-002]`

```
GIVEN a running AgentX orchestrator
AND   `agentx.toml` exists with default values
WHEN  the user runs `agentx surface launch config`
THEN  the surface attaches to the orchestrator
AND   the surface renders a tree of config sections ([agentx.ollama], [agentx.llamacpp], etc.)
AND   the first section is selected by default
AND   the status bar shows "loaded"
```

### Scenario: Navigate config sections `[PD-CONFIG-AF-002]`

```
GIVEN a config surface with sections [agentx.ollama], [agentx.llamacpp], [agentx.classification], ...
WHEN  the user presses `↓`
THEN  the selection moves to the next section
WHEN  the user presses `↑`
THEN  the selection moves to the previous section
WHEN  the user presses `↵` on a collapsed section
THEN  the section expands to show its keys
WHEN  the user presses `↵` on an expanded section
THEN  the section collapses
```

### Scenario: Edit an integer key `[PD-CONFIG-AF-003, AF-006]`

```
GIVEN a config surface with `[agentx.classification].retries = 2` selected
WHEN  the user presses `↵`
THEN  the key enters edit mode with the current value ("2") highlighted
WHEN  the user types "5"
THEN  the value changes to "5"
WHEN  the user presses `↵` to confirm
THEN  the value is validated as an integer ≥ 0
AND   the orchestrator applies the change immediately
AND   the status bar shows "saved"
```

### Scenario: Reject invalid integer input `[PD-CONFIG-AF-006]`

```
GIVEN a config surface with `[agentx.classification].retries` in edit mode
WHEN  the user types "abc"
AND   presses `↵` to confirm
THEN  the input is rejected with an inline error: "must be an integer"
AND   the value remains "2" (the previous valid value)
```

### Scenario: Test host before accepting `[PD-CONFIG-AF-004]`

```
GIVEN a config surface with `[agentx.ollama].host` in edit mode
AND   the user types "localhost:11434"
WHEN  the user presses `↵` to confirm
THEN  the surface sends `POST /test/host` with the proposed host
AND   the orchestrator probes the endpoint
IF    the endpoint returns 200 OK
THEN  the change is accepted
AND   the status bar shows "saved"
ELSE  the status bar shows "host unreachable"
AND   the value reverts to the previous value
```

### Scenario: Populate model dropdown from provider `[PD-CONFIG-AF-005]`

```
GIVEN a config surface with `[agentx.ollama].model` selected
WHEN  the user presses `↵` to edit
THEN  a dropdown appears populated with models from `GET /api/tags` on the configured host
WHEN  the user selects "nemotron-cascade-2:latest" from the dropdown
AND   presses `↵` to confirm
THEN  the surface sends `POST /test/host` to verify the model is available
AND   if the model is available, the change is accepted
```

### Scenario: Save changes and apply live `[PD-CONFIG-AF-007]`

```
GIVEN a config surface with an unsaved change to `[agentx.theme].active_border_color`
WHEN  the user presses `s` (Save)
THEN  the surface serializes the config to TOML
AND   writes to `agentx.toml`
AND   sends `POST /config` to the orchestrator
AND   the orchestrator applies the change immediately
AND   the status bar shows "saved"
AND   the unsaved indicator clears
```

### Scenario: Restart-required change requires confirmation `[PD-CONFIG-AF-009]`

```
GIVEN a config surface with an unsaved change to `[agentx].provider` (switching from "ollama" to "llamacpp")
WHEN  the user presses `s` (Save)
THEN  a confirmation dialog appears: "The following changes require a restart: provider. Restart now?"
WHEN  the user selects "Restart now"
THEN  the orchestrator restarts
AND   the surface reattaches after restart
AND   the new provider is active
```

### Scenario: Detect external file change `[PD-CONFIG-AF-008]`

```
GIVEN a config surface is running
WHEN  the user edits `agentx.toml` in an external editor
THEN  the surface receives a `config_changed` event from the orchestrator
AND   the changed keys are highlighted (yellow background)
AND   a prompt appears: "File changed externally. Reload?"
WHEN  the user selects "Reload"
THEN  the surface reloads the file
AND   the TUI state matches the file
```

### Scenario: Quit with unsaved changes `[PD-CONFIG-AF-012]`

```
GIVEN a config surface with an unsaved change
WHEN  the user presses `q`
THEN  a confirmation dialog appears: "Unsaved changes. Quit without saving?"
WHEN  the user selects "Quit"
THEN  the surface exits
AND   the changes are lost
```

---

## Transport Contract

New endpoints required for the config surface:

### `GET /config`

Returns the current effective configuration as JSON.

**Response:**

```json
{
  "agentx": {
    "provider": "ollama",
    "ollama": {
      "host": "localhost:11434",
      "model": "nemotron-cascade-2:latest"
    },
    "llamacpp": {
      "host": "",
      "model": ""
    },
    "classification": {
      "retries": 2,
      "clarification_options": 3
    },
    "output": {
      "max_widget_lines": 20,
      "input_max_lines": 8,
      "markdown_renderer": "native"
    },
    "thinking": {
      "enabled": true,
      "time_budget_seconds": 180,
      "routes": {
        "respond_directly": false,
        "single_tool": true,
        "invoke_planner": true
      }
    },
    "theme": {
      "active_border_color": "cyan",
      "inactive_border_color": "dark gray"
    },
    "tools": {
      "enabled": true,
      "read_only": true,
      "timeout_seconds": 30,
      "output_max_bytes": 65536,
      "absolute_max_bytes": 2097152
    },
    "transport": {
      "enabled": true,
      "host": "127.0.0.1",
      "port_start": 8420,
      "port_end": 8460
    },
    "wavefront": {
      "enabled": false
    }
  }
}
```

### `POST /config`

Accepts a full config JSON payload, validates it, applies live-reloadable keys immediately, writes to `agentx.toml` via a transactional write with semaphore lock, and queues restart-required keys.

**Request body:** Same structure as `GET /config` response.

**Response:**

```json
{
  "status": "applied",
  "live_applied": ["agentx.theme.active_border_color"],
  "restart_required": ["agentx.provider"],
  "errors": [],
  "normalized_keys": ["chat_backend" → "provider"],
  "write": {
    "path": "~/.config/agentx/agentx.toml",
    "semaphore": "~/.cache/agentx/config.lock",
    "temp_file": "~/.cache/agentx/config_1712345678.tmp"
  }
}
```

- `live_applied`: list of keys that were applied immediately
- `restart_required`: list of keys that require restart
- `errors`: list of validation errors (if any)
- `normalized_keys`: list of keys that were normalized (e.g., `chat_backend` → `provider`)
- `write`: metadata about the write (path, semaphore, temp file) for debugging

**Transactional write process:**

1. Acquire `~/.cache/agentx/config.lock` (timeout: 5s).
2. Write the new config to `~/.cache/agentx/config_<timestamp>.tmp`.
3. Atomically rename the temp file to `~/.config/agentx/agentx.toml` (or project-root `agentx.toml`).
4. Release the semaphore.
5. Apply live-reloadable keys to the running orchestrator state.
6. Queue restart-required keys for the next restart.

If the semaphore is held by another writer, the orchestrator waits (with timeout) or returns `409 Conflict: config is being edited by another process`.

### `GET /provider/{name}/models`

Lists available models for a provider. `name` is "ollama" or "llamacpp".

**Response:**

```json
{
  "provider": "ollama",
  "models": [
    "nemotron-cascade-2:latest",
    "phi4:latest",
    "llama3.1:8b"
  ]
}
```

### `POST /test/host`

Tests a host endpoint. The body specifies the provider and host.

**Request body:**

```json
{
  "provider": "ollama",
  "host": "localhost:11434"
}
```

**Response (200 OK):**

```json
{
  "status": "ok",
  "provider": "ollama",
  "host": "localhost:11434",
  "models": ["nemotron-cascade-2:latest", ...]
}
```

**Response (502 Bad Gateway):**

```json
{
  "error": "host unreachable",
  "category": "validation"
}
```

### `GET /config/schema`

Returns the config schema with validation rules, types, and metadata. Used by the TUI to render appropriate editors.

**Response:**

```json
{
  "sections": [
    {
      "name": "agentx.ollama",
      "keys": [
        {
          "name": "host",
          "type": "host",
          "description": "Ollama API host:port",
          "restart_required": true,
          "validate": {
            "type": "endpoint",
            "provider": "ollama"
          }
        },
        {
          "name": "model",
          "type": "model",
          "description": "Ollama model name",
          "restart_required": true,
          "validate": {
            "type": "dropdown",
            "provider": "ollama"
          }
        }
      ]
    }
  ]
}
```

---

## Two-Way Sync Mechanism

### File → TUI (external changes)

1. **Filesystem watch in orchestrator.** The orchestrator watches `agentx.toml` for modifications (using `fsnotify` or polling at 1s intervals).
2. **Event on change.** When the file changes, the orchestrator publishes a `config_changed` event to the config surface's event stream.
3. **Surface reloads.** The config surface receives the event, reloads `agentx.toml` via `GET /config`, and diffs against the current TUI state.
4. **Highlight changes.** Changed keys are highlighted (yellow background). Unsaved TUI changes trigger a prompt: "File changed externally. Discard TUI changes and reload, or keep TUI changes?"

### TUI → File (user edits)

1. **User edits a key.** The TUI updates the in-memory config state.
2. **Auto-save triggers.** (Auto-save is ON by default. The surface saves changes to the orchestrator immediately on edit.)
3. **Serialize and POST.** The TUI serializes the config to TOML and sends `POST /config` to the orchestrator.
4. **Orchestrator validates.** The orchestrator validates the config (type-appropriate validation, host testing, etc.).
5. **Orchestrator acquires semaphore.** The orchestrator acquires `~/.cache/agentx/config.lock` (timeout: 5s).
6. **Transactional write.** The orchestrator writes to `~/.cache/agentx/config_<timestamp>.tmp`, then atomically renames to `~/.config/agentx/agentx.toml` (or project-root `agentx.toml`).
7. **Apply live-reloadable keys.** The orchestrator applies changes that don't require restart immediately to the running session.
8. **Queue restart-required keys.** The orchestrator queues changes that require restart for the next restart.
9. **Release semaphore.** The orchestrator releases the lock.
10. **Response.** The orchestrator responds with `{"status": "applied", ...}`. The TUI updates the status bar.

### Edge case: conflicting changes

If the user edits a key in the TUI and the file changes externally before the TUI saves, the orchestrator resolves the conflict by keeping the TUI changes (the TUI is the "active" edit). The file change is noted in the status bar: "External change discarded (TUI changes take precedence)."

### Edge case: semaphore contention

If another process holds `~/.cache/agentx/config.lock` when the orchestrator tries to acquire it, the orchestrator waits (timeout: 5s). If the timeout expires, the orchestrator returns `409 Conflict: config is being edited by another process`. The TUI shows this error to the user and retries after a delay.

### Edge case: orchestrator crash during write

If the orchestrator crashes during the transactional write (after writing to the temp file but before atomic rename), the temp file is left in `~/.cache/agentx/`. The orchestrator's startup sequence should clean up any stale temp files. The semaphore file is also cleaned up on startup.

### Normalization

The `chat_backend` key is deprecated. The orchestrator normalizes `chat_backend` to `provider` on save. The response includes a `normalized_keys` list showing which keys were normalized. The surface displays a warning if the user is editing the deprecated key.

---

## Provider/Model Dropdown Behavior

### Ollama

1. **User selects `[agentx.ollama].model` for editing.**
2. **TUI sends `GET /provider/ollama/models`.**
3. **Orchestrator calls `GET http://{configured_host}/api/tags`.**
4. **Orchestrator returns the model list.**
5. **TUI populates the dropdown.**
6. **User selects a model (or types a custom value).**
7. **TUI sends `POST /test/host` with the proposed host and model.**
8. **Orchestrator calls `GET http://{host}/api/tags` and checks if the model is in the list.**
9. **If yes, accepts the change. If no, rejects with "model not available."**

### llama.cpp

1. **User selects `[agentx.llamacpp].model` for editing.**
2. **TUI sends `GET /provider/llamacpp/models`.**
3. **Orchestrator calls `GET http://{configured_host}/v1/models`.**
4. **Orchestrator returns the model list.**
5. **TUI populates the dropdown.**
6. **User selects a model (or types a custom value).**
7. **TUI sends `POST /test/host` with the proposed host and model.**
8. **Orchestrator calls `GET http://{host}/v1/models/{model}`.**
9. **If 200 OK, accepts the change. If 404, rejects with "model not available."**

### Compatibility

AgentX does not filter models by VRAM, context length, or capability. If the server hosts a model, it is compatible. The provider API is the source of truth.

### Edge cases

- **Provider unreachable:** Dropdown shows "(unreachable)" with a retry button. The user can retry or proceed with a custom model name (which will be tested).
- **Empty model list:** Dropdown shows "(no models available)". The user can still type a custom model name.
- **Custom model name:** If the user types a model name not in the dropdown, the TUI still tests it against the provider API (via `POST /test/host`). If the test passes, the change is accepted.

---

## Type-Appropriate Validation

### Integer fields

- **Validation:** Must be a valid integer. Leading/trailing whitespace is trimmed.
- **Range:** Each key has a min/max (see Config Key Taxonomy). Out-of-range values are rejected with an inline error.
- **Editor:** Text input with numeric keyboard on mobile (if applicable). Arrow keys increment/decrement by 1.

### String fields

- **Validation:** Non-empty after trimming. Host fields additionally tested against the live endpoint (AF-004).
- **Editor:** Text input.

### Boolean fields

- **Validation:** Toggle only (no free-text).
- **Editor:** Toggle switch (on/off).

### Enum fields

- **Validation:** Dropdown selection only.
- **Editor:** Dropdown with the allowed values.

### Color fields

- **Validation:** Must be a valid color name (from the named palette), ANSI 256 index (0-255), or hex (`#RRGGBB`).
- **Editor:** **Visual color picker** (required). The picker accepts CSS name ("cyan"), ANSI 256 index ("240"), or hex ("#00afaf") as fallback text input.

### Host fields

- **Validation:** Must be a valid URL/hostname:port. Tested against the live endpoint (AF-004).
- **Editor:** Text input with a "Test" button that triggers the probe.

### Model fields

- **Validation:** Must be a model name available on the provider. Populated from the provider API (AF-005).
- **Editor:** Dropdown (populated) with an option to "Add custom model" (which triggers the host test).

---

## Complex Change Handling

### Context size (future key)

Shrinking the context window is non-trivial because it may discard pending context. The surface handles this as follows:

1. **User attempts to shrink context size.**
2. **Surface shows a warning:** "Shrinking the context window may discard pending context. Current size: {old} → New size: {new}. Continue?"
3. **User confirms:** Change proceeds.
4. **User cancels:** Change is reverted.

### Provider switch

Switching the provider (e.g., from "ollama" to "llamacpp") requires restarting the orchestrator because the model adapter must be recreated. The surface handles this as follows:

1. **User changes `[agentx].provider`.**
2. **Surface marks the key with a 🔁 icon.**
3. **User saves.**
4. **Surface shows a confirmation dialog:** "Switching the provider requires a restart. Restart now?"
5. **User confirms:** Orchestrator restarts. The surface reattaches after restart.
6. **User declines:** Changes are persisted but not applied. A 🔁 indicator remains until restart.

---

## Test Mapping

| Affordance ID | Feature file | Scenario | Step file / Go test | Status |
|---------------|--------------|----------|----------------------|--------|
| PD-CONFIG-AF-001 | `tests/features/surfaces/config_surface.feature` | Launch and attach | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-002 | `tests/features/surfaces/config_surface.feature` | Navigate sections | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-003 | `tests/features/surfaces/config_surface.feature` | Edit a key | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-004 | `tests/features/surfaces/config_surface.feature` | Test host before accepting | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-005 | `tests/features/surfaces/config_surface.feature` | Populate model dropdown | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-006 | `tests/features/surfaces/config_surface.feature` | Type-appropriate validation | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-007 | `tests/features/surfaces/config_surface.feature` | Save and apply changes | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-008 | `tests/features/surfaces/config_surface.feature` | Detect external file changes | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-009 | `tests/features/surfaces/config_surface.feature` | Confirm restart | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-010 | `tests/features/surfaces/config_surface.feature` | Complex change handling | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-011 | `tests/features/surfaces/config_surface.feature` | Help and documentation | `tests/steps/surfaces/config_steps.go` | Planned |
| PD-CONFIG-AF-012 | `tests/features/surfaces/config_surface.feature` | Quit and cleanup | `tests/steps/surfaces/config_steps.go` | Planned |

---

## Implementation Plan

### Phase 1: Transport endpoints (L)

- [ ] Add `GET /config` to the transport server.
- [ ] Add `POST /config` to the transport server.
- [ ] Add `GET /provider/{name}/models` to the transport server.
- [ ] Add `POST /test/host` to the transport server.
- [ ] Add `GET /config/schema` to the transport server.
- [ ] Extend the `Provider` interface with `Config()`, `SetConfig()`, `ListModels()`, `TestHost()`, `ConfigSchema()`.
- [ ] Implement the orchestrator's transactional write with semaphore lock.
- [ ] Implement the orchestrator's config normalization (`chat_backend` → `provider`).
- [ ] Implement the orchestrator's config validation (type-appropriate, host testing, etc.).
- [ ] Implement the orchestrator's live reload for tunable keys.
- [ ] Implement the orchestrator's restart-required key queuing.
- [ ] Write Godog tests for the transport endpoints.

### Phase 2: Surface framework (M)

- [ ] Create `internal/surfaces/config/` package.
- [ ] Implement `ConfigModel` (SurfaceModel interface).
- [ ] Implement the tree navigation (SectionTree, KeyList, KeyRow).
- [ ] Implement the value editors (text, dropdown, toggle, **visual color picker**).
- [ ] Implement the status bar and hint row.
- [ ] Implement the dialog overlay (confirm restart, error, model picker).
- [ ] Wire up the transport client (config, provider, test-host endpoints).
- [ ] Wire up auto-save (**always ON**, no user option to disable).
- [ ] Wire up the surface in `internal/cli/surface_launch.go`.
- [ ] Write Godog tests for the surface.

### Phase 3: Two-way sync (M)

- [ ] Implement filesystem watch in the orchestrator.
- [ ] Implement `config_changed` event publishing.
- [ ] Implement the surface's file-change detection and diff highlighting.
- [ ] Implement the conflict resolution (TUI changes take precedence).
- [ ] Write Godog tests for the two-way sync.

### Phase 4: Live reload and restart (M)

- [ ] Implement live reload for tunable keys in the orchestrator.
- [ ] Implement restart-required key queuing in the orchestrator.
- [ ] Implement the restart flow (surface prompts user, orchestrator restarts, surface reattaches).
- [ ] Write Godog tests for the live reload and restart flow.

### Phase 5: Documentation and polish (S)

- [ ] Write the config schema endpoint response.
- [ ] Add help documentation for each config key.
- [ ] Add color picker for color fields (optional, future enhancement).
- [ ] Add auto-save option (configurable).
- [ ] Update `docs/ux/03_PANEL_DETAILS.md` with the PD-CONFIG section.
- [ ] Update `docs/ux/UX_LIFECYCLE.md` with the PD-CONFIG traceability row.

---

## Disambiguations

The following 10 open questions from earlier drafts have been resolved:

1. **Should the config surface support editing prompt files inline?** → **No.** Managed separately via "open in editor" affordance.
2. **Should the config surface support managing tool blacklists and approvals?** → **No.** Out of scope (future command policy surface).
3. **Should the config surface support managing the zellij layout (`agentx.kdl`)?** → **No.** Out of scope.
4. **Should the config surface support managing the `agentx_tools.toml` file?** → **No.** Out of scope at this time.
5. **Should the config surface support managing the `prompts.toml` file?** → **No.** Out of scope.
6. **Should the config surface support managing the `agentx-tool-approvals.toml` file?** → **No.** Out of scope.
7. **Should the config surface support managing the `agentx-tool-blacklist.toml` file?** → **No.** Out of scope.
8. **Should the config surface support managing the `agentx-continuation-verbs-allowed.md` file?** → **No.** Out of scope.
9. **Should the config surface support managing the `agentx-continuation-verbs-denied.md` file?** → **No.** Out of scope.
10. **Should the config surface support managing the `agentx-tool-output-overrides.toml` file?** → **No.** Out of scope.

## Design Decisions

1. **Compatibility:** AgentX does not filter models by VRAM or capability. If the server hosts a model, it is compatible. The provider API is the source of truth.
2. **Auto-save:** Always ON. No user option to disable. Changes are applied to the orchestrator immediately on edit. The status bar shows "auto-saved" after each successful write.
3. **Color picker:** A visual color picker is required as the primary editor for color fields. It accepts CSS name ("cyan"), ANSI 256 index ("240"), or hex ("#00afaf") as fallback text input.
4. **Race condition prevention:** The orchestrator uses a semaphore file (`~/.cache/agentx/config.lock`) to prevent concurrent writes to `agentx.toml`. Both the surface and the orchestrator acquire the semaphore before writing. If the semaphore is held, the writer waits (with a timeout) or aborts with a "config is being edited" error.
5. **Transactional writes:** The orchestrator writes to a temporary file (`~/.cache/agentx/config_<timestamp>.tmp`) first, then atomically renames it to `~/.config/agentx/agentx.toml` (or the project-root `agentx.toml`). This prevents partial writes if the orchestrator crashes during save.
6. **`chat_backend` normalization:** The `chat_backend` key is deprecated. The surface normalizes to `provider` on save. If the user is editing the deprecated key, the surface displays a warning: "`chat_backend` is deprecated. Use `provider` instead."
7. **Provider model selector:** The dropdown shows all models hosted on the provider server. AgentX does not filter by VRAM, context length, or capability. The user is responsible for selecting a compatible model.
8. **Live reload mechanism:** The orchestrator accepts the full config JSON from the surface, validates it, applies live-reloadable keys immediately, and queues restart-required keys for the next restart. The orchestrator writes to a temp file, acquires the semaphore, and atomically renames to `agentx.toml`.
9. **Conflict resolution:** The surface detects external file changes via the orchestrator's filesystem watch. If the user has unsaved TUI changes, the orchestrator keeps the TUI changes (the TUI is the "active" edit). The file change is noted in the status bar.
10. **Prompt files:** Prompt files are managed separately via "open in editor" affordance. The surface displays them as file paths with an "open" button.

## Deferred (later slices)

- Inline editing of prompt files (PD-07-style)
- Managing tool blacklists/approvals (command policy surface)
- Managing the zellij layout (`agentx.kdl`)
- Auto-save configuration toggle (not an option — auto-save is always ON)
