# Service Mode - Deferred Scope
**Status:** Deferred

## Deferred Features
- [ ] **[Feature] System-level startup (service install/uninstall)**
  - *Priority:* Medium
  - *Description:* Install devtether as a systemd, launchd, or OpenRC daemon to wake on boot and hold port 80/443 permanently.
- [ ] **[Feature] Interactive Wizard for Service Install**
  - *Priority:* Medium
  - *Description:* Guided interactive CLI prompts to configure system service (detects systemd vs launchd, applies setcap, handles privileges safely).
