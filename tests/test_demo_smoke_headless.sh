#!/usr/bin/env bash
# test_demo_smoke_headless.sh
# Headless smoke test for DemoMode artifact capture.
# It runs the AgentX binary in --demo-headless mode against a fake tmux executable,
# sends X at the first prompt, and verifies deterministic artifact output.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
USERNAME="demo-smoke"
SESSION_ID="smoke_${RANDOM}_$$"
ARTIFACT_DIR="$ROOT_DIR/logs/demo/$SESSION_ID/e2e-001"
DEMO_OUTPUT_FILE="$TMP_DIR/demo_output.log"

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
  if [[ "$3" == *"_demo" ]]; then
    echo 'demo_smoke_session_demo|0|0|%8|testControler|1|120|20'
    echo 'demo_smoke_session_demo|0|1|%9|liveCore|0|120|40'
    exit 0
  fi
  echo 'demo_smoke_session|0|0|%1|output|0|140|30'
  echo 'demo_smoke_session|0|1|%2|system|1|140|18'
  echo 'demo_smoke_session|0|2|%3|input|0|140|12'
  echo 'demo_smoke_session|1|0|%4|logs|0|140|24'
  exit 0
fi

if [[ "$1" == "list-windows" ]]; then
  if [[ "$3" == *"_demo" ]]; then
    echo '0|demo-control|1'
  else
    echo '0|tui-chat|1'
    echo '1|logs|0'
  fi
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

if [[ "$1" == "new-session" || "$1" == "select-pane" || "$1" == "split-window" || "$1" == "new-window" || "$1" == "select-window" || "$1" == "respawn-pane" || "$1" == "send-keys" ]]; then
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
  --demo-headless \
  --demo-start e2e-001 \
  <<'EOF_INPUT' | tee "$DEMO_OUTPUT_FILE"
X
EOF_INPUT

if [[ ! -d "$ARTIFACT_DIR" ]]; then
  echo "FAIL: expected artifact directory $ARTIFACT_DIR"
  exit 1
fi

for marker in '[AgentX Demo] Ordered test sequence (Gherkin):' 'e2e-greet-001' 'e2e-cycle-001' 'e2e-system-001'; do
  if ! grep -Fq "$marker" "$DEMO_OUTPUT_FILE"; then
    echo "FAIL: expected parity evidence marker in demo output: $marker"
    exit 1
  fi
done

for marker in '[AgentX Demo] Failed test: e2e-001' '[AgentX Demo] Artifact paths:' '[AgentX Demo] Readiness: Not ready for UAT'; do
  if ! grep -Fq "$marker" "$DEMO_OUTPUT_FILE"; then
    echo "FAIL: expected summary marker in demo output: $marker"
    exit 1
  fi
done

for required in metadata.json core_tmux_list_windows.txt core_tmux_list_panes.txt core_tmux_display_message.txt split_tmux_list_windows.txt split_tmux_list_panes.txt split_tmux_display_message.txt tmux_list_panes.txt tmux_display_message.txt; do
  if [[ ! -f "$ARTIFACT_DIR/$required" ]]; then
    echo "FAIL: missing artifact file $ARTIFACT_DIR/$required"
    exit 1
  fi
done

if ! grep -q '^0|tui-chat|1$' "$ARTIFACT_DIR/core_tmux_list_windows.txt"; then
  echo "FAIL: expected active core window metadata for tui-chat"
  exit 1
fi
if ! grep -q '^1|logs|0$' "$ARTIFACT_DIR/core_tmux_list_windows.txt"; then
  echo "FAIL: expected logs window metadata in core window list"
  exit 1
fi
if ! grep -q '^0|demo-control|1$' "$ARTIFACT_DIR/split_tmux_list_windows.txt"; then
  echo "FAIL: expected active split demo-control window metadata"
  exit 1
fi

CORE_PANES_FILE="$ARTIFACT_DIR/core_tmux_list_panes.txt"
if [[ ! -s "$CORE_PANES_FILE" ]]; then
  echo "FAIL: expected non-empty pane metadata in $CORE_PANES_FILE"
  exit 1
fi

for expected in '|%1|output|0|140|30' '|%2|system|1|140|18' '|%3|input|0|140|12' '|%4|logs|0|140|24'; do
  if ! grep -q "$expected" "$CORE_PANES_FILE"; then
    echo "FAIL: expected core pane metadata fragment $expected"
    exit 1
  fi
done

SPLIT_PANES_FILE="$ARTIFACT_DIR/split_tmux_list_panes.txt"
for expected in '|%8|testControler|1|120|20' '|%9|liveCore|0|120|40'; do
  if ! grep -q "$expected" "$SPLIT_PANES_FILE"; then
    echo "FAIL: expected split pane metadata fragment $expected"
    exit 1
  fi
done

while IFS='|' read -r _ _ _ PANE_ID _ _; do
  [[ -z "${PANE_ID:-}" ]] && continue
  if [[ ! -f "$ARTIFACT_DIR/core_pane_${PANE_ID}.txt" ]]; then
    echo "FAIL: missing core pane capture for $PANE_ID"
    exit 1
  fi
  if [[ ! -f "$ARTIFACT_DIR/pane_${PANE_ID}.txt" ]]; then
    echo "FAIL: missing legacy pane capture for $PANE_ID"
    exit 1
  fi
done < "$CORE_PANES_FILE"

echo "PASS: demo smoke artifact capture matches UX contract."