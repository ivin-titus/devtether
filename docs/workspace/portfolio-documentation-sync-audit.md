# Repository Documentation Consistency Audit — DevTether

**Audit date:** 2026-09-28
**Repository:** ivin-titus/devtether
**Branch reviewed:** develop
**Baseline:** v2.0.0-beta.7 (2026-09-20)

This audit covers DevTether's own repository documentation only. It is not a portfolio-site audit. It records contradictions, stale status, misleading current-vs-future wording, broken references, and source-of-truth drift across README, docs/, ADRs, CHANGELOG, contributor docs, CI documentation, and workspace records.

## 1. Current implementation baseline

The current shipped beta is **Engine 1: local static routing / local networking**.

Implemented:
- named .localhost routes
- embedded route-aware DNS
- HTTP reverse proxy
- WebSocket/SSE-friendly proxy streaming
- strict hostname and route validation
- loopback-only backend routing
- Unix-socket IPC daemon
- secure runtime/socket lifecycle
- instance locking, PID/nonce handling and stale-socket cleanup
- devtether init interactive OS-aware setup
- detached daemon mode
- status, routes, logs, doctor and down
- graceful shutdown
- Linux + macOS builds, amd64 + arm64
- Go 1.27.1 baseline
- CI race tests, vet, lint, vulnerability scanning and release automation

Not implemented in beta.7:
- process orchestration / process groups
- dynamic PORT allocation and process supervision
- LAN sharing
- WAN tunneling
- self-hosted relay runtime
- RBAC / IAM / access-control engine
- .internal DNS namespace
- dynamic route hot reload
- Web GUI / traffic-inspection UI

Future architecture is valid roadmap material, but must not read as shipped functionality.

## 2. Documentation issues

### 2.1 README current-status table is too broad
README.md describes the Networking Layer as Reverse Proxy + DNS + Smart CORS + Traffic Inspection and marks it Beta Complete. The current source tree does not contain a shipped traffic-inspection subsystem or a distinct Smart CORS implementation matching that description.

**Action:** narrow the current-status table to verifiably shipped beta.7 capabilities; move traffic inspection/GUI work to clearly marked future sections.

### 2.2 README and PRD mix current and future architecture
README.md and docs/PRD.md place the three-layer / four-engine target architecture beside current usage instructions. Readers can mistake planned orchestration, tunneling and access control for current capabilities.

**Action:** clearly separate “Implemented in beta.7” from “Planned architecture”.

### 2.3 PRD dependency section mixes current and future dependencies
docs/PRD.md lists packages such as gorilla/websocket and golang.org/x/crypto/acme as future dependencies inside a generic “Key Dependencies” section.

**Action:** split current dependencies from planned dependencies.

### 2.4 Architecture document needs current-vs-target separation
docs/architecture.md contains a diagram and prose covering orchestrator, access/tunneling, self-hosted relay and Web GUI even though the current source does not ship these pieces.

**Action:** split into “Current beta architecture” and “Target/future architecture”. Current architecture should describe what actually runs in beta.7.

### 2.5 Hot reload must remain explicitly absent
Current beta.7 route changes require a daemon restart. Live route mutation was intentionally deferred.

**Action:** remove any present-tense hot-reload wording. Keep it only as future work/history.

### 2.6 DNS port documentation has historical drift
Current beta.7 default is 127.0.0.1:5335. Older documentation/history includes port 53 defaults and 5353 fallback behavior.

**Action:** make 127.0.0.1:5335 the canonical current default; retain 53/5353 behavior only as historical release information.

### 2.7 Proxy fallback documentation has historical drift
Current behavior is configured proxy port -> 8080 fallback -> fatal if fallback cannot bind. There is no current OS-assigned ephemeral port tier.

**Action:** remove any current-facing :0/ephemeral fallback claim.

### 2.8 .internal is future-only
The current DNS engine serves .localhost. .internal is reserved for future LAN/tunnel functionality.

**Action:** mark .internal as future-only everywhere outside historical material.

### 2.9 “Zero dependency” wording should be normalized
The project has Go module dependencies. The intended product property is **single Go binary with no external runtime services**.

**Action:** standardize this wording across README, PRD, architecture docs and release messaging.

### 2.10 Platform wording should distinguish support from testing confidence
Current releases support Linux and macOS on amd64/arm64. Linux receives the most regular development testing; macOS needs broader community feedback and OS-specific testing.

**Action:** state both as supported, then separately describe the difference in routine testing coverage. Windows remains unsupported.

### 2.11 Go toolchain wording is stale
The current go.mod requires Go 1.27.1.

**Action:** replace stale Go 1.21+ wording in current setup documentation with Go 1.27.1+ where appropriate.

### 2.12 ADR-012 reference appears inconsistent
CHANGELOG.md for beta.7 says ADR-012 was created, but the reviewed docs/adr tree contains ADR-001 through ADR-011 and no ADR-012 file.

**Action:** determine whether ADR-012 is missing or the changelog/reference is wrong, then reconcile the ADR index and release documentation.

### 2.13 Workspace index is stale
docs/workspace/README.md still calls beta.6 the currently live release while CHANGELOG.md contains beta.7 dated 2026-09-20.

**Action:** refresh the workspace index when beta.7 documentation is finalized.

### 2.14 Workspace audit artifacts contain historical intermediate states
docs/workspace/v2.0.0-beta.7 artifacts contain audit-era findings, corrections and superseded states. That is useful history, but unsafe as current product truth.

**Action:** preserve the audit trail, but clearly distinguish historical audit state from final/current implementation state. Current docs should not depend on workspace artifacts as their authority.

### 2.15 Service-management references require reconciliation
Historical beta.7 workspace material references an internal/cli/service.go implementation and systemd/launchd service management, but that file was not present in the reviewed develop tree.

**Action:** reconcile repository history, current source, ADRs and CHANGELOG before making any current claim about shipped service-management commands.

### 2.16 Current CLI documentation should be source-driven
The shipped command surface includes init, up, down, status, routes, logs, doctor and version.

**Action:** use current CLI code/help/tests as the authority for command documentation instead of older roadmap text.

## 3. Recommended cleanup sequence

### Pass A — Current-state truth
Synchronize README, architecture and the ADR index with beta.7 implementation.

### Pass B — Future-state labeling
Explicitly label orchestration, tunneling, relay, access control, traffic inspection and GUI as planned/future where applicable.

### Pass C — Historical separation
Keep release history and architectural evolution, but prevent superseded behavior from reading like current runtime behavior.

### Pass D — Repository-wide integrity
Run a final Markdown/link/content consistency audit after wording changes.

### Pass E — Release verification
Verify that current documentation describes the exact tagged/current implementation and CLI surface.

## 4. Source-of-truth hierarchy

For future synchronization, use this order:

1. Current source code and tests
2. Current CI/release configuration
3. Current CHANGELOG
4. Current accepted ADRs
5. Current README / architecture docs
6. PRD
7. Workspace planning/audit artifacts

Workspace artifacts preserve reasoning and history; they should not override current implementation state.



# 5. Deep Repository-Only Contradiction Pass

The original audit already identified broad README/PRD/architecture drift. This section records the more granular contradictions found by reading the current repository documentation and workspace artifacts against each other and against the current develop implementation where needed.

## DOC-17 — Web GUI is current in architecture but future everywhere else

**Severity:** Critical

Evidence:
- docs/architecture.md describes a Web GUI at devtether.localhost in present tense.
- docs/architecture.md describes GUI config editing, an inspector, and a unified CLI/GUI IPC API as existing architecture.
- docs/adr/009-proxy-security-lessons.md says the project does not yet host a Web GUI or SSE stream.
- docs/workspace/.ideas/gui_rest_api.md says the GUI is deferred until a richer IPC API exists.
- The current source tree contains no GUI implementation.

**Impact:** A contributor following architecture.md can design against components that do not exist.

**Action:** Architecture must have a hard current/future boundary. The GUI and inspector should appear only in the future section until implemented.

---

## DOC-18 — GUI implies dynamic route editing while current routing requires restart

**Severity:** High

Evidence:
- docs/architecture.md says route changes currently require devtether down && devtether up.
- The same document says the GUI can update devtether.yaml through IPC when a service is added.
- The same document describes future mutating IPC endpoints as planned.

**Action:** Keep dynamic config mutation entirely under future architecture. Current beta documentation should say the running router is populated from startup configuration and route changes require restart.

---

## DOC-19 — Engine/layer taxonomy is internally inconsistent

**Severity:** High

Evidence:
- ADR-001 defines four engines and maps Engine 3 (Tunneling) and Engine 4 (Access Control) into Layer 3.
- architecture.md describes cross-device tunnels as part of the remaining Layer 1.
- README's three-layer table has no explicit Tunneling layer and makes Layer 3 appear to be only access control.
- PRD similarly mixes LAN/WAN sharing and access control under one Layer 3 label.

**Action:** Establish one canonical mapping and repeat it consistently:
Engine 1 = local static networking; Engine 2 = orchestration; Engine 3 = LAN/WAN tunneling; Engine 4 = access control.

---

## DOC-20 — ADR-002's original DNS response rule was not rewritten after its amendment

**Severity:** High

Evidence:
- The original ADR-002 decision says all other queries receive NXDOMAIN.
- Its 2026-09-07 amendment says an existing route with an unsupported qtype must return NOERROR with zero answers (NODATA).
- Both statements remain in the same accepted ADR.

**Action:** Rewrite the base decision so the current NXDOMAIN/NODATA contract appears once. An amendment should not leave an earlier absolute sentence looking authoritative.

---

## DOC-21 — .internal has mutually exclusive statuses

**Severity:** High

Evidence:
- ADR-002 says .internal/custom TLD support was explicitly dropped.
- ADR-005 says .internal is reserved for future Engine 3.
- architecture.md defines .internal as the future LAN-sharing namespace.
- Workspace backlog documents .internal as future work.

**Action:** Decide whether .internal is permanently rejected or deferred. The rest of the repository treats it as deferred, so ADR-002 should be reconciled with that position.

---

## DOC-22 — Current throttle-cache architecture is incorrectly described as LRU/TTL

**Severity:** High

Evidence:
- architecture.md calls the mechanism a hybrid LRU/TTL random map eviction cache.
- ADR-009 uses similarly broad terminology.
- docs/workspace/.ideas/hybrid_ttl_eviction.md explicitly records that the actual implementation is pure random eviction with cooldown timestamps and that true TTL eviction is future work.
- The current proxy handler uses a bounded map and deletes one arbitrary key when the limit is reached; there is no LRU list or TTL sweeper.

**Action:** Document the current mechanism precisely. Do not call it LRU/TTL until those behaviors actually exist.

---

## DOC-23 — ADR-001's activation model is not the current config behavior

**Severity:** Medium

Evidence:
- ADR-001 says an engine is activated by the presence of its config section.
- The current config validator explicitly rejects orchestrate:, tunnel:, and access: because those engines are not implemented.
- ADR-005 presents those sections as future schema.

**Action:** Change ADR-001 wording to make the config-section activation rule a target architecture decision, not a current beta contract.

---

## DOC-24 — ADR-005 labels a non-current schema as “Currently Supported”

**Severity:** High

Evidence:
- ADR-005's current Engine 1 example uses DNS port 53.
- Other canonical docs and config defaults use 127.0.0.1:5335.
- The repository uses strict YAML decoding, so unsupported keys in examples can fail startup rather than merely being ignored.

**Action:** Make the current schema example exactly match the fields accepted by the current binary.

---

## DOC-25 — README/architecture and ADR-006 disagree on macOS support confidence

**Severity:** Medium

Evidence:
- README says macOS and Linux are “Fully supported”.
- ADR-006 calls Linux Tier 1 / Fully tested and macOS Tier 2 / Community-tested.
- The release build ships both platforms, while CI tests both, but the documented verification confidence differs.

**Action:** Use “supported” for both platforms and separately describe testing/verification coverage.

---

## DOC-26 — ADR-006 incorrectly describes CI as a single Ubuntu runner

**Severity:** Medium

Evidence:
- ADR-006 says CI/CD is a single ubuntu-latest runner and therefore needs no platform matrix.
- .github/workflows/ci.yml explicitly uses a matrix with ubuntu-latest and macos-latest for tests.
- The release build job is Ubuntu-only, but the repository CI as a whole is not.

**Action:** Narrow the statement to the release/build pipeline if that is what was intended.

---

## DOC-27 — ADR-006's Windows roadmap and PRD roadmap use different commitment language

**Severity:** Low

Evidence:
- PRD says native Windows support will be prioritized in future releases.
- ADR-006 says Windows is future consideration and may be reconsidered when demand is demonstrated.

**Action:** Use one neutral roadmap statement. Windows is currently unsupported; future work depends on demonstrated demand.

---

## DOC-28 — Engineering Standards overstates what CI mechanically enforces

**Severity:** Medium

Evidence:
- docs/engineering-standards.md opens by saying its rules are enforced by CI and local tooling.
- Several rules are inherently review-based: YAGNI, SoC, ADR compliance, AI accountability, no patchwork, documentation consistency, and architectural judgment.

**Action:** Separate automated gates from review-enforced standards.

---

## DOC-29 — Engineering Standards' test-coverage status is stale

**Severity:** Medium

Evidence:
- The coverage section says internal/daemon is currently untested by design.
- It says internal/netutil is scheduled for comprehensive test coverage.
- The current tree contains tests for both daemon lifecycle and netutil behavior.

**Action:** Replace “untested” with an accurate description of remaining coverage gaps.

---

## DOC-30 — Engineering Standards define an access logging namespace that is not a current subsystem

**Severity:** Medium

Evidence:
- The logging namespace table includes [access] as a standard emitted by an access/traffic layer.
- Current source uses the proxy logging namespace for the relevant current proxy path.
- The access-control engine is not implemented.

**Action:** Mark [access] explicitly as a future namespace or remove it from the current namespace contract.

---

## DOC-31 — CONTRIBUTING falsely claims scripts/test.sh mirrors CI

**Severity:** High

Evidence:
- CONTRIBUTING.md says the script mirrors CI and that passing it means CI will pass.
- scripts/test.sh can skip golangci-lint and govulncheck when they are not installed.
- CI always runs those jobs.
- CI tests on both Ubuntu and macOS; scripts/test.sh runs on the contributor's current OS and only cross-builds Darwin.

**Action:** Call scripts/test.sh a local QA approximation, not an exact CI mirror. State that CI remains authoritative.

---

## DOC-32 — Pull request checklist conflicts with optional local checks

**Severity:** Medium

Evidence:
- .github/PULL_REQUEST_TEMPLATE.md asks for scripts/test.sh to pass with 0 warnings/errors including golangci-lint and govulncheck.
- scripts/test.sh explicitly permits those checks to be skipped.

**Action:** Align the checklist with actual script behavior, or make the tools mandatory before declaring the local suite complete.

---

## DOC-33 — CONTRIBUTING's first-run commands omit required configuration setup

**Severity:** Medium

Evidence:
- CONTRIBUTING.md instructs a new contributor to run go run ./cmd/devtether up after cloning.
- Current up requires devtether.yaml.
- The documented setup does not create that file first.

**Action:** Add devtether init or a minimal test config to the contributor flow.

---

## DOC-34 — Beta.7 changelog timing claim does not match the current liveness interval

**Severity:** Medium

Evidence:
- CHANGELOG says daemon liveness polling is 5ms when idle.
- internal/daemon/lifecycle.go defines a 500ms polling interval.

**Action:** Correct the historical release note or remove the implementation-specific timing claim.

---

## DOC-35 — Beta.7 changelog claims an ADR that is absent from the repository

**Severity:** Critical

Evidence:
- CHANGELOG says ADR-011 and ADR-012 were created.
- docs/adr/README.md ends at ADR-011.
- No ADR-012 file exists in the current develop tree.

**Action:** Either restore/index ADR-012 or correct the changelog. Do not leave a phantom canonical document reference.

---

## DOC-36 — Beta.7 changelog overstates the completeness of documentation cleanup

**Severity:** Medium

Evidence:
- CHANGELOG says contradictory comments and documentation issues were purged.
- The current repository still contains material status and architecture contradictions identified throughout this audit.

**Action:** Keep the historical note, but avoid wording that implies repository-wide documentation consistency was permanently achieved.

---

## DOC-37 — Workspace final verification declares ADRs perfectly synchronized when they are not

**Severity:** High

Evidence:
- docs/workspace/v2.0.0-beta.7/artifacts/audit_report.md says ADRs 001-011 “perfectly reflect” the shipped architecture.
- ADR-009 still has the service-management implementation claim.
- architecture.md still presents the GUI as current.
- ADR-002/ADR-005 still disagree on DNS/TLD contracts.

**Action:** Add a superseded/historical marker to the old audit conclusion. Historical verification results should not masquerade as the latest repository truth.

---

## DOC-38 — Workspace final audit says documentation drift was resolved, but later evidence disproves that

**Severity:** High

Evidence:
- The beta.7 audit report marks widespread documentation drift as resolved.
- The current branch contains several unresolved contradictions that survive that “resolution”.

**Action:** Preserve the historical report but explicitly state that the conclusion was superseded by the 2026-09-28 repository-only audit.

---

## DOC-39 — Beta.7 tracker contains a checkbox/status contradiction

**Severity:** High

Evidence:
- “Phase 4 Polish” is marked [x].
- Its own Status line says “Implemented — Awaiting Human Verification”.
- The text then says the checkbox stays open until system-owner verification.

**Action:** Make the checkbox and status agree.

---

## DOC-40 — Beta.7 tracker simultaneously defers and documents service management as implemented

**Severity:** Critical

Evidence:
- The tracker header says the OS Service module was entirely deferred.
- Later entries say Deep Documentation Sync formalized OS Service Management and historical tasks describe service install work as delivered.
- The current source tree contains no service implementation.

**Action:** Clearly label those service entries as historical/superseded or remove their implementation claim from current-state summaries.

---

## DOC-41 — Service mode deferred artifacts conflict with permanent ADR status

**Severity:** Critical

Evidence:
- docs/workspace/.ideas/service_mode/ is explicitly deferred.
- ADR-009 says service install is implemented.
- The current develop tree has no matching service implementation.

**Action:** ADR-009 must be reconciled with the current source state. Deferred workspace artifacts can remain, but they need a prominent historical/superseded label.

---

## DOC-42 — The deferred event-driven logging note has stale implementation timing

**Severity:** Low

Evidence:
- docs/workspace/.ideas/event_driven_logs.md describes a 50ms lock poll.
- Current lifecycle code uses 500ms.

**Action:** Mark the document as a historical snapshot or update its “current context” paragraph.

---

## DOC-43 — “Zero configuration” in Advanced Port Configuration is misleading

**Severity:** Medium

Evidence:
- docs/advanced-port-configuration.md starts by describing DevTether as working with “zero configuration”.
- Actual onboarding requires a config file and normally uses devtether init, with optional OS-level DNS changes requiring user consent.

**Action:** Use “safe defaults” or “minimal setup”.

---

## DOC-44 — Advanced Port Configuration does not explain the difference between requested and actual proxy port

**Severity:** Medium

Evidence:
- It says privileged proxy ports must be run with sudo/setcap.
- The current proxy first tries the requested port and can fall back to 8080 when the requested bind fails due to permission/address-in-use conditions.

**Action:** Explain that privileges are required to retain the requested privileged port, while the daemon may still start on its configured fallback.

---

## DOC-45 — README/PRD examples imply generic database protocol routing

**Severity:** Medium

Evidence:
- README uses db.localhost -> 8042 in its routing examples.
- PRD lists PostgreSQL, Redis and Elasticsearch under “databases — static port routing”.
- Current data plane is an HTTP reverse proxy, not a generic TCP proxy.

**Action:** Use HTTP services in current examples; reserve raw TCP/database routing for a future engine if it is actually planned.

---

## DOC-46 — PRD “one command” solution wording omits the actual initialization flow

**Severity:** Low

Evidence:
- PRD presents “One YAML file. One binary. One command: devtether up.”
- Current Quick Start requires initialization plus editing the YAML before up.

**Action:** Treat the one-command phrasing as a product goal/example, not the complete current onboarding contract.

---

## DOC-47 — Architecture current section still documents future LAN mode as if it belongs to the current DNS engine

**Severity:** Medium

Evidence:
- architecture.md documents DNS LAN bind 0.0.0.0:53 and LAN IP responses under the shared DNS implementation section.
- Current beta is loopback-only and the LAN/tunneling engine is unimplemented.

**Action:** Move LAN-specific behavior under future Engine 3 documentation.

---

## DOC-48 — ADR-003's security threat table mixes current and future attack surfaces without status markers

**Severity:** Medium

Evidence:
- ADR-003 lists orchestrator, WAN tunnel, and token/RBAC threats together with current IPC/DNS/proxy threats.
- The project has not implemented those future engines.

The information is valid as design planning, but the table visually reads as one current security surface.

**Action:** Split current beta threats from future-engine threat models.

---

## DOC-49 — ADR-009 current-status sections are inconsistent within the same document

**Severity:** High

Evidence:
- ADR-009 says service management is “SAFE / IMPLEMENTED”.
- The same ADR says the Web GUI/SSE is not yet hosted.
- Current source confirms GUI and service implementation are both absent.
- This means the ADR itself is internally asymmetric: one future feature is presented as shipped while another is explicitly future.

**Action:** Add explicit Current / Future status to each major section and make the service claim match the tree.

---

## DOC-50 — ADR-004 describes a multi-build release process that the current release configuration does not implement

**Severity:** Medium

Evidence:
- ADR-004 says GoReleaser will build both devtether and devtether-relay from separate cmd/ entry points.
- .goreleaser.yaml currently defines only the devtether build.
- No current cmd/devtether-relay source exists.

**Action:** Either mark ADR-004's build plan as future, or create the relay implementation/config when Engine 3 actually begins.

---

## DOC-51 — Workspace README’s “operational truth” wording is too broad for append-only historical artifacts

**Severity:** Medium

Evidence:
- docs/workspace/README.md calls the workspace “Operational Truth”.
- The beta.7 workspace contains append-only audits and discussions that record superseded designs, old port values, deferred features, and intermediate completion states.

**Action:** Define operational truth as the current tracker/state plus current linked implementation evidence, while explicitly labeling historical artifacts as non-authoritative.

---

# 6. Consolidated severity view

Critical:
- GUI current vs deferred/absent
- DNS contract conflict
- service management implemented vs deferred/absent
- phantom ADR-012
- tracker service-state contradiction
- ADR-009 service claim
- historical audit “perfectly synchronized” claims that are now false

High:
- README feature-status inflation
- current/future architecture blending
- engine/layer misclassification
- proxy fallback status conflict
- .internal decision conflict
- throttle-cache terminology
- invalid current schema example
- CONTRIBUTING/CI mismatch
- tracker completion contradiction
- ADR-009 current/future inconsistency

Medium:
- engineering standards enforcement overclaim
- stale test-coverage wording
- logging namespace drift
- platform/testing-confidence wording
- advanced-port onboarding wording
- database/TCP routing examples
- future security surfaces lacking status
- relay build-plan wording
- historical workspace status labeling

Low:
- Windows roadmap wording
- beta.7 timing/history wording
- one-command PRD phrasing
- stale deferred-note timing

---

# 7. Minimum clean documentation model

The repository would be much easier to keep consistent if every major document answered one of these questions only:

### Current behavior
README, current architecture, CLI/help docs.

### Architectural decisions
ADRs, with amendments that explicitly supersede old contracts.

### Product vision
PRD, clearly separated from what is implemented today.

### Release history
CHANGELOG, never used as a substitute for current implementation truth.

### Execution state
Current workspace tracker.

### Historical reasoning
Workspace audit/diary/discussion artifacts and .ideas, explicitly non-authoritative.

The current repository mixes these roles in several places. That is the root cause behind many of the individual contradictions above.

---

# 8. Final repository-only audit status

The previous portfolio-sync audit was too broad and stopped at high-level drift. The deeper repository review shows that the main remaining problem is not a single stale sentence; it is **state ambiguity across multiple documentation layers**.

The most important cleanup is therefore not “fix every sentence independently”. First establish a trustworthy current-state contract, then re-label historical/future material around it.

Until that is done, a new contributor reading only the repository docs can reasonably reach contradictory conclusions about:

- whether the GUI exists;
- whether service management exists;
- whether .internal exists or was dropped;
- which DNS port is current;
- what DNS returns for unsupported record types;
- whether proxy fallback is implemented;
- whether the throttle cache is LRU/TTL;
- whether route updates are restart-based or dynamic;
- which engines belong to which layer;
- whether the local QA script really matches CI;
- which ADRs actually exist;
- whether beta.7 documentation cleanup is complete.

This audit should therefore be treated as the current documentation-consistency baseline for the develop branch.
