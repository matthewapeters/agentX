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
    with urllib.request.urlopen(request, timeout=OLLAMA_TIMEOUT_SEC) as response:
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


def _render_system_surface(payload: dict) -> str:
    """Render the deterministic text contract for the system pane."""
    config = _load_runtime_config()
    agentx_cfg = config.get("agentx", {}) if isinstance(config.get("agentx"), dict) else {}
    agentix_cfg = config.get("agentix", {}) if isinstance(config.get("agentix"), dict) else {}
    turns = payload.get("turns", []) or []
    turn_count = int(payload.get("turn_count", len(turns)))
    last_turn = turns[-1] if turns else {}
    entries = _safe_listdir(PROJECT_DIR)
    session_dir = os.path.join(PROJECT_DIR, "sessions", USERNAME)
    session_history = _safe_listdir(session_dir)
    recent_history = list(reversed(turns[-2:]))
    visualizer = _context_visualizer(turns)

    preview_entry = "none"
    if entries:
        preview_path = os.path.join(PROJECT_DIR, entries[0])
        preview_entry = f"{_classify_entry(preview_path)} {entries[0]}"

    lines = [
        "[SYSTEM]",
        "== FILES ==",
        f"root: {_trim_single_line(PROJECT_DIR, 48)} | entries: {len(entries)} | preview: {_trim_single_line(preview_entry, 24)}",
        "== CONFIGURATION ==",
        f"model: {_trim_single_line(agentx_cfg.get('ollama_model', OLLAMA_MODEL), 24)} | theme: {_trim_single_line(agentx_cfg.get('theme_mode', 'none'), 16)} | backend: {_trim_single_line(CHAT_BACKEND, 12)}",
        f"ollama_host: {_trim_single_line(agentx_cfg.get('ollama_host', OLLAMA_HOST), 40)} | prompts: {_trim_single_line(agentix_cfg.get('system_prompts_dir', 'none'), 18)}",
        "== CONTEXT ==",
        f"session_id: {_trim_single_line(payload.get('session_id', SESSION_ID), 28)} | turn_count: {turn_count}",
        f"last_user: {_trim_single_line(last_turn.get('prompt', 'none'), 56)}",
        f"last_assistant: {_trim_single_line(last_turn.get('response', 'none'), 51)}",
        "== CONTEXT HISTORY ==",
        f"history_context_count: {len(session_history)} | latest_history_session: {_trim_single_line(session_history[-1] if session_history else 'none', 20)}",
        f"recent_prompt: {_trim_single_line(recent_history[0].get('prompt', 'none') if recent_history else 'none', 57)}",
        "== CONTEXT VISUALIZER ==",
        f"max_tokens: {visualizer['max_tokens']} | user: {visualizer['user']} | assistant: {visualizer['assistant']} | tool: {visualizer['tool']}",
        f"working_memory: {visualizer['working_memory']} | system: {visualizer['system']} | attachments: {visualizer['attachments']} | thinking: {visualizer['thinking']}",
    ]
    return "\n".join(lines)


def run_input_affordance_loop():
    """Run line-based input affordance for interactive tmux input pane."""
    print_ui_line("Input ready. Enter prompt and press Enter.")
    print_ui_line("Commands: :q shuts down the session, :clear clears chat output.")
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
                    except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError) as submit_exc:
                        print_ui_line(f"Shutdown failed: {exc}; submit fallback failed: {submit_exc}")
            return 0

        try:
            routed_response = _submit_prompt(prompt)
            print_ui_line(f"Submitted: {prompt}")
            print_ui_line(f"Response: {routed_response}")
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError) as exc:
            print_ui_line(f"Submit failed: {exc}")

    return 0


def run_chat_affordance_loop():
    """Poll context endpoint and display assistant responses as they arrive."""
    print_ui_line("Chat ready.")
    last_turn_count = 0
    while not shutdown_requested:
        try:
            payload = _get_json("/context")
            turns = payload.get("turns", [])
            turn_count = int(payload.get("turn_count", len(turns)))
            if turn_count > last_turn_count:
                new_turns = turns[last_turn_count:turn_count]
                for turn in new_turns:
                    prompt = str(turn.get("prompt", "")).strip()
                    response = str(turn.get("response", "")).strip()
                    if prompt:
                        print_ui_line(f"User: {prompt}")
                    if response:
                        print_ui_line(f"Agent: {response}")
                last_turn_count = turn_count
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(0.5)
    return 0


def run_context_affordance_loop():
    """Poll context endpoint and render the deterministic system-pane surface."""
    last_signature = ""
    while not shutdown_requested:
        try:
            payload = _get_json("/context")
            rendered = _render_system_surface(payload)
            signature = rendered
            if signature != last_signature:
                clear_visible_screen()
                print_ui_line(rendered)
                last_signature = signature
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(1.0)
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
        "messages": [{"role": "user", "content": prompt}],
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
        "messages": [{"role": "user", "content": prompt}],
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
