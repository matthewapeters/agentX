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
import urllib.error
import urllib.request

# Applet metadata
APPLET_NAME = os.getenv("AGENTX_APPLET_NAME", "template")
SESSION_ID = os.getenv("AGENTX_SESSION_ID", "unknown")
IPC_INPUT = os.getenv("AGENTX_IPC_INPUT", None)
IPC_OUTPUT = os.getenv("AGENTX_IPC_OUTPUT", None)
CHAT_BACKEND = os.getenv("AGENTX_CHAT_BACKEND", "echo").strip().lower()
OLLAMA_HOST = os.getenv("AGENTX_OLLAMA_HOST", "localhost:11434").strip()
OLLAMA_MODEL = os.getenv("AGENTX_OLLAMA_MODEL", "llama3.2").strip()
OLLAMA_TIMEOUT_SEC = float(os.getenv("AGENTX_OLLAMA_TIMEOUT_SEC", "30"))

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

            response_text = generate_chat_response(prompt)
            print(json.dumps({"type": "response", "response": response_text}))
            sys.stdout.flush()
            if args.bridge_chat:
                return 0

        print(json.dumps({"type": "error", "error": "no prompt received"}))
        sys.stdout.flush()
        return 1

    print_ready()
    print_output(f"Applet '{APPLET_NAME}' started in session {SESSION_ID}")
    print_output(f"IPC paths: input={IPC_INPUT}, output={IPC_OUTPUT}")

    # Placeholder loop: read from stdin (or FIFO in production)
    # In first iteration, applet just displays a placeholder and waits for shutdown.
    try:
        while not shutdown_requested:
            time.sleep(0.5)
    except KeyboardInterrupt:
        pass
    finally:
        print_output(f"Applet '{APPLET_NAME}' shutting down...")
        return 0


if __name__ == "__main__":
    sys.exit(main())
