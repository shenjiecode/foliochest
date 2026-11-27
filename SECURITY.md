# Security Policy

## Status

FolioChest has **not** undergone an independent third-party cryptographic or security audit yet. Do not rely on it as your sole copy of critical credentials; keep independent encrypted backups.

## Reporting a Vulnerability

**Do not open a public GitHub issue** for security vulnerabilities or suspected cryptographic flaws, and never attach real vaults, master passwords, bearer tokens, or recovery codes to any issue or PR.

Please report vulnerabilities privately to the repository maintainer via GitHub Security Advisories (Private Vulnerability Reporting) if enabled on `shenjiecode/foliochest`, or by contacting the maintainer directly with:

- A clear description of the issue and affected component/version
- Minimal reproduction steps using synthetic data only
- Any suggested fix or mitigation if available

## Security Model & Known Boundaries

- **What is protected:** `value` and attachments are encrypted with AES-256-GCM using a random 32-byte vault key wrapped by Argon2id (`64 MiB`, 3 iterations). Metadata (`key`, `description`, `group`, policies, and revisions) is authenticated via AAD.
- **What is intentionally plaintext:** `key`, `description`, `group`, file counts, sizes, and modification timestamps are plaintext to support AI discovery and sync compatibility. Never store secrets in `key`, `description`, or `group`.
- **Threat boundaries:** FolioChest cannot protect against an attacker who already controls the host OS, process memory, or local user session. Plaintext already returned to an MCP client cannot be revoked afterward. Changing the master password rewraps the existing vault key and does not revoke old backups or previously exported keys.

For full consistency and trust-boundary details, see [docs/BOUNDARIES.md](docs/BOUNDARIES.md) and [docs/FORMAT.md](docs/FORMAT.md).
