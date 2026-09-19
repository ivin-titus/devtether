#!/bin/sh
# Isolated smoke test for the Phase 4 polish changes (no system ports used).
set -u

REPO="/media/ivintitus/Data/My Projects/Main/DevTether"
WORK="/tmp/dt-smoke"

rm -rf "$WORK"
mkdir -p "$WORK/bin" "$WORK/run/devtether" "$WORK/project"

cd "$REPO" || exit 1
go build -o "$WORK/bin/devtether" ./cmd/devtether || exit 1

export XDG_RUNTIME_DIR="$WORK/run"
BIN="$WORK/bin/devtether"
SOCK="$XDG_RUNTIME_DIR/devtether/devtether.sock"

echo "=== 1. root help (no completion / no cheat-sheet) ==="
$BIN --help
echo "-- grep results --"
$BIN --help | grep -c "completion" || echo "completion count: 0"
$BIN --help | grep -c "devtether up -c" || echo "cheat-sheet count: 0"

echo
echo "=== 2. init help (flags printed once) ==="
$BIN init --help
echo "-- count of '--setcap' occurrences (expect 1) --"
$BIN init --help | grep -c -- "--setcap"
echo "-- count of 'Supported Flags' (expect 0) --"
$BIN init --help | grep -c "Supported Flags" || echo 0

echo
echo "=== 3. strict .localhost validation ==="
cd "$WORK/project" || exit 1
printf 'routes:\n  app.internal: 3000\n' > bad.yaml
$BIN routes -c bad.yaml; echo "bad exit=$?"
printf 'routes:\n  "*.localhost": 3000\n' > wild.yaml
$BIN routes -c wild.yaml; echo "wildcard exit=$?"
printf 'routes:\n  abc: 3000\n' > single.yaml
$BIN routes -c single.yaml; echo "single-label exit=$?"
printf 'routes:\n  app.localhost: 3000\n  api.dev.localhost: 3001\n' > good.yaml
$BIN routes -c good.yaml; echo "good exit=$?"

echo
echo "=== 4. orphaned socket recovery (QA-1) ==="
echo "-- 4a: refused socket with no PID file must be unlinked --"
touch "$SOCK"
$BIN down; echo "down-orphan exit=$?"
if [ -e "$SOCK" ]; then echo "FAIL: orphan socket still present"; else echo "OK: orphan socket unlinked"; fi

echo "-- 4b: refused socket with a LIVE PID must be preserved --"
sleep 300 &
FAKE_PID=$!
printf '%d\n' "$FAKE_PID" > "$XDG_RUNTIME_DIR/devtether/devtether.pid"
touch "$SOCK"
$BIN status; echo "status-livepid exit=$?"
if [ -e "$SOCK" ]; then echo "OK: socket preserved (live PID guard)"; else echo "FAIL: socket removed despite live PID"; fi
rm -f "$XDG_RUNTIME_DIR/devtether/devtether.pid" "$SOCK"
kill "$FAKE_PID" 2>/dev/null || true

echo
echo "=== 5. daemon lifecycle (detached start/status/down + socket unlink) ==="
printf 'proxy:\n  port: 18080\ndns:\n  bind: "127.0.0.1:15335"\nroutes:\n  app.localhost: 9099\n' > up.yaml
$BIN up -d -c up.yaml; echo "up exit=$?"
sleep 2
$BIN status; echo "status exit=$?"
echo "-- runtime dir while running --"
ls -la "$XDG_RUNTIME_DIR/devtether/"
$BIN down; echo "down exit=$?"
sleep 1
echo "-- runtime dir after down (socket must be gone) --"
ls -la "$XDG_RUNTIME_DIR/devtether/"
$BIN status; echo "status-after exit=$?"

$BIN down >/dev/null 2>&1 || true
pkill -f "$WORK/bin/devtether" >/dev/null 2>&1 || true
echo "SMOKE_DONE"
