# Manual test plan

Use this checklist on the first x86_64 machine or VM that runs the image.
Record the build (`VERSION`, ISO SHA-256), date, host and result for every
item. Attach screenshots or command output to the release evidence.

Status values: `PASS`, `FAIL`, `BLOCKED`, `N/A`. A release is not ready while
any item is `FAIL`, and no `BLOCKED` item may be silently dropped.

## 0. Artifacts and rollback safety

- [ ] 0.1 Build the ISO with a real OTA public key and record its SHA-256.
- [ ] 0.2 Run a VM snapshot or use a disposable disk before booting bare metal.
- [ ] 0.3 Keep a second machine on the same network for client-side tests.
- [ ] 0.4 Record the console and network access method (monitor/keyboard, SSH).

## A. Boot and network

- [ ] A1 Boot the ISO in UEFI mode; GRUB starts and the live system reaches a
      login prompt without a kernel panic.
- [ ] A2 Boot the same ISO in legacy BIOS mode (if the target supports it).
- [ ] A3 Log in on the console with the documented live credentials.
- [ ] A4 `ip addr` shows a DHCP address; `ping -c1 1.1.1.1` succeeds.
- [ ] A5 `systemctl --failed` is empty.
- [ ] A6 `mazima.local` resolves from a second machine over mDNS. If it does
      not, add a hosts entry and record that as a deployment note.
- [ ] A7 `curl -k https://mazima.local/healthz` returns HTTP 200.
- [ ] A8 `sudo journalctl -u mazimad -u caddy -u avahi-daemon -u nftables`
      shows no repeated crashes.

## B. First-run setup and account security

- [ ] B1 `sudo mazimactl setup-token --config /etc/mazima/config.json` prints a
      token and the file is mode `0600`.
- [ ] B2 Open `https://mazima.local`; the setup wizard renders and the
      certificate warning is expected until the local CA is trusted.
- [ ] B3 Complete setup with hostname, language, timezone, admin username and
      password. Confirm the dashboard loads.
- [ ] B4 Re-open the setup URL or reload the token; setup is no longer
      available and `/var/lib/mazima/setup.token` is gone.
- [ ] B5 Log out, open a private window and log in again. The session cookie is
      `HttpOnly`, `Secure` and `SameSite` as designed.
- [ ] B6 Enter a wrong password repeatedly; the API returns 401 and the UI
      shows a generic error without leaking whether the user exists.
- [ ] B7 Change the admin password, log out, and log in with the new password.
- [ ] B8 Enroll TOTP, log out, log in with a code, then use one recovery code.
      Disable and re-enable TOTP.
- [ ] B9 Change language to English and back to Italian; the choice survives a
      reload.
- [ ] B10 Change the hostname; verify the UI updates and a reboot keeps it (or
      records the documented live-image limitation).

## C. Desktop, windows and PWA

- [ ] C1 The dock shows all expected applications and the clock/status area.
- [ ] C2 Open every app, move and resize two windows, minimize, restore, and
      reload the page. Geometry and z-order are restored.
- [ ] C3 Drag a window partly off-screen and reload; it is brought back into
      the usable area.
- [ ] C4 Open the command palette with the keyboard shortcut, run a command
      with Enter, and close it with Escape.
- [ ] C5 Toggle light/dark/AMOLED themes and change accent, transparency and
      wallpaper; text stays readable in every combination.
- [ ] C6 Open Settings and change UI scale; no control is clipped.
- [ ] C7 Install the PWA from the browser, launch it standalone, and confirm
      the icon and name.
- [ ] C8 With the PWA open once, stop the network and reload. The shell must
      load from the service-worker cache. Record the browser and exact result.
- [ ] C9 Reconnect and confirm API state syncs without a full page reset.

## D. Docker and destructive actions

- [ ] D1 Docker window lists containers, images, volumes and networks without
      a 500 error.
- [ ] D2 Start a disposable container from the UI, inspect logs, stop it and
      remove it.
- [ ] D3 Remove an image only after typing/confirming the id; cancellation
      leaves it untouched.
- [ ] D4 Prune stopped containers/images with the explicit confirmation; the
      API rejects a request without it.
- [ ] D5 Create/remove a volume and a network; a network still in use is not
      removed.
- [ ] D6 Pull a small public image and confirm progress/errors are shown.
- [ ] D7 Verify that only loopback ports are published for installed apps:
      `ss -ltnp` shows `127.0.0.1:<port>`, not `0.0.0.0`.
- [ ] D8 Reboot and confirm containers with `restart: unless-stopped` return.

## E. Store and Caddy proxy

- [ ] E1 The store lists File Browser with description, version, category and
      license.
- [ ] E2 Install File Browser; the operation completes and the app appears
      with a URL.
- [ ] E3 Open `/apps/filebrowser/` from the desktop; the login page loads with
      CSS/JS, not a blank page or the MazimaOS SPA.
- [ ] E4 Log in with the initial credentials, change the password, upload a
      file and download it. Data survives a container restart.
- [ ] E5 Stop and start the app from the UI; the route is removed and restored
      correctly.
- [ ] E6 Uninstall the app; its route, container and compose project are gone.
      Confirm what happens to app data.
- [ ] E7 Add a test alias-mode manifest with hostname `files.example.test`
      (temporarily in `/etc/hosts`), install it, and verify
      `https://files.example.test/` is served from `apps-aliases.caddy` while
      path routes remain in `apps.caddy`.
- [ ] E8 Confirm a failed Caddy reload leaves the old config serving traffic
      and is reported in the app response/log.

## F. Linux streaming

- [ ] F1 The Remote window reports streaming as supported.
- [ ] F2 Start the terminal app. The first frame arrives and the keyboard
      types in the terminal.
- [ ] F3 Test Ctrl/Alt/Shift/Meta chords, arrows, function keys and paste.
- [ ] F4 Clipboard synchronization works both browser-to-session and
      session-to-browser.
- [ ] F5 Open the browser app; it starts inside Xvfb and input works.
- [ ] F6 Resize the viewer; the stream renegotiates or letterboxes without
      crashing.
- [ ] F7 Stop a session from the UI; the process tree and Xvfb are cleaned up.
- [ ] F8 Kill the WebSocket/network; reconnect resumes or reports a clear
      error and the session is not leaked.
- [ ] F9 Start several sessions; idle sessions expire after the configured
      timeout.
- [ ] F10 Verify no X11/ffmpeg processes remain after all sessions stop.

## G. Updates and rollback

- [ ] G1 Configure a release repository and confirm `GET /api/updates` shows
      the current version and no spurious update.
- [ ] G2 Publish a newer signed release; `Check` finds it and shows notes,
      channel and bundle size.
- [ ] G3 Install it with the correct password and `INSTALL` confirmation. The
      daemon restarts into the new version.
- [ ] G4 Open the UI after the update; the new UI assets are served and no
      stale cached chunk breaks the page.
- [ ] G5 Reboot and confirm the new version remains active.
- [ ] G6 Roll back with the correct password and `ROLLBACK`; the previous
      version and UI return.
- [ ] G7 Try an install with a tampered manifest/signature; it is rejected and
      the running version is untouched.
- [ ] G8 Try an install with a wrong password or without the confirmation
      string; it is rejected.
- [ ] G9 Try a version below `min_version`; it is rejected.
- [ ] G10 Interrupt the download or restart during installation; the updater
      recovers to a bootable known-good version.

## H. Tailscale

- [ ] H1 The Remote window reports Tailscale as unavailable when the socket is
      absent, without breaking the page.
- [ ] H2 Install/authenticate Tailscale on the host and confirm status shows
      the tailnet name and this node.
- [ ] H3 Enable Serve; open the `https://<node>.<tailnet>.ts.net` URL from
      another tailnet device.
- [ ] H4 Disable Serve and confirm the URL stops working.
- [ ] H5 Enable Funnel only with explicit owner approval; verify the public
      URL and record the exposure window.
- [ ] H6 Disable Funnel and confirm the public URL stops working.
- [ ] H7 Log out of Tailscale; the status and Serve/Funnel state are cleared.
- [ ] H8 Confirm no Tailscale auth key is written to logs or the audit export.

## I. Backups and restore

- [ ] I1 Create `/etc/mazima/restic.password` with mode `0600` and configure a
      remote `RESTIC_REPOSITORY`.
- [ ] I2 Initialize with `RESTIC_INIT=1`; a second run does not reinitialize.
- [ ] I3 Run `sudo systemctl start mazima-backup.service`; the journal shows a
      snapshot id and no password.
- [ ] I4 Create a file, run the backup again, and confirm the snapshot
      contains it.
- [ ] I5 Check retention: old snapshots beyond daily/weekly/monthly limits are
      forgotten, current ones remain.
- [ ] I6 Restore `/var/lib/mazima` and `/data/apps` into a scratch directory;
      compare state, master key and an app file.
- [ ] I7 Restore onto a fresh host/VM and log in with the restored admin
      account; apps and desktop state return.
- [ ] I8 Force a backup failure (wrong password/unreachable repo); the service
      exits non-zero and does not silently report success.

## J. Reboot, persistence and upgrades

- [ ] J1 Place `/data` and `/var/lib/mazima` on persistent storage, reboot,
      and confirm users, apps, files and desktop state survive.
- [ ] J2 Boot without the data disk; the UI shows a clear error instead of
      corrupting state.
- [ ] J3 Cut power during a write; the JSON state is still readable or
      recovered from the documented fallback.
- [ ] J4 Fill the data disk; the UI reports the condition and writes fail
      safely.
- [ ] J5 Upgrade the image/release while preserving `/data`; apps still start.
- [ ] J6 Re-run `mazimactl selftest`; it reports `ok` and the expected version.

## K. Network and security

- [ ] K1 From another LAN host, only ports 22, 80 and 443 are reachable;
      `8088` and app ports are not.
- [ ] K2 With nftables stopped, confirm the expected failure mode and do not
      leave the machine exposed.
- [ ] K3 Log in over HTTP (if reachable); the app redirects or refuses because
      cookies are secure.
- [ ] K4 Send a request with a foreign `Origin`; WebSocket/API rejects it.
- [ ] K5 Attempt path traversal on static assets; it returns 404/400.
- [ ] K6 Inspect response headers: CSP, `X-Content-Type-Options`,
      `Referrer-Policy`, `X-Frame-Options` on the main app.
- [ ] K7 Export/download the audit log; secrets, passwords, tokens and
      clipboard content are redacted.
- [ ] K8 Confirm the setup token, master key, restic password and OTA private
      key are not present in the repo, ISO, logs or release assets.

## L. Performance and soak

- [ ] L1 Idle CPU and RAM are acceptable for the target hardware; record
      `uptime`, `free -m` and `docker stats`.
- [ ] L2 Open several windows and two streams; measure browser and daemon
      memory for 30 minutes.
- [ ] L3 Run the backup and an update check concurrently with normal use; no
      deadlock or UI freeze.
- [ ] L4 Leave the machine running for 24 hours; check for leaks, crashed
      units and session buildup.
- [ ] L5 Confirm logs rotate and do not fill the live filesystem.

## Sign-off

| Field | Value |
| --- | --- |
| Version / ISO SHA-256 | |
| Hardware / VM | |
| Date and tester | |
| Passed | |
| Failed / blocked | |
| Evidence location | |
| Release decision | |
