#!/usr/bin/env python3
"""
AgentX Python Applet Template.

Each applet runs in a dedicated tmux pane and communicates with the Go core via IPC (FIFOs, env vars).
This template demonstrates the minimal structure for a TUI applet.

Applet Lifecycle:
1. Applet starts in tmux pane (launched by Go core)
2. Applet reads env vars: AGENTX_APPLET_NAME, AGENTX_SESSION_ID, AGENTX_IPC_INPUT, AGENTX_IPC_OUTPUT
3. Applet sends READY signal on startup: print("READY applet=<name>")
4. Applet listens to Go core on input FIFO
5. Applet responds on output FIFO
6. Applet receives shutdown signal via ctx (or FIFO close)
7. Applet exits cleanly
"""

import argparse
import json
import os
import signal
import sys
import time
import tomllib
import urllib.error
import urllib.request

CORE_BASE_URL = os.getenv("AGENTX_CORE_HTTP", "http://127.0.0.1:9876").rstrip("/")

# Applet metadata
APPLET_NAME = os.getenv("AGENTX_APPLET_NAME", "template")
SESSION_ID = os.getenv("AGENTX_SESSION_ID", "unknown")
IPC_INPUT = os.getenv("AGENTX_IPC_INPUT", None)
IPC_OUTPUT = os.getenv("AGENTX_IPC_OUTPUT", None)
CHAT_BACKEND = os.getenv("AGENTX_CHAT_BACKEND", "echo").strip().lower()
OLLAMA_HOST = os.getenv("AGENTX_OLLAMA_HOST", "localhost:11434").strip()
OLLAMA_MODEL = os.getenv("AGENTX_OLLAMA_MODEL", "llama3.2").strip()
OLLAMA_TIMEOUT_SEC = float(os.getenv("AGENTX_OLLAMA_TIMEOUT_SEC", "30"))
PROJECT_DIR = os.getenv("AGENTX_PROJECT_DIR", os.getcwd()).strip() or os.getcwd()
USERNAME = os.getenv("AGENTX_USERNAME", os.getenv("USER", "agentx")).strip() or "agentx"

# Global shutdown flag
shutdown_requested = False

_ANSI_RESET = "\033[0m"

_CONTEXT_BANDS = (
    {"key": "working_memory", "label": "Working Memory", "emoji": "💾", "ansi": "96"},
    {"key": "system", "label": "System", "emoji": "🧠", "ansi": "36"},
    {"key": "user", "label": "User", "emoji": "👤", "ansi": "34"},
    {"key": "attachments", "label": "Attachments", "emoji": "📎", "ansi": "93"},
    {"key": "thinking", "label": "Thinking", "emoji": "🤔", "ansi": "35"},
    {"key": "assistant", "label": "Agent", "emoji": "🤖", "ansi": "32"},
    {"key": "tool", "label": "Tools", "emoji": "🔧", "ansi": "33"},
)

_SYSTEM_TAB_ALIASES = {
    "full": "full",
    "all": "full",
    "files": "files",
    "configuration": "configuration",
    "config": "configuration",
    "context": "context",
    "context-history": "context-history",
    "context_history": "context-history",
    "history": "context-history",
    "context-visualizer": "context-visualizer",
    "context_visualizer": "context-visualizer",
    "visualizer": "context-visualizer",
}


def signal_handler(signum, frame):
    """Handle SIGTERM and SIGINT for graceful shutdown."""
    global shutdown_requested
    shutdown_requested = True
    print(f"\n[{APPLET_NAME}] Received signal {signum}, shutting down...", file=sys.stderr)


def print_ready():
    """Signal to Go core that applet is ready."""
    ready_msg = json.dumps(
        {"type": "ready", "applet": APPLET_NAME, "session": SESSION_ID, "timestamp": int(time.time() * 1000)}
    )
    print(f"READY {ready_msg}")
    sys.stdout.flush()


def print_output(msg: str, level: str = "info"):
    """Write output message (displayed in tmux pane)."""
    timestamp = time.strftime("%H:%M:%S")
    emoji = {"info": "ℹ️", "warn": "⚠️", "error": "❌", "ok": "✅"}.get(level, "•")
    print(f"[{timestamp}] {emoji} {msg}")
    sys.stdout.flush()


def print_ui_line(msg: str):
    """Write a plain user-facing line in interactive pane modes."""
    print(msg)
    sys.stdout.flush()


def clear_visible_screen():
    """Clear pane viewport so startup command noise is not kept on screen."""
    print("\033[2J\033[H", end="")
    sys.stdout.flush()


def _get_json(path: str):
    """Fetch JSON payload from a core endpoint path."""
    url = f"{CORE_BASE_URL}{path}"
    request = urllib.request.Request(url=url, method="GET")
    with urllib.request.urlopen(request, timeout=OLLAMA_TIMEOUT_SEC) as response:
        body = response.read().decode("utf-8")
    return json.loads(body)


def _submit_prompt(prompt: str) -> str:
    """Submit prompt to core and return routed response text."""
    url = f"{CORE_BASE_URL}/submit"
    payload = json.dumps({"prompt": prompt}).encode("utf-8")
    request = urllib.request.Request(
        url=url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=_resolve_submit_timeout_sec()) as response:
        body = response.read().decode("utf-8")
    decoded = json.loads(body)
    routed = str(decoded.get("response", "")).strip()
    if not routed:
        raise ValueError("submit endpoint returned empty response")
    return routed


def _request_shutdown() -> None:
    """Request graceful runtime shutdown from the core."""
    url = f"{CORE_BASE_URL}/shutdown"
    request = urllib.request.Request(url=url, data=b"{}", headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(request, timeout=OLLAMA_TIMEOUT_SEC) as response:
        body = response.read().decode("utf-8")
    decoded = json.loads(body)
    if str(decoded.get("status", "")).strip() != "shutting_down":
        raise ValueError("shutdown endpoint returned unexpected payload")


def _load_runtime_config() -> dict:
    """Load AgentX runtime configuration from agentx.toml when available."""
    config_path = os.path.join(PROJECT_DIR, "agentx.toml")
    if not os.path.exists(config_path):
        return {}
    with open(config_path, "rb") as handle:
        return tomllib.load(handle)


def _resolve_submit_timeout_sec() -> float:
    """Resolve submit timeout from config first, then env override."""
    timeout_sec = 120.0
    config = _load_runtime_config()
    agentx_cfg = config.get("agentx", {}) if isinstance(config.get("agentx"), dict) else {}
    configured = agentx_cfg.get("submit_timeout_seconds")
    if isinstance(configured, (int, float)) and configured > 0:
        timeout_sec = float(configured)

    env_value = os.getenv("AGENTX_SUBMIT_TIMEOUT_SEC", "").strip()
    if env_value:
        try:
            parsed = float(env_value)
            if parsed > 0:
                timeout_sec = parsed
        except ValueError:
            pass

    return timeout_sec


def _load_bootstrap_prompt() -> str | None:
    """Load optional startup bootstrap prompt from project-local .agentx path."""
    prompt_path = os.path.join(PROJECT_DIR, ".agentx", "bootstrap-prompt.md")
    if not os.path.isfile(prompt_path):
        return None
    try:
        with open(prompt_path, "r", encoding="utf-8") as handle:
            prompt = handle.read().strip()
    except OSError:
        return None
    return prompt or None


def _load_agentx_instructions() -> str | None:
    """Load optional AgentX identity instructions from project-local .agentx path."""
    instructions_path = os.path.join(PROJECT_DIR, ".agentx", "agentx-instructions.md")
    if not os.path.isfile(instructions_path):
        return None
    try:
        with open(instructions_path, "r", encoding="utf-8") as handle:
            instructions = handle.read().strip()
    except OSError:
        return None
    return instructions or None


def _build_ollama_messages(prompt: str) -> list[dict[str, str]]:
    """Build chat messages with optional AgentX identity context."""
    messages: list[dict[str, str]] = []
    instructions = _load_agentx_instructions()
    if instructions:
        # Mirror GUI bootstrap behavior: inject AgentX instructions via working-memory system block.
        working_memory_block = f"<working_memory>\n👤 agentx-instructions: {instructions}\n</working_memory>"
        messages.append({"role": "system", "content": working_memory_block})
    messages.append({"role": "user", "content": prompt})
    return messages


def _trim_single_line(value: str, limit: int = 72) -> str:
    """Normalize user-facing values into one stable rendered line."""
    single_line = " ".join(str(value).split())
    if not single_line:
        return "none"
    if len(single_line) <= limit:
        return single_line
    return single_line[: limit - 3].rstrip() + "..."


def _safe_listdir(path: str) -> list[str]:
    """Return a sorted directory listing or an empty list when unavailable."""
    try:
        return sorted(os.listdir(path))
    except OSError:
        return []


def _classify_entry(path: str) -> str:
    """Return a short entry type label for system pane previews."""
    if os.path.isdir(path):
        return "dir"
    if os.path.isfile(path):
        return "file"
    return "other"


def _context_visualizer(turns: list[dict]) -> dict[str, int]:
    """Build a small deterministic token-style summary from current turns."""
    user_tokens = sum(max(1, len(str(turn.get("prompt", "")).split())) for turn in turns if turn.get("prompt"))
    assistant_tokens = sum(max(1, len(str(turn.get("response", "")).split())) for turn in turns if turn.get("response"))
    return {
        "max_tokens": 0,
        "working_memory": 0,
        "system": 0,
        "user": user_tokens,
        "attachments": 0,
        "thinking": 0,
        "assistant": assistant_tokens,
        "tool": 0,
    }


def _normalize_system_tab(raw_value: str | None) -> str:
    """Normalize user/system tab selector into known tab key."""
    normalized = str(raw_value or "").strip().lower()
    return _SYSTEM_TAB_ALIASES.get(normalized, "")


def _resolve_system_tab_state_path() -> str:
    """Resolve state file path used for system tab synchronization."""
    configured = os.getenv("AGENTX_SYSTEM_PANEL_TAB_STATE_FILE", "").strip()
    if configured:
        return configured
    return os.path.join(PROJECT_DIR, ".agentx", "system-panel-tab.txt")


def _resolve_selected_system_tab() -> str:
    """Resolve active system tab from state file with env/default fallback."""
    state_file = _resolve_system_tab_state_path()
    try:
        with open(state_file, "r", encoding="utf-8") as handle:
            tab = _normalize_system_tab(handle.read())
            if tab:
                return tab
    except OSError:
        pass

    env_tab = _normalize_system_tab(os.getenv("AGENTX_SYSTEM_PANEL_TAB", ""))
    if env_tab:
        return env_tab

    return "full"


def _meter_row(label: str, value: int, max_tokens: int, width: int = 18) -> str:
    """Render one deterministic text meter row for a token band."""
    safe_max = max(max_tokens, 1)
    safe_val = max(value, 0)
    filled = min(width, int((safe_val / safe_max) * width))
    bar = "#" * filled + "." * (width - filled)
    pct = int(round((safe_val / safe_max) * 100))
    return f"{label:<20} [{bar}] {safe_val} ({pct}%)"


def _resolve_context_window_tokens(agentx_cfg: dict, model_name: str, used_tokens: int) -> int:
    """Resolve context window size for percentage computations."""
    for key in ("context_window_tokens", "max_context_tokens", "context_tokens"):
        value = agentx_cfg.get(key)
        if isinstance(value, int) and value > 0:
            return max(value, used_tokens, 1)

    known_model_windows = {
        "qwen3.6:latest": 32768,
        "qwen2.5:latest": 32768,
        "phi4-mini:3.8b": 16384,
        "gpt-oss:latest": 32768,
        "llama3.2": 8192,
    }
    if model_name in known_model_windows:
        return max(known_model_windows[model_name], used_tokens, 1)

    return max(8192, used_tokens, 1)


def _render_context_usage_bar(breakdown: dict[str, int], max_tokens: int, width: int = 36) -> list[str]:
    """Render ANSI-color usage bar plus compact percent summary."""
    safe_max = max(max_tokens, 1)
    band_values = {band["key"]: max(0, int(breakdown.get(band["key"], 0))) for band in _CONTEXT_BANDS}
    used_tokens = sum(band_values.values())
    usage_pct = int(round((used_tokens / safe_max) * 100))

    used_slots = min(width, int(round((used_tokens / safe_max) * width)))
    segments: list[str] = []
    consumed_slots = 0
    if used_tokens > 0 and used_slots > 0:
        for index, band in enumerate(_CONTEXT_BANDS):
            value = band_values[band["key"]]
            if value <= 0:
                continue
            if index == len(_CONTEXT_BANDS) - 1:
                slots = max(0, used_slots - consumed_slots)
            else:
                slots = int(round((value / used_tokens) * used_slots))
            if slots <= 0:
                continue
            consumed_slots += slots
            segments.append(f"\033[{band['ansi']}m{'█' * slots}{_ANSI_RESET}")

    remaining_slots = max(0, width - consumed_slots)
    if remaining_slots:
        segments.append(f"\033[90m{'░' * remaining_slots}{_ANSI_RESET}")

    summary_parts = []
    for band in _CONTEXT_BANDS:
        value = band_values[band["key"]]
        if value <= 0:
            continue
        pct = int(round((value / safe_max) * 100))
        summary_parts.append(f"{band['emoji']} {pct}% {band['label']}")

    summary = " | ".join(summary_parts) if summary_parts else "No context contributors yet."
    return [f"consumed: {usage_pct}% ({used_tokens}/{safe_max})", "".join(segments), summary]


def _render_top_contributors(breakdown: dict[str, int], max_tokens: int, width: int = 16) -> list[str]:
    """Render top context contributors with emoji and bar lengths."""
    safe_max = max(max_tokens, 1)
    ranked = sorted(
        ((band, max(0, int(breakdown.get(band["key"], 0)))) for band in _CONTEXT_BANDS),
        key=lambda pair: pair[1],
        reverse=True,
    )
    top = [(band, tokens) for band, tokens in ranked if tokens > 0][:4]
    if not top:
        return ["Top Contributors:", "  none"]

    lines = ["Top Contributors:"]
    for idx, (band, tokens) in enumerate(top, start=1):
        pct = int(round((tokens / safe_max) * 100))
        slots = max(1, int(round((tokens / safe_max) * width)))
        bar = f"\033[{band['ansi']}m{'█' * slots}{_ANSI_RESET}"
        lines.append(f"  {idx}. {band['emoji']} {band['label']:<16} {bar} {pct}%")
    return lines


def _format_elapsed_ms(elapsed_ms: int) -> str:
    """Format milliseconds as HH:MM:SS.mmm."""
    safe_ms = max(0, int(elapsed_ms))
    total_seconds = safe_ms // 1000
    hours = total_seconds // 3600
    minutes = (total_seconds % 3600) // 60
    seconds = total_seconds % 60
    millis = safe_ms % 1000
    return f"{hours:02d}:{minutes:02d}:{seconds:02d}.{millis:03d}"


def _prompt_cycle_row(label: str, icon: str, phase_payload: dict | None) -> str:
    """Render one prompt-cycle status row from payload data."""
    phase = phase_payload if isinstance(phase_payload, dict) else {}
    state = str(phase.get("state", "pending")).strip().lower() or "pending"
    elapsed_ms = int(phase.get("elapsed_ms", 0) or 0)

    if state == "done":
        marker = "✓"
    elif state == "running":
        marker = "↻"
    elif state == "failed":
        marker = "✗"
    else:
        marker = "○"

    elapsed = _format_elapsed_ms(elapsed_ms) if state in {"done", "running", "failed"} else "--:--:--.---"
    return f"{marker} {icon} {label:<8} {elapsed}"


def _cycle_summary_line(label: str, phase_payload: dict | None) -> str:
    """Render one concise chat-pane summary line for prompt cycle phase."""
    phase = phase_payload if isinstance(phase_payload, dict) else {}
    state = str(phase.get("state", "pending")).strip().lower() or "pending"
    elapsed_ms = int(phase.get("elapsed_ms", 0) or 0)
    elapsed = _format_elapsed_ms(elapsed_ms)
    return f"{state} ({elapsed})"


def _classify_prompt_intent(prompt: str) -> str:
    """Provide a lightweight UX-facing intent summary for chat pane classification rows."""
    text = str(prompt).strip().lower()
    if not text:
        return "respond_directly -> assistant_response"
    if text.startswith(":"):
        return "control_command -> runtime_control"

    planner_terms = (
        "plan",
        "roadmap",
        "migrate",
        "architecture",
        "refactor",
        "multi-step",
    )
    if any(term in text for term in planner_terms):
        return "complex_action -> invoke_planner"

    tool_terms = (
        "read file",
        "search",
        "grep",
        "run test",
        "build",
        "compile",
        "debug",
    )
    if any(term in text for term in tool_terms):
        return "tool_assisted -> invoke_tool"

    return "respond_directly -> assistant_response"


def _derive_activity_state(prompt_cycle: dict | None) -> tuple[str, str]:
    """Derive activity state/phase from prompt cycle data when direct activity is unavailable."""
    cycle = prompt_cycle if isinstance(prompt_cycle, dict) else {}

    def _phase_state(name: str) -> str:
        phase = cycle.get(name)
        if isinstance(phase, dict):
            return str(phase.get("state", "pending")).strip().lower() or "pending"
        return "pending"

    states = {
        "classify": _phase_state("classify"),
        "thinking": _phase_state("thinking"),
        "tool": _phase_state("tool"),
        "respond": _phase_state("respond"),
    }

    for phase_name in ("respond", "tool", "thinking", "classify"):
        if states[phase_name] == "failed":
            return "failed", phase_name
    for phase_name in ("respond", "tool", "thinking", "classify"):
        if states[phase_name] == "running":
            return "working", phase_name
    for phase_name in ("respond", "tool", "thinking", "classify"):
        if states[phase_name] == "done":
            return "completed", phase_name
    return "idle", "none"


def _resolve_activity_state(activity_payload: dict | None, prompt_cycle: dict | None) -> tuple[str, str]:
    """Resolve activity state from /activity payload with prompt-cycle fallback."""
    activity = activity_payload if isinstance(activity_payload, dict) else {}
    state = str(activity.get("state", "")).strip().lower()
    phase = str(activity.get("phase", "")).strip().lower()
    if state:
        return state, phase or "none"
    return _derive_activity_state(prompt_cycle)


def _render_system_surface(payload: dict, selected_tab: str = "full", activity_payload: dict | None = None) -> str:
    """Render a deterministic text analogue of the GUI status context widget."""
    config = _load_runtime_config()
    agentx_cfg = config.get("agentx", {}) if isinstance(config.get("agentx"), dict) else {}
    turns = payload.get("turns", []) or []
    turn_count = int(payload.get("turn_count", len(turns)))
    session_id = _trim_single_line(payload.get("session_id", SESSION_ID), 36)
    visualizer = _context_visualizer(turns)
    activity = activity_payload if isinstance(activity_payload, dict) else {}
    prompt_cycle = activity.get("prompt_cycle") if isinstance(activity.get("prompt_cycle"), dict) else {}
    if not prompt_cycle:
        prompt_cycle = payload.get("prompt_cycle") if isinstance(payload.get("prompt_cycle"), dict) else {}
    activity_state, activity_phase = _resolve_activity_state(activity, prompt_cycle)
    model_name = _trim_single_line(agentx_cfg.get("ollama_model", OLLAMA_MODEL), 24)
    used_tokens = sum(
        int(visualizer.get(key, 0))
        for key in ["working_memory", "system", "user", "attachments", "thinking", "assistant", "tool"]
    )
    safe_max_tokens = _resolve_context_window_tokens(agentx_cfg, model_name, used_tokens)
    remaining = max(0, safe_max_tokens - used_tokens)
    usage_lines = _render_context_usage_bar(visualizer, safe_max_tokens)
    top_lines = _render_top_contributors(visualizer, safe_max_tokens)

    filesystem_entries = _safe_listdir(PROJECT_DIR)
    preview_items = []
    for entry in filesystem_entries[:3]:
        full_path = os.path.join(PROJECT_DIR, entry)
        preview_items.append(f"- {_classify_entry(full_path)}: {_trim_single_line(entry, 36)}")
    if not preview_items:
        preview_items = ["- none"]

    last_prompt = _trim_single_line(turns[-1].get("prompt", ""), 56) if turns else "none"
    last_response = _trim_single_line(turns[-1].get("response", ""), 56) if turns else "none"
    recent_prompt = _trim_single_line(turns[-2].get("prompt", ""), 56) if len(turns) > 1 else "none"

    sections = {
        "files": [
            "== FILES ==",
            f"project_dir: {_trim_single_line(PROJECT_DIR, 64)}",
            f"entry_count: {len(filesystem_entries)}",
            "preview:",
            *preview_items,
        ],
        "configuration": [
            "== CONFIGURATION ==",
            f"model: {model_name}",
            f"backend: {_trim_single_line(CHAT_BACKEND, 24)}",
            f"ollama_host: {_trim_single_line(OLLAMA_HOST, 40)}",
        ],
        "context": [
            "== CONTEXT ==",
            f"session_id: {session_id}",
            f"turn_count: {turn_count}",
            f"last_user: {last_prompt}",
            f"last_agent: {last_response}",
        ],
        "context-history": [
            "== CONTEXT HISTORY ==",
            f"history_context_count: {turn_count}",
            f"recent_prompt: {recent_prompt}",
        ],
        "context-visualizer": [
            "== CONTEXT WINDOW ==",
            f"model: {model_name} | backend: {_trim_single_line(CHAT_BACKEND, 12)}",
            usage_lines[0],
            usage_lines[1],
            usage_lines[2],
            *top_lines,
            "== ACTIVITY ==",
            f"state: {activity_state}",
            f"phase: {activity_phase}",
            "== CONTEXT VISUALIZER ==",
            _meter_row("💾 Working Memory", int(visualizer.get("working_memory", 0)), safe_max_tokens),
            _meter_row("🧠 System Prompts", int(visualizer.get("system", 0)), safe_max_tokens),
            _meter_row("👤 User Prompts", int(visualizer.get("user", 0)), safe_max_tokens),
            _meter_row("📎 Attachments", int(visualizer.get("attachments", 0)), safe_max_tokens),
            _meter_row("🤔 Thinking", int(visualizer.get("thinking", 0)), safe_max_tokens),
            _meter_row("🤖 Agent Response", int(visualizer.get("assistant", 0)), safe_max_tokens),
            _meter_row("🔧 Tool Calls", int(visualizer.get("tool", 0)), safe_max_tokens),
            _meter_row("░ Remaining", remaining, safe_max_tokens),
            "== PROMPT CYCLE ==",
            _prompt_cycle_row("Classify", "🤔", prompt_cycle.get("classify")),
            _prompt_cycle_row("Think", "💭", prompt_cycle.get("thinking")),
            _prompt_cycle_row("Tool", "🔧", prompt_cycle.get("tool")),
            _prompt_cycle_row("Respond", "🤖", prompt_cycle.get("respond")),
        ],
    }

    active_tab = _normalize_system_tab(selected_tab) or "full"
    lines = ["[SYSTEM]", f"[SYSTEM TAB] active={active_tab}"]

    if active_tab == "full":
        ordered_tabs = ["files", "configuration", "context", "context-history", "context-visualizer"]
        for tab_name in ordered_tabs:
            lines.extend(sections[tab_name])
    else:
        lines.extend(sections.get(active_tab, sections["context-visualizer"]))

    lines.append(f"turn_count: {turn_count}")
    return "\n".join(lines)


def run_input_affordance_loop():
    """Run line-based input affordance for interactive tmux input pane."""
    print_ui_line("Input ready. Enter prompt and press Enter.")
    print_ui_line("Commands: :q shuts down the session, :clear clears input panel only.")
    while not shutdown_requested:
        try:
            prompt = input("agentx> ")
        except EOFError:
            time.sleep(0.1)
            continue
        except KeyboardInterrupt:
            print_output("Input interrupt received.", level="warn")
            continue

        prompt = prompt.strip()
        if not prompt:
            continue
        if prompt == ":q":
            try:
                _request_shutdown()
                print_ui_line("Session shutdown requested.")
            except (
                urllib.error.URLError,
                urllib.error.HTTPError,
                TimeoutError,
                ValueError,
                json.JSONDecodeError,
            ) as exc:
                try:
                    _submit_prompt(":q")
                    print_ui_line("Session shutdown requested via submit fallback.")
                except (
                    urllib.error.URLError,
                    urllib.error.HTTPError,
                    TimeoutError,
                    ValueError,
                    json.JSONDecodeError,
                ) as submit_exc:
                    print_ui_line(f"Shutdown failed: {exc}; submit fallback failed: {submit_exc}")
            return 0

        try:
            _submit_prompt(prompt)
            print_ui_line(f"Submitted: {prompt}")
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError) as exc:
            print_ui_line(f"Submit failed: {exc}")

    return 0


def run_chat_affordance_loop():
    """Poll context endpoint and display assistant responses as they arrive."""
    print_ui_line("Chat ready.")
    core_owns_bootstrap = os.getenv("AGENTX_CORE_OWNS_STARTUP_BOOTSTRAP", "").strip().lower() in {
        "1",
        "true",
        "yes",
        "on",
    }
    bootstrap_prompt = None if core_owns_bootstrap else _load_bootstrap_prompt()
    if bootstrap_prompt:
        if CHAT_BACKEND == "ollama":
            try:
                bootstrap_response = _chat_with_ollama(bootstrap_prompt)
                print_ui_line(f"Agent: {bootstrap_response}")
            except (
                urllib.error.URLError,
                urllib.error.HTTPError,
                TimeoutError,
                ValueError,
                json.JSONDecodeError,
            ) as exc:
                print_ui_line("Agent startup check failed: Ollama backend did not return a response.")
                print_ui_line(f"Backend error: {exc}")
        else:
            print_ui_line(
                f"Agent startup check skipped: backend='{CHAT_BACKEND}' (expected 'ollama' for LLM bootstrap response)."
            )
            print_ui_line('Set chat_backend="ollama" in agentx.toml or export AGENTX_CHAT_BACKEND=ollama.')
    last_turn_count = 0
    while not shutdown_requested:
        try:
            payload = _get_json("/context")
            turns = payload.get("turns", [])
            turn_count = int(payload.get("turn_count", len(turns)))
            if turn_count > last_turn_count:
                prompt_cycle = payload.get("prompt_cycle") if isinstance(payload.get("prompt_cycle"), dict) else {}
                new_turns = turns[last_turn_count:turn_count]
                for turn in new_turns:
                    prompt = str(turn.get("prompt", "")).strip()
                    response = str(turn.get("response", "")).strip()
                    if prompt:
                        print_ui_line(f"User: {prompt}")
                        print_ui_line(f"⚙️ Classification: {_classify_prompt_intent(prompt)}")
                    print_ui_line("💭 [thinking block - " + _cycle_summary_line("Think", prompt_cycle.get("thinking")) + "]")
                    if response:
                        print_ui_line(f"Agent: {response}")
                last_turn_count = turn_count
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(0.5)
    return 0


def run_context_affordance_loop():
    """Poll context endpoint and render deterministic system-surface sections."""
    print_ui_line("System panel ready.")
    last_render = ""
    while not shutdown_requested:
        try:
            payload = _get_json("/context")
            activity_payload = None
            try:
                activity_payload = _get_json("/activity")
            except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError):
                mirrored = payload.get("activity")
                if isinstance(mirrored, dict):
                    activity_payload = mirrored
            render = _render_system_surface(payload, _resolve_selected_system_tab(), activity_payload)
            if render != last_render:
                clear_visible_screen()
                print_ui_line(render)
                last_render = render
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(0.5)
    return 0


def run_thin_render_loop():
    """Thin renderer: read rendering instructions/data from stdin (or FIFO) and print directly."""
    print_ui_line("Thin renderer ready. Waiting for rendering instructions...")
    while not shutdown_requested:
        try:
            # Read a line of JSON from stdin (or FIFO)
            raw_line = sys.stdin.readline()
            if not raw_line:
                time.sleep(0.1)
                continue
            try:
                payload = json.loads(raw_line)
            except json.JSONDecodeError as exc:
                print_ui_line(f"[thin-renderer] Invalid JSON: {exc}")
                continue
            # Expect a 'render' key with the content to display
            render_content = payload.get("render")
            if render_content is not None:
                clear_visible_screen()
                print_ui_line(str(render_content))
            else:
                print_ui_line(f"[thin-renderer] No 'render' key in payload: {payload}")
        except Exception as exc:
            print_ui_line(f"[thin-renderer] Exception: {exc}")
            time.sleep(0.1)
    return 0


def _normalize_ollama_base_url(host_or_url: str) -> str:
    """Normalize Ollama host into an absolute base URL."""
    value = host_or_url.strip()
    if value.startswith("http://") or value.startswith("https://"):
        return value.rstrip("/")
    return f"http://{value.rstrip('/')}"


def _chat_with_ollama(prompt: str) -> str:
    """Send a non-streaming chat request to Ollama and return model text."""
    payload = {
        "model": OLLAMA_MODEL,
        "messages": _build_ollama_messages(prompt),
        "stream": False,
    }
    data = json.dumps(payload).encode("utf-8")
    base_url = _normalize_ollama_base_url(OLLAMA_HOST)
    request = urllib.request.Request(
        url=f"{base_url}/api/chat",
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    with urllib.request.urlopen(request, timeout=OLLAMA_TIMEOUT_SEC) as response:
        body = response.read().decode("utf-8")

    decoded = json.loads(body)
    message = decoded.get("message", {})
    content = ""
    if isinstance(message, dict):
        content = str(message.get("content", "")).strip()
    if not content:
        content = str(decoded.get("response", "")).strip()
    if not content:
        raise ValueError("ollama response missing content")
    return content


def _stream_chat_with_ollama(prompt: str):
    """Stream chat deltas from Ollama and return accumulated response text."""
    payload = {
        "model": OLLAMA_MODEL,
        "messages": _build_ollama_messages(prompt),
        "stream": True,
    }
    data = json.dumps(payload).encode("utf-8")
    base_url = _normalize_ollama_base_url(OLLAMA_HOST)
    request = urllib.request.Request(
        url=f"{base_url}/api/chat",
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    chunks = []
    with urllib.request.urlopen(request, timeout=OLLAMA_TIMEOUT_SEC) as response:
        for raw_line in response:
            line = raw_line.decode("utf-8").strip()
            if not line:
                continue
            decoded = json.loads(line)
            message = decoded.get("message", {})
            delta = ""
            if isinstance(message, dict):
                delta = str(message.get("content", "")).strip()
            if delta:
                chunks.append(delta)
                yield delta
            if decoded.get("done"):
                break

    if not chunks:
        raise ValueError("ollama stream produced no chunks")

    return " ".join(chunks)


def generate_chat_response(prompt: str) -> str:
    """Generate response using configured backend with deterministic fallback."""
    if CHAT_BACKEND != "ollama":
        return f"Echo: {prompt}"

    try:
        return _chat_with_ollama(prompt)
    except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError) as exc:
        # Keep the bridge responsive even when Ollama is unavailable.
        print(f"[{APPLET_NAME}] Ollama request failed, falling back to echo: {exc}", file=sys.stderr)
        return f"Echo: {prompt}"


def emit_streaming_bridge_events(response_text: str):
    """Emit chunk events followed by a final response envelope."""
    parts = [part for part in response_text.split() if part]
    if len(parts) <= 1:
        print(json.dumps({"type": "response", "response": response_text}))
        sys.stdout.flush()
        return

    for part in parts:
        print(json.dumps({"type": "chunk", "delta": part}))
        sys.stdout.flush()

    print(json.dumps({"type": "response", "response": response_text}))
    sys.stdout.flush()


def emit_ollama_streaming_bridge_events(prompt: str) -> str:
    """Emit chunk events from real Ollama stream and return final response text."""
    stream_iter = _stream_chat_with_ollama(prompt)
    chunks = []
    try:
        while True:
            delta = next(stream_iter)
            chunks.append(delta)
            print(json.dumps({"type": "chunk", "delta": delta}))
            sys.stdout.flush()
    except StopIteration as stop:
        final_response = stop.value if stop.value else " ".join(chunks)
        if not final_response:
            raise ValueError("ollama stream returned empty final response")
        print(json.dumps({"type": "response", "response": final_response}))
        sys.stdout.flush()
        return final_response


def main():
    """Main applet loop."""
    parser = argparse.ArgumentParser(description="AgentX applet template")
    parser.add_argument(
        "--bridge-chat", action="store_true", help="Run one-shot chat bridge mode over stdin/stdout JSONL"
    )
    parser.add_argument(
        "--bridge-chat-server",
        action="store_true",
        help="Run persistent chat bridge server mode over stdin/stdout JSONL",
    )
    args = parser.parse_args()

    # Register signal handlers
    signal.signal(signal.SIGTERM, signal_handler)
    signal.signal(signal.SIGINT, signal_handler)

    if args.bridge_chat or args.bridge_chat_server:
        print_ready()
        for raw_line in sys.stdin:
            line = raw_line.strip()
            if not line:
                continue
            try:
                request = json.loads(line)
            except json.JSONDecodeError as exc:
                print(json.dumps({"type": "error", "error": f"invalid json request: {exc}"}))
                sys.stdout.flush()
                continue

            if request.get("type") != "prompt":
                print(json.dumps({"type": "error", "error": "unsupported request type"}))
                sys.stdout.flush()
                continue

            prompt = str(request.get("prompt", "")).strip()
            if not prompt:
                print(json.dumps({"type": "error", "error": "empty prompt"}))
                sys.stdout.flush()
                continue

            if CHAT_BACKEND == "ollama":
                try:
                    emit_ollama_streaming_bridge_events(prompt)
                except (
                    urllib.error.URLError,
                    urllib.error.HTTPError,
                    TimeoutError,
                    ValueError,
                    json.JSONDecodeError,
                ) as exc:
                    print(f"[{APPLET_NAME}] Ollama stream failed, falling back to echo: {exc}", file=sys.stderr)
                    response_text = f"Echo: {prompt}"
                    emit_streaming_bridge_events(response_text)
            else:
                response_text = generate_chat_response(prompt)
                emit_streaming_bridge_events(response_text)
            if args.bridge_chat:
                return 0

        print(json.dumps({"type": "error", "error": "no prompt received"}))
        sys.stdout.flush()
        return 1

    if APPLET_NAME in {"input", "chat", "context"}:
        clear_visible_screen()

    if APPLET_NAME == "input":
        return run_input_affordance_loop()
    if APPLET_NAME == "chat":
        return run_chat_affordance_loop()
    if APPLET_NAME == "context":
        context_mode = os.getenv("AGENTX_CONTEXT_PANE_MODE", "surface").strip().lower()
        if context_mode in {"thin", "bridge"}:
            return run_thin_render_loop()
        return run_context_affordance_loop()

    # Logs/other panes retain a passive heartbeat loop.
    try:
        print_ready()
        print_output(f"Applet '{APPLET_NAME}' started in session {SESSION_ID}")
        print_output(f"IPC paths: input={IPC_INPUT}, output={IPC_OUTPUT}")
        while not shutdown_requested:
            time.sleep(0.5)
    except KeyboardInterrupt:
        pass

    print_output(f"Applet '{APPLET_NAME}' shutting down...")
    return 0


if __name__ == "__main__":
    sys.exit(main())
