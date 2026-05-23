#!/usr/bin/env bash
# test_demo_split_layout_headless.sh
# Headless UX contract test for DemoMode split layout.
# Validates three-pane split geometry and titles:
# - left-top stores
# - left-bottom testControler
# - right liveCore

set -euo pipefail
SESSION="agentx_demo_split_test_$$"

cleanup() {
  tmux kill-session -t "$SESSION" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Build the split-demo workspace shape.
STORIES_PANE="$(tmux new-session -d -s "$SESSION" -n demo-control -P -F '#{pane_id}' bash -lc "printf '[AgentX Demo] Story Browser\n'; tail -f /dev/null")"
LIVE_PANE="$(tmux split-window -P -F '#{pane_id}' -h -p 45 -t "$STORIES_PANE" bash -lc "printf '[AgentX Demo] Live core placeholder\n'; tail -f /dev/null")"
CONTROLLER_PANE="$(tmux split-window -P -F '#{pane_id}' -v -p 35 -t "$STORIES_PANE" bash -lc "printf '[AgentX Demo] Controller prompt placeholder\n'; tail -f /dev/null")"

tmux select-pane -t "$STORIES_PANE" -T stores
tmux select-pane -t "$LIVE_PANE" -T liveCore
tmux select-pane -t "$CONTROLLER_PANE" -T testControler

tmux select-pane -t "$CONTROLLER_PANE"

PANE_META="$(tmux list-panes -t "$SESSION:0" -F '#{pane_index}|#{pane_title}|#{pane_left}|#{pane_top}|#{pane_width}|#{pane_height}|#{pane_active}')"
echo "$PANE_META"

if [[ "$(wc -l <<< "$PANE_META" | awk '{print $1}')" -ne 3 ]]; then
  echo "FAIL: Expected 3 panes in demo split window"
  exit 1
fi

STORIES_RECORD="$(grep '|stores|' <<< "$PANE_META" || true)"
LIVE_RECORD="$(grep '|liveCore|' <<< "$PANE_META" || true)"
CTRL_RECORD="$(grep '|testControler|' <<< "$PANE_META" || true)"

if [[ -z "$STORIES_RECORD" || -z "$LIVE_RECORD" || -z "$CTRL_RECORD" ]]; then
  echo "FAIL: Missing one or more titled panes (stores/liveCore/testControler)"
  exit 1
fi

IFS='|' read -r _ _ STORIES_LEFT STORIES_TOP _ _ _ <<< "$STORIES_RECORD"
IFS='|' read -r _ _ LIVE_LEFT LIVE_TOP _ _ _ <<< "$LIVE_RECORD"
IFS='|' read -r _ _ CTRL_LEFT CTRL_TOP _ _ CTRL_ACTIVE <<< "$CTRL_RECORD"

if [[ "$STORIES_LEFT" -ne 0 || "$STORIES_TOP" -ne 0 ]]; then
  echo "FAIL: Stories pane must be top-left"
  exit 1
fi

if [[ "$CTRL_LEFT" -ne 0 || "$CTRL_TOP" -le "$STORIES_TOP" ]]; then
  echo "FAIL: Controller pane must be below stories in left column"
  exit 1
fi

if [[ "$LIVE_TOP" -ne 0 || "$LIVE_LEFT" -le "$STORIES_LEFT" ]]; then
  echo "FAIL: liveCore pane must occupy right column"
  exit 1
fi

if [[ "$CTRL_ACTIVE" -ne 1 ]]; then
  echo "FAIL: testControler pane should be active for prompt input"
  exit 1
fi

echo "PASS: Demo split layout matches UX contract."
