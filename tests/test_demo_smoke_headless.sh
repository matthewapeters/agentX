#!/usr/bin/env bash
# test_demo_smoke_headless.sh
# Headless smoke test for DemoMode artifact capture.
# It runs the AgentX binary in --demo mode against a fake tmux executable,
# sends X at the first prompt, and verifies deterministic artifact output.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
USERNAME="demo-smoke"
SESSION_ID="smoke_${RANDOM}_$$"
TMUX_SESSION="demo_smoke_session"
ARTIFACT_DIR="$ROOT_DIR/logs/demo/$SESSION_ID/e2e-001"

cleanup() {
  rm -rf "$TMP_DIR"
  rm -rf "$ROOT_DIR/logs/demo/$SESSION_ID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

cat > "$TMP_DIR/tmux" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "$1" == "list-panes" ]]; then
  echo '%1|chat'
  echo '%2|context'
  echo '%3|input'
  echo '%4|logs'
  exit 0
fi

if [[ "$1" == "display-message" ]]; then
  echo 'demo_smoke_session:0.0'
  exit 0
fi

if [[ "$1" == "capture-pane" ]]; then
  case "$*" in
    *"%1"*) echo 'chat pane content' ;;
    *"%2"*) echo 'context pane content' ;;
    *"%3"*) echo 'input pane content' ;;
    *"%4"*) echo 'logs pane content' ;;
    *) echo "unexpected pane target: $*" >&2 ; exit 1 ;;
  esac
  exit 0
fi

if [[ "$1" == "new-session" || "$1" == "select-pane" || "$1" == "split-window" || "$1" == "new-window" || "$1" == "select-window" ]]; then
  exit 0
fi

echo "unexpected tmux args: $*" >&2
exit 1
EOF

chmod +x "$TMP_DIR/tmux"

PATH="$TMP_DIR:$PATH" \
  "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --demo \
  --demo-start e2e-001 \
  --demo-tmux-session "$TMUX_SESSION" \
  <<'EOF_INPUT'
X
EOF_INPUT

if [[ ! -d "$ARTIFACT_DIR" ]]; then
  echo "FAIL: expected artifact directory $ARTIFACT_DIR"
  exit 1
fi

for required in metadata.json tmux_list_panes.txt tmux_display_message.txt pane_%1.txt pane_%2.txt pane_%3.txt pane_%4.txt; do
  if [[ ! -f "$ARTIFACT_DIR/$required" ]]; then
    echo "FAIL: missing artifact file $ARTIFACT_DIR/$required"
    exit 1
  fi
done

echo "PASS: demo smoke artifact capture matches UX contract."