# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

Security fixes are applied to the latest release on the `main` branch.

## Reporting a Vulnerability

**Please do not report security vulnerabilities in public GitHub issues.**

Instead, use one of the following channels:

1. **GitHub Security Advisories** (preferred): open a [private vulnerability report](https://github.com/Fleetdock/fleetdock/security/advisories/new) on this repository.
2. **Email**: contact the maintainers at `security@tajbrains.com` with a detailed description.

Include as much of the following as you can:

- Type of issue (e.g. authentication bypass, SQL injection, privilege escalation)
- Full paths of affected source files
- Step-by-step reproduction instructions
- Proof-of-concept or exploit code (if available)
- Impact assessment

## Response Timeline

| Stage             | Target                       |
| ----------------- | ---------------------------- |
| Initial response  | 3 business days              |
| Triage complete   | 7 business days              |
| Fix or mitigation | 30 days (severity-dependent) |

We will acknowledge receipt, keep you informed of progress, and credit reporters in the advisory unless you prefer to remain anonymous.

## Scope

In scope:

- The Fleetdock API (`backend/cmd/api`)
- The server agent (`backend/cmd/agent`)
- The web dashboard (`frontend/`)
- Official install script served at `/install.sh`

Out of scope:

- Vulnerabilities in third-party database engines (MariaDB, MySQL, PostgreSQL) managed by Fleetdock
- Misconfigurations on user-operated infrastructure (weak passwords, exposed ports, missing TLS)
- Social engineering attacks against operators

## Safe Harbor

We consider good-faith security research conducted in accordance with this policy to be authorized. Do not access, modify, or delete data belonging to others. Do not perform denial-of-service testing against production systems without prior written consent.
