# Headless tmux Layout Test for AgentX

This script launches the AgentX tmux layout, captures pane metadata, and validates the UX layout programmatically.

## Steps

1. Launch AgentX Core (or the tmux layout script) in a headless session.
2. Use `tmux list-panes` and `tmux display-message` to capture pane titles, indices, and placeholder text.
3. Parse the output to confirm:
   - All expected panes (chat, context, input) exist
   - Each pane is in the correct position (by index and title)
   - Primary window `0:tui-chat` is active after logs window creation
   - Logs window exists as `1:logs` and remains inactive
   - Placeholder text matches the intended UX
4. Assert the layout matches the UX spec:
   - Active window: `0:tui-chat`
   - Top left: chat
   - Top right: context
   - Bottom: input (full width)
   - Pane order in primary window: index 0=chat, 1=context, 2=input
   - Hidden/logs window present as `1:logs`
5. Report pass/fail with details for CI or developer review.

## Example (Bash)

```bash
SESSION="agentx_test_$$"
tmux new-session -d -s "$SESSION" -x 120 -y 40
# ... (run the same split/rename commands as AgentX Core) ...
# Capture pane info:
tmux list-panes -t "$SESSION:0" -F '#{pane_index} #{pane_title} #{pane_current_command}' > panes.txt
cat panes.txt
# Optionally, capture placeholder text from each pane:
for i in 0 1 2; do
  tmux capture-pane -t "$SESSION:0.$i" -p > pane_$i.txt
done
# Clean up:
tmux kill-session -t "$SESSION"
```

## Integration

- Add this script to CI to validate layout after every change.
- Parse output in Python/Go to assert correctness.
- Fail the build if layout does not match spec.

---
_Last updated: 2026-05-19 (v0.56.1)_
