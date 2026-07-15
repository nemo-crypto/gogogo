#!/usr/bin/env bash
# Shared helpers for local paper/live stack scripts.

ensure_go_path() {
  local candidate
  for candidate in \
    /usr/local/go/bin \
    /opt/homebrew/bin \
    /usr/local/bin \
    "$HOME/go/bin" \
    /usr/local/Homebrew/bin
  do
    if [ -x "$candidate/go" ]; then
      case ":$PATH:" in
        *":$candidate:"*) ;;
        *) PATH="$candidate:$PATH" ;;
      esac
    fi
  done
  export PATH

  if ! command -v go >/dev/null 2>&1; then
    echo "go: command not found" >&2
    echo "Install Go 1.22+ first, then retry. Example:" >&2
    echo "  brew install go" >&2
    echo "  # or download from https://go.dev/dl/" >&2
    return 1
  fi
}

ensure_go_proxy() {
  # Default proxy.golang.org is often unreachable in CN networks.
  if [ -z "${GOPROXY:-}" ] || [ "$GOPROXY" = "https://proxy.golang.org,direct" ]; then
    export GOPROXY="${GOPROXY_OVERRIDE:-https://goproxy.cn,direct}"
  fi
  export GOSUMDB="${GOSUMDB:-off}"
}

require_go() {
  ensure_go_path || return 1
  ensure_go_proxy
  echo "using go: $(command -v go) ($(go env GOVERSION 2>/dev/null || go version)) GOPROXY=$(go env GOPROXY)"
}

archive_current_logs() {
  local log_dir="$1"
  local archive_root="$log_dir/archive"
  local run_id
  run_id="$(date -u +%Y%m%dT%H%M%SZ)"

  mkdir -p "$log_dir"
  if ! find "$log_dir" -maxdepth 1 -type f -name '*.log' -print -quit | grep -q .; then
    return 0
  fi

  local archive_dir="$archive_root/$run_id"
  mkdir -p "$archive_dir"
  find "$log_dir" -maxdepth 1 -type f -name '*.log' -exec mv {} "$archive_dir/" \;
  echo "archived previous logs: $archive_dir"
}
