#!/usr/bin/env bash
# test_demo_ux_use_cases_layout_headless.sh
# Runs three UX parity demo use-cases through --demo-headless with --layout-file overlay.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
USERNAME="demo-layout"
SESSION_ID="demo_layout_${RANDOM}_$$"
OUTPUT_FILE="$TMP_DIR/demo_layout_output.log"
LAYOUT_FILE="$TMP_DIR/layout.yaml"

cleanup() {
  rm -rf "$TMP_DIR"
  tmux kill-session -t "agentx_${USERNAME}_${SESSION_ID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

cat >"$LAYOUT_FILE" <<'EOF_LAYOUT'
# Overlay pane topology only; AgentX owns session/windows.
session_name: ${SESSION}
windows:
  - window_name: tui-chat
    panes:
      - shell_command: ""
      - shell_command: ""
      - shell_command: ""
  - window_name: logs
    panes:
      - shell_command: ""
EOF_LAYOUT

if ! command -v tmuxp >/dev/null 2>&1; then
  echo "SKIP: tmuxp not found; cannot validate --layout-file overlay path"
  exit 0
fi

AGENTX_CHAT_BACKEND=ollama "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --layout-file "$LAYOUT_FILE" \
  --demo-headless \
  --demo-start e2e-greet-001 \
  <<'EOF_INPUT' | tee "$OUTPUT_FILE"
N
N
N
EOF_INPUT

for marker in \
  '[AgentX Demo] Running test 1/3: e2e-greet-001' \
  '[AgentX Demo] Running test 2/3: e2e-cycle-001' \
  '[AgentX Demo] Running test 3/3: e2e-system-001' \
  '[AgentX Demo]   - e2e-greet-001: PASS' \
  '[AgentX Demo]   - e2e-cycle-001: PASS' \
  '[AgentX Demo]   - e2e-system-001: PASS' \
  '[AgentX Demo] Readiness: Ready for UAT'; do
  if ! grep -Fq "$marker" "$OUTPUT_FILE"; then
    echo "FAIL: expected layout-path demo marker missing: $marker"
    exit 1
  fi
done

if grep -Fq '[AgentX Demo] Failed test:' "$OUTPUT_FILE"; then
  echo "FAIL: layout-path demo reported failed test"
  exit 1
fi

echo "PASS: demo UX use-cases pass through --demo-headless with --layout-file overlay."
