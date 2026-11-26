# Synthetic migration fixture

`v1/` was generated with the actual `0.3.0-dev` binary in an isolated temporary directory, exclusively for compatibility tests. It is not a user's vault.

- Test-only password: `synthetic-test-password`.
- Key: `legacy/synthetic`.
- Value: `SYNTHETIC_LEGACY_VALUE`.
- Both AI policies: `ask`.
- Contains only format-1 configuration and one encrypted synthetic record; no device enrollment, MCP token, browser cookie or real credential.

`v2/` was generated with the actual `0.4.0-dev` binary under an isolated HOME/XDG and nonexistent D-Bus sockets. It uses the same test-only password. It contains a format-2 configuration and two encrypted records: one initial batch with `legacy/v2` (Markdown `SYNTHETIC_V2_VALUE`, attachment `synthetic.txt` containing `SYNTHETIC_ATTACHMENT`) and `legacy/trash` (`SYNTHETIC_TRASH_VALUE`), followed by a tombstone for `legacy/trash`. Both policies are `ask`. No device enrollment, tokens, cookies, plaintext keys or real credentials are included.

Do not replace these fixtures with data from a real vault. Tests copy it into a disposable directory before upgrading or modifying it.
