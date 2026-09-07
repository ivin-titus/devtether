<!-- 
Thank you for contributing to DevTether! 
Please read `CONTRIBUTING.md` and `docs/engineering-standards.md` before submitting your PR.
-->

## 📖 Description
Please include a summary of the change and which issue is fixed. Please also include relevant motivation and context.
If this is a structural or architectural change, please link to the relevant discussion or ADR.

Fixes # (issue)

## 🏗️ Type of Change
Please select the relevant options:
- [ ] 🐛 Bug fix (non-breaking change which fixes an issue)
- [ ] ✨ New feature (non-breaking change which adds functionality)
- [ ] 💥 Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] 📚 Documentation update
- [ ] 🧹 Refactor / Code Quality (no functional changes)
- [ ] 🚀 Performance optimization

## 🧪 How Has This Been Tested?
Please describe the tests that you ran to verify your changes. Provide instructions so we can reproduce. 
- [ ] `go test -race ./...` (Mandatory for all PRs)
- [ ] `./scripts/test.sh` (Highly recommended for Linux/macOS contributors)
- [ ] Manual end-to-end testing (e.g. running `devtether up` locally)
- [ ] Cross-platform compilation (`GOOS=darwin go build ...`)

**Test Configuration / Environment:**
- OS: 
- Go Version: 
- Network constraints (if applicable):

## 📋 Strict Engineering Standards Checklist:
*Before requesting a review, you must verify all of the following:*

- [ ] I have read and strictly followed the `docs/engineering-standards.md`.
- [ ] I have read `docs/architecture.md` and the relevant `docs/adr/` records if making structural changes.
- [ ] I have run `./scripts/test.sh` locally and it passes with **0 warnings and 0 errors** (including `golangci-lint` and `govulncheck`).
- [ ] My code introduces **no new `sync` package deadlocks or data races** (verified via `-race`).
- [ ] I have added/updated unit tests that prove my fix is effective or that my feature works.
- [ ] I have verified that errors are correctly wrapped via `fmt.Errorf("...: %w", err)` and not swallowed.
- [ ] I have explicitly handled `context.Context` propagation where applicable.
- [ ] I have updated the documentation accordingly (e.g., `README.md`, `docs/architecture.md`, `CHANGELOG.md`).
