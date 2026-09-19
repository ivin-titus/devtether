#!/bin/sh
# Isolated triage of brutal-harness failures E8, E12, E14-E16 (fresh cwd each time).
set -u
W=/tmp/dt-repro
rm -rf "$W"; mkdir -p "$W/bin" "$W/run" "$W/proj"
cd '/media/ivintitus/Data/My Projects/Main/DevTether' || exit 1
go build -o "$W/bin/devtether" ./cmd/devtether || { echo BUILD_FAIL; exit 1; }
export XDG_RUNTIME_DIR="$W/run"
BIN="$W/bin/devtether"
pkill -f "$BIN" 2>/dev/null

echo '=== E12: service-manager foreground guard (expect rc=124, stays running) ==='
cd "$W/proj"
printf 'settings:\n  daemon: true\nproxy:\n  port: 18082\ndns:\n  bind: "127.0.0.1:15355"\nroutes:\n  app.localhost: 9099\n' > autodaemon.yaml
timeout 3 env DEVTETHER_FOREGROUND=1 "$BIN" up -c "$W/proj/autodaemon.yaml" > "$W/e12.out" 2>&1
echo "E12_RC=$?"; cat "$W/e12.out"
pkill -f "$BIN" 2>/dev/null; sleep 0.5

echo '=== E8: symlinked runtime dir (expect rc=1 AND "symlink" visible to user) ==='
rm -rf "$W/run" "$W/proj"; mkdir -p "$W/run" "$W/proj"; cd "$W/proj"
mkdir -p "$W/symlink-target"
ln -s "$W/symlink-target" "$W/run/devtether"
printf 'proxy:\n  port: 18080\ndns:\n  bind: "127.0.0.1:15335"\nroutes:\n  app.localhost: 9099\n' > up.yaml
"$BIN" up -d -c "$W/proj/up.yaml" > "$W/e8.out" 2>&1
echo "E8_RC=$?"; echo '--- stdout+stderr ---'; cat "$W/e8.out"
echo '--- daemon log file ---'; cat "$W/proj/.logs/devtether.log" 2>/dev/null
rm -f "$W/run/devtether"

echo '=== E14: logs -f with no daemon (expect rc=0 + honest message) ==='
rm -rf "$W/run" "$W/proj"; mkdir -p "$W/run" "$W/proj" "$W/proj/.logs"; cd "$W/proj"
printf 'old line\n' > "$W/proj/.logs/devtether.log"
timeout 5 "$BIN" logs -f > "$W/e14.out" 2>&1
echo "E14_RC=$?"; cat "$W/e14.out"

echo '=== E15: logs with binary content (expect rc=0) ==='
printf '\000\001\002binary\377junk\n' > "$W/proj/.logs/devtether.log"
"$BIN" logs > "$W/e15.out" 2>&1
echo "E15_RC=$?"; cat -v "$W/e15.out"

echo '=== E16: unreadable log file (expect rc=1 + message mentions log file) ==='
printf 'secret\n' > "$W/proj/.logs/devtether.log"
chmod 000 "$W/proj/.logs/devtether.log"
"$BIN" logs > "$W/e16.out" 2>&1
echo "E16_RC=$?"; cat "$W/e16.out"
chmod 644 "$W/proj/.logs/devtether.log" 2>/dev/null

echo '=== TRIAGE_DONE ==='
