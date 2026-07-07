# Security Policy

Sheeld is a security tool, so we take vulnerabilities in it seriously and
appreciate responsible disclosure.

## Reporting a vulnerability

**Please do not report security issues through public GitHub issues, pull
requests, or discussions.**

Instead, use one of:

- GitHub's [private vulnerability reporting](https://github.com/cyacco/Sheeld/security/advisories/new)
  (Security → Report a vulnerability), or
- Email **cyacco@gmail.com** with the details.

Please include:

- A description of the issue and its impact.
- Steps to reproduce (a minimal proof-of-concept if possible).
- Affected version or commit, and your environment.

We will acknowledge your report within a few days, keep you updated on progress,
and credit you in the release notes unless you prefer to remain anonymous.

## Scope

Sheeld sits on the LLM request path and handles secrets (provider API keys,
audit data), so we're especially interested in:

- Cross-tenant / authorization bypasses (accessing another organization's
  sources, guardrails, transformers, or audit logs).
- Leakage of secrets (LLM API keys, guard/transformer config secrets, the
  workspace-config payload) via logs or API responses.
- SSRF or request smuggling through user-configured guard/transformer URLs.
- Authentication weaknesses (JWT handling, API-key lifecycle, the data-plane
  token).
- Guardrail bypasses — input reaching the LLM, or output reaching the client,
  that a configured guard should have blocked.

## Supported versions

Sheeld is pre-1.0. Security fixes land on `main` and in the latest release. Once
`v1.0.0` ships, this section will document the supported release line(s).
