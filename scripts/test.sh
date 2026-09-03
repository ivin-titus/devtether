#!/bin/sh
# DevTether local QA script.
# Mirrors CI checks so issues are caught before code leaves the dev machine.
#
# Usage:
#   ./scripts/test.sh
#
# Rule: this script must pass before pushing. If it passes locally, CI will pass.

set -eu

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

pass_count=0
skip_count=0
total_start="$(date +%s)"

step() {
    printf "\n${BOLD}==> %s${RESET}\n" "$1"
}

pass() {
    pass_count=$((pass_count + 1))
    printf "${GREEN}    ✓ passed${RESET}\n"
}

skip() {
    skip_count=$((skip_count + 1))
    printf "${YELLOW}    ⊘ skipped: %s${RESET}\n" "$1"
}

fail() {
    printf "${RED}    ✗ FAILED${RESET}\n" >&2
    exit 1
}

# ---------- 1. Module integrity ----------
step "Module integrity (go mod verify)"
go mod verify || fail
pass

# ---------- 2. Module hygiene ----------
step "Module hygiene (go mod tidy)"
go mod tidy
if ! git diff --quiet go.mod go.sum 2>/dev/null; then
    printf "${RED}    go.mod or go.sum has uncommitted changes after 'go mod tidy'.${RESET}\n" >&2
    printf "${RED}    Commit the changes and try again.${RESET}\n" >&2
    fail
fi
pass

# ---------- 3. Linting ----------
step "Linting (golangci-lint)"
if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint run --timeout=5m ./... || fail
    pass
else
    skip "golangci-lint not installed — CI will catch lint issues"
fi

# ---------- 4. Vet ----------
step "Vet (go vet)"
go vet ./... || fail
pass

# ---------- 5. Security scan ----------
step "Vulnerability scan (govulncheck)"
if command -v govulncheck >/dev/null 2>&1; then
    govulncheck ./... || fail
    pass
else
    skip "govulncheck not installed — CI will catch vulnerabilities"
fi

# ---------- 6. Unit tests + race detector ----------
step "Unit tests (go test -race)"
go test -race -count=1 ./... || fail
pass

# ---------- 7. Cross-compile check ----------
step "Cross-compile (darwin/arm64)"
GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/devtether || fail
pass

# ---------- 8. Build ----------
step "Build (linux)"
go build -o /dev/null ./cmd/devtether || fail
pass

# ---------- Summary ----------
total_end="$(date +%s)"
elapsed=$((total_end - total_start))

printf "\n${BOLD}────────────────────────────────${RESET}\n"
printf "${GREEN}${BOLD}  ✓ All checks passed${RESET} (%d passed, %d skipped, %ds)\n" "$pass_count" "$skip_count" "$elapsed"
printf "${BOLD}────────────────────────────────${RESET}\n"
