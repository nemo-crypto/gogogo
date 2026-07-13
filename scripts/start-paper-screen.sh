#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/Users/guilinzhou/Desktop/test-nemo/gogogo}"
SESSION="${SESSION:-gogogo-paper}"

if screen -list | grep -q "[.]$SESSION[[:space:]]"; then
  echo "screen session already running: $SESSION"
  echo "attach: screen -r $SESSION"
  exit 0
fi

screen -dmS "$SESSION" bash -lc "cd '$ROOT_DIR' && ./scripts/run-paper-local-stack.sh"

echo "screen session started: $SESSION"
echo "attach: screen -r $SESSION"
echo "detach after attach: Ctrl+A then D"
echo "status: ./scripts/status-paper-local.sh"
echo "stop: ./scripts/stop-paper-local.sh && screen -S $SESSION -X quit"
