#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
# shellcheck disable=SC1091
source "$ROOT_DIR/scripts/lib/common.sh"

RUNTIME_DIR="$ROOT_DIR/.runtime"
PID_DIR="$RUNTIME_DIR/pids"
LOG_DIR="$RUNTIME_DIR/logs"
PID_FILE="$PID_DIR/paper-local-stack.pid"
READY_FILE="$RUNTIME_DIR/paper-local-stack.ready"
HTTP_ADDR="${HTTP_ADDR:-:8082}"
READY_TIMEOUT_SEC="${READY_TIMEOUT_SEC:-300}"

mkdir -p "$PID_DIR" "$LOG_DIR"

if [ -f "$PID_FILE" ]; then
  old_pid="$(cat "$PID_FILE")"
  if kill -0 "$old_pid" 2>/dev/null; then
    echo "paper local stack already running: pid=$old_pid"
    echo "Stop it first if you need to switch paper/live settings:"
    echo "  $ROOT_DIR/scripts/stop-paper-local.sh"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

rm -f "$READY_FILE"
if [ "${ARCHIVE_LOGS_ON_START:-true}" = "true" ]; then
  archive_current_logs "$LOG_DIR"
fi

require_go

# Explicitly propagate trading switches into the background supervisor.
# Command-line values override .env.local inside run-paper-local-stack.sh.
export ONEBULLEX_LIVE_TRADING="${ONEBULLEX_LIVE_TRADING:-false}"
export SUBMIT_EXCHANGE="${SUBMIT_EXCHANGE:-false}"
export SYMBOL="${SYMBOL:-BTCUSDT}"
export LIVE_ACCOUNT="${LIVE_ACCOUNT:-live-main}"
if [ -z "${ACCOUNT:-}" ]; then
  if [ "$ONEBULLEX_LIVE_TRADING" = "true" ] && [ "$SUBMIT_EXCHANGE" = "true" ]; then
    ACCOUNT="live-main"
  elif [ "$ONEBULLEX_LIVE_TRADING" = "true" ]; then
    ACCOUNT="paper-live-main"
  else
    ACCOUNT="paper"
  fi
fi
export ACCOUNT
export PROFILE="${PROFILE:-aggressive}"
export EQUITY="${EQUITY:-1000}"
export POSITION_MODEL="${POSITION_MODEL:-}"
export HTTP_ADDR
export DATABASE_DSN="${DATABASE_DSN:-$ROOT_DIR/data.db}"
export POLL_INTERVAL="${POLL_INTERVAL:-15s}"
export PERSIST_INTERVAL="${PERSIST_INTERVAL:-1m}"
export BACKTEST_INTERVAL="${BACKTEST_INTERVAL:-5m}"
export FUNDING_POLL_INTERVAL="${FUNDING_POLL_INTERVAL:-30m}"
export ACCOUNT_SNAPSHOT_POLL_INTERVAL="${ACCOUNT_SNAPSHOT_POLL_INTERVAL:-1m}"
export MARK_PRICE_RETENTION="${MARK_PRICE_RETENTION:-168h}"
export PATH
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export GOPROXY
export GOSUMDB
export SKIP_LOG_ARCHIVE=1

if [ "$SUBMIT_EXCHANGE" = "true" ] && [ "$ONEBULLEX_LIVE_TRADING" != "true" ]; then
  echo "SUBMIT_EXCHANGE=true requires ONEBULLEX_LIVE_TRADING=true" >&2
  exit 1
fi

echo "starting paper local stack ..."
echo "  ONEBULLEX_LIVE_TRADING=$ONEBULLEX_LIVE_TRADING"
echo "  SUBMIT_EXCHANGE=$SUBMIT_EXCHANGE"
echo "  SYMBOL=$SYMBOL ACCOUNT=$ACCOUNT LIVE_ACCOUNT=$LIVE_ACCOUNT PROFILE=$PROFILE"
echo "  POLL_INTERVAL=$POLL_INTERVAL PERSIST_INTERVAL=$PERSIST_INTERVAL BACKTEST_INTERVAL=$BACKTEST_INTERVAL"
echo "  (first start may take 1-2 minutes while Go builds binaries)"

# Detach from this shell's process group so closing the parent terminal/command
# does not tear down the supervisor.
(
  cd "$ROOT_DIR"
  nohup "$ROOT_DIR/scripts/run-paper-local-stack.sh" >>"$LOG_DIR/supervisor.log" 2>&1 &
  echo $! >"$PID_FILE"
)
pid="$(cat "$PID_FILE")"


elapsed=0
while [ "$elapsed" -lt "$READY_TIMEOUT_SEC" ]; do
  if [ -f "$READY_FILE" ]; then
    break
  fi
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "paper local stack failed to start. Recent logs:" >&2
    echo "----- $LOG_DIR/supervisor.log -----" >&2
    tail -n 40 "$LOG_DIR/supervisor.log" 2>/dev/null || true
    if [ -f "$LOG_DIR/quantdb.log" ]; then
      echo "----- $LOG_DIR/quantdb.log -----" >&2
      tail -n 40 "$LOG_DIR/quantdb.log" 2>/dev/null || true
    fi
    if [ -f "$LOG_DIR/accountsnapshot.log" ]; then
      echo "----- $LOG_DIR/accountsnapshot.log -----" >&2
      tail -n 40 "$LOG_DIR/accountsnapshot.log" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
    exit 1
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

if [ ! -f "$READY_FILE" ]; then
  echo "paper local stack did not become ready within ${READY_TIMEOUT_SEC}s" >&2
  echo "check logs under $LOG_DIR" >&2
  exit 1
fi

# Prefer the supervisor's self-written PID if present.
if [ -f "$PID_FILE" ]; then
  pid="$(cat "$PID_FILE")"
fi

if [[ "$HTTP_ADDR" == :* ]]; then
  DASHBOARD_URL="http://localhost$HTTP_ADDR"
else
  DASHBOARD_URL="http://$HTTP_ADDR"
fi

mode="paper simulation"
if [ "$ONEBULLEX_LIVE_TRADING" = "true" ] && [ "$SUBMIT_EXCHANGE" = "true" ]; then
  mode="LIVE trading (real orders)"
fi

cat <<EOF
paper local stack is running ($mode).

dashboard:
  $DASHBOARD_URL

logs:
  $LOG_DIR

status:
  $ROOT_DIR/scripts/status-paper-local.sh

stop:
  $ROOT_DIR/scripts/stop-paper-local.sh

EOF
