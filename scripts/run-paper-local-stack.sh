#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/Users/guilinzhou/Desktop/test-nemo/gogogo}"
DSN="${DATABASE_DSN:-$ROOT_DIR/data.db}"
SYMBOL="${SYMBOL:-BTCUSDT}"
ACCOUNT="${ACCOUNT:-paper}"
PROFILE="${PROFILE:-aggressive}"
EQUITY="${EQUITY:-1000}"
HTTP_ADDR="${HTTP_ADDR:-:8082}"
POLL_INTERVAL="${POLL_INTERVAL:-15s}"
FUNDING_POLL_INTERVAL="${FUNDING_POLL_INTERVAL:-5m}"
INTERVAL="${INTERVAL:-}"

if [ -z "$INTERVAL" ]; then
  case "$PROFILE" in
    aggressive|small-aggressive|small_aggressive|small)
      INTERVAL="3m"
      ;;
    *)
      INTERVAL="5m"
      ;;
  esac
fi

CONFIG_DSN="$DSN"
CONFIG_SYMBOL="$SYMBOL"
CONFIG_ACCOUNT="$ACCOUNT"
CONFIG_PROFILE="$PROFILE"
CONFIG_EQUITY="$EQUITY"
CONFIG_HTTP_ADDR="$HTTP_ADDR"
CONFIG_POLL_INTERVAL="$POLL_INTERVAL"
CONFIG_FUNDING_POLL_INTERVAL="$FUNDING_POLL_INTERVAL"
CONFIG_INTERVAL="$INTERVAL"

RUNTIME_DIR="$ROOT_DIR/.runtime"
PID_DIR="$RUNTIME_DIR/pids"
LOG_DIR="$RUNTIME_DIR/logs"

mkdir -p "$PID_DIR" "$LOG_DIR"
cd "$ROOT_DIR"

if [ -f "$ROOT_DIR/.env.local" ]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env.local"
  set +a
fi

DSN="$CONFIG_DSN"
SYMBOL="$CONFIG_SYMBOL"
ACCOUNT="$CONFIG_ACCOUNT"
PROFILE="$CONFIG_PROFILE"
EQUITY="$CONFIG_EQUITY"
HTTP_ADDR="$CONFIG_HTTP_ADDR"
POLL_INTERVAL="$CONFIG_POLL_INTERVAL"
FUNDING_POLL_INTERVAL="$CONFIG_FUNDING_POLL_INTERVAL"
INTERVAL="$CONFIG_INTERVAL"

export DATABASE_DSN="$DSN"
export EXCHANGE_NAME="${EXCHANGE_NAME:-onebullex}"
export ONEBULLEX_LIVE_TRADING=false
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"

children=()

cleanup() {
  for pid in "${children[@]:-}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  sleep 1
  for pid in "${children[@]:-}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill -9 "$pid" 2>/dev/null || true
    fi
  done
}

trap cleanup EXIT INT TERM

start_child() {
  local name="$1"
  shift
  local log_file="$LOG_DIR/$name.log"
  echo "starting $name ..."
  "$@" >>"$log_file" 2>&1 &
  local pid="$!"
  children+=("$pid")
  echo "$pid" >"$PID_DIR/$name.pid"
  echo "$name started: pid=$pid log=$log_file"
}

echo "$$" >"$PID_DIR/paper-local-stack.pid"
echo "initializing database schema ..."
go run ./cmd/quantdb >>"$LOG_DIR/quantdb.log" 2>&1

start_child "marketsync-klines" \
  go run ./cmd/marketsync \
    -dsn "$DSN" \
    -dataset klines \
    -exchange onebullex \
    -market perpetual \
    -symbols "$SYMBOL" \
    -interval "$INTERVAL" \
    -limit 120 \
    -watch \
    -poll-interval "$POLL_INTERVAL"

start_child "marketsync-mark-price" \
  go run ./cmd/marketsync \
    -dsn "$DSN" \
    -dataset mark-price \
    -exchange onebullex \
    -market perpetual \
    -symbols "$SYMBOL" \
    -watch \
    -poll-interval "$POLL_INTERVAL"

start_child "marketsync-funding" \
  go run ./cmd/marketsync \
    -dsn "$DSN" \
    -dataset funding \
    -exchange onebullex \
    -market perpetual \
    -symbols "$SYMBOL" \
    -watch \
    -poll-interval "$FUNDING_POLL_INTERVAL"

start_child "papertrade" \
  go run ./cmd/papertrade \
    -dsn "$DSN" \
    -account "$ACCOUNT" \
    -profile "$PROFILE" \
    -symbol "$SYMBOL" \
    -interval "$INTERVAL" \
    -equity "$EQUITY" \
    -watch \
    -poll-interval "$POLL_INTERVAL"

start_child "dashboard" \
  env HTTP_ADDR="$HTTP_ADDR" DATABASE_DSN="$DSN" go run ./cmd/dashboard

echo "paper local stack supervisor started."

while true; do
  for pid in "${children[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "child process stopped: pid=$pid"
      exit 1
    fi
  done
  sleep 5
done
