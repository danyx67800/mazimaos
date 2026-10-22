# Architecture

MazimaOS is one Go daemon plus a static browser application.

## Runtime

- `mazimad` serves the UI, REST API, WebSocket session endpoint and `/healthz`.
- `mazimactl` provides local operations (`version`, `selftest`, `setup-token`).
- State is stored as atomic JSON documents under the configured data directory.
- The browser desktop is plain ES modules; there is no UI build step.

## Trust boundaries

- The daemon authenticates browser sessions with HttpOnly cookies and CSRF tokens.
- Sensitive operations require password and typed confirmation; some also require TOTP.
- Docker, systemd, Tailscale and updater operations are admin-only.
- The UI never receives the master key or setup token after setup.
- The image runs `mazimad` as `mazima`, but that account is in the `docker`
  group (root-equivalent through the Docker socket) and has a narrow polkit
  rule for unit management and power actions. The admin session is therefore
  the real security boundary; do not give untrusted users admin accounts.

## Main subsystems

| Subsystem | Package | Responsibility |
| --- | --- | --- |
| HTTP API | `daemon/internal/api` | Routing, static files, CSP, auth middleware, handlers |
| Auth | `daemon/internal/auth` | Password hashing, sessions, TOTP, CSRF, master key |
| Store | `daemon/internal/store` | Atomic JSON state, migrations, sessions |
| Desktop | `ui/assets/js/desktop.js`, `windows.js` | Shell, dock, windows, persistence, command palette |
| Sessions | `daemon/internal/session` | Linux app discovery, Xvfb/ffmpeg streaming, input |
| Docker | `daemon/internal/dockerstore` | Docker CLI adapter, catalog/compose validation |
| App proxy | `daemon/internal/proxy` | Caddy path routes, alias sites, reload |
| Updates | `daemon/internal/updater` | Signed manifest verification, install/rollback |
| Tailscale | `daemon/internal/tailscale` | Status, login, Serve/Funnel |
| Audit | `daemon/internal/audit` | Append-only redacted event log |
| OTA tooling | `daemon/cmd/mazimaota` | Key generation, manifest hashing, signing, verification |

## Data flow

1. `boot.js` calls `/api/setup/status` and `/api/auth/session`.
2. On success, `renderDesktop` loads `/api/desktop` and mounts the shell.
3. Window views call authenticated JSON endpoints through `api.js`.
4. Streamed sessions obtain a short-lived token, then connect to `/api/ws/session`.
5. Desktop geometry is saved with a debounced `PUT /api/desktop`.
6. Installed apps bind to a random loopback port; `proxy` writes path routes to
   `/etc/mazima/caddy/apps.caddy` and alias sites to
   `/etc/mazima/caddy/apps-aliases.caddy`, then reloads Caddy.
7. The updater reads a signed manifest, downloads the bundle, verifies the
   embedded Ed25519 key and keeps the previous release for rollback.

## Deployment

The supported deployment is Debian 13 x86_64 with systemd:

- `mazimad` binds `127.0.0.1:8088`; Caddy terminates TLS at `mazima.local`.
- Caddy imports `apps.caddy` inside the main site and `apps-aliases.caddy` at
  the top level. They must not be merged into one snippet.
- Avahi publishes `_https._tcp`; nftables allows only SSH, HTTP, HTTPS, mDNS,
  DHCP and the Tailscale interface.
- Releases live in `/opt/mazima/releases/<version>` with `current` as a
  relative symlink, so an update is an atomic symlink swap.
- `mazima-backup.timer` runs `scripts/backup.sh` as root with restic; the
  password file is separate from the repository.

The image inputs are under `image/live-build`; the ISO build is not verified
until it has been booted on real hardware or a VM.
