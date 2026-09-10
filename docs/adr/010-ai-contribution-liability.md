# ADR 010: AI Contribution Liability & The Developer Certificate of Origin

## Status
Accepted

## Context
As an open-source project, DevTether is expected to receive contributions from multiple developers utilizing various Artificial Intelligence (AI) coding assistants (e.g., Google AntiGravity, GitHub Copilot, Cursor). 

Because AI agents can generate vast amounts of code, there is a risk of identity collision, unverified logic, and unclear legal liability regarding code licensing and provenance. We need a rigorous, watertight framework to ensure that all AI-assisted code meets the project's quality bar and is legally submittable under the project's open-source license.

## Decision
We establish the following **Human Accountability Policy** mirroring the standard Linux Developer Certificate of Origin (DCO):

1. **AI Agents are Tooling, Not Authors:**
   An AI agent is considered a development tool (akin to a compiler or IDE). It does not hold copyright, nor can it legally "author" a commit.
   
2. **Absolute Human Liability:**
   The human developer invoking the AI agent assumes 100% legal, functional, and security responsibility for the generated code. By committing code (via a Git signature or DCO sign-off), the human author certifies they have the right to submit the code under DevTether's open-source license.

3. **Mandatory Auditing:**
   All AI-generated outputs MUST be manually audited by the human developer against `docs/engineering-standards.md` before being merged. "The AI wrote it" is never an acceptable defense for failing CI, lacking tests, or violating ADRs, engineering standards, or DRY/SoC principles.

4. **Multi-Model Transparency:**
   To maintain traceability across a fragmented AI ecosystem, tasks tracked in the workspace must explicitly declare the AI tool being used. 
   - *Example Syntax:* `@github-username (via Agent:Model)` -> `@ivin-titus (via Google AntiGravity: Gemini 3.1 Pro)`.

## Consequences
- **Positive:** Protects the project from legal ambiguity regarding AI-generated code.
- **Positive:** Prevents identity collisions in task trackers when multiple developers use agents with default names like "Antigravity".
- **Positive:** Enforces a high quality bar by legally and socially obligating the human to review the code.
- **Negative:** Adds a slight overhead to task claiming and commit signing for human contributors.
