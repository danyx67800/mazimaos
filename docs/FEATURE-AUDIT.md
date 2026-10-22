# Feature audit

Audit date: 2026-10-22
Target: `0.1.0-rc.3` (do not tag `v1.0.0` until the gaps below are closed)

Status values:

- **VERIFICATO** — exercised by an automated or recorded manual check in this workspace.
- **PARZIALE** — the surface exists and was exercised, but important behaviour or failure paths were not fully verified.
- **NON VERIFICATO** — cannot be verified here (hardware, credentials, signing key or external service missing).

## Evidence

| Evidence | Command / artifact |
| --- | --- |
| Formatting | `cd daemon && gofmt -l .` (empty) |
| Static analysis | `cd daemon && go vet ./...` |
| Go tests | `cd daemon && go test -cover ./...` |
| Linux cross-build | `cd daemon && GOOS=linux GOARCH=amd64 go build ./...` |
| Browser E2E | `npm run test:e2e` |
| Full audit log | `make audit` → `docs/evidence/feature-audit.txt` |
| PWA check | E2E asserts active service worker, scope and `mazima-shell-v2` cache |
| OTA bundle | `./scripts/release.sh 0.1.0-rc.3` (manifest + Ed25519 signature + verification) |
| Published prerelease | [`v0.1.0-rc.3`](https://github.com/danyx67800/mazimaos/releases/tag/v0.1.0-rc.3): ISO 1,302,233,088 bytes (sha256 `16c5aa7a...`), beta OTA bundle `fa5210c3...` (`5513127` bytes); signature reverified after download |
| Store catalog | `go test -count=1 ./internal/dockerstore -run TestBundledCatalogManifests` |
| Caddy snippet split | `go test ./internal/proxy -run TestRegenerateSeparatesPathAndAliasSnippets` |
| Backup argument flow | Stubbed `restic` exercised `scripts/backup.sh`; real repo/restore not run |
| CI | GitHub Actions run [39781882015](https://github.com/danyx67800/mazimaos/actions/runs/39781882015) passed (8m4s) |
| Live-build config | `lb config` passed in a Debian trixie container (amd64 config generated on arm64) |
| ISO build | GitHub Actions run [39819407265](https://github.com/danyx67800/mazimaos/actions/runs/39819407265) built `mazima-0.1.0-rc.3-amd64.iso` (1.3G) with `user-setup`, `whiptail` and `mazima-setup`; checksum verified, boot not tested |
| Guided setup script | Stub `whiptail` + real `mazimad` created an admin, removed `setup.token`, and produced the expected API payload; `tty1` boot path not tested |
| Manual matrix | [MANUAL-TESTING.md](MANUAL-TESTING.md) |

## Matrix

| Area | Status | Evidence / gap |
| --- | --- | --- |
| First-run setup (hostname, language, timezone, admin, token) | VERIFICATO | `tests/e2e/ui.spec.js` completes setup from a fresh data directory |
| Guided first-boot console setup | PARZIALE | Script creates the admin against a real daemon with a stub `whiptail`; the systemd `tty1` path has not been booted |
| Login and logout | VERIFICATO | E2E logs out through the user menu and logs back in |
| Session cookie and CSRF handling | PARZIALE | Used by every E2E request; no dedicated negative/expiry tests |
| TOTP enrollment, login challenge and recovery | NON VERIFICATO | No E2E coverage; needs an authenticator flow |
| Account profile and password change | NON VERIFICATO | UI/API exist; not exercised |
| Desktop shell, dock and window manager | VERIFICATO | E2E opens apps, system, Docker, remote, updates, audit and settings windows |
| Window geometry persistence | PARZIALE | Manual Playwright drag/reload check passed; not in the committed E2E suite |
| Command palette | VERIFICATO | E2E opens and closes it with Escape |
| Theme switching | VERIFICATO | E2E toggles the theme through the settings API |
| Italian/English i18n | PARZIALE | Both dictionaries and persisted language exist; language switch is not E2E-tested |
| PWA manifest, icons and service worker | VERIFICATO | Manifest 200, 192/512 PNG 200, SW active with full shell cache |
| Offline navigation | PARZIALE | Shell cache is populated; Playwright offline navigation was blocked before the SW could respond, so offline reload is unverified |
| Static asset handling, traversal rejection and CSP | VERIFICATO | Manual checks: missing assets 404, SPA routes 200, traversal rejected, CSP present |
| System telemetry snapshot | PARZIALE | Window renders; values are not asserted |
| Processes, services, logs and power actions | NON VERIFICATO | Destructive/system-level paths not exercised |
| Docker containers/images/volumes/networks UI | PARZIALE | Window renders; Docker socket unavailable in the test environment (HTTP 500) |
| Docker destructive actions and prune | NON VERIFICATO | Requires a disposable Docker host |
| Tailscale status/login/Serve/Funnel | NON VERIFICATO | No tailnet credentials or Funnel approval |
| Linux app sessions and streamed input | NON VERIFICATO | Cross-build succeeds; runtime needs Xvfb, ffmpeg, xdotool, xclip/xsel on Linux |
| Update manifest signing and verification | VERIFICATO | `scripts/release.sh` produces a signed manifest and `mazimaota verify` accepts it |
| Update check/install/rollback | NON VERIFICATO | Prerelease published; install and rollback not exercised |
| Audit log read, redaction and pagination | PARZIALE | Unit tests cover redaction and pagination; UI reads events |
| App catalog schema and compose policy | VERIFICATO | `TestBundledCatalogManifests` validates the bundled File Browser manifest |
| App install/remove through Caddy | PARZIALE | Manifest/compose validate; Docker Compose plugin is broken on the audit host and Caddy is not installed |
| Caddy path/alias generation | PARZIALE | Unit test verifies separate `apps.caddy`/`apps-aliases.caddy`; no live reload tested |
| Backups | PARZIALE | Script argument flow tested with a stub; `restic` is not installed and restore was not run |
| Debian live image and first boot | PARZIALE | `rc.1` lacked `user-setup` (console login failed); `rc.2` adds it; `rc.3` adds the guided console wizard and builds a checksum-verified 1.3G ISO, but it has not been booted |
| x86_64 hardware and GPU telemetry | NON VERIFICATO | Audit host is ARM64 macOS |
| CI pipeline | VERIFICATO | GitHub Actions run [39781882015](https://github.com/danyx67800/mazimaos/actions/runs/39781882015) passed |

## Known defects fixed during this audit

- `0.1.0-rc.1` omitted the Debian `user-setup` package, so live-config never
  created the console user and login failed with `Authentication failure`.
  `0.1.0-rc.2` installs `user-setup`; the live credentials are `user` / `live`.
- Double-applied window transparency caused unreadable stacked windows; replaced with per-theme `--glass-base` alpha formulas.
- Window restore pruned not-yet-restored windows through premature `persist()` calls; restore now suppresses saves until the loop completes.
- Missing static assets fell back to `index.html` with HTTP 200; they now return 404. Asset caching changed to `no-cache` because filenames are not fingerprinted.
- `views/docker.js` and `views/updates.js` crashed on load because click listeners referenced `const` handlers before initialization.
- PWA icons and the service-worker shell were incomplete; PNG icons, all views, `dashboard.js` and `gateway.js` are now precached.
- Linux cross-build failed in `platform_linux.go` because `ClipboardSet` was assigned to two variables.
- Caddy generated path routes and alias site blocks in one snippet, which cannot be imported in both site and global scope; they are now split into `apps.caddy` and `apps-aliases.caddy`.
- The release script built host-architecture binaries and named them `linux-amd64`; it now cross-compiles and verifies the ELF target.
- File Browser now uses `-b /apps/filebrowser/` so absolute asset paths work under the path proxy. Other catalog apps without base-path support were removed.

## Release blockers

1. Run the Linux streaming integration on a supported host and record evidence.
2. Replace the development OTA key with a production-owned key, store it in the release secret store, and test install plus rollback against the published prerelease.
3. Authenticate Tailscale and verify Serve; verify Funnel only with explicit approval.
4. Boot the built ISO on x86_64 hardware or a VM in both UEFI and legacy mode.
5. Install `restic` and verify a restore, not only a backup.
6. Exercise app install through Caddy with a real Docker host.
7. Resolve the remaining decisions in [OPEN-QUESTIONS.md](OPEN-QUESTIONS.md).
