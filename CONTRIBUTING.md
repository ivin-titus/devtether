# Contributing to DevTether

Thank you for considering contributing to **DevTether**! This document covers
everything you need to know before submitting a pull request.

## Code of Conduct

By participating in this project, you are expected to uphold our
[Code of Conduct](CODE_OF_CONDUCT.md).

## Engineering Standards

**Read before writing code:** [docs/engineering-standards.md](docs/engineering-standards.md)

This document defines the quality bar for all contributions — DRY, SoC, error
handling, testing, security, naming, and more. CI enforces these rules
automatically, so save yourself a round-trip and read them first.

### 📦 Third-Party Dependencies (Zero Bloat)
DevTether follows a strict "Standard Library First" philosophy to keep the binary small and secure. Every new third-party dependency significantly increases the audit surface area. 
- **Do not introduce new external dependencies** without a strong architectural justification in your PR. 
- Whenever possible, write idiomatic Go using standard library alternatives.

### 🔐 Secrets Handling
**Never commit secrets, API keys, or sensitive configuration data.** Always rely on local `.env` files or Git-ignored configurations when testing locally.

## AI Assistance & Liability

We welcome contributions generated with the help of AI agents (e.g., Google AntiGravity, GitHub Copilot). However, you must adhere to our strict Human Accountability Policy (see [ADR-010](docs/adr/010-ai-contribution-liability.md)):

1. **You are the Author:** AI agents are considered *tooling*. The human developer submitting the pull request assumes 100% legal, functional, and security liability for the code.
2. **Mandatory Auditing:** All AI outputs must be audited against these engineering standards before being pushed. "The AI wrote it" is not a valid excuse for failing tests or violating project architecture.
3. **Traceability:** When claiming a task in our workspace trackers, append the AI model you are using to your handle (e.g., `@github-username (via Agent:Model)`).

## How to Contribute

### 🐛 Reporting Bugs

> **SECURITY:** If you have found a security vulnerability, **DO NOT** open a public issue. Please follow the instructions in our [Security Policy](SECURITY.md) to report it privately.

Before creating a bug report, check existing issues. When filing:

- Use a clear title that identifies the problem
- Describe exact steps to reproduce
- Include your OS, Go version, and `devtether version` output
- Attach your `devtether.yaml` (sanitized) if relevant

### 💡 Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. Include:

- A clear title identifying the suggestion
- Step-by-step description of the proposed behavior
- Why this would be useful to most DevTether users
- Reference relevant [ADRs](docs/adr/) if the change affects architecture

### 💻 Pull Requests

#### Before You Start

1. Read [docs/engineering-standards.md](docs/engineering-standards.md)
2. Read [docs/architecture.md](docs/architecture.md) for large features
3. Read [ADRs](./docs/adr/)
4. Check existing issues and PRs to avoid duplicate work

#### Development Setup

```bash
# 1. Clone your fork
git clone https://github.com/YOUR_USERNAME/devtether.git
cd devtether

# 2. Install dependencies
go mod tidy

# 3. Create a starter configuration
go run ./cmd/devtether init

# 4. Add at least one route to devtether.yaml

# 5. Run the daemon
go run ./cmd/devtether up

# 6. Verify everything works
make test
```

**Requirements:**
- Go 1.27.1+ (see `go.mod` for exact version)
- Unix-based OS (Linux or macOS) — see [ADR-006](docs/adr/006-platform-support-and-cgo-policy.md)

**Recommended tools** (CI uses these — install locally for faster feedback):
- [`golangci-lint`](https://golangci-lint.run/welcome/install/) — linter suite
- [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) — vulnerability scanner

#### Before You Push

```bash
# Run the local QA suite
make test
```

The script covers the core QA categories but may skip optional tools that are not installed locally and runs on your current OS. CI remains authoritative and tests both Linux and macOS. The script runs:

1. Module integrity verification (`go mod verify`)
2. Module hygiene check (`go mod tidy` drift detection)
3. Linting (`golangci-lint` if installed)
4. `go vet`
5. Security scanning (`govulncheck` if installed)
6. Unit tests with race detection (`go test -race`)
7. Cross-compilation check (macOS build)
8. Binary build

#### Pull Request Process

1. **Fork the repo** and create your branch from `develop`. *(Note: `develop` is our active integration branch. `main` is strictly reserved for stable releases. Do not PR against `main`.)*
2. **Write tests** — all new code must have tests, bug fixes must include regression tests
3. **Update docs** if you changed CLI flags, behavior, or architecture
4. **Run `make test`** and make sure it passes
5. **Mandatory Manual Testing** — Passing `make test` is **not** enough. You must manually verify that your code works exactly as intended in a real environment and that you have not accidentally touched or broken unrelated, out-of-scope components.
6. **Open the PR** with a descriptive title following Conventional Commits

#### Branch Naming

| Pattern | Use Case |
|---|---|
| `feature/feature-name` | New features |
| `fix/issue-description` | Bug fixes |
| `docs/doc-updates` | Documentation |
| `refactor/component-name` | Structural changes |
| `ci/pipeline-changes` | CI/CD changes |
| `test/test-additions` | Test additions |

#### Commit Messages

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

#### PR Requirements

- [ ] CI passes (all jobs: lint, security, test)
- [ ] All new/changed code has tests
- [ ] Bug fixes include regression tests
- [ ] No unresolved lint warnings
- [ ] Commit messages follow Conventional Commits
- [ ] Documentation updated if CLI flags or behavior changed

## Architecture

Before working on large features, read [docs/architecture.md](docs/architecture.md).
DevTether is built around modular engines (Static Routing, Orchestration,
Tunneling, Access Control) that share common DNS + Proxy infrastructure.
Understanding how the Routing Engine interacts with the DNS Resolver ensures
your PR aligns with the system design.

## Questions?

Open a [Discussion](https://github.com/ivin-titus/devtether/discussions) on
GitHub. We welcome contributors from all backgrounds and experience levels.
