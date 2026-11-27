# Contributing to FolioChest

Thank you for your interest in contributing to FolioChest!

## Ground Rules

- **Never commit real data**: Do not commit real vaults, master passwords, OS credentials, MCP tokens, recovery codes, private keys, or plaintext exports. All tests must use synthetic data only.
- **Preserve compatibility identifiers**: Do not globally rename cryptographic AAD/HMAC prefixes (`keyfolio/*/v1`), OS credential namespaces (`io.github.shenjiecode.keyfolio`), default device directory (`keyfolio`), or cookie/autostart identifiers—changing them breaks existing vaults and local device state.
- **Respect format & security boundaries**: Read [docs/BOUNDARIES.md](docs/BOUNDARIES.md) and [docs/FORMAT.md](docs/FORMAT.md) before changing vault storage, locking, MCP approval, or authentication logic.
- **UI & build constraints**: `web/src/main.tsx` is the single web entrypoint, built into `web/dist/` and embedded via `web/embed.go`. Keep strict CSP rules (no `unsafe-inline` scripts or `unsafe-eval`; dynamic styles use the per-page nonce).

## Development Setup

Requirements:

- Go `1.26+`
- Node.js `24+`

Build and run locally:

```bash
npm --prefix web ci
npm --prefix web run build
go run ./cmd/foliochest --device-dir "$PWD/.local/dev-device" --background
```

Run full verification before opening a PR:

```bash
make check
```

If you modify platform code (`internal/platform/`), verify cross-compilation as well:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath ./cmd/foliochest
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath ./cmd/foliochest
```

## Pull Requests

1. Target the `develop` branch.
2. Keep changes focused and include tests for behavioral or security-boundary changes.
3. Verify `git status` contains no `.local/`, `bin/`, `dist/`, or real vault artifacts.
