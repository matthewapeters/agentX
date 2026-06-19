#!/usr/bin/env bash
# test_applet_resize_api_headless.sh
# Headless contract test for the --applet-resize mode.
# It seeds deterministic pane dimensions, queries /render, and verifies
# the exact frame strings exposed by the resize applet API.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
PORT_FILE="$TMP_DIR/resize_api_port"
STDOUT_LOG="$TMP_DIR/applet_resize_stdout.log"
STDERR_LOG="$TMP_DIR/applet_resize_stderr.log"
APPLET_PID=""

cleanup() {
  if [[ -n "$APPLET_PID" ]] && kill -0 "$APPLET_PID" >/dev/null 2>&1; then
    kill "$APPLET_PID" >/dev/null 2>&1 || true
    wait "$APPLET_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "FAIL: required command 'curl' is not available"
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "FAIL: required command 'python3' is not available"
  exit 1
fi

python3 - "$PORT_FILE" <<'PY'
import socket
import sys

port_file = sys.argv[1]
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
with open(port_file, "w", encoding="utf-8") as handle:
    handle.write(str(port))
PY

API_PORT="$(cat "$PORT_FILE")"
API_ADDR="127.0.0.1:${API_PORT}"

AGENTX_WIDGET_PANE_HEIGHT=8 \
AGENTX_WIDGET_PANE_WIDTH=12 \
LINES=8 \
COLUMNS=12 \
  "$BIN_PATH" \
  --applet-resize \
  --applet-resize-api-addr "$API_ADDR" \
  >"$STDOUT_LOG" 2>"$STDERR_LOG" &
APPLET_PID="$!"

for _ in $(seq 1 80); do
  if curl -fsS "http://$API_ADDR/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

if ! curl -fsS "http://$API_ADDR/health" >/dev/null 2>&1; then
  echo "FAIL: resize applet API did not become healthy at $API_ADDR"
  echo "--- stderr ---"
  cat "$STDERR_LOG"
  exit 1
fi

RENDER_PAYLOAD="$TMP_DIR/render.json"
curl -fsS "http://$API_ADDR/render" > "$RENDER_PAYLOAD"

python3 - "$RENDER_PAYLOAD" <<'PY'
import json
import sys

payload_path = sys.argv[1]
with open(payload_path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)

expected_height = 8
expected_width = 12
expected_top = "+----------+"
expected_inner = "|          |"
expected_lines = [expected_top] + [expected_inner] * 6 + [expected_top]
expected_frame = "\n".join(expected_lines)

if payload.get("height") != expected_height:
    raise SystemExit(f"expected height={expected_height}, got {payload.get('height')}")
if payload.get("width") != expected_width:
    raise SystemExit(f"expected width={expected_width}, got {payload.get('width')}")
if payload.get("lines") != expected_lines:
    raise SystemExit(f"unexpected render lines: {payload.get('lines')}")
if payload.get("frame") != expected_frame:
    raise SystemExit("unexpected frame string returned by /render")
if not isinstance(payload.get("sequence"), int) or payload.get("sequence", 0) < 1:
    raise SystemExit(f"expected sequence>=1, got {payload.get('sequence')}")
if not payload.get("updated_at"):
    raise SystemExit("expected non-empty updated_at")
PY

if ! grep -Fq "[APPLET RESIZE] API http://$API_ADDR/render" "$STDERR_LOG"; then
  echo "FAIL: expected API discovery line in stderr"
  echo "--- stderr ---"
  cat "$STDERR_LOG"
  exit 1
fi

echo "PASS: resize applet API returns expected frame contract for seeded pane dimensions."
