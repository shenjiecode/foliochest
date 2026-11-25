# Synthetic migration fixture

`v1/` was generated with the actual `0.3.0-dev` binary in an isolated temporary directory, exclusively for compatibility tests. It is not a user's vault.

- Test-only password: `synthetic-test-password`.
- Key: `legacy/synthetic`.
- Value: `SYNTHETIC_LEGACY_VALUE`.
- Both AI policies: `ask`.
- Contains only format-1 configuration and one encrypted synthetic record; no device enrollment, MCP token, browser cookie or real credential.

Do not replace this fixture with data from a real vault. Tests copy it into a disposable directory before upgrading or modifying it.
