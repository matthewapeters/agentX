#!/usr/bin/env bash
# test_system_tab_routing_headless.sh
# Validates deterministic/stateful system tab routing in headless runtime.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
SESSION_ID="system_tab_${RANDOM}_$$"
USERNAME="uxe2e"
TMUX_SESSION="agentx_${USERNAME}_${SESSION_ID}"
STATE_DIR="$ROOT_DIR/.agentx"
STATE_FILE="$STATE_DIR/system-panel-tab.txt"
BACKUP_FILE=""
CORE_PID=""

cleanup() {
  if [[ -n "$CORE_PID" ]]; then
    kill "$CORE_PID" >/dev/null 2>&1 || true
    wait "$CORE_PID" >/dev/null 2>&1 || true
  fi
  tmux kill-session -t "$TMUX_SESSION" >/dev/null 2>&1 || true
  if [[ -n "$BACKUP_FILE" && -f "$BACKUP_FILE" ]]; then
    mv "$BACKUP_FILE" "$STATE_FILE"
  else
    rm -f "$STATE_FILE"
  fi
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

mkdir -p "$STATE_DIR"
if [[ -f "$STATE_FILE" ]]; then
  BACKUP_FILE="${STATE_FILE}.bak.${RANDOM}_$$"
  cp "$STATE_FILE" "$BACKUP_FILE"
fi

echo "files" > "$STATE_FILE"

AGENTX_CHAT_BACKEND=echo AGENTX_SYSTEM_PANEL_TAB=files "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --attach=false \
  >/tmp/agentx_system_tab_${SESSION_ID}.log 2>&1 &
CORE_PID="$!"

for _ in $(seq 1 60); do
  if tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

if ! tmux has-session -t "$TMUX_SESSION" >/dev/null 2>&1; then
  echo "FAIL: tmux session did not start"
  exit 1
fi

CONTEXT_TARGET=""
for _ in $(seq 1 50); do
  PANE_TABLE="$(tmux list-panes -t "$TMUX_SESSION:0" -F '#{pane_id}|#{pane_title}')"
  CONTEXT_TARGET="$(grep '|system$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  if [[ -n "$CONTEXT_TARGET" ]]; then
    break
  fi
  sleep 0.2
done

if [[ -z "$CONTEXT_TARGET" ]]; then
  echo "FAIL: could not resolve system pane target"
  exit 1
fi

for _ in $(seq 1 80); do
  SYSTEM_CAPTURE="$(tmux capture-pane -t "$CONTEXT_TARGET" -p -S -300)"
  SYSTEM_CAPTURE_COMPACT="$(echo "$SYSTEM_CAPTURE" | tr -d '[:space:]')"
  if grep -Fq "[SYSTEMTAB]active=files" <<< "$SYSTEM_CAPTURE_COMPACT" && grep -Fq "== FILES ==" <<< "$SYSTEM_CAPTURE"; then
    break
  fi
  sleep 0.25
done

SYSTEM_CAPTURE="$(tmux capture-pane -t "$CONTEXT_TARGET" -p -S -300)"
SYSTEM_CAPTURE_COMPACT="$(echo "$SYSTEM_CAPTURE" | tr -d '[:space:]')"
if ! grep -Fq "[SYSTEMTAB]active=files" <<< "$SYSTEM_CAPTURE_COMPACT"; then
  echo "FAIL: expected active files tab in system pane"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi
if ! grep -Fq "== FILES ==" <<< "$SYSTEM_CAPTURE"; then
  echo "FAIL: expected FILES section for files tab"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi
if grep -Fq "== CONTEXT WINDOW ==" <<< "$SYSTEM_CAPTURE"; then
  echo "FAIL: context visualizer should not render while files tab is active"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi

echo "context-visualizer" > "$STATE_FILE"

for _ in $(seq 1 80); do
  SYSTEM_CAPTURE="$(tmux capture-pane -t "$CONTEXT_TARGET" -p -S -300)"
  SYSTEM_CAPTURE_COMPACT="$(echo "$SYSTEM_CAPTURE" | tr -d '[:space:]')"
  if grep -Fq "[SYSTEMTAB]active=context-visualizer" <<< "$SYSTEM_CAPTURE_COMPACT" && grep -Fq "== CONTEXT WINDOW ==" <<< "$SYSTEM_CAPTURE"; then
    break
  fi
  sleep 0.25
done

SYSTEM_CAPTURE="$(tmux capture-pane -t "$CONTEXT_TARGET" -p -S -300)"
SYSTEM_CAPTURE_COMPACT="$(echo "$SYSTEM_CAPTURE" | tr -d '[:space:]')"
if ! grep -Fq "[SYSTEMTAB]active=context-visualizer" <<< "$SYSTEM_CAPTURE_COMPACT"; then
  echo "FAIL: expected active context-visualizer tab after state-file update"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi
if ! grep -Fq "== CONTEXT WINDOW ==" <<< "$SYSTEM_CAPTURE"; then
  echo "FAIL: expected context window section after tab switch"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi

echo "PASS: system tab routing is deterministic and stateful via tab state file."
