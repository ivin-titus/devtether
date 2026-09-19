#!/bin/sh
# Brutal smoke test for DevTether (isolated: high ports + private XDG_RUNTIME_DIR).
# Never aborts early: every check is recorded so a full issue list is produced.
set -u

REPO="/media/ivintitus/Data/My Projects/Main/DevTether"
WORK="/tmp/dt-brutal"
BIN="$WORK/bin/devtether"
RT="$WORK/run"
PROJ="$WORK/proj"

pass=0
fail=0
ISSUES=""

record() { # name result(got) want
    if [ "$2" = "$3" ]; then
        pass=$((pass + 1))
        printf '  PASS  %s\n' "$1"
    else
        fail=$((fail + 1))
        printf '  FAIL  %s (got [%s], want [%s])\n' "$1" "$2" "$3"
        ISSUES="$ISSUES
  - $1: got [$2], want [$3]"
    fi
}

record_rc() { # name actual_rc expected_rc
    record "$1 (exit code)" "$2" "$3"
}

record_contains() { # name file needle
    if grep -qF -- "$3" "$2" 2>/dev/null; then
        pass=$((pass + 1)); printf '  PASS  %s\n' "$1"
    else
        fail=$((fail + 1)); printf '  FAIL  %s (missing [%s])\n' "$1" "$3"
        ISSUES="$ISSUES
  - $1: output missing [$3]"
    fi
}

record_absent() { # name file needle
    if grep -qF -- "$3" "$2" 2>/dev/null; then
        fail=$((fail + 1)); printf '  FAIL  %s (unexpected [%s])\n' "$1" "$3"
        ISSUES="$ISSUES
  - $1: output unexpectedly contains [$3]"
    else
        pass=$((pass + 1)); printf '  PASS  %s\n' "$1"
    fi
}

reset_all() {
    if [ -f "$RT/devtether/devtether.pid" ]; then
        kill "$(cat "$RT/devtether/devtether.pid" 2>/dev/null)" 2>/dev/null || true
    fi
    pkill -f "$BIN" 2>/dev/null || true
    sleep 0.3
    # Never delete the shell's own cwd: it breaks every later relative path
    # (and getcwd) for the rest of the run.
    cd "$WORK" || exit 1
    rm -rf "$RT" "$PROJ"
    mkdir -p "$RT" "$PROJ"
    cd "$PROJ" || exit 1
}

kill_daemon() {
    pkill -f "$BIN" 2>/dev/null || true
    sleep 0.3
}

wait_ready() {
    i=0
    while [ "$i" -lt 60 ]; do
        if [ -S "$RT/devtether/devtether.sock" ] && "$BIN" status >/dev/null 2>&1; then
            return 0
        fi
        sleep 0.1
        i=$((i + 1))
    done
    return 1
}

# ---------------------------------------------------------------- setup
rm -rf "$WORK"
mkdir -p "$WORK/bin" "$RT" "$PROJ"
cd "$REPO" || exit 1
if ! go build -o "$BIN" ./cmd/devtether; then
    echo "BUILD FAILED"
    exit 1
fi
export XDG_RUNTIME_DIR="$RT"

CFG_UP="proxy:
  port: 18080
dns:
  bind: \"127.0.0.1:15335\"
routes:
  app.localhost: 9099
  api.dev.localhost: 9098
"

printf '=== A. CLI surface & help discipline ===\n'
cd "$PROJ" || exit 1
"$BIN" --help > "$WORK/about.txt" 2>&1; record_rc "root --help" "$?" 0
record_contains "root help has native command list" "$WORK/about.txt" "Available Commands:"
record_absent "root help hides completion" "$WORK/about.txt" "completion"
record_absent "root help has no cheat-sheet" "$WORK/about.txt" "Start in the background (logs to .logs/)"

"$BIN" completion > "$WORK/out.txt" 2>&1; rc=$?
if [ "$rc" -ne 0 ]; then record "completion command rejected" "rejected" "rejected"
else record "completion command rejected" "accepted" "rejected"; fi

"$BIN" frobnicate > "$WORK/out.txt" 2>&1; rc=$?
if [ "$rc" -ne 0 ]; then record "unknown command rejected" "rejected" "rejected"
else record "unknown command rejected" "accepted" "rejected"; fi
record_contains "unknown command message" "$WORK/out.txt" "unknown command"

"$BIN" up --nonexistent-flag > "$WORK/out.txt" 2>&1; rc=$?
if [ "$rc" -ne 0 ]; then record "unknown flag rejected" "rejected" "rejected"
else record "unknown flag rejected" "accepted" "rejected"; fi

"$BIN" --version > "$WORK/out.txt" 2>&1; record_rc "--version" "$?" 0
record_contains "--version prints a version" "$WORK/out.txt" "dev"
"$BIN" version > "$WORK/out.txt" 2>&1; record_rc "version subcommand" "$?" 0

for sub in up down status routes logs doctor init version; do
    "$BIN" "$sub" --help > "$WORK/out.txt" 2>&1; record_rc "$sub --help" "$?" 0
done
"$BIN" help up > "$WORK/out.txt" 2>&1; record_rc "help up" "$?" 0

"$BIN" init --help > "$WORK/init.txt" 2>&1
record_contains "init help has flags block" "$WORK/init.txt" "Flags:"
record_absent "init help has no duplicate flag docs" "$WORK/init.txt" "Supported Flags"

printf '\n=== B. Config validation matrix ===\n'
mkcfg() { # filename yaml (%b so \n escapes are expanded)
    printf '%b' "$2" > "$PROJ/$1"
}

expect_ok() { # name yaml
    mkcfg "_t.yaml" "$2"
    "$BIN" routes -c "$PROJ/_t.yaml" > "$WORK/out.txt" 2>&1
    record_rc "$1" "$?" 0
}
expect_fail() { # name yaml needle
    mkcfg "_t.yaml" "$2"
    "$BIN" routes -c "$PROJ/_t.yaml" > "$WORK/out.txt" 2>&1
    record_rc "$1 rejected" "$?" 1
    record_contains "$1 message" "$WORK/out.txt" "$3"
}

expect_ok "simple route"            'routes:\n  app.localhost: 3000\n'
expect_ok "hyphenated route"        'routes:\n  my-app.localhost: 3000\n'
expect_ok "nested subdomain"        'routes:\n  a.b.c.localhost: 3000\n'
expect_ok "uppercase route"         'routes:\n  APP.LOCALHOST: 3000\n'
expect_ok "empty routes map"        'routes: {}\n'
expect_ok "no routes key at all"    'settings:\n  daemon: false\n'
expect_ok "boundary port 1"         'routes:\n  app.localhost: 1\n'
expect_ok "boundary port 65535"     'routes:\n  app.localhost: 65535\n'
expect_ok "proxy config accepted"   'proxy:\n  port: 18081\n  timeouts:\n    idle: 30s\nroutes:\n  app.localhost: 3000\n'
expect_ok "dns bind accepted"       'dns:\n  bind: "127.0.0.1:15335"\nroutes:\n  app.localhost: 3000\n'

expect_fail "other TLD"             'routes:\n  app.internal: 3000\n'                    'ending in .localhost'
expect_fail "wildcard domain"       'routes:\n  "*.localhost": 3000\n'                   'ending in .localhost'
expect_fail "wildcard subdomain"    'routes:\n  "*.ivin.localhost": 3000\n'             'ending in .localhost'
expect_fail "single label"          'routes:\n  abc: 3000\n'                             'ending in .localhost'
expect_fail "bare localhost"        'routes:\n  localhost: 3000\n'                       'ending in .localhost'
expect_fail "trailing dot"          'routes:\n  app.localhost.: 3000\n'                  'ending in .localhost'
expect_fail "underscore label"      'routes:\n  app_v2.localhost: 3000\n'                'ending in .localhost'
expect_fail "embedded space"        'routes:\n  "bad host.localhost": 3000\n'            'ending in .localhost'
expect_fail "empty domain"          'routes:\n  "": 3000\n'                              'empty domain'
expect_fail "whitespace domain"     "routes:\n  ' app.localhost ': 3000\n"               'surrounding whitespace'
expect_fail "port zero"             'routes:\n  app.localhost: 0\n'                      'out of valid range'
expect_fail "port negative"         'routes:\n  app.localhost: -1\n'                     'out of valid range'
expect_fail "port too high"         'routes:\n  app.localhost: 70000\n'                  'out of valid range'
expect_fail "non-numeric port"      'routes:\n  app.localhost: abc\n'                    'cannot unmarshal'
expect_fail "unknown top-level key" 'setings:\n  daemon: true\nroutes:\n  app.localhost: 3000\n' 'field setings not found'
expect_fail "dead proxy timeout"    'proxy:\n  timeouts:\n    write: 5s\n'               'field write not found'
expect_fail "legacy services key"   'services:\n  api:\n    domain: x.localhost\n'       'legacy'
expect_fail "orchestrate deferred"  'orchestrate:\n  api:\n    domain: x.localhost\n'    'not yet implemented'
expect_fail "tunnel deferred"       'tunnel:\n  lan: true\n'                             'not yet implemented'
expect_fail "access deferred"       'access:\n  enabled: true\n'                         'not yet implemented'
expect_fail "invalid idle duration" 'proxy:\n  timeouts:\n    idle: nope\n'              'invalid duration'
expect_fail "proxy port out of range" 'proxy:\n  port: 99999\n'                          'out of valid range'
expect_fail "malformed yaml"        'routes: [this is not: valid\n'                      'failed to parse'

"$BIN" routes -c "$WORK/does-not-exist.yaml" > "$WORK/out.txt" 2>&1
record_rc "missing config routes" "$?" 0
record_contains "missing config hint" "$WORK/out.txt" "No"

"$BIN" up -c "$WORK/does-not-exist.yaml" > "$WORK/out.txt" 2>&1
record_rc "missing config up" "$?" 1
record_contains "missing config up hint" "$WORK/out.txt" "devtether init"

mkdir -p "$PROJ/adir"
"$BIN" routes -c "$PROJ/adir" > "$WORK/out.txt" 2>&1
rc=$?
if [ "$rc" -ne 0 ]; then record "config path is a directory rejected" "rejected" "rejected"
else record "config path is a directory rejected" "accepted" "rejected"; fi

mkcfg "_big.yaml" "routes:\n"
i=1
while [ "$i" -le 300 ]; do
    printf '  svc%d.localhost: %d\n' "$i" "$((20000 + i))" >> "$PROJ/_big.yaml"
    i=$((i + 1))
done
"$BIN" routes -c "$PROJ/_big.yaml" > "$WORK/out.txt" 2>&1
record_rc "300-route config loads" "$?" 0
record "300 routes rendered" "$(grep -c 'svc' "$WORK/out.txt")" "300"
record "routes output sorted" "$(head -3 "$WORK/out.txt" | tail -1 | awk '{print $1}')" "svc1.localhost"

printf '\n=== C. No-daemon behaviour (status/down/logs/routes/doctor) ===\n'
reset_all
cd "$PROJ" || exit 1

"$BIN" status > "$WORK/out.txt" 2>&1; record_rc "status without daemon" "$?" 1
record_contains "status says not running" "$WORK/out.txt" "not running"
"$BIN" status 2> "$WORK/err.txt" > /dev/null
if [ -s "$WORK/err.txt" ]; then record "status errors go to stderr" "stderr" "stderr"
else record "status errors go to stderr" "empty" "stderr"; fi

"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down without daemon (idempotent)" "$?" 0
"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down twice without daemon" "$?" 0

mkcfg "good.yaml" 'routes:\n  app.localhost: 9099\n'
"$BIN" routes -c "$PROJ/good.yaml" > "$WORK/out.txt" 2>&1; record_rc "routes from config" "$?" 0
record_contains "routes table header" "$WORK/out.txt" "DOMAIN"

"$BIN" routes > "$WORK/out.txt" 2>&1; record_rc "routes without daemon or config" "$?" 0

"$BIN" logs > "$WORK/out.txt" 2>&1; record_rc "logs without log file" "$?" 1
record_contains "logs hint mentions daemon mode" "$WORK/out.txt" "daemon mode"

"$BIN" logs --lines 0 > "$WORK/out.txt" 2>&1; record_rc "logs --lines 0 rejected" "$?" 1
"$BIN" logs --lines -5 > "$WORK/out.txt" 2>&1; record_rc "logs --lines -5 rejected" "$?" 1
record_contains "logs --lines validation message" "$WORK/out.txt" "at least 1"

"$BIN" doctor -c "$PROJ/good.yaml" > "$WORK/doc.txt" 2>&1
rc=$?
if [ "$rc" -eq 0 ] || [ "$rc" -eq 1 ]; then record "doctor no-daemon exits 0/1" "$rc" "$rc"
else record "doctor no-daemon exits 0/1" "$rc" "0 or 1"; fi
record_contains "doctor runs checks" "$WORK/doc.txt" "DevTether Doctor"
record_absent "doctor does not panic" "$WORK/doc.txt" "panic:"

"$BIN" doctor -c "$WORK/does-not-exist.yaml" > "$WORK/out.txt" 2>&1
record_rc "doctor missing config fails" "$?" 1
record_contains "doctor suggests init" "$WORK/out.txt" "devtether init"

printf '\n=== D. Daemon lifecycle ===\n'
reset_all
cd "$PROJ" || exit 1
mkcfg "up.yaml" "$CFG_UP"

"$BIN" up -d -c "$PROJ/up.yaml" > "$WORK/out.txt" 2>&1; record_rc "up -d starts" "$?" 0
if wait_ready; then record "daemon becomes ready" "ready" "ready"; else record "daemon becomes ready" "timeout" "ready"; fi

record "socket exists" "$([ -S "$RT/devtether/devtether.sock" ] && echo yes || echo no)" "yes"
record "pid file exists" "$([ -f "$RT/devtether/devtether.pid" ] && echo yes || echo no)" "yes"
record "lock file exists" "$([ -f "$RT/devtether/devtether.lock" ] && echo yes || echo no)" "yes"
record "socket mode 0600" "$(stat -c '%a' "$RT/devtether/devtether.sock")" "600"
record "lock mode 0600" "$(stat -c '%a' "$RT/devtether/devtether.lock")" "600"
record "runtime dir mode 0700" "$(stat -c '%a' "$RT/devtether")" "700"

# Readiness race: a tight status loop must never report "not running".
lost=0
i=0
while [ "$i" -lt 15 ]; do
    "$BIN" status > /dev/null 2>&1 || lost=$((lost + 1))
    i=$((i + 1))
done
record "status tight-loop misses" "$lost" "0"

"$BIN" status > "$WORK/out.txt" 2>&1
record_rc "status while running" "$?" 0
record_contains "status reports PID" "$WORK/out.txt" "PID"
record_contains "status reports config path" "$WORK/out.txt" "up.yaml"

"$BIN" routes > "$WORK/out.txt" 2>&1
record_contains "live routes show app" "$WORK/out.txt" "app.localhost"
record_contains "live routes show nested" "$WORK/out.txt" "api.dev.localhost"

"$BIN" logs > "$WORK/out.txt" 2>&1; record_rc "logs with log file" "$?" 0
"$BIN" logs --lines 1 > "$WORK/out.txt" 2>&1; record_rc "logs --lines 1" "$?" 0
"$BIN" logs --lines 999999 > "$WORK/out.txt" 2>&1; record_rc "logs huge --lines" "$?" 0

# Follow mode must keep watching while the daemon is alive.
timeout 2 "$BIN" logs -f > "$WORK/out.txt" 2>&1
record_rc "logs -f keeps following a live daemon" "$?" 124

# A second instance must be refused by the instance lock.
"$BIN" up -d -c "$PROJ/up.yaml" > "$WORK/out.txt" 2>&1
record_rc "second up -d refused" "$?" 1
record_contains "second up -d explains conflict" "$WORK/out.txt" "already running"

"$BIN" up -c "$PROJ/up.yaml" > "$WORK/out.txt" 2>&1
record_rc "foreground up refused while running" "$?" 1

"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down stops the daemon" "$?" 0
record "socket removed after down" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "absent"
record "pid removed after down" "$([ -e "$RT/devtether/devtether.pid" ] && echo present || echo absent)" "absent"
record "no stray process after down" "$(pgrep -f "$BIN" | wc -l | tr -d ' ')" "0"
"$BIN" status > /dev/null 2>&1; record_rc "status after down" "$?" 1
"$BIN" down > /dev/null 2>&1; record_rc "down after down" "$?" 0

wait_ready 2>/dev/null; record "wait_ready fails when stopped" "$?" "1"

printf '\n=== E. Extraordinary / adversarial ===\n'

# E1: graceful SIGTERM must reclaim every state file.
reset_all
mkcfg "up.yaml" "$CFG_UP"
"$BIN" up -d -c "$PROJ/up.yaml" > /dev/null 2>&1
wait_ready
kill -TERM "$(cat "$RT/devtether/devtether.pid")" 2>/dev/null
sleep 1
record "SIGTERM removes socket" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "absent"
record "SIGTERM removes pid file" "$([ -e "$RT/devtether/devtether.pid" ] && echo present || echo absent)" "absent"
"$BIN" status > /dev/null 2>&1; record_rc "status after SIGTERM" "$?" 1

# E2: SIGKILL leaves an orphan; status must reclaim it (QA-1).
reset_all
mkcfg "up.yaml" "$CFG_UP"
"$BIN" up -d -c "$PROJ/up.yaml" > /dev/null 2>&1
wait_ready
kill -KILL "$(cat "$RT/devtether/devtether.pid")" 2>/dev/null
sleep 0.5
record "SIGKILL leaves orphan socket" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "present"
"$BIN" status > "$WORK/out.txt" 2>&1; record_rc "status after SIGKILL" "$?" 1
record "status unlinks orphan socket" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "absent"

# E3: refused socket with no PID file -> down reclaims it (QA-1).
reset_all
mkdir -p "$RT/devtether"
touch "$RT/devtether/devtether.sock"
"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down reclaims pid-less orphan" "$?" 0
record "pid-less orphan unlinked" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "absent"

# E4: refused socket with a LIVE PID must be preserved (guard).
reset_all
mkdir -p "$RT/devtether"
sleep 30 &
FAKE_PID=$!
printf '%d\n' "$FAKE_PID" > "$RT/devtether/devtether.pid"
touch "$RT/devtether/devtether.sock"
"$BIN" status > /dev/null 2>&1
record "live PID guard preserves socket" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "present"
kill "$FAKE_PID" 2>/dev/null

# E5: tampered nonce -> down must fall back to SIGTERM and still stop the daemon.
reset_all
mkcfg "up.yaml" "$CFG_UP"
"$BIN" up -d -c "$PROJ/up.yaml" > /dev/null 2>&1
wait_ready
printf 'tampered\n' > "$RT/devtether/devtether.nonce"
"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down recovers from nonce mismatch" "$?" 0
sleep 0.5
"$BIN" status > /dev/null 2>&1; record_rc "daemon stopped after nonce fallback" "$?" 1

# E6: deleted nonce -> same recovery path.
reset_all
mkcfg "up.yaml" "$CFG_UP"
"$BIN" up -d -c "$PROJ/up.yaml" > /dev/null 2>&1
wait_ready
rm -f "$RT/devtether/devtether.nonce"
"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down recovers from missing nonce" "$?" 0
sleep 0.5
"$BIN" status > /dev/null 2>&1; record_rc "daemon stopped after missing nonce" "$?" 1

# E7: tampered nonce AND corrupted PID -> down must fail loudly, not silently.
reset_all
mkcfg "up.yaml" "$CFG_UP"
"$BIN" up -d -c "$PROJ/up.yaml" > /dev/null 2>&1
wait_ready
printf 'tampered\n' > "$RT/devtether/devtether.nonce"
printf 'not-a-pid\n' > "$RT/devtether/devtether.pid"
"$BIN" down > "$WORK/out.txt" 2>&1; record_rc "down fails without a fallback" "$?" 1
record_contains "down explains missing fallback" "$WORK/out.txt" "PID"
kill_daemon

# E8: a symlinked runtime directory must be refused (ADR-003).
reset_all
ln -s "$WORK/symlink-target" "$RT/devtether"
mkcfg "up.yaml" "$CFG_UP"
"$BIN" up -d -c "$PROJ/up.yaml" > "$WORK/out.txt" 2>&1
up_rc=$?
cp "$WORK/out.txt" "$WORK/e8.out"
record_rc "symlinked runtime dir refused" "$up_rc" 1
record_contains "symlink refusal is explicit" "$WORK/out.txt" "symlink"
rm -f "$RT/devtether"

# E9: two daemons, same configured ports, isolated runtime dirs -> fallback.
reset_all
mkdir -p "$WORK/runA" "$WORK/runB"
mkcfg "clash.yaml" "$CFG_UP"
XDG_RUNTIME_DIR="$WORK/runA" "$BIN" up -d -c "$PROJ/clash.yaml" > "$WORK/outA.txt" 2>&1
record_rc "clash: daemon A starts" "$?" 0
XDG_RUNTIME_DIR="$WORK/runB" "$BIN" up -d -c "$PROJ/clash.yaml" > "$WORK/outB.txt" 2>&1
record_rc "clash: daemon B starts with fallback" "$?" 0
record_contains "clash: B reports truthful fallback" "$WORK/outB.txt" "fell back to port 8080"
record_absent "clash: B does not blame permissions" "$WORK/outB.txt" "permission"
XDG_RUNTIME_DIR="$WORK/runB" "$BIN" status > /dev/null 2>&1; record_rc "clash: B status" "$?" 0
XDG_RUNTIME_DIR="$WORK/runB" "$BIN" down > /dev/null 2>&1; record_rc "clash: B down" "$?" 0
XDG_RUNTIME_DIR="$WORK/runA" "$BIN" down > /dev/null 2>&1; record_rc "clash: A down" "$?" 0

# E10: concurrent contenders -> flock must serialize to exactly one winner.
reset_all
mkcfg "up.yaml" "$CFG_UP"
("$BIN" up -d -c "$PROJ/up.yaml" > "$WORK/c1.txt" 2>&1; echo $? > "$WORK/c1.rc") &
("$BIN" up -d -c "$PROJ/up.yaml" > "$WORK/c2.txt" 2>&1; echo $? > "$WORK/c2.rc") &
wait
wins=0
losses=0
for c in c1 c2; do
    if [ "$(cat "$WORK/$c.rc")" = "0" ]; then wins=$((wins + 1)); else losses=$((losses + 1)); fi
done
record "concurrent up -d winners" "$wins" "1"
record "concurrent up -d losers" "$losses" "1"
wait_ready
"$BIN" status > /dev/null 2>&1; record_rc "concurrent winner is healthy" "$?" 0
kill_daemon

# E11: settings.daemon=true must detach even without -d.
reset_all
mkcfg "autodaemon.yaml" 'settings:\n  daemon: true\nproxy:\n  port: 18082\ndns:\n  bind: "127.0.0.1:15355"\nroutes:\n  app.localhost: 9099\n'
"$BIN" up -c "$PROJ/autodaemon.yaml" > "$WORK/out.txt" 2>&1
record_rc "settings.daemon detaches" "$?" 0
record_contains "detached parent reports start" "$WORK/out.txt" "daemon started"
wait_ready; record_rc "auto-daemon becomes ready" "$?" 0
"$BIN" down > /dev/null 2>&1

# E12: DEVTETHER_FOREGROUND=1 (service-manager guard) must prevent forking.
reset_all
mkcfg "autodaemon.yaml" 'settings:\n  daemon: true\nproxy:\n  port: 18082\ndns:\n  bind: "127.0.0.1:15355"\nroutes:\n  app.localhost: 9099\n'
timeout 3 env DEVTETHER_FOREGROUND=1 "$BIN" up -c "$PROJ/autodaemon.yaml" > "$WORK/out.txt" 2>&1
record_rc "service guard keeps foreground" "$?" 124
record_absent "foreground run has no detach banner" "$WORK/out.txt" "daemon started"
kill_daemon

# E13: ANSI discipline — non-TTY and NO_COLOR output must be plain text.
reset_all
mkcfg "good.yaml" 'routes:\n  app.localhost: 9099\n'
NO_COLOR=1 timeout 2 "$BIN" up -c "$PROJ/good.yaml" > "$WORK/plain.txt" 2>&1
if grep -q "$(printf '\033')" "$WORK/plain.txt" 2>/dev/null; then record "NO_COLOR strips ANSI" "ansi" "plain";
else record "NO_COLOR strips ANSI" "plain" "plain"; fi
timeout 2 "$BIN" up -c "$PROJ/good.yaml" > "$WORK/nontty.txt" 2>&1
if grep -q "$(printf '\033')" "$WORK/nontty.txt" 2>/dev/null; then record "non-TTY strips ANSI" "ansi" "plain";
else record "non-TTY strips ANSI" "plain" "plain"; fi
kill_daemon

# E14: logs -f with no daemon must exit 0 with an honest message (DX-5).
reset_all
mkdir -p "$PROJ/.logs"
printf 'old line\n' > "$PROJ/.logs/devtether.log"
timeout 5 "$BIN" logs -f > "$WORK/out.txt" 2>&1
record_rc "logs -f with no daemon exits" "$?" 0
record_contains "logs -f honesty message" "$WORK/out.txt" "No running daemon detected"
record_absent "logs -f does not claim shutdown" "$WORK/out.txt" "shut down"

# E15: non-text log content must not crash the reader.
printf '\000\001\002binary\377junk\n' > "$PROJ/.logs/devtether.log"
"$BIN" logs > "$WORK/out.txt" 2>&1; record_rc "logs with binary content" "$?" 0

# E16: an unreadable log file must produce a clear failure.
reset_all
mkdir -p "$PROJ/.logs"
printf 'secret\n' > "$PROJ/.logs/devtether.log"
chmod 000 "$PROJ/.logs/devtether.log"
"$BIN" logs > "$WORK/out.txt" 2>&1
record_rc "logs with unreadable file" "$?" 1
record_contains "logs unreadable message" "$WORK/out.txt" "log file"
chmod 644 "$PROJ/.logs/devtether.log"

# E17: repeated start/stop cycles stay clean and leak no processes.
reset_all
mkcfg "up.yaml" "$CFG_UP"
cyc=0
cycfail=0
while [ "$cyc" -lt 3 ]; do
    "$BIN" up -d -c "$PROJ/up.yaml" > /dev/null 2>&1 || cycfail=$((cycfail + 1))
    wait_ready || cycfail=$((cycfail + 1))
    "$BIN" down > /dev/null 2>&1 || cycfail=$((cycfail + 1))
    cyc=$((cyc + 1))
done
record "3 restart cycles clean" "$cycfail" "0"
record "no orphans after cycles" "$(pgrep -f "$BIN" | wc -l | tr -d ' ')" "0"
record "socket absent after cycles" "$([ -e "$RT/devtether/devtether.sock" ] && echo present || echo absent)" "absent"

# E18: shell entry points must stay syntactically valid and safely idempotent.
sh -n "$REPO/scripts/uninstall.sh"; record_rc "uninstall.sh parses" "$?" 0
sh "$REPO/scripts/uninstall.sh" -b "$WORK/no-such-bin-dir" > "$WORK/un.txt" 2>&1
record_rc "uninstall.sh on missing binary" "$?" 0
record_contains "uninstall.sh reports nothing to do" "$WORK/un.txt" "Nothing to uninstall"
sh -n "$REPO/scripts/install.sh"; record_rc "install.sh parses" "$?" 0

printf '\n=== F. Summary ===\n'
printf 'passed: %d   failed: %d\n' "$pass" "$fail"
if [ "$fail" -gt 0 ]; then
    printf 'ISSUES FOUND:%s\n' "$ISSUES"
else
    printf 'No issues found.\n'
fi
kill_daemon
printf 'BRUTAL_DONE\n'
