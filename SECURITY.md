# Security Policy for DevTether

Security is a core priority for the DevTether project. We appreciate your efforts to responsibly disclose your findings, and we will make every effort to acknowledge your contributions.

## Supported Versions

Currently, DevTether is in Beta. Only the latest release on the `main` branch is actively supported with security updates. 

| Version | Supported          | Details |
| ------- | ------------------ | ------- |
| v2.x.x  | :white_check_mark: | Currently supported and receiving active security patches. |
| v1.x.x  | :x:                | Legacy prototype. Deprecated and no longer supported. |

If you are using an unsupported version, we strongly recommend upgrading to the latest release immediately.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you believe you have found a security vulnerability in DevTether, please report it privately via email to:
* **ivintitus@hotmail.com**
* **yukina-hirawa@outlook.com**

Please include both email addresses in your disclosure to ensure a prompt response.

### What to Include in Your Report
To help us triage and fix the vulnerability as quickly as possible, please provide the following details:
- **Description:** A detailed description of the vulnerability and its potential impact (e.g., privilege escalation, data leak, DoS).
- **Steps to Reproduce:** A step-by-step guide to reproducing the issue.
- **Environment Details:** 
  - OS and architecture
  - Output of `devtether version`
  - Output of `go version` (if building from source)
- **Proof of Concept (PoC):** Any scripts, network dumps, or `devtether.yaml` configuration files (sanitized) that demonstrate the vulnerability.
- **Suggested Fix:** (Optional) If you have a proposed mitigation or patch, please include it.

## Vulnerability Response Process

When you report a vulnerability, the DevTether maintainers follow this process:

1. **Acknowledgment (within 48 hours):** We will acknowledge receipt of your vulnerability report.
2. **Triage (within 72 hours):** We will investigate the issue and determine its validity and severity. We may ask for additional information or clarification.
3. **Drafting a Fix:** If the vulnerability is verified, we will develop a patch in a private fork.
4. **Embargo Period:** We ask that you maintain confidentiality until the patch is publicly released.
5. **Disclosure:** Once the patch is released, we will publish a security advisory. With your permission, we will credit you for the discovery.

## Scope

**In Scope:**
- Vulnerabilities in the core DevTether proxy, routing, or DNS engines.
- Privilege escalation or bypasses of the IPC daemon socket permissions.
- Remote Code Execution (RCE) or Denial of Service (DoS) triggered by malicious network requests.
- Information disclosure or leaks of sensitive configuration data.

**Out of Scope:**
- Vulnerabilities in third-party orchestrated processes spawned by DevTether.
- Issues related to physically compromised developer machines.
- Theoretical attacks without a viable proof of concept.

Thank you for helping keep DevTether secure!
