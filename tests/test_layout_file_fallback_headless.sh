#!/usr/bin/env bash
# test_layout_file_fallback_headless.sh
# Verifies --layout-file gracefully falls back when file is invalid/unloadable.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="$ROOT_DIR/bin/agentx"
TMP_DIR="$(mktemp -d)"
USERNAME="layout-fallback"
SESSION_ID="layout_fallback_${RANDOM}_$$"
OUTPUT_FILE="$TMP_DIR/fallback_output.log"
LAYOUT_FILE="$TMP_DIR/bad-layout.yaml"

cleanup() {
  rm -rf "$TMP_DIR"
  tmux kill-session -t "agentx_${USERNAME}_${SESSION_ID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$BIN_PATH" ]]; then
  echo "FAIL: missing executable $BIN_PATH (run 'make build-core')"
  exit 1
fi

# Intentionally malformed YAML to force tmuxp load failure when tmuxp is present.
cat >"$LAYOUT_FILE" <<'EOF_BAD'
session_name: ${SESSION}
windows:
  - window_name: tui-chat
    panes
      - shell_command: ""
EOF_BAD

AGENTX_CHAT_BACKEND=echo "$BIN_PATH" \
  --project-dir "$ROOT_DIR" \
  --user "$USERNAME" \
  --session-id "$SESSION_ID" \
  --layout-file "$LAYOUT_FILE" \
  --demo-headless \
  --demo-start e2e-greet-001 \
  <<'EOF_INPUT' 2>&1 | tee "$OUTPUT_FILE"
X fallback probe
EOF_INPUT

for marker in \
  '[AgentX Demo] Running test 1/3: e2e-greet-001' \
  '[AgentX Demo] Status ledger:' \
  '[AgentX Demo] Readiness: Not ready for UAT'; do
  if ! grep -Fq "$marker" "$OUTPUT_FILE"; then
    echo "FAIL: expected fallback-run marker missing: $marker"
    exit 1
  fi
done

if ! grep -Eq 'Layout overlay (skipped|failed)' "$OUTPUT_FILE"; then
  echo "FAIL: expected layout overlay fallback signal not found in output"
  exit 1
fi

echo "PASS: --layout-file fallback path is non-fatal and demo runtime continues."
