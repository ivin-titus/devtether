# Agent Diary - Scratchpad

*(This file is append-only for the duration of the task. It is a messy workspace for diagrams, math, or raw logs.)*

## Dump: 09/14/2026 21:30

**Workspace Sync:**
Cleared out all legacy Phase 1 (beta 3-6) implementation plans, discussion dumps, and audit findings from the beta.7 workspace. This workspace is now perfectly scoped and zeroed-in on the sprint direction for System Integration (The Tri-Layered Daemon, Setup Wizard `init`, and CLI `status`/`doctor`).

## Dump: 09/17/2026 (Workspace Reconciliation — Cline: GLM 5.3 Flash)

**Reconciliation pass completed** across `tracker.md`, `implementation_plan.md`, `audit_report.md`, and `discussion_space.md`:

- Verified every SC/P3/R4 finding in `audit_report_cline.md` against the actual source tree (24/26 P3/R4 open; R4-8 confirmed open at `proxy/handler.go:67`).
- Subphase renumbering: 3.2.1 + 3.2.2 → **3.3 (Consolidated Remediation)**; old 3.3 → **3.4 (Documentation Sync & Final Consistency)**.
- Plan rewritten in future tense; regression-test mandate attached to every fix.
- Architectural-level resolutions recorded in `discussion_space.md` (exit codes, proxy streaming exemption, service-manager precedence, synchronous IPC bind, PID fallback, syscall test seam, "doctor reports, init applies").
- Deferred items logged in `tracker.md` Scope Discoveries; deviation record in `audit_report.md` §13.

**Tooling note:** this environment exposed no native file-editing tool, so the edits were applied via precise, assert-guarded Python string replacements in the shell. Flagging the deviation from the AGENTS.md native-tool preference for reviewer awareness. Source code, tests, and runtime configs were not touched (verified via `git status`: only `docs/workspace/v2.0.0-beta.7/` files changed).

## Dump: 09/17/2026 (Product-Level Blind-Spot Audit & Polish Planning — Cline)

**Scope:** product/DX audit + polish planning for v2.0.0-beta.7 @ 8e87771 (diffed against `develop` = v2.0.0-beta.6). The shipped delta is exactly Phases 1–3 of `implementation_plan.md` (commits 949ad64, a28aed2, 8e87771). `make test` passed 8/8 at audit time.

**Method:** real user-journey walkthrough (install → init → up → daily use → failure/recovery), then each gap traced into code, docs, tests, and DX. Existing artifacts used as seeds and re-verified against source, not treated as truth. No source code touched.

**Tooling correction:** the note above ("no native file-editing tool") applied to that earlier session only. In this session the native `editor` tool **is** available and was used for every artifact edit below — no shell-based file writes.

**Verified anchors behind the new findings (recorded as DX-1…DX-15 in `audit_report.md` §15):**
- `examples/full-config.yaml:36-39` ships `proxy.timeouts.read/write`; strict decoding (`config.go:113` `KnownFields(true)`) rejects the repo's own reference example. Its comment also advertises the removed OS-assigned-port tier (SC-10).
- `up.go` startup summary + `daemonize()` readiness warning key fallback detection to the literal `:80` and always blame "root access / setcap"; proxy fallback on `EADDRINUSE` is Debug-only (`proxy/server.go:88-101`) → the surfaced reason is wrong whenever a custom `proxy.port` is set or the port is merely occupied.
- Total DNS bind failure is logged Debug-only in `up.go:141-146`; the startup summary has no DNS line; route health checks probe backend ports, not DNS → "Started" + "online" UI while `.localhost` doesn't resolve.
- `logs -f`: `WaitForExit` returns `nil` when the lock file can't be opened (missing OR cross-UID permission denied) → false "DevTether daemon shut down." message; also printed when no daemon was ever running.
- `logs --lines 0` silently switches into follow mode; negative values print nothing and exit 0.
- Single-instance-per-user model invisible: `CheckRunning` error is a bare socket path; `StatusResponse` carries no config identity → multi-project users can't tell which daemon `down` will stop.
- `docs/architecture.md` claims the route table is "Hot-reloadable via the IPC daemon (no restart needed)" while the IPC API is read-only (`/routes`, `/status`, `/shutdown`) and reload is deferred.
- `init`: warns-but-proceeds on missing config but fails to provide a verification hint.
- `generateUnit`: system tier sets `User=nobody` when the proxy port is unprivileged, with `WorkingDirectory` under the invoking user's home → unit cannot start; design question parked in `discussion_space.md`.
- Cross-UID root-daemon hint exists only in `status.go`; `routes` and `up` print misleading "is it running?" / "permission denied" messages.
- `doctor`: missing-config failure lacks the `devtether init` hint that `up` has; port-80 check ignores the configured `proxy.port` and hardcodes the "8080 fallback" narrative.
- `down`: non-permission IPC failures (403 nonce mismatch) return bare errors; the PID+SIGTERM fallback is wired only for permission errors.
- README: Commands table missing `service` commands; `status` described as showing routes (it shows a count); DNS snippet routes `~internal` to a daemon that only serves `.localhost`; examples pointer advertises "proxy timeouts" (see DX-1).
- No-routes startup hint says "Create one with: devtether init" — but `init` exits 1 because the config file exists (up already loaded it).

**Renumbering applied:** existing Subphase 3.4 (Docs sync) → **3.5** across tracker / plan / audit_report / discussion_space; new **Subphase 3.4 = Product & DX Polish / Missing Pieces** consolidating the DX findings. `audit_report_cline.md` left immutable (contains no 3.4 references).
**Reconciliation pass (post-edit re-read, same session):**
- Fixed 5 stale renumbering references that still called the *documentation* subphase "3.4" (`audit_report.md` lines 622, 665, 709, 745; `discussion_space.md` line 442) → now "3.5", each noting the intervening Product & DX Polish 3.4.
- Found and fixed a cross-artifact contradiction **not** introduced by this session: `tracker.md` marked **Subphase 3.3 (Consolidated Remediation) as `[x]` complete**, while `implementation_plan.md` Subphase 3.3 says "planned work, not completed work" and `audit_report.md` §13.1 confirms 24/26 P3/R4 findings open in the shipped tree (`8e87771`). Restored to `[ ]` with an explicit *Status:* line. This is exactly the kind of accidental "marked resolved" state the workspace discipline forbids.
- Added the DX-1…DX-15 severity distribution (0/1/7/7/0) to the §15 update note in `audit_report.md`; the dated 09/15 Risk Summary table was deliberately left unmodified and the note says so, so no historical record is falsified.
- Verified: the shipped delta vs `develop` (v2.0.0-beta.6) is exactly Phases 1–3 of `implementation_plan.md` (commits `949ad64`, `a28aed2`, `8e87771`); only `docs/workspace/v2.0.0-beta.7/` files are modified; no source, test, or config file touched.

**Verification-readiness verdict:** Subphase 3.4 is executable without guessing — every step names its finding ID, target file, expected behavior, and test; Parts A–E map 1:1 to §15 DX-1…DX-15 (DX-14's remainder routed to 3.5), Part D Step 2 implements the guard rail while the design choice itself stays parked in `discussion_space.md`, and the completion condition (implemented-with-test or moved-to-backlog, `make test` 8/8, outputs match docs) is checkable.

## Dump: 09/17/2026 (ADR Review + Completion-State Re-verification — Cline)

**Trigger:** the first pass left two gaps — the ADRs were only cited second-hand, and several §15 findings lacked explicit Root-cause/Direction fields.

**ADR review (all 10 ADRs + index read directly).** Three material results:

1. **DX-16 (new, Medium):** the permanent docs advertise a schema the binary rejects.
   - `docs/architecture.md:66` claims DNS answers `.localhost`, `.internal`, `.test`; `config.applyDefaults` sets the TLD set to `["localhost"]` only.
   - `docs/adr/005-yaml-config-schema.md:43-53` — the canonical example uses `proxy.tls`, `proxy.timeouts.read/write`, `dns.tld: ["localhost","internal"]`. `ProxyConfig`/`TimeoutConfig` expose only `port` + `timeouts.idle`, and `decoder.KnownFields(true)` (`config.go:113`, verified present) makes the extras fatal. ADR-005 is the schema contract the other docs are validated against, so the contract itself has drifted — this is the root of DX-1 and DX-14.
2. **DX-17 (new, Low):** ADR-009 section mis-citation. ADR-009's numbered decisions place §5 = "Strict Origin Validation (Future Web GUI)" and **§6 = "Secure System Daemonization"**. `implementation_plan.md:284` and `discussion_space.md:578` both cited §5 for the root-ownership/privilege-drop rule; `implementation_plan.md:419` cited "§4/§5" for Origin validation. `audit_report_cline.md:228` carries the same error but is immutable. Consequence is concrete: an agent following "§5" reads the Web-GUI rule and misses the daemonization mandate — including the `Group=` half.
3. **DX-10 strengthened:** `service.go:41` emits `User={{.RunAs}}` with **no `Group=`**, while ADR-009 §6 requires "`User=` **and** `Group=` directives". So the installer *partially* implements the mandate; the mis-citation in DX-17 is the plausible cause.
   Also **DX-11 strengthened:** ADR-003 Amendment §3 declares mixed-privilege operation (root daemon + unprivileged CLI) a *supported* scenario, so the missing cross-UID hints are a gap in a supported path.

**DX-18 (new, High, blocks the Subphase 3.3 gate) — the completion state is inverted.** Verifying ADR claims against the tree exposed that the "24 of 26 P3/R4 findings open" claim (§13.1, re-affirmed in the earlier reconciliation) is not supported by the shipped revision. Directly verified as **implemented in `8e87771`**:
- P3: 9 (`cli/tty.go`), 10 (`service.go:184` before `:187`), 11 (`systemdAvailableAt`), 12 (`activateLaunchd`→`deactivateLaunchd`), 13/14 (`WorkingDirectory` in template), 15 (`service.go:158` 0700), 16 (`applySetcap` warning), **17 (`service.go:42` quoted `ExecStart` — the "release-critical" item)**, 18 (`validateServiceTier`), 19 (`%q`).
- R4: 1 (typed errors), 2/SC-17 (`up.go:127,270` + `logger.go:37`), 4 (`up.go:94-98`), 5 (`errors.Is`), 6 (`client.go` dialer timeout), 7 (`client.go` ctx), **8 (`handler.go:67` narrow `interface{ HeadersSent() bool }` — the item the audit could not confirm)**, **9 (`daemon.go:146-147` `defer syscall.Umask(oldUmask)` — labelled Critical by the audit)**, 10 (`router.go:88`), 12 (`router.go:122,141`), 13 (`netutil/errors.go:19,22,31`), 15 (`TrimSpace` absent).
- SC: 1 (`up.go:445`), 2 (`up.go:77` + template `Environment=`), **3 (`lifecycle.go` + `TestReadPIDHandlesEmptyFile`)**, 4 (`down.go:35-48`/`status.go:41`), 5 (`ipcDaemon.Listen` before `Serve` — `up.go:155`/`:185`), 7 (`daemon.go:280`), 10 (`proxy/server.go:106`), 11 (`up.go:220`), 13 (`logs.go`), 14 (`up.go:410`), **16 (`config.go:113`)**.
- Genuinely still open: regression tests for F18–F21/A1–A7/SC-1/SC-4/SC-5 (only SC-3, service templates, `systemdAvailableAt`, `validateSystemBinaryRejectsWritableFile` exist), SC-9's *terminal* DNS-failure visibility (= DX-4), SC-15 build-tag isolation, Step 13 syscall seam.

**Self-correction disclosed:** the earlier pass reverted the tracker's Subphase 3.3 `[x]` → `[ ]` with the text "Not implemented", on the strength of §13.1. That text is wrong. The box legitimately stays `[ ]` *only* because the subphase's own rule demands a regression test per fix; the status now reads "Implemented in `8e87771`, under-tested — per-item re-verification required", and Part F owns the systematic pass. §13.4's stale "R4-8 confirmed open" line was corrected for the same reason.

**Artifacts changed this session:** `audit_report.md` (§15 header/disposition to DX-18, Root-cause+Direction added to DX-2…DX-15, new DX-16/DX-17/DX-18, §1 severity note, §13.4 corrections), `implementation_plan.md` (ADR-009 §5→§6 and §4/§5→§5 citations, Subphase 3.3 Part 1 Step 0 `Group=`, Subphase 3.4 Parts D/E/F + completion condition), `discussion_space.md` (§5→§6), `tracker.md` (3.3 status corrected; 3.4 description DX-1…DX-18). All within `docs/workspace/`; no source, test, or config file touched. `make test` 8/8, exit 0.
