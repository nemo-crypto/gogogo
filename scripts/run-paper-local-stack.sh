#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
# shellcheck disable=SC1091
source "$ROOT_DIR/scripts/lib/common.sh"

RUNTIME_DIR="$ROOT_DIR/.runtime"
PID_DIR="$RUNTIME_DIR/pids"
LOG_DIR="$RUNTIME_DIR/logs"
READY_FILE="$RUNTIME_DIR/paper-local-stack.ready"

mkdir -p "$PID_DIR" "$LOG_DIR"
if [ "${SKIP_LOG_ARCHIVE:-0}" != "1" ] && [ "${ARCHIVE_LOGS_ON_START:-true}" = "true" ]; then
  archive_current_logs "$LOG_DIR"
fi
cd "$ROOT_DIR"
rm -f "$READY_FILE"
require_go
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"

# Capture CLI/exported overrides before sourcing .env.local.
# Keys/API secrets still come from .env.local when not already set.
DSN="${DATABASE_DSN:-$ROOT_DIR/data.db}"
SYMBOL="${SYMBOL:-BTCUSDT}"
ACCOUNT="${ACCOUNT:-}"
LIVE_ACCOUNT="${LIVE_ACCOUNT:-live-main}"
PROFILE="${PROFILE:-micro-trend-1m}"
EQUITY="${EQUITY:-1000}"
POSITION_MODEL="${POSITION_MODEL:-}"
SUBMIT_EXCHANGE="${SUBMIT_EXCHANGE:-false}"
LIVE_TRADING="${ONEBULLEX_LIVE_TRADING:-${LIVE_TRADING:-false}}"
HTTP_ADDR="${HTTP_ADDR:-:8082}"
POLL_INTERVAL="${POLL_INTERVAL:-15s}"
KLINE_POLL_INTERVAL="${KLINE_POLL_INTERVAL:-$POLL_INTERVAL}"
KLINE_POLL_INTERVAL_1M="${KLINE_POLL_INTERVAL_1M:-15s}"
KLINE_POLL_INTERVAL_5M="${KLINE_POLL_INTERVAL_5M:-$KLINE_POLL_INTERVAL}"
KLINE_POLL_INTERVAL_15M="${KLINE_POLL_INTERVAL_15M:-90s}"
KLINE_POLL_INTERVAL_1H="${KLINE_POLL_INTERVAL_1H:-5m}"
KLINE_OVERLAP="${KLINE_OVERLAP:-3}"
FUNDING_POLL_INTERVAL="${FUNDING_POLL_INTERVAL:-30m}"
ACCOUNT_SNAPSHOT_POLL_INTERVAL="${ACCOUNT_SNAPSHOT_POLL_INTERVAL:-1m}"
PERSIST_INTERVAL="${PERSIST_INTERVAL:-1m}"
BACKTEST_INTERVAL="${BACKTEST_INTERVAL:-5m}"
MAX_CANDLE_AGE="${MAX_CANDLE_AGE:-}"
MARK_PRICE_RETENTION="${MARK_PRICE_RETENTION:-168h}"
MARKET_DATA_READY_TIMEOUT="${MARKET_DATA_READY_TIMEOUT:-90}"
MIN_SIGNAL_CANDLES="${MIN_SIGNAL_CANDLES:-60}"
INTERVAL="${INTERVAL:-}"
TREND_FILTER="${TREND_FILTER:-true}"
case "$PROFILE" in
  micro|micro-trend|micro_trend|micro-trend-1m|micro_trend_1m|micro-1m|micro_1m|one-min|one_min|one-minute|one_minute|1m|1m-scalp|1m_scalp|scalp-1m|scalp_1m|minute-scalp|minute_scalp)
    DEFAULT_TREND_INTERVAL="5m"
    DEFAULT_MACRO_TREND_INTERVAL="5m"
    DEFAULT_TREND_FAST="8"
    DEFAULT_TREND_SLOW="21"
    DEFAULT_TREND_MIN_SPREAD_PCT="0.005"
    DEFAULT_MAX_CANDLE_AGE="2m"
    ;;
  small-scalp-fast|small_scalp_fast|small-fast|small_fast|fast-scalp|fast_scalp|micro-scalp-fast|micro_scalp_fast|300u|300u-fast|300u_fast)
    DEFAULT_TREND_INTERVAL="15m"
    DEFAULT_MACRO_TREND_INTERVAL="15m"
    DEFAULT_TREND_FAST="8"
    DEFAULT_TREND_SLOW="21"
    DEFAULT_TREND_MIN_SPREAD_PCT="0.01"
    DEFAULT_MAX_CANDLE_AGE=""
    ;;
  small|small-scalp|small_scalp|small-capital|small_capital|micro-scalp|micro_scalp)
    DEFAULT_TREND_INTERVAL="15m"
    DEFAULT_MACRO_TREND_INTERVAL="15m"
    DEFAULT_TREND_FAST="8"
    DEFAULT_TREND_SLOW="21"
    DEFAULT_TREND_MIN_SPREAD_PCT="0.02"
    DEFAULT_MAX_CANDLE_AGE=""
    ;;
  *)
    DEFAULT_TREND_INTERVAL="15m"
    DEFAULT_MACRO_TREND_INTERVAL="1h"
    DEFAULT_TREND_FAST="20"
    DEFAULT_TREND_SLOW="60"
    DEFAULT_TREND_MIN_SPREAD_PCT="0.05"
    DEFAULT_MAX_CANDLE_AGE=""
    ;;
esac
TREND_INTERVAL="${TREND_INTERVAL:-$DEFAULT_TREND_INTERVAL}"
MACRO_TREND_INTERVAL="${MACRO_TREND_INTERVAL:-$DEFAULT_MACRO_TREND_INTERVAL}"
TREND_FAST="${TREND_FAST:-$DEFAULT_TREND_FAST}"
TREND_SLOW="${TREND_SLOW:-$DEFAULT_TREND_SLOW}"
TREND_MIN_SPREAD_PCT="${TREND_MIN_SPREAD_PCT:-$DEFAULT_TREND_MIN_SPREAD_PCT}"
MAX_CANDLE_AGE="${MAX_CANDLE_AGE:-$DEFAULT_MAX_CANDLE_AGE}"
BREAKEVEN_STOP="${BREAKEVEN_STOP:-true}"
BREAKEVEN_TRIGGER_R="${BREAKEVEN_TRIGGER_R:-1.0}"
TRAILING_STOP="${TRAILING_STOP:-true}"
TRAILING_ACTIVATION_R="${TRAILING_ACTIVATION_R:-1.5}"
TRAILING_ATR_MULT="${TRAILING_ATR_MULT:-1.2}"

if [ -z "$INTERVAL" ]; then
  case "$PROFILE" in
    micro|micro-trend|micro_trend|micro-trend-1m|micro_trend_1m|micro-1m|micro_1m|one-min|one_min|one-minute|one_minute|1m|1m-scalp|1m_scalp|scalp-1m|scalp_1m|minute-scalp|minute_scalp)
      INTERVAL="1m"
      ;;
    aggressive|small-aggressive|small_aggressive|small|small-scalp|small_scalp|small-capital|small_capital|micro-scalp|micro_scalp|small-scalp-fast|small_scalp_fast|small-fast|small_fast|fast-scalp|fast_scalp|micro-scalp-fast|micro_scalp_fast|300u|300u-fast|300u_fast)
      INTERVAL="5m"
      ;;
    *)
      INTERVAL="5m"
      ;;
  esac
fi

CONFIG_DSN="$DSN"
CONFIG_SYMBOL="$SYMBOL"
CONFIG_ACCOUNT="$ACCOUNT"
CONFIG_LIVE_ACCOUNT="$LIVE_ACCOUNT"
CONFIG_PROFILE="$PROFILE"
CONFIG_EQUITY="$EQUITY"
CONFIG_POSITION_MODEL="$POSITION_MODEL"
CONFIG_SUBMIT_EXCHANGE="$SUBMIT_EXCHANGE"
CONFIG_LIVE_TRADING="$LIVE_TRADING"
CONFIG_HTTP_ADDR="$HTTP_ADDR"
CONFIG_POLL_INTERVAL="$POLL_INTERVAL"
CONFIG_KLINE_POLL_INTERVAL="$KLINE_POLL_INTERVAL"
CONFIG_KLINE_POLL_INTERVAL_1M="$KLINE_POLL_INTERVAL_1M"
CONFIG_KLINE_POLL_INTERVAL_5M="$KLINE_POLL_INTERVAL_5M"
CONFIG_KLINE_POLL_INTERVAL_15M="$KLINE_POLL_INTERVAL_15M"
CONFIG_KLINE_POLL_INTERVAL_1H="$KLINE_POLL_INTERVAL_1H"
CONFIG_KLINE_OVERLAP="$KLINE_OVERLAP"
CONFIG_FUNDING_POLL_INTERVAL="$FUNDING_POLL_INTERVAL"
CONFIG_ACCOUNT_SNAPSHOT_POLL_INTERVAL="$ACCOUNT_SNAPSHOT_POLL_INTERVAL"
CONFIG_PERSIST_INTERVAL="$PERSIST_INTERVAL"
CONFIG_BACKTEST_INTERVAL="$BACKTEST_INTERVAL"
CONFIG_MAX_CANDLE_AGE="$MAX_CANDLE_AGE"
CONFIG_MARK_PRICE_RETENTION="$MARK_PRICE_RETENTION"
CONFIG_MARKET_DATA_READY_TIMEOUT="$MARKET_DATA_READY_TIMEOUT"
CONFIG_MIN_SIGNAL_CANDLES="$MIN_SIGNAL_CANDLES"
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

if [ -f "$ROOT_DIR/.env.local" ]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env.local"
  set +a
fi

DSN="$CONFIG_DSN"
SYMBOL="$CONFIG_SYMBOL"
ACCOUNT="$CONFIG_ACCOUNT"
LIVE_ACCOUNT="$CONFIG_LIVE_ACCOUNT"
PROFILE="$CONFIG_PROFILE"
EQUITY="$CONFIG_EQUITY"
POSITION_MODEL="$CONFIG_POSITION_MODEL"
SUBMIT_EXCHANGE="$CONFIG_SUBMIT_EXCHANGE"
LIVE_TRADING="$CONFIG_LIVE_TRADING"
HTTP_ADDR="$CONFIG_HTTP_ADDR"
POLL_INTERVAL="$CONFIG_POLL_INTERVAL"
KLINE_POLL_INTERVAL="$CONFIG_KLINE_POLL_INTERVAL"
KLINE_POLL_INTERVAL_1M="$CONFIG_KLINE_POLL_INTERVAL_1M"
KLINE_POLL_INTERVAL_5M="$CONFIG_KLINE_POLL_INTERVAL_5M"
KLINE_POLL_INTERVAL_15M="$CONFIG_KLINE_POLL_INTERVAL_15M"
KLINE_POLL_INTERVAL_1H="$CONFIG_KLINE_POLL_INTERVAL_1H"
KLINE_OVERLAP="$CONFIG_KLINE_OVERLAP"
FUNDING_POLL_INTERVAL="$CONFIG_FUNDING_POLL_INTERVAL"
ACCOUNT_SNAPSHOT_POLL_INTERVAL="$CONFIG_ACCOUNT_SNAPSHOT_POLL_INTERVAL"
PERSIST_INTERVAL="$CONFIG_PERSIST_INTERVAL"
BACKTEST_INTERVAL="$CONFIG_BACKTEST_INTERVAL"
MAX_CANDLE_AGE="$CONFIG_MAX_CANDLE_AGE"
MARK_PRICE_RETENTION="$CONFIG_MARK_PRICE_RETENTION"
MARKET_DATA_READY_TIMEOUT="$CONFIG_MARKET_DATA_READY_TIMEOUT"
MIN_SIGNAL_CANDLES="$CONFIG_MIN_SIGNAL_CANDLES"
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
export PATH
export GOPROXY
export GOSUMDB

if [ -z "$ACCOUNT" ]; then
  if [ "$ONEBULLEX_LIVE_TRADING" = "true" ] && [ "$SUBMIT_EXCHANGE" = "true" ]; then
    ACCOUNT="live-main"
  elif [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
    ACCOUNT="paper-live-main"
  else
    ACCOUNT="paper"
  fi
fi

if [ "$SUBMIT_EXCHANGE" = "true" ] && [ "$ONEBULLEX_LIVE_TRADING" != "true" ]; then
  echo "SUBMIT_EXCHANGE=true requires ONEBULLEX_LIVE_TRADING=true"
  exit 1
fi

if [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
  if [ -z "${ONEBULLEX_API_KEY:-}" ] || [ -z "${ONEBULLEX_SECRET_KEY:-}" ]; then
    echo "ONEBULLEX_LIVE_TRADING=true requires ONEBULLEX_API_KEY and ONEBULLEX_SECRET_KEY in .env.local" >&2
    exit 1
  fi
fi

children=()
BIN_DIR="$RUNTIME_DIR/bin"
mkdir -p "$BIN_DIR"

cleanup() {
  rm -f "$READY_FILE"
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

build_bin() {
  local name="$1"
  local pkg="$2"
  local out="$BIN_DIR/$name"
  echo "building $name ..."
  go build -o "$out" "$pkg"
}

start_child() {
  local name="$1"
  shift
  local log_file="$LOG_DIR/$name.log"
  : >"$log_file"
  printf 'run_id=%s service=%s started_at=%s\n' "$RUN_ID" "$name" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$log_file"
  echo "starting $name ..."
  nohup "$@" >>"$log_file" 2>&1 &
  local pid="$!"
  children+=("$pid")
  echo "$pid" >"$PID_DIR/$name.pid"
  echo "$name started: pid=$pid log=$log_file"
}

poll_interval_for_kline() {
  case "$1" in
    1m) printf '%s\n' "$KLINE_POLL_INTERVAL_1M" ;;
    5m) printf '%s\n' "$KLINE_POLL_INTERVAL_5M" ;;
    15m) printf '%s\n' "$KLINE_POLL_INTERVAL_15M" ;;
    1h) printf '%s\n' "$KLINE_POLL_INTERVAL_1H" ;;
    *) printf '%s\n' "$KLINE_POLL_INTERVAL" ;;
  esac
}

start_kline_sync() {
  local name="$1"
  local interval="$2"
  local poll_interval
  poll_interval="$(poll_interval_for_kline "$interval")"
  start_child "$name" \
    "$BIN_DIR/marketsync" \
      -dsn "$DSN" \
      -dataset klines \
      -exchange onebullex \
      -market perpetual \
      -symbols "$SYMBOL" \
      -interval "$interval" \
      -limit 120 \
      -incremental \
      -kline-overlap "$KLINE_OVERLAP" \
      -watch \
      -poll-interval "$poll_interval"
}

wait_for_market_data() {
  if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "sqlite3 not found; skipping market data readiness wait"
    return 0
  fi
  local deadline=$((SECONDS + MARKET_DATA_READY_TIMEOUT))
  local ready=0
  local intervals="$INTERVAL"
  if [ "$TREND_FILTER" = "true" ]; then
    intervals="$intervals $TREND_INTERVAL $MACRO_TREND_INTERVAL"
  fi
  while [ "$SECONDS" -lt "$deadline" ]; do
    local mark_count seen all_ready
    mark_count="$(sqlite3 "$DSN" "SELECT COUNT(*) FROM mark_prices WHERE exchange='onebullex' AND symbol='$SYMBOL';" 2>/dev/null || echo 0)"
    seen=" "
    all_ready=1
    for interval in $intervals; do
      [ -n "$interval" ] || continue
      case "$seen" in
        *" $interval "*) continue ;;
      esac
      seen="$seen$interval "
      local candle_count
      candle_count="$(sqlite3 "$DSN" "SELECT COUNT(*) FROM candles WHERE exchange='onebullex' AND market_type='perpetual' AND symbol='$SYMBOL' AND interval='$interval';" 2>/dev/null || echo 0)"
      if [ "${candle_count:-0}" -lt "$MIN_SIGNAL_CANDLES" ]; then
        all_ready=0
        break
      fi
    done
    if [ "$all_ready" = "1" ] && [ "${mark_count:-0}" -ge 1 ]; then
      ready=1
      break
    fi
    sleep 2
  done
  if [ "$ready" = "1" ]; then
    echo "market data ready: intervals=[$intervals] candles>=$MIN_SIGNAL_CANDLES mark_prices>=1"
  else
    echo "market data readiness wait timed out after ${MARKET_DATA_READY_TIMEOUT}s; starting papertrade anyway" >&2
  fi
}

echo "$$" >"$PID_DIR/paper-local-stack.pid"
echo "building local binaries ..."
build_bin quantdb ./cmd/quantdb
build_bin marketsync ./cmd/marketsync
build_bin papertrade ./cmd/papertrade
build_bin dashboard ./cmd/dashboard
if [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
  build_bin accountsnapshot ./cmd/accountsnapshot
fi

echo "initializing database schema ..."
"$BIN_DIR/quantdb" >>"$LOG_DIR/quantdb.log" 2>&1

if [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
  echo "syncing live account snapshot ..."
  "$BIN_DIR/accountsnapshot" \
    -dsn "$DSN" \
    -account "$LIVE_ACCOUNT" \
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
  "$BIN_DIR/marketsync" \
    -dsn "$DSN" \
    -dataset mark-price \
    -exchange onebullex \
    -market perpetual \
    -symbols "$SYMBOL" \
    -watch \
    -retention "$MARK_PRICE_RETENTION" \
    -poll-interval "$POLL_INTERVAL"

start_child "marketsync-funding" \
  "$BIN_DIR/marketsync" \
    -dsn "$DSN" \
    -dataset funding \
    -exchange onebullex \
    -market perpetual \
    -symbols "$SYMBOL" \
    -limit 10 \
    -incremental \
    -watch \
    -poll-interval "$FUNDING_POLL_INTERVAL"

if [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
  start_child "accountsnapshot" \
    "$BIN_DIR/accountsnapshot" \
      -dsn "$DSN" \
      -account "$LIVE_ACCOUNT" \
      -exchange onebullex \
      -market perpetual \
      -symbol "$SYMBOL" \
      -sync-live \
      -sync-position-configs=false \
      -watch \
      -poll-interval "$ACCOUNT_SNAPSHOT_POLL_INTERVAL"
fi

wait_for_market_data

papertrade_cmd=(
  "$BIN_DIR/papertrade"
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
  -persist-interval "$PERSIST_INTERVAL"
  -backtest-interval "$BACKTEST_INTERVAL"
)
if [ -n "$MAX_CANDLE_AGE" ]; then
  papertrade_cmd+=(-max-candle-age "$MAX_CANDLE_AGE")
fi
if [ -n "$POSITION_MODEL" ]; then
  papertrade_cmd+=(-position-model "$POSITION_MODEL")
fi
if [ "$SUBMIT_EXCHANGE" = "true" ]; then
  papertrade_cmd+=(-submit-exchange)
fi

start_child "papertrade" "${papertrade_cmd[@]}"

start_child "dashboard" \
  env HTTP_ADDR="$HTTP_ADDR" DATABASE_DSN="$DSN" "$BIN_DIR/dashboard"

# Give children a moment so we don't mark ready if they die instantly.
sleep 2
for pid in "${children[@]}"; do
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "child process exited immediately after start: pid=$pid" >&2
    exit 1
  fi
done

date -u +"%Y-%m-%dT%H:%M:%SZ ready RUN_ID=$RUN_ID ONEBULLEX_LIVE_TRADING=$ONEBULLEX_LIVE_TRADING SUBMIT_EXCHANGE=$SUBMIT_EXCHANGE SYMBOL=$SYMBOL ACCOUNT=$ACCOUNT LIVE_ACCOUNT=$LIVE_ACCOUNT" >"$READY_FILE"
echo "paper local stack supervisor started."
echo "mode: ONEBULLEX_LIVE_TRADING=$ONEBULLEX_LIVE_TRADING SUBMIT_EXCHANGE=$SUBMIT_EXCHANGE ACCOUNT=$ACCOUNT LIVE_ACCOUNT=$LIVE_ACCOUNT SYMBOL=$SYMBOL"

while true; do
  for pid in "${children[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "child process stopped: pid=$pid"
      exit 1
    fi
  done
  sleep 5
done
