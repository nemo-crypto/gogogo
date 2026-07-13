#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/Users/guilinzhou/Desktop/test-nemo/gogogo}"
RUNTIME_DIR="$ROOT_DIR/.runtime"
PID_DIR="$RUNTIME_DIR/pids"
LOG_DIR="$RUNTIME_DIR/logs"
PID_FILE="$PID_DIR/paper-local-stack.pid"
HTTP_ADDR="${HTTP_ADDR:-:8082}"

mkdir -p "$PID_DIR" "$LOG_DIR"

if [ -f "$PID_FILE" ]; then
  old_pid="$(cat "$PID_FILE")"
  if kill -0 "$old_pid" 2>/dev/null; then
    echo "paper local stack already running: pid=$old_pid"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

echo "starting paper local stack ..."
nohup "$ROOT_DIR/scripts/run-paper-local-stack.sh" >>"$LOG_DIR/supervisor.log" 2>&1 &
pid="$!"
echo "$pid" >"$PID_FILE"

sleep 1
if ! kill -0 "$pid" 2>/dev/null; then
  echo "paper local stack failed to start. log=$LOG_DIR/supervisor.log"
  exit 1
fi

if [[ "$HTTP_ADDR" == :* ]]; then
  DASHBOARD_URL="http://localhost$HTTP_ADDR"
else
  DASHBOARD_URL="http://$HTTP_ADDR"
fi

cat <<EOF
paper local stack is running.

dashboard:
  $DASHBOARD_URL

logs:
  $LOG_DIR

status:
  $ROOT_DIR/scripts/status-paper-local.sh

stop:
  $ROOT_DIR/scripts/stop-paper-local.sh

EOF
