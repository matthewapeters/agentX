#!/usr/bin/env bash
# test_tmux_attached_runtime_headless.sh
# Headless attached-runtime E2E for startup focus and full-session :q shutdown.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
SESSION_ID="attached_${RANDOM}_$$"
USERNAME="uxe2e"
TMUX_SESSION="agentx_${USERNAME}_${SESSION_ID}"
LOG_PATH="/tmp/${TMUX_SESSION}.typescript"
LAUNCH_PID=""

cleanup() {
  if [[ -n "$LAUNCH_PID" ]] && kill -0 "$LAUNCH_PID" >/dev/null 2>&1; then
    kill "$LAUNCH_PID" >/dev/null 2>&1 || true
  fi
  tmux kill-session -t "$TMUX_SESSION" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

if ! command -v script >/dev/null 2>&1; then
  echo "FAIL: required command 'script' is not available"
  exit 1
fi

script -qfec "cd '$ROOT_DIR' && AGENTX_CHAT_BACKEND=echo '$BIN_PATH' --project-dir '$ROOT_DIR' --user '$USERNAME' --session-id '$SESSION_ID'" "$LOG_PATH" &
LAUNCH_PID="$!"

for _ in $(seq 1 100); do
  if tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

if ! tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
  echo "FAIL: attached tmux session did not start"
  exit 1
fi

PANE_TABLE=""
for _ in $(seq 1 100); do
  PANE_TABLE="$(tmux list-panes -t "$TMUX_SESSION:0" -F '#{pane_id}|#{pane_title}|#{pane_active}|#{pane_index}')"
  if grep -q '|input|' <<< "$PANE_TABLE" && grep -q '|output|' <<< "$PANE_TABLE" && grep -q '|system|' <<< "$PANE_TABLE"; then
    break
  fi
  sleep 0.2
done

if ! grep -q '|input|' <<< "$PANE_TABLE"; then
  echo "FAIL: input pane title not found"
  echo "$PANE_TABLE"
  exit 1
fi

if ! awk -F'|' '$2=="input" && $3=="1"{found=1} END{exit(found?0:1)}' <<< "$PANE_TABLE"; then
  echo "FAIL: input pane was not active after attached startup"
  echo "$PANE_TABLE"
  exit 1
fi

INPUT_PANE="$(awk -F'|' '$2=="input"{print $1; exit}' <<< "$PANE_TABLE")"
if [[ -z "$INPUT_PANE" ]]; then
  echo "FAIL: could not resolve input pane id"
  exit 1
fi

# Detach the startup-attached client after focus validation so scripted key injection is reliable.
tmux detach-client -s "$TMUX_SESSION" >/dev/null 2>&1 || true

HEALTH_ADDR="$(grep -Eo '127\.0\.0\.1:[0-9]+' "$LOG_PATH" | tail -n 1 || true)"
if [[ -z "$HEALTH_ADDR" ]]; then
  echo "FAIL: could not resolve health endpoint address from runtime log"
  exit 1
fi

if ! curl -sS -X POST "http://$HEALTH_ADDR/submit" -H 'Content-Type: application/json' -d '{"prompt":":q"}' >/dev/null; then
  echo "FAIL: submit endpoint did not accept :q shutdown prompt"
  exit 1
fi

for _ in $(seq 1 80); do
  if ! tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

if tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
  echo "FAIL: tmux session still exists after :q"
  tmux list-panes -t "$TMUX_SESSION:0" -F '#{pane_id}|#{pane_title}|#{pane_active}|#{pane_index}' || true
  exit 1
fi

if kill -0 "$LAUNCH_PID" >/dev/null 2>&1; then
  kill "$LAUNCH_PID" >/dev/null 2>&1 || true
fi

echo "PASS: attached runtime starts focused on input and :q shuts down the full session."
