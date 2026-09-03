# DevTether Engineering Standards

This document defines the code quality standards that **all contributions** must
follow. These are not suggestions — they are enforced by CI and local tooling.

Read this before writing your first line of code.

---

## Core Principles

### 1. Match Production-Grade Go Quality

The codebase follows patterns found in production Go infrastructure (Kubernetes,
etcd, CoreDNS): clean interfaces, explicit error handling, `errgroup`-based
lifecycle management, and documented ADRs. New code must match this bar.

### 2. No AI Slop

No boilerplate-heavy, over-abstracted, or "just in case" code. Every function,
type, and file must earn its existence. If it doesn't solve a concrete problem,
it doesn't ship.

### 3. No Patchwork

Don't bolt on quick fixes that create technical debt. If a fix requires touching
3 files with band-aids, step back and find the proper abstraction.

### 4. No Overengineering

Don't build abstractions for problems that don't exist yet. Don't add config
options nobody asked for. Don't create interfaces with one implementation. Write
the simplest correct solution.

### 5. The Balance

Write code that a Kubernetes contributor would review and approve — not
impressed by cleverness, but by clarity, correctness, and restraint.

---

## DRY (Don't Repeat Yourself)

- **No duplicate logic across files.** If the same pattern appears in 2+ places,
  extract it into a shared package. Example: network error detection must live in
  one place, not be reimplemented differently in `proxy/` and `dns/`.
- **No duplicate constants or format strings.** Version templates, default values,
  and error messages must be defined once and referenced everywhere else.
- **DRY does NOT mean premature abstraction.** Two lines that look similar but
  serve different purposes are fine. DRY applies to **logic and intent**, not
  surface syntax.

---

## Separation of Concerns (SoC)

- **One responsibility per file.** A CLI command file handles flag parsing and
  output formatting — not server lifecycle, not data transformation. If a
  function does 3 things, it should call 3 separate functions.
- **Package boundaries are API contracts.** Each `internal/` package owns one
  domain:

  | Package | Owns |
  |---|---|
  | `config` | YAML parsing, schema validation, defaults |
  | `router` | In-memory routing table, domain → target resolution |
  | `dns` | DNS protocol handling, A record responses |
  | `proxy` | HTTP reverse proxying, Host header routing |
  | `daemon` | Unix socket IPC, CLI ↔ daemon communication |
  | `cli` | Flag parsing, user-facing output, error formatting |

  Don't leak responsibilities across boundaries.
- **CLI is a thin layer.** Commands should parse flags, call domain logic, format
  output, and return errors. Business logic does not belong in `internal/cli/`.

---

## Error Handling

- **Use `errors.Is` / `errors.As` for wrapped errors.** Never use
  `os.IsNotExist` or direct `==` comparison on potentially wrapped errors.
  `fmt.Errorf("...: %w", err)` wraps errors — only `errors.Is` unwraps them.
- **No string-matching on error messages for control flow.** Use typed error
  checks (`errors.As(err, &opErr)`, `syscall.EACCES`). Error message text can
  change between Go versions and OS locales.
- **Cobra commands use `RunE`, not `Run` + `log.Fatalf`.** `log.Fatalf` kills
  the process, bypasses Cobra error handling, prevents testing, and exits with
  code 2 instead of 1. Return errors and let Cobra handle them.
- **Deterministic output.** Map iteration in Go is random. Any output (CLI
  tables, JSON API responses, log lines) derived from maps must be sorted before
  rendering.
- **Errors are for callers; logs are for operators.** Return errors up the call
  stack with context (`fmt.Errorf("dns: failed to bind: %w", err)`). Only log at
  the point where you handle the error — never log and return.

---

## Concurrency

- **Race detector is mandatory.** All tests run with `-race`. Any data race is
  a P0 bug that blocks release.
- **Protect shared state.** If a field is accessed from multiple goroutines, it
  must be protected by a `sync.Mutex`, `sync.RWMutex`, or atomic operation.
  Document the locking invariant in a comment.
- **Context must be propagated.** HTTP clients, dialers, and long-running
  operations must accept and respect `context.Context` for cancellation and
  timeouts. Never ignore a context parameter.
- **Prefer `errgroup` for goroutine lifecycle.** The codebase already uses
  `errgroup` in `up.go`. New concurrent work should follow the same pattern:
  launch goroutines via `g.Go()`, return errors, and let the group coordinate
  shutdown.

---

## Testing

- **All new code ships with tests.** No exceptions. If a function is worth
  writing, it's worth testing.
- **Bug fixes must include regression tests.** A fix without a test that proves
  the bug existed is incomplete — it can silently regress.
- **Table-driven tests.** Follow the existing pattern in
  [config_test.go](../internal/config/config_test.go) and
  [router_test.go](../internal/router/router_test.go): define test cases as
  `[]struct{ name, input, want, wantErr }` and iterate with `t.Run`.
- **Use `t.Helper()`, `t.TempDir()`, `t.Cleanup()`.** These are standard Go
  test utilities. Don't reinvent temp file management.
- **Test Modules & Structure.** Isolate tests logically. Use separate test files
  for distinct components (`server_test.go`, `config_test.go`). Ensure edge
  cases, invalid inputs, and concurrency paths are thoroughly covered.
- **Ephemeral ports for network tests.** Never hardcode ports. Bind to `:0`
  and let the kernel assign an available port. Use `t.Cleanup()` to close
  listeners.

### Test Coverage Strategy (Current State)

While 100% coverage is the long-term goal, current test coverage prioritizes core business logic (`config`, `dns`, `router`). Some infrastructural packages are currently untested by design, pending architectural refactors:

- **`internal/cli`**: Currently uses `log.Fatalf`, which kills the process and breaks test runners. **Strategy:** Refactor commands to return errors (`RunE` pattern) before writing CLI unit tests.
- **`internal/proxy`**: Requires setting up mock backend HTTP servers (`httptest.Server`) to assert on headers (like `X-Forwarded-For`). **Strategy:** Add integration-style tests in a future phase.
- **`internal/daemon`**: Manages OS-level IPC (Unix domain sockets) which introduces cross-platform flakiness. **Strategy:** Isolate platform-specific dialing logic before testing.
- **`internal/netutil`**: Contains minimal wrapper logic (`errors.As`). Extremely low risk, but should be tested when time permits.

---

## Dependencies

- **Pure Go only.** No CGo dependencies. This is codified in
  [ADR-006](adr/006-platform-support-and-cgo-policy.md) and enforced
  by `CGO_ENABLED=0` in all builds.
- **Minimal dependency surface.** Every new `require` in `go.mod` must be
  justified. Prefer stdlib over third-party when the stdlib solution is
  reasonable. The current dependency count is intentionally small:
  - `miekg/dns` — DNS protocol (stdlib has no DNS server)
  - `spf13/cobra` — CLI framework (industry standard)
  - `x/sync` — errgroup (not in stdlib yet)
  - `gopkg.in/yaml.v3` — YAML parsing

- **Verify before build.** `go mod verify` runs both locally and in CI.
  Binaries are never built from unverified dependencies.
- **Vulnerability scanning.** `govulncheck` and `gosec` run on every PR.

---

## Naming & Exports

- **Unexported by default.** Only export types, functions, and variables that
  are part of the package's public API. If it's only used within the package,
  it's unexported (lowercase).
- **File names match content.** `version.go` contains version logic. `init.go`
  contains the init command. No grab-bag files like `utils.go` or `helpers.go`.
- **Package names are singular nouns.** `config`, `router`, `proxy`, `daemon` —
  not `configs`, `routers`, `proxies`.
- **Godoc on all exported symbols.** Every exported function, type, and constant
  must have a doc comment starting with its name.

---

## Logging

- **Prefix-based structured logging.** All log lines use `[component]` prefixes:
  `[dns]`, `[proxy]`, `[daemon]`, `[route]`, `[devtether]`. This is the
  existing convention — maintain it.
- **Log at the right level.** `log.Printf` for informational, `log.Fatalf` only
  in `main()` (never in library code). In library packages, return errors
  instead of logging.
- **Don't log and return.** Either log the error and handle it, or wrap it and
  return it. Doing both creates duplicate log lines.

---

## Security

- **Restrictive permissions by default.** Unix sockets use `0600`, socket
  directories use `0700`. Config files use `0644`.
- **No hardcoded secrets or credentials.** This includes test fixtures.
  Use environment variables or file paths.
- **Validate all user input at the boundary.** Config values, CLI flags,
  and HTTP headers must be validated before use. The `config` package's
  `Validate()` method is the model to follow.
- **Supply chain integrity.** `go mod verify` checks module checksums
  against `go.sum`. `govulncheck` checks for known vulnerabilities.
  `gosec` scans for insecure coding patterns.

---

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>
```

| Type | When to Use |
|---|---|
| `feat` | New feature or command |
| `fix` | Bug fix |
| `ci` | CI/CD pipeline changes |
| `docs` | Documentation only |
| `test` | Adding or fixing tests |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `chore` | Build system, dependency updates |

---

## Pre-Push Checklist

Before pushing any branch:

```bash
# Run the full local QA suite (mirrors CI)
./scripts/test.sh
```

This script runs: module verification → linting → vet → security scanning →
unit tests with race detection → cross-compilation check → build.

**If `scripts/test.sh` passes locally, CI will pass.**

---

## Pull Request Requirements

1. CI must pass (all 3 jobs: lint, security, test)
2. All new/changed code must have tests
3. Bug fixes must include regression tests
4. No unresolved lint warnings
5. Commit messages follow Conventional Commits format
6. Documentation updated if CLI flags or behavior changed
