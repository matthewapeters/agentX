#!/usr/bin/env bash
# test_tmux_ux_flow_what_is_2_plus_2_headless.sh
# End-to-end UX flow contract for prompt: "what is 2+2?"
# Validates pane responsibilities and key visualization semantics.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
SESSION_ID="ux_flow_${RANDOM}_$$"
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
  >/tmp/agentx_ux_flow_${SESSION_ID}.log 2>&1 &
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

INPUT_TARGET=""
CHAT_TARGET=""
SYSTEM_TARGET=""

for _ in $(seq 1 40); do
  PANE_TABLE="$(tmux list-panes -t "$TMUX_SESSION:0" -F '#{pane_id}|#{pane_title}')"
  INPUT_TARGET="$(grep '|input$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  CHAT_TARGET="$(grep '|output$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  SYSTEM_TARGET="$(grep '|system$' <<< "$PANE_TABLE" | cut -d'|' -f1 | head -n 1 || true)"
  if [[ -n "$INPUT_TARGET" && -n "$CHAT_TARGET" && -n "$SYSTEM_TARGET" ]]; then
    break
  fi
  sleep 0.2
done

if [[ -z "$INPUT_TARGET" || -z "$CHAT_TARGET" || -z "$SYSTEM_TARGET" ]]; then
  echo "FAIL: could not resolve input/output/system panes by title"
  exit 1
fi

# Submit the exact UX prompt through the input pane.
tmux send-keys -t "$INPUT_TARGET" "what is 2+2?" Enter

for _ in $(seq 1 120); do
  CHAT_READY=0
  SYSTEM_READY=0
  if tmux capture-pane -t "$CHAT_TARGET" -p | grep -q "Agent: Echo: what is 2+2?"; then
    CHAT_READY=1
  fi
  if tmux capture-pane -t "$SYSTEM_TARGET" -p | grep -q "== PROMPT CYCLE ==" \
    && tmux capture-pane -t "$SYSTEM_TARGET" -p | grep -q "🤖 Respond"; then
    SYSTEM_READY=1
  fi
  if [[ "$CHAT_READY" -eq 1 && "$SYSTEM_READY" -eq 1 ]]; then
    break
  fi
  sleep 0.25
done

INPUT_CAPTURE="$(tmux capture-pane -t "$INPUT_TARGET" -p -S -200)"
CHAT_CAPTURE="$(tmux capture-pane -t "$CHAT_TARGET" -p -S -200)"
SYSTEM_CAPTURE="$(tmux capture-pane -t "$SYSTEM_TARGET" -p -S -300)"

# Output pane assertions: conversational turns + lifecycle rows.
for required in "User: what is 2+2?" "Agent: Echo: what is 2+2?" "Classification:" "Thinking:" "Response:"; do
  if ! grep -Fq "$required" <<< "$CHAT_CAPTURE"; then
    echo "FAIL: output pane missing required UX content: $required"
    echo "--- output capture ---"
    echo "$CHAT_CAPTURE"
    exit 1
  fi
done

# Input pane assertions: command-entry only (no mirrored agent response payload).
if ! grep -Fq "Submitted: what is 2+2?" <<< "$INPUT_CAPTURE"; then
  echo "FAIL: input pane missing submit acknowledgement"
  echo "--- input capture ---"
  echo "$INPUT_CAPTURE"
  exit 1
fi

if grep -Fq "Response:" <<< "$INPUT_CAPTURE"; then
  echo "FAIL: input pane must not mirror agent response text"
  echo "--- input capture ---"
  echo "$INPUT_CAPTURE"
  exit 1
fi

# System pane assertions: context visualization + prompt cycle only.
for required in "== CONTEXT WINDOW ==" "consumed:" "Top Contributors:" "== PROMPT CYCLE ==" "🤖 Respond"; do
  if ! grep -Fq "$required" <<< "$SYSTEM_CAPTURE"; then
    echo "FAIL: system pane missing required UX content: $required"
    echo "--- system capture ---"
    echo "$SYSTEM_CAPTURE"
    exit 1
  fi
done

for required in "💾 Working Memory" "🧠 System Prompts" "👤 User Prompts" "📎 Attachments" "🤔 Thinking" "🤖 Agent Response" "🔧 Tool Calls" "░ Remaining"; do
  if ! grep -Fq "$required" <<< "$SYSTEM_CAPTURE"; then
    echo "FAIL: system pane missing emoji context row: $required"
    echo "--- system capture ---"
    echo "$SYSTEM_CAPTURE"
    exit 1
  fi
done

# Ensure broken denominator state (e.g. usage 9/1 or consumed .../1) does not recur.
if grep -Eq "(usage:|consumed:).*/1\)?" <<< "$SYSTEM_CAPTURE"; then
  echo "FAIL: system pane shows invalid context denominator of 1"
  echo "--- system capture ---"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi

# Session snapshot/debug surface must not render in system pane.
if grep -Fq "== SESSION SNAPSHOT ==" <<< "$SYSTEM_CAPTURE"; then
  echo "FAIL: system pane should not render session snapshot"
  echo "--- system capture ---"
  echo "$SYSTEM_CAPTURE"
  exit 1
fi

echo "PASS: UX flow e2e (what is 2+2?) matches pane contracts."
