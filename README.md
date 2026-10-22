# MazimaOS

[![CI](https://github.com/danyx67800/mazimaos/actions/workflows/ci.yml/badge.svg)](https://github.com/danyx67800/mazimaos/actions/workflows/ci.yml)

MazimaOS is a browser-controlled personal cloud OS for mini-PCs and home servers. A single Go daemon serves the desktop UI, Docker app management, streamed Linux applications, system telemetry, updates and Tailscale access.

> **Status: pre-release (`0.1.0-rc.3`).** The UI and core daemon paths are usable, but this repository is **not** a verified 1.0.0 release. The [GitHub prerelease](https://github.com/danyx67800/mazimaos/releases/tag/v0.1.0-rc.3) includes a checksum-verified x86_64 live ISO with a guided console first-boot setup and a signed OTA bundle; the ISO has not been boot-tested. Streaming, signed OTA updates, Tailscale Serve/Funnel and app installation require hardware or credentials that are not available in this workspace. See [docs/FEATURE-AUDIT.md](docs/FEATURE-AUDIT.md) and [docs/MANUAL-TESTING.md](docs/MANUAL-TESTING.md) before deploying.

## Quick start

Requirements: Go 1.26+, Node.js 20+ for the end-to-end tests.

```sh
make build
./bin/mazimad --dev --data-dir ./dev-data --ui-dir ./ui --listen 127.0.0.1:8088
```

On first start the daemon writes a one-time setup token to `./dev-data/setup.token`. Open `http://127.0.0.1:8088`, read the token locally, and complete setup. The token is deleted after the first account is created.

`make dev` starts the same server through `scripts/dev.sh` using `.dev-data` and port `8088`.

## Tests

```sh
make test        # Go tests plus Playwright end-to-end
make audit       # formatting, vet, coverage and E2E, writes docs/evidence/
make release     # signed release dry run (see scripts/release.sh)
```

The end-to-end test starts its own daemon on `127.0.0.1:18090`, completes first-run setup, exercises the desktop, and verifies the PWA shell. Test data lives in `.e2e-data/` and is removed on every run.

## Release and image

```sh
./scripts/release.sh 0.1.0        # local signed bundle + manifest (dry run)
OTA_PUBLIC_KEY="$(cat signing/ota.pub)" ./image/build.sh
```

The `Image` GitHub Actions workflow builds the Debian ISO on an x86_64 runner
and can publish it as a prerelease; see [docs/RELEASING.md](docs/RELEASING.md).

`scripts/release.sh` cross-compiles `linux/amd64`, embeds the OTA public key,
creates `dist/mazima-<version>-linux-amd64.tar.gz`, signs the manifest and
verifies the signature. Set `PUBLISH=1` only when a GitHub release is intended.
The Debian image build requires a Debian x86_64 host with `live-build`; see
[image/README.md](image/README.md).

## Backups

`scripts/backup.sh` is a restic backup with daily/weekly/monthly retention. The
image ships a `mazima-backup.service` and `mazima-backup.timer`; configure
`/etc/mazima/backup.env` and `/etc/mazima/restic.password`, then initialize the
repository with `RESTIC_INIT=1`. Use a remote repository for disaster recovery.

## Layout

- `daemon/` — Go daemon, API, auth, sessions, updater, Docker and Tailscale adapters.
- `ui/` — dependency-free ES-module desktop, views, service worker and icons.
- `tests/e2e/` — Playwright browser tests.
- `store/apps/` — validated app catalog manifests and compose files.
- `image/` — Debian live-build, systemd, Caddy, Avahi, nftables and backup units.
- `scripts/` — dev server, E2E server, audit, release and backup tooling.
- `docs/` — architecture, manual test plan, audit evidence and open decisions.

## Security

MazimaOS is intended to be exposed only through a trusted network or a TLS-terminating reverse proxy. Do not expose the daemon directly to the public internet. Setup tokens, the master key and session data are written with restrictive permissions under the data directory; never commit them.

## License

MIT — see [LICENSE](LICENSE).
