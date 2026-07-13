#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/Users/guilinzhou/Desktop/test-nemo/gogogo}"
LABEL="${LABEL:-com.gogogo.paper-local-stack}"
PLIST_DIR="$HOME/Library/LaunchAgents"
PLIST_PATH="$PLIST_DIR/$LABEL.plist"
LOG_DIR="$ROOT_DIR/.runtime/logs"

mkdir -p "$PLIST_DIR" "$LOG_DIR" "$ROOT_DIR/.runtime/pids"

cat >"$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$ROOT_DIR/scripts/run-paper-local-stack.sh</string>
  </array>
  <key>WorkingDirectory</key>
  <string>$ROOT_DIR</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>$LOG_DIR/launchd.out.log</string>
  <key>StandardErrorPath</key>
  <string>$LOG_DIR/launchd.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>ROOT_DIR</key>
    <string>$ROOT_DIR</string>
    <key>DATABASE_DSN</key>
    <string>$ROOT_DIR/data.db</string>
    <key>ONEBULLEX_LIVE_TRADING</key>
    <string>false</string>
  </dict>
</dict>
</plist>
EOF

launchctl bootout "gui/$(id -u)" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH"
launchctl enable "gui/$(id -u)/$LABEL"

echo "installed launchd agent: $PLIST_PATH"
echo "status: launchctl print gui/$(id -u)/$LABEL"
echo "logs: $LOG_DIR"
