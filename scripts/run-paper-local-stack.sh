#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DSN="${DATABASE_DSN:-$ROOT_DIR/data.db}"
SYMBOL="${SYMBOL:-BTCUSDT}"
ACCOUNT="${ACCOUNT:-paper}"
PROFILE="${PROFILE:-aggressive}"
EQUITY="${EQUITY:-1000}"
POSITION_MODEL="${POSITION_MODEL:-}"
SUBMIT_EXCHANGE="${SUBMIT_EXCHANGE:-false}"
LIVE_TRADING="${ONEBULLEX_LIVE_TRADING:-${LIVE_TRADING:-false}}"
HTTP_ADDR="${HTTP_ADDR:-:8082}"
POLL_INTERVAL="${POLL_INTERVAL:-15s}"
FUNDING_POLL_INTERVAL="${FUNDING_POLL_INTERVAL:-5m}"
INTERVAL="${INTERVAL:-}"
TREND_FILTER="${TREND_FILTER:-true}"
TREND_INTERVAL="${TREND_INTERVAL:-15m}"
MACRO_TREND_INTERVAL="${MACRO_TREND_INTERVAL:-1h}"
TREND_FAST="${TREND_FAST:-20}"
TREND_SLOW="${TREND_SLOW:-60}"
TREND_MIN_SPREAD_PCT="${TREND_MIN_SPREAD_PCT:-0.05}"
BREAKEVEN_STOP="${BREAKEVEN_STOP:-true}"
BREAKEVEN_TRIGGER_R="${BREAKEVEN_TRIGGER_R:-1.0}"
TRAILING_STOP="${TRAILING_STOP:-true}"
TRAILING_ACTIVATION_R="${TRAILING_ACTIVATION_R:-1.5}"
TRAILING_ATR_MULT="${TRAILING_ATR_MULT:-1.2}"

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
CONFIG_POSITION_MODEL="$POSITION_MODEL"
CONFIG_SUBMIT_EXCHANGE="$SUBMIT_EXCHANGE"
CONFIG_LIVE_TRADING="$LIVE_TRADING"
CONFIG_HTTP_ADDR="$HTTP_ADDR"
CONFIG_POLL_INTERVAL="$POLL_INTERVAL"
CONFIG_FUNDING_POLL_INTERVAL="$FUNDING_POLL_INTERVAL"
CONFIG_INTERVAL="$INTERVAL"
CONFIG_TREND_FILTER="$TREND_FILTER"
CONFIG_TREND_INTERVAL="$TREND_INTERVAL"
CONFIG_MACRO_TREND_INTERVAL="$MACRO_TREND_INTERVAL"
CONFIG_TREND_FAST="$TREND_FAST"
CONFIG_TREND_SLOW="$TREND_SLOW"
CONFIG_TREND_MIN_SPREAD_PCT="$TREND_MIN_SPREAD_PCT"
CONFIG_BREAKEVEN_STOP="$BREAKEVEN_STOP"
CONFIG_BREAKEVEN_TRIGGER_R="$BREAKEVEN_TRIGGER_R"
CONFIG_TRAILING_STOP="$TRAILING_STOP"
CONFIG_TRAILING_ACTIVATION_R="$TRAILING_ACTIVATION_R"
CONFIG_TRAILING_ATR_MULT="$TRAILING_ATR_MULT"

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
POSITION_MODEL="$CONFIG_POSITION_MODEL"
SUBMIT_EXCHANGE="$CONFIG_SUBMIT_EXCHANGE"
LIVE_TRADING="$CONFIG_LIVE_TRADING"
HTTP_ADDR="$CONFIG_HTTP_ADDR"
POLL_INTERVAL="$CONFIG_POLL_INTERVAL"
FUNDING_POLL_INTERVAL="$CONFIG_FUNDING_POLL_INTERVAL"
INTERVAL="$CONFIG_INTERVAL"
TREND_FILTER="$CONFIG_TREND_FILTER"
TREND_INTERVAL="$CONFIG_TREND_INTERVAL"
MACRO_TREND_INTERVAL="$CONFIG_MACRO_TREND_INTERVAL"
TREND_FAST="$CONFIG_TREND_FAST"
TREND_SLOW="$CONFIG_TREND_SLOW"
TREND_MIN_SPREAD_PCT="$CONFIG_TREND_MIN_SPREAD_PCT"
BREAKEVEN_STOP="$CONFIG_BREAKEVEN_STOP"
BREAKEVEN_TRIGGER_R="$CONFIG_BREAKEVEN_TRIGGER_R"
TRAILING_STOP="$CONFIG_TRAILING_STOP"
TRAILING_ACTIVATION_R="$CONFIG_TRAILING_ACTIVATION_R"
TRAILING_ATR_MULT="$CONFIG_TRAILING_ATR_MULT"

export DATABASE_DSN="$DSN"
export EXCHANGE_NAME="${EXCHANGE_NAME:-onebullex}"
export ONEBULLEX_LIVE_TRADING="$LIVE_TRADING"
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"

if [ "$SUBMIT_EXCHANGE" = "true" ] && [ "$ONEBULLEX_LIVE_TRADING" != "true" ]; then
  echo "SUBMIT_EXCHANGE=true requires ONEBULLEX_LIVE_TRADING=true"
  exit 1
fi

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

start_kline_sync() {
  local name="$1"
  local interval="$2"
  start_child "$name" \
    go run ./cmd/marketsync \
      -dsn "$DSN" \
      -dataset klines \
      -exchange onebullex \
      -market perpetual \
      -symbols "$SYMBOL" \
      -interval "$interval" \
      -limit 120 \
      -watch \
      -poll-interval "$POLL_INTERVAL"
}

echo "$$" >"$PID_DIR/paper-local-stack.pid"
echo "initializing database schema ..."
go run ./cmd/quantdb >>"$LOG_DIR/quantdb.log" 2>&1

if [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
  echo "syncing live account snapshot ..."
  go run ./cmd/accountsnapshot \
    -dsn "$DSN" \
    -account "$ACCOUNT" \
    -exchange onebullex \
    -market perpetual \
    -symbol "$SYMBOL" \
    -sync-live >>"$LOG_DIR/accountsnapshot.log" 2>&1
fi

start_kline_sync "marketsync-klines-$INTERVAL" "$INTERVAL"

if [ "$TREND_FILTER" = "true" ]; then
  if [ "$TREND_INTERVAL" != "$INTERVAL" ]; then
    start_kline_sync "marketsync-klines-$TREND_INTERVAL" "$TREND_INTERVAL"
  fi
  if [ "$MACRO_TREND_INTERVAL" != "$INTERVAL" ] && [ "$MACRO_TREND_INTERVAL" != "$TREND_INTERVAL" ]; then
    start_kline_sync "marketsync-klines-$MACRO_TREND_INTERVAL" "$MACRO_TREND_INTERVAL"
  fi
fi

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

papertrade_cmd=(
  go run ./cmd/papertrade
  -dsn "$DSN"
  -account "$ACCOUNT"
  -profile "$PROFILE"
  -symbol "$SYMBOL"
  -interval "$INTERVAL"
  -equity "$EQUITY"
  -trend-filter="$TREND_FILTER"
  -trend-interval "$TREND_INTERVAL"
  -macro-trend-interval "$MACRO_TREND_INTERVAL"
  -trend-fast "$TREND_FAST"
  -trend-slow "$TREND_SLOW"
  -trend-min-spread-pct "$TREND_MIN_SPREAD_PCT"
  -breakeven-stop="$BREAKEVEN_STOP"
  -breakeven-trigger-r "$BREAKEVEN_TRIGGER_R"
  -trailing-stop="$TRAILING_STOP"
  -trailing-activation-r "$TRAILING_ACTIVATION_R"
  -trailing-atr-mult "$TRAILING_ATR_MULT"
  -watch
  -poll-interval "$POLL_INTERVAL"
)
if [ -n "$POSITION_MODEL" ]; then
  papertrade_cmd+=(-position-model "$POSITION_MODEL")
fi
if [ "$SUBMIT_EXCHANGE" = "true" ]; then
  papertrade_cmd+=(-submit-exchange)
fi

start_child "papertrade" "${papertrade_cmd[@]}"

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
