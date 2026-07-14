#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
PID_DIR="$ROOT_DIR/.runtime/pids"
LOG_DIR="$ROOT_DIR/.runtime/logs"

if [ ! -d "$PID_DIR" ]; then
  echo "no services have been started yet."
  exit 0
fi

for pid_file in "$PID_DIR"/*.pid; do
  [ -e "$pid_file" ] || {
    echo "no pid files found."
    exit 0
  }

  name="$(basename "$pid_file" .pid)"
  pid="$(cat "$pid_file")"
  log_file="$LOG_DIR/$name.log"

  if kill -0 "$pid" 2>/dev/null; then
    echo "$name: running pid=$pid log=$log_file"
  else
    echo "$name: stopped stale_pid=$pid log=$log_file"
  fi
done
