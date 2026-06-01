#!/usr/bin/env bash
# test_tmux_pane_affordances_headless.sh
# Headless UX contract test for live pane affordances.
# It launches AgentX core, submits a prompt through the input pane,
# and validates chat/context panes show sanctioned user-facing output only.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
SESSION_ID="ux_affordance_${RANDOM}_$$"
USERNAME="uxe2e"
TMUX_SESSION="agentx_${USERNAME}_${SESSION_ID}"
CORE_PID=""

cleanup() {
  if [[ -n "$CORE_PID" ]]; then
    kill "$CORE_PID" >/dev/null 2>&1 || true
    wait "$CORE_PID" >/dev/null 2>&1 || true
  fi
  tmux kill-session -t "$TMUX_SESSION" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

AGENTX_CHAT_BACKEND=echo "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --attach=false \
  >/tmp/agentx_pane_affordances_${SESSION_ID}.log 2>&1 &
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

# Input pane is pane index 2 in window 0.
INPUT_TARGET=""
CHAT_TARGET=""
CONTEXT_TARGET=""

for _ in $(seq 1 40); do
  PANE_TABLE="$(tmux list-panes -t "$TMUX_SESSION:0" -F '#{pane_id}|#{pane_title}')"
  INPUT_TARGET="$(grep '|input$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  CHAT_TARGET="$(grep '|output$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  CONTEXT_TARGET="$(grep '|system$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  if [[ -n "$INPUT_TARGET" && -n "$CHAT_TARGET" && -n "$CONTEXT_TARGET" ]]; then
    break
  fi
  sleep 0.2
done

if [[ -z "$INPUT_TARGET" || -z "$CHAT_TARGET" || -z "$CONTEXT_TARGET" ]]; then
  echo "FAIL: could not resolve output/system/input pane targets by title"
  exit 1
fi

tmux send-keys -t "$INPUT_TARGET" "what is 2+2?" Enter

for _ in $(seq 1 80); do
  CHAT_READY=0
  CONTEXT_READY=0
  if tmux capture-pane -t "$CHAT_TARGET" -p | grep -q "Agent: Echo: what is 2+2\?"; then
    CHAT_READY=1
  fi
  if tmux capture-pane -t "$CONTEXT_TARGET" -p | grep -q "== PROMPT CYCLE ==" && tmux capture-pane -t "$CONTEXT_TARGET" -p | grep -q "🤖 Respond"; then
    CONTEXT_READY=1
  fi
  if [[ "$CHAT_READY" -eq 1 && "$CONTEXT_READY" -eq 1 ]]; then
    break
  fi
  sleep 0.25
done

CHAT_CAPTURE="$(tmux capture-pane -t "$CHAT_TARGET" -p)"
CONTEXT_CAPTURE="$(tmux capture-pane -t "$CONTEXT_TARGET" -p)"
CONTEXT_CAPTURE_FULL="$(tmux capture-pane -t "$CONTEXT_TARGET" -p -S -300)"
INPUT_CAPTURE="$(tmux capture-pane -t "$INPUT_TARGET" -p)"

if ! grep -q "User: what is 2+2?" <<< "$CHAT_CAPTURE"; then
  echo "FAIL: chat pane missing user message"
  exit 1
fi

if ! grep -q "Agent: Echo: what is 2+2?" <<< "$CHAT_CAPTURE"; then
  echo "FAIL: chat pane missing agent response"
  exit 1
fi

for required in "== CONTEXT WINDOW ==" "== PROMPT CYCLE ==" "consumed:"; do
  if ! grep -Fq "$required" <<< "$CONTEXT_CAPTURE_FULL"; then
    echo "FAIL: system pane missing required content: $required"
    echo "--- system capture ---"
    echo "$CONTEXT_CAPTURE_FULL"
    exit 1
  fi
done

for required in "Working Memory" "System Prompts" "User Prompts" "Attachments" "Thinking" "Agent Response" "Tool Calls" "Remaining"; do
  if ! grep -Fq "$required" <<< "$CONTEXT_CAPTURE_FULL"; then
    echo "FAIL: system pane missing context meter row: $required"
    echo "--- system capture ---"
    echo "$CONTEXT_CAPTURE_FULL"
    exit 1
  fi
done

for required in "Classify" "Think" "Tool" "Respond"; do
  if ! grep -Fq "$required" <<< "$CONTEXT_CAPTURE_FULL"; then
    echo "FAIL: system pane missing prompt cycle row: $required"
    echo "--- system capture ---"
    echo "$CONTEXT_CAPTURE_FULL"
    exit 1
  fi
done

for required in "💾 Working Memory" "🧠 System Prompts" "👤 User Prompts" "🤖 Agent Response" "░ Remaining"; do
  if ! grep -Fq "$required" <<< "$CONTEXT_CAPTURE_FULL"; then
    echo "FAIL: system pane missing emoji context meter row: $required"
    echo "--- system capture ---"
    echo "$CONTEXT_CAPTURE_FULL"
    exit 1
  fi
done

if grep -Fq "== SESSION SNAPSHOT ==" <<< "$CONTEXT_CAPTURE_FULL"; then
  echo "FAIL: system pane should not render session snapshot block"
  echo "--- system capture ---"
  echo "$CONTEXT_CAPTURE_FULL"
  exit 1
fi

if grep -Fq "Response:" <<< "$INPUT_CAPTURE"; then
  echo "FAIL: input pane must not mirror agent response text"
  echo "--- input capture ---"
  echo "$INPUT_CAPTURE"
  exit 1
fi

for banned in "READY {" "IPC paths:" "echo '[assistant-stream]" "send-keys -t"; do
  if grep -Fq "$banned" <<< "$CHAT_CAPTURE"; then
    echo "FAIL: chat pane contains unsanctioned output: $banned"
    exit 1
  fi
  if grep -Fq "$banned" <<< "$CONTEXT_CAPTURE"; then
    echo "FAIL: context pane contains unsanctioned output: $banned"
    exit 1
  fi
done

echo "PASS: pane affordances match UX contract (sanctioned outputs only)."
