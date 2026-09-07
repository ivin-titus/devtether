---
name: Bug report
about: Create a report to help us improve DevTether
title: '[BUG] '
labels: bug
assignees: ''
---

<!-- 
Thank you for reporting a bug! 
Please search existing issues first to avoid duplicates.
If this is a security vulnerability, do NOT open an issue. Please refer to SECURITY.md.
-->

## 🐛 Bug Description
A clear and concise description of what the bug is.

## 🔄 Steps to Reproduce
Steps to reproduce the behavior:
1. Run command `...`
2. Configuration used `...`
3. Network request made `...`
4. See error

## 🎯 Expected Behavior
A clear and concise description of what you expected to happen.

## 📸 Screenshots / Logs
If applicable, add screenshots, panic traces, or daemon logs to help explain your problem.
*(Please sanitize any sensitive tokens, API keys, or personal domains)*

```
Paste logs or code snippets here.
```

## 🖥️ Environment
- **OS**: [e.g. Ubuntu 26.04.1 LTS, macOS 14 Sonoma]
- **Architecture**: [e.g. amd64, arm64]
- **Go Version**: [e.g. 1.27.1] (if compiling from source)
- **DevTether Version**: [Paste output of `devtether version`]
- **Installation Method**: [e.g. curl script, GitHub release, built from source]

## 📝 devtether.yaml Configuration
Please provide the relevant sections of your configuration file.
*(Ensure you redact any sensitive information before posting!)*

```yaml
routes:
  # Paste relevant config here
```

## 🔍 Additional Context
Add any other context about the problem here, such as relevant network architecture, DNS configurations (systemd-resolved/dnsmasq), or if you are running in a specific containerized environment.
