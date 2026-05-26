#!/usr/bin/env bash
# test_startup_ollama_bootstrap_headless.sh
# Headless startup E2E that verifies TUI consumes runtime config/backend wiring
# and renders a real Ollama bootstrap response using a local mock endpoint.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
SESSION_ID="startup_ollama_${RANDOM}_$$"
USERNAME="uxe2e"
TMUX_SESSION="agentx_${USERNAME}_${SESSION_ID}"
TMP_DIR="$(mktemp -d)"
PORT_FILE="$TMP_DIR/ollama_port"
OLLAMA_PID=""
CORE_PID=""

cleanup() {
  if [[ -n "$CORE_PID" ]] && kill -0 "$CORE_PID" >/dev/null 2>&1; then
    kill "$CORE_PID" >/dev/null 2>&1 || true
    wait "$CORE_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$OLLAMA_PID" ]] && kill -0 "$OLLAMA_PID" >/dev/null 2>&1; then
    kill "$OLLAMA_PID" >/dev/null 2>&1 || true
    wait "$OLLAMA_PID" >/dev/null 2>&1 || true
  fi
  tmux kill-session -t "$TMUX_SESSION" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "FAIL: required command 'python3' is not available"
  exit 1
fi

python3 -u - "$PORT_FILE" >"$TMP_DIR/ollama_server.log" 2>&1 <<'PY' &
import http.server
import json
import socketserver
import sys

port_file = sys.argv[1]


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/api/chat":
            self.send_response(404)
            self.end_headers()
            return
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)
        payload = {
            "message": {
                "content": "Mock bootstrap from Ollama"
            },
            "done": True,
        }
        body = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format, *_args):
        return


with socketserver.TCPServer(("127.0.0.1", 0), Handler) as server:
    with open(port_file, "w", encoding="utf-8") as f:
        f.write(str(server.server_address[1]))
    server.serve_forever()
PY
OLLAMA_PID="$!"

for _ in $(seq 1 80); do
  if [[ -s "$PORT_FILE" ]]; then
    break
  fi
  sleep 0.1
done

if [[ ! -s "$PORT_FILE" ]]; then
  echo "FAIL: mock Ollama server did not start"
  exit 1
fi

OLLAMA_PORT="$(cat "$PORT_FILE")"

AGENTX_OLLAMA_HOST="127.0.0.1:${OLLAMA_PORT}" \
AGENTX_OLLAMA_MODEL="mock-startup-model" \
  "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --attach=false \
  >"$TMP_DIR/agentx_startup.log" 2>&1 &
CORE_PID="$!"

for _ in $(seq 1 80); do
  if tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

if ! tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
  echo "FAIL: tmux session did not start"
  exit 1
fi

CHAT_TARGET=""
CONTEXT_TARGET=""
for _ in $(seq 1 60); do
  PANE_TABLE="$(tmux list-panes -t "$TMUX_SESSION:0" -F '#{pane_id}|#{pane_title}')"
  CHAT_TARGET="$(grep '|output$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  CONTEXT_TARGET="$(grep '|system$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  if [[ -n "$CHAT_TARGET" && -n "$CONTEXT_TARGET" ]]; then
    break
  fi
  sleep 0.1
done

if [[ -z "$CHAT_TARGET" || -z "$CONTEXT_TARGET" ]]; then
  echo "FAIL: could not resolve output/system pane targets"
  exit 1
fi

CHAT_OK=0
CONTEXT_OK=0
for _ in $(seq 1 80); do
  CHAT_CAPTURE="$(tmux capture-pane -t "$CHAT_TARGET" -p -S -200)"
  CONTEXT_CAPTURE="$(tmux capture-pane -t "$CONTEXT_TARGET" -p -S -200)"

  if grep -Fq "Agent: Mock bootstrap from Ollama" <<< "$CHAT_CAPTURE"; then
    CHAT_OK=1
  fi
  if grep -Fq "backend: ollama" <<< "$CONTEXT_CAPTURE"; then
    CONTEXT_OK=1
  fi

  if [[ "$CHAT_OK" -eq 1 && "$CONTEXT_OK" -eq 1 ]]; then
    break
  fi
  sleep 0.2
done

if [[ "$CHAT_OK" -ne 1 ]]; then
  echo "FAIL: chat pane missing Ollama bootstrap response"
  echo "--- chat capture ---"
  echo "$CHAT_CAPTURE"
  exit 1
fi

if [[ "$CONTEXT_OK" -ne 1 ]]; then
  echo "FAIL: context pane missing backend=ollama startup state"
  echo "--- context capture ---"
  echo "$CONTEXT_CAPTURE"
  exit 1
fi

echo "PASS: startup consumes runtime backend config and renders Ollama bootstrap response."
