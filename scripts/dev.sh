#!/usr/bin/env bash
# Local development, one command, one URL: http://localhost:8080.
# The Go agent serves everything, exactly like production, except that it
# proxies the UI to an internal Vite dev server so UI changes hot-reload.
#   - UI changes hot-reload in the browser.
#   - Go changes rebuild and restart the API automatically.
#   - Ctrl+C stops everything.
# Override with env vars, e.g.:  PORT=9090 MONITORS_FILE= make run
# Works with the bash 3.2 that ships with macOS.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${PORT:-8080}"
VITE_PORT="${VITE_PORT:-5173}" # internal; you never open this one
export DATA_DIR="${DATA_DIR:-./data}"
export MONITORS_FILE="${MONITORS_FILE-./examples/dev.yaml}"
# A fixed, printed login so local development needs no setup. Never use
# this outside `make run`; production generates a password or uses yours.
export ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
export ADMIN_PASSWORD="${ADMIN_PASSWORD:-uptimy-dev}"
BIN=bin/uptimy-agent-dev

info() { printf '\033[2m[run]\033[0m %s\n' "$*"; }
# Prefix each output line, flushing immediately (portable: no sed -u/-l).
prefix() { awk -v p="$1" '{ print p " " $0; fflush() }'; }

command -v go >/dev/null || { echo "Go 1.26+ is required: https://go.dev/dl"; exit 1; }
command -v npm >/dev/null || { echo "Node.js 22+ is required: https://nodejs.org"; exit 1; }

if [ ! -d web/node_modules ] || [ web/package-lock.json -nt web/node_modules ]; then
  info "installing UI dependencies"
  (cd web && npm install --no-audit --no-fund --loglevel=error)
  touch web/node_modules
fi
mkdir -p bin "$DATA_DIR"

# Fail fast if something (often an earlier `make run`) already holds a port;
# otherwise the browser would quietly keep talking to the old instance.
port_busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
for p in "$PORT" "$VITE_PORT"; do
  if port_busy "$p"; then
    echo "Port $p is already in use. Is another 'make run' still running?"
    echo "Stop it, or pick other ports: PORT=9090 VITE_PORT=5174 make run"
    exit 1
  fi
done

# Job control puts each background pipeline (server + its log prefixer) in
# its own process group, so one signal to the group stops the whole thing.
set -m

api_pgid=""
web_pgid=""
stamp="$(mktemp)"

pgid_of() { ps -o pgid= -p "$1" | tr -d ' '; }
alive() { [ -n "$1" ] && kill -0 -- "-$1" 2>/dev/null; }

# stop_groups sends SIGTERM to every given process group at once, waits up to
# 5s for them to exit, then force-kills (and reports) any that are stuck.
stop_groups() {
  local pgid i stuck
  for pgid in "$@"; do alive "$pgid" && kill -TERM -- "-$pgid" 2>/dev/null || true; done
  for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    stuck=""
    for pgid in "$@"; do alive "$pgid" && stuck="$stuck $pgid"; done
    [ -z "$stuck" ] && return 0
    sleep 0.25
  done
  for pgid in $stuck; do
    info "process group $pgid didn't stop within 5s; force-killing"
    kill -KILL -- "-$pgid" 2>/dev/null || true
  done
}

cleanup() {
  trap - INT TERM EXIT
  echo
  info "stopping"
  stop_groups "$api_pgid" "$web_pgid"
  rm -f "$stamp"
}
# Ctrl+C is the normal way to stop, so exit 0 (no "make: *** Error" noise).
trap 'cleanup; exit 0' INT TERM
trap cleanup EXIT

build_api() { go build -o "$BIN" ./cmd/uptimy-agent; }

start_api() {
  PORT="$PORT" UI_DEV_SERVER="http://127.0.0.1:$VITE_PORT" "./$BIN" 2>&1 | prefix $'\033[36m[agent]\033[0m' &
  api_pgid="$(pgid_of $!)"
  disown # we manage it by process group; skip bash's "Terminated" notices
}

# Vite first, so the UI is ready by the time the agent starts proxying to it.
# --logLevel warn hides Vite's own "Local: http://localhost:5173" banner,
# since that address isn't meant to be opened directly.
(cd web && exec ./node_modules/.bin/vite --port "$VITE_PORT" --strictPort \
  --clearScreen false --logLevel warn) 2>&1 | prefix $'\033[35m[ui]\033[0m' &
web_pgid="$(pgid_of $!)"
disown

info "building the agent"
build_api
touch "$stamp"
start_api

sleep 1
printf '\n  \033[1mUptimy Agent dev\033[0m  →  \033[1mhttp://localhost:%s\033[0m\n' "$PORT"
echo "  Sign in as $ADMIN_USERNAME / $ADMIN_PASSWORD"
echo "  UI changes hot-reload; Go changes rebuild and restart the agent."
echo "  Data $DATA_DIR · monitors file ${MONITORS_FILE:-none} · Ctrl+C stops everything."
echo

# Poll for Go changes: portable, no fswatch/inotify needed.
while true; do
  sleep 1
  if [ -n "$(find cmd internal web/embed.go go.mod go.sum -newer "$stamp" \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) 2>/dev/null | head -1)" ]; then
    touch "$stamp"
    info "Go change detected, rebuilding the agent"
    if build_api; then
      stop_groups "$api_pgid"
      start_api
      info "agent restarted"
    else
      info "build failed; the previous agent keeps running"
    fi
  fi
  if ! alive "$web_pgid"; then
    info "UI dev server exited (is port $VITE_PORT in use? set VITE_PORT to change it)"
    exit 1
  fi
  if [ -n "$api_pgid" ] && ! alive "$api_pgid"; then
    info "agent exited (is port $PORT in use?); fix and save a .go file to retry"
    api_pgid=""
  fi
done
