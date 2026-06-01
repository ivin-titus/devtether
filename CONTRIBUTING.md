# Contributing to DevTether

First off, thank you for considering contributing to **DevTether**! It's people like you that make the open-source community such an amazing place to learn, inspire, and create.

This document serves as a set of guidelines for contributing to this project. These are mostly guidelines, not strict rules. Use your best judgment, and feel free to propose changes to this document in a pull request.

## Code of Conduct

By participating in this project, you are expected to uphold our [Code of Conduct](CODE_OF_CONDUCT.md). Please report unacceptable behavior to the project maintainers.

## How Can I Contribute?

### 🐛 Reporting Bugs

Before creating bug reports, please check the existing issues as you might find out that you don't need to create one. When you are creating a bug report, please include as many details as possible:

*   Use a clear and descriptive title for the issue to identify the problem.
*   Describe the exact steps which reproduce the problem in as many details as possible.
*   Provide specific examples to demonstrate the steps.
*   Describe the behavior you observed after following the steps and point out what exactly is the problem with that behavior.
*   Explain which behavior you expected to see instead and why.
*   Include your Operating System, Go version, and local DNS setup.

### 💡 Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement request:

*   Use a clear and descriptive title for the issue to identify the suggestion.
*   Provide a step-by-step description of the suggested enhancement in as many details as possible.
*   Explain why this enhancement would be useful to most DevTether users.
*   You may also create an architectural diagram or draft a PR describing the enhancement if it modifies core subsystems (like DNS or the Reverse Proxy).

### 💻 Pull Requests

The process described here has several goals:
- Maintain DevTether's quality
- Fix problems that are important to users
- Engage the community in working toward the best possible lightweight developer tool

**Pull Request Process:**

1.  **Fork the repo** and create your branch from `main`.
2.  If you've added code that should be tested, **add unit tests or E2E tests**.
3.  If you've changed APIs or CLI flags, **update the documentation**.
4.  Ensure the test suite passes (`go test ./...`).
5.  Format your code with `go fmt`.
6.  Issue that pull request!

### Branch Naming Convention

If you plan to open a PR, please format your branch names intuitively:

*   `feature/feature-name` (for new features)
*   `fix/issue-description` (for bug fixes)
*   `docs/doc-updates` (for documentation)
*   `refactor/component-name` (for structural code changes)

## Development Setup

To get up and running:

1.  Ensure you have **Go 1.21+** installed.
2.  Clone your fork: `git clone https://github.com/YOUR_USERNAME/devtether.git`
3.  Navigate into the project: `cd devtether`
4.  Install dependencies: `go mod tidy`
5.  Run it: `go run ./cmd/devtether start`

## Architecture Review

Before working on large features, it's highly recommended you read the `docs/architecture.md` file. DevTether is built around 4 independent engines (Static Routing, Orchestration, Tunneling, Access Control) that share a common DNS + Proxy infrastructure. Understanding how the Routing Engine interacts with the DNS Resolver ensures your PR aligns with the system design.

## Community

We are building DevTether to make local development networking frictionless. If you have questions, feel free to open a "Discussion" on GitHub. We welcome contributors from all backgrounds and experience levels!
