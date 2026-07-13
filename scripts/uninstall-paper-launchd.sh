#!/usr/bin/env bash
set -euo pipefail

LABEL="${LABEL:-com.gogogo.paper-local-stack}"
PLIST_PATH="$HOME/Library/LaunchAgents/$LABEL.plist"

launchctl bootout "gui/$(id -u)" "$PLIST_PATH" >/dev/null 2>&1 || true
rm -f "$PLIST_PATH"

echo "uninstalled launchd agent: $LABEL"
