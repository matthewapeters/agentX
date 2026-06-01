#!/usr/bin/env bash
# test_demo_system_panel_tour_headless.sh
# Runs the dedicated system-panel tour demo use-case via --demo-headless
# and verifies the new tour story passes when started directly.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
USERNAME="demo-tour"
SESSION_ID="demo_tour_${RANDOM}_$$"
OUTPUT_FILE="$TMP_DIR/demo_tour_output.log"

cleanup() {
  rm -rf "$TMP_DIR"
  tmux kill-session -t "agentx_${USERNAME}_${SESSION_ID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

AGENTX_CHAT_BACKEND=ollama "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --demo-headless \
  --demo-start e2e-system-tour-001 \
  <<'EOF_INPUT' | tee "$OUTPUT_FILE"
N
N
N
N
N
N
EOF_INPUT

for marker in \
  '[AgentX Demo] Running test 1/6: e2e-system-tour-001' \
  '[AgentX Demo] Running test 2/6: e2e-greet-001' \
  '[AgentX Demo] Running test 3/6: e2e-cycle-001' \
  '[AgentX Demo] Running test 4/6: e2e-input-001' \
  '[AgentX Demo] Running test 5/6: e2e-logs-001' \
  '[AgentX Demo] Running test 6/6: e2e-system-001' \
  '[AgentX Demo]   - e2e-system-tour-001: PASS' \
  '[AgentX Demo]   - e2e-greet-001: PASS' \
  '[AgentX Demo]   - e2e-cycle-001: PASS' \
  '[AgentX Demo]   - e2e-input-001: PASS' \
  '[AgentX Demo]   - e2e-logs-001: PASS' \
  '[AgentX Demo]   - e2e-system-001: PASS' \
  '[AgentX Demo] Readiness: Ready for UAT'; do
  if ! grep -Fq "$marker" "$OUTPUT_FILE"; then
    echo "FAIL: expected system-panel tour marker missing: $marker"
    exit 1
  fi
done

if grep -Fq '[AgentX Demo] Failed test:' "$OUTPUT_FILE"; then
  echo "FAIL: system-panel tour demo reported failed test"
  exit 1
fi

echo "PASS: system-panel tour demo use-case passes through --demo-headless."
