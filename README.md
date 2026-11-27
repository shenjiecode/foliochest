<p align="center">
  <img src="packaging/assets/logo.png" alt="FolioChest / 资料匣" width="128" height="128" />
</p>

# FolioChest / 资料匣

English | [简体中文](README.zh.md)

A locally run encrypted vault managed via a browser UI, with per-key AI access control over MCP.

`key`, `description`, and an optional single-level `group` are stored in plaintext so AI agents can discover relevant entries first, while `value` and attachments are encrypted with AES-256-GCM. External tools (such as Nutstore/Syncthing) handle file sync; FolioChest handles encryption, versioning, conflict detection, and per-key permissions.

> **Status (`0.6.1-dev`):** Working prototype. Windows runtime testing, native installer verification, and an independent security review have not been completed yet. Start with synthetic data and keep backups.

## Features

- **One Vault, One Master Password**: Bind one vault per local instance and organize it with groups. Locking stops both web and MCP data access. Maintenance supports location migration and same-vault recovery, not switching libraries.
- **Local Web UI + Single Binary**: Go backend with an embedded React + TypeScript + shadcn/ui web app; no runtime Node.js or external CDN required.
- **Plaintext Discovery, Encrypted Payloads**: Argon2id (`64 MiB`, 3 iterations) key derivation + AES-256-GCM encryption for bodies and attachments.
- **Content & Organization**: Plaintext, Markdown (with safe preview, no HTML/remote image loading), JSON editing assistance, encrypted attachments, single-level groups, history browsing/restore, conflict merging, and soft-delete trash.
- **Per-Key MCP Permissions**: Independent `allow` / `ask` / `deny` policies for read and replace per key, with client bearer tokens, session form elicitation, and fallback webpage approval.
- **Backup & Recovery**: Full encrypted ZIP backup export and validated restore into a new/empty directory.
- **Device Convenience & Locking**: Optional OS credential storage (Windows Credential Manager / Linux Secret Service), browser remember-me (90 days), user-level login autostart, idle auto-lock, and system lock/sleep event locking.

## Quick Start (From Source)

Requires **Go 1.26+** and **Node.js 24+**.

```bash
npm --prefix web ci
npm --prefix web run build
go run ./cmd/foliochest
```

Open `http://127.0.0.1:7463/` to create, attach, or restore your single vault on first use; subsequent visits use its master password. Location migration retains the original files and copies local empty groups. Independent empty groups are not included in ZIP backups yet.

Build a standalone binary:

```bash
make build
./bin/foliochest
```

For isolated development testing without touching your default config directory:

```bash
./bin/foliochest --device-dir "$PWD/.local/test-device"
```

### CLI Flags

- `--device-dir`: Local device state directory (excluded from vault sync; defaults to `<UserConfigDir>/keyfolio`)
- `--vault`: Vault directory path (can also be chosen in the web UI)
- `--listen`: Loopback listen address (default `127.0.0.1:7463`)
- `--background`: Start the background service without opening the browser

Closing the browser tab does not stop the background service; use the **Exit** button in the web UI.

## MCP Integration

1. Unlock the vault in the web UI and open **Settings → MCP** to create a client token.
2. Point your MCP client to `http://127.0.0.1:7463/mcp` with `Authorization: Bearer <token>`.

| Tool | Purpose |
| --- | --- |
| `discover_entries` | Search plaintext `key`, `description`, and `group` (with optional exact `group` filter); never returns bodies |
| `read_entries` | Request plaintext `value` or attachments for specific keys under per-key read policy |
| `replace_entries` | Submit full replacements bound to `expected_revision` under per-key write policy |
| `request_result` | Poll or claim the result of a pending/approved request once |

Example with `pi` (after exporting `FOLIOCHEST_TOKEN`):

```bash
pi mcp add foliochest --url http://127.0.0.1:7463/mcp --bearer-token-env-var FOLIOCHEST_TOKEN
```

## Packaging & Cross-Compilation

Build portable archives (`dist/`):

```bash
make package
```

Build portable archives plus Windows NSIS installers (`x64`, experimental `x86`) and Linux `DEB`/`RPM` (`amd64`, `arm64`) using **NSIS 3.11** and **nFPM 2.49.0**:

```bash
make installers
```

See [packaging/README.md](packaging/README.md) and [docs/RELEASING.md](docs/RELEASING.md) for toolchain and release details.

## Testing

```bash
make check
```

Runs frontend type-checking, frontend unit tests, `go vet ./...`, and `go test -race ./...` (requires `npm --prefix web run build` first when running Go tests directly because `web/embed.go` embeds `web/dist/`).

## Prototype Limits

- Body max: `1 MiB`; attachments: up to `16` per entry, `2 MiB` each.
- Write/read batch max: `20` entries, `8 MiB` serialized payload.
- Vault records max: `64 MiB` total, `4096` files (including retained history).
- Approval TTL: `5` minutes, single-claim result.
- Different-`entry_id` key name collisions are flagged as conflicts and not auto-merged.

## Documentation

- [Design Overview](docs/DESIGN.md)
- [Security & Consistency Boundaries](docs/BOUNDARIES.md)
- [Vault File Format](docs/FORMAT.md)
- [Security Policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
- [Release Process](docs/RELEASING.md)

## License

[MIT License](LICENSE). Third-party components and notices are preserved in [web/src/components/ui/NOTICE.md](web/src/components/ui/NOTICE.md) and collected into release packages by `tools/package`.
