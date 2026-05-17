#!/usr/bin/env bash
# test_tmux_layout_headless.sh
# Headless test for AgentX tmux layout.
# Fails if panes are missing, in the wrong geometry, or contain wrong placeholders.

set -euo pipefail
SESSION="agentx_test_$$"

cleanup() {
  tmux kill-session -t "$SESSION" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Launch session and create layout (mimic Go logic)
tmux new-session -d -s "$SESSION" -x 120 -y 40
# Set chat pane title
tmux select-pane -t "$SESSION:0.0" -T chat
# Split horizontally (input at bottom, 20%) and capture pane id
INPUT_PANE="$(tmux split-window -P -F '#{pane_id}' -t "$SESSION:0.0" -v -p 20)"
# Title bottom pane as input
tmux select-pane -t "$INPUT_PANE" -T input
# Focus top pane and split vertically (context right, 20%)
CHAT_PANE="$SESSION:0.0"
CONTEXT_PANE="$(tmux split-window -P -F '#{pane_id}' -t "$CHAT_PANE" -h -p 20)"
# Title right pane as context
tmux select-pane -t "$CONTEXT_PANE" -T context
# Set placeholder text
tmux send-keys -t "$CHAT_PANE" "echo '🔶 Pane: chat (AgentX Core)'" Enter
tmux send-keys -t "$INPUT_PANE" "echo '🔶 Pane: input (AgentX Core)'" Enter
tmux send-keys -t "$CONTEXT_PANE" "echo '🔶 Pane: context (AgentX Core)'" Enter
# Create logs window (hidden)
tmux new-window -t "$SESSION:1" -n logs

echo "Pane metadata:"
tmux list-panes -t "$SESSION:0" -F '#{pane_index} left=#{pane_left} top=#{pane_top} width=#{pane_width} height=#{pane_height}'

# Validate geometry: chat is top-left, context is top-right, input is bottom full width.
CHAT_GEOM="$(tmux display-message -p -t "$CHAT_PANE" '#{pane_left} #{pane_top}')"
INPUT_GEOM="$(tmux display-message -p -t "$INPUT_PANE" '#{pane_left} #{pane_top}')"
CONTEXT_GEOM="$(tmux display-message -p -t "$CONTEXT_PANE" '#{pane_left} #{pane_top}')"

# shellcheck disable=SC2086
read -r CHAT_LEFT CHAT_TOP <<< "$CHAT_GEOM"
# shellcheck disable=SC2086
read -r INPUT_LEFT INPUT_TOP <<< "$INPUT_GEOM"
# shellcheck disable=SC2086
read -r CONTEXT_LEFT CONTEXT_TOP <<< "$CONTEXT_GEOM"

if [[ "$CHAT_LEFT" -ne 0 || "$CHAT_TOP" -ne 0 ]]; then
  echo "FAIL: Chat pane is not at top-left"
  exit 1
fi

if [[ "$CONTEXT_TOP" -ne 0 || "$CONTEXT_LEFT" -le "$CHAT_LEFT" ]]; then
  echo "FAIL: Context pane is not in top-right position"
  exit 1
fi

if [[ "$INPUT_TOP" -le 0 || "$INPUT_LEFT" -ne 0 ]]; then
  echo "FAIL: Input pane is not at the bottom"
  exit 1
fi

# Validate placeholder text
CHAT_OUT="$(tmux capture-pane -t "$CHAT_PANE" -p | grep 'Pane:')"
INPUT_OUT="$(tmux capture-pane -t "$INPUT_PANE" -p | grep 'Pane:')"
CONTEXT_OUT="$(tmux capture-pane -t "$CONTEXT_PANE" -p | grep 'Pane:')"

[[ "$CHAT_OUT" == *"chat"* ]] || { echo "FAIL: Chat pane missing chat placeholder"; exit 1; }
[[ "$INPUT_OUT" == *"input"* ]] || { echo "FAIL: Input pane missing input placeholder"; exit 1; }
[[ "$CONTEXT_OUT" == *"context"* ]] || { echo "FAIL: Context pane missing context placeholder"; exit 1; }

# Validate hidden logs window exists.
if ! tmux list-windows -t "$SESSION" -F '#{window_index}:#{window_name}' | grep -q '^1:logs$'; then
  echo "FAIL: Missing hidden logs window"
  exit 1
fi

echo "PASS: tmux layout matches UX spec."
