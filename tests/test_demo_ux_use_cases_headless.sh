#!/usr/bin/env bash
# test_demo_ux_use_cases_headless.sh
# Runs five basic UX parity demo use-cases via --demo-headless and verifies summary outcomes.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
USERNAME="demo-ux"
SESSION_ID="demo_ux_${RANDOM}_$$"
OUTPUT_FILE="$TMP_DIR/demo_ux_output.log"

cleanup() {
  rm -rf "$TMP_DIR"
  tmux kill-session -t "agentx_${USERNAME}_${SESSION_ID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

# Start at the first UX parity use-case and accept all five if they pass.
AGENTX_CHAT_BACKEND=ollama "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --demo-headless \
  --demo-start e2e-greet-001 \
  <<'EOF_INPUT' | tee "$OUTPUT_FILE"
N
N
N
N
N
EOF_INPUT

# Validate the five intended use-cases were executed and reported.
for marker in \
  '[AgentX Demo] Running test 1/5: e2e-greet-001' \
  '[AgentX Demo] Running test 2/5: e2e-cycle-001' \
  '[AgentX Demo] Running test 3/5: e2e-input-001' \
  '[AgentX Demo] Running test 4/5: e2e-logs-001' \
  '[AgentX Demo] Running test 5/5: e2e-system-001'; do
  if ! grep -Fq "$marker" "$OUTPUT_FILE"; then
    echo "FAIL: expected demo run marker missing: $marker"
    exit 1
  fi
done

# Status ledger must show all five as PASS with UAT readiness.
for marker in \
  '[AgentX Demo]   - e2e-greet-001: PASS' \
  '[AgentX Demo]   - e2e-cycle-001: PASS' \
  '[AgentX Demo]   - e2e-input-001: PASS' \
  '[AgentX Demo]   - e2e-logs-001: PASS' \
  '[AgentX Demo]   - e2e-system-001: PASS' \
  '[AgentX Demo] Readiness: Ready for UAT'; do
  if ! grep -Fq "$marker" "$OUTPUT_FILE"; then
    echo "FAIL: expected PASS/readiness marker missing: $marker"
    exit 1
  fi
done

if grep -Fq '[AgentX Demo] Failed test:' "$OUTPUT_FILE"; then
  echo "FAIL: demo reported failed test despite all-accept flow"
  exit 1
fi

echo "PASS: demo UX use-cases (greet/cycle/input/logs/system) pass through --demo-headless."
