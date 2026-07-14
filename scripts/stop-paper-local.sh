#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
PID_DIR="$ROOT_DIR/.runtime/pids"

if [ ! -d "$PID_DIR" ]; then
  echo "no pid directory: $PID_DIR"
  exit 0
fi

if [ -f "$PID_DIR/paper-local-stack.pid" ]; then
  supervisor_pid="$(cat "$PID_DIR/paper-local-stack.pid")"
  if kill -0 "$supervisor_pid" 2>/dev/null; then
    echo "stopping paper-local-stack pid=$supervisor_pid ..."
    kill "$supervisor_pid" 2>/dev/null || true
    sleep 2
  fi
fi

stop_service() {
  local pid_file="$1"
  local name
  name="$(basename "$pid_file" .pid)"
  local pid
  pid="$(cat "$pid_file")"

  if kill -0 "$pid" 2>/dev/null; then
    echo "stopping $name pid=$pid ..."
    kill "$pid" 2>/dev/null || true
    for _ in 1 2 3 4 5; do
      if ! kill -0 "$pid" 2>/dev/null; then
        break
      fi
      sleep 1
    done
    if kill -0 "$pid" 2>/dev/null; then
      echo "$name still running, sending SIGKILL ..."
      kill -9 "$pid" 2>/dev/null || true
    fi
  else
    echo "$name not running"
  fi

  rm -f "$pid_file"
}

for pid_file in "$PID_DIR"/*.pid; do
  [ -e "$pid_file" ] || break
  stop_service "$pid_file"
done

echo "local paper stack stopped."
