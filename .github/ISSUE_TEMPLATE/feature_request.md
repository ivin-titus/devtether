---
name: Feature request
about: Suggest an idea for this project
title: '[FEAT] '
labels: enhancement
assignees: ''
---

<!-- 
Thank you for suggesting a feature! 
Please search existing issues first to avoid duplicates.
Before proposing large architectural changes, please review our ADRs in `docs/adr/`.
-->

## 🎯 Problem Statement
A clear and concise description of the problem this feature solves. 
*Example: "I'm always frustrated when trying to use X because Y..."*

## 💡 Proposed Solution
A clear and concise description of what you want to happen. 
Describe the intended behavior, new CLI flags, or changes to the `devtether.yaml` configuration.

## 🔄 Alternatives Considered
A clear and concise description of any alternative solutions or features you've considered. Why did you choose this proposed solution over the alternatives?

## 📐 Architecture & Standards Context
If this affects the core architecture (Proxy, DNS, Router, Daemon), please specify:
- Does this align with the **Three-Layer Architecture** described in `docs/architecture.md`?
- Does this interact with existing ADRs (e.g., `ADR-001`, `ADR-003`)?
- Are there any backward compatibility concerns?

## 📸 Mockups / Examples
If applicable, add mockups, CLI output examples, or pseudo-config blocks.

```yaml
# Proposed config example
routes:
  my-new-feature.localhost: 3000
```

## 🔍 Additional Context
Add any other context about the feature request here.
