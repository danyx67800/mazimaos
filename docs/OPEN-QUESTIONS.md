# Open questions

These decisions must be resolved before a 1.0.0 release.

1. **License** — **RESOLVED 2026-10-22**: MIT, see [`LICENSE`](../LICENSE).
2. **Repository visibility** — **RESOLVED 2026-10-22**: the owner approved a
   public GitHub repository. It is live at
   <https://github.com/danyx67800/mazimaos>; the initial commit was pushed to
   `main` and CI run 39781882015 passed.
3. **Update signing** — tooling exists (`mazimaota`, `scripts/release.sh`,
   `.github/workflows/release.yml`) and a development key was generated outside
   the repo. Still open: production key ownership, secret storage, rotation and
   revocation. The image workflow reads `MAZIMA_OTA_PUBLIC_KEY`; the local
   build script reads `OTA_PUBLIC_KEY`. The published `v0.1.0-rc.2` bundle is
   signed with the development key.
4. **Tailscale Funnel** — public Funnel activation needs explicit approval and
   a reviewed exposure policy. Serve-only is the safer default.
5. **Supported hardware** — the image targets Debian 13 x86_64 only. Confirm the
   CPU baseline and whether ARM64 is in scope for 1.0. The `v0.1.0-rc.3` ISO
   exists but has not been booted.
6. **Backups** — a restic script, service and timer now exist, but the
   repository location/credential model is still undecided. The default local
   path is not disaster recovery.
7. **App catalog** — only File Browser is bundled, because it supports a base
   path. Define curation, signing, review and base-path requirements before
   adding apps.
8. **Admin 2FA** — TOTP is implemented but not mandatory. Decide whether admin
   accounts must enroll before 1.0.
9. **Network exposure** — Caddy with an internal CA is the default and
   `mazimad` binds loopback only. Confirm whether plain HTTP is ever allowed
   outside localhost and how the local CA is distributed.
10. **Data retention** — audit-log retention is not implemented. Choose a
    retention period and rotation policy; session defaults are 12 hours with a
    30-minute stream idle timeout.
