# MazimaOS image

This directory builds a Debian 13 (`trixie`) live ISO for x86_64 machines.
The image is intentionally a live system: it boots read-only from the ISO and
keeps state in RAM unless `/data` and `/var/lib/mazima` are moved to persistent
storage. There is no automated disk installer yet.

## Build (Debian x86_64 host)

```sh
sudo apt install live-build
OTA_PUBLIC_KEY="$(cat signing/ota.pub)" ./image/build.sh
```

`OTA_PUBLIC_KEY` is optional. Without it the image starts with no OTA trust
anchor and will refuse signed updates. The output is
`dist/mazima-<version>-amd64.iso` plus a SHA-256 file.

The build cross-compiles `mazimad`/`mazimactl` for `linux/amd64` and stages:

- `/opt/mazima/releases/<version>` with `bin`, `ui`, `store`
- `/opt/mazima/current` as a relative symlink for atomic updates
- `/etc/mazima/config.json`
- Caddy, Avahi, nftables, systemd units and the restic backup timer

## Write and boot

```sh
sudo dd if=dist/mazima-0.1.0-amd64.iso of=/dev/sdX bs=4M status=progress conv=fsync
```

Boot the target machine in UEFI or legacy mode. Resolve `mazima.local` with
mDNS (Avahi), or add it to the client's hosts file. Caddy uses its internal CA,
so the first browser visit shows a certificate warning until the local root CA
is trusted.

On the first boot, `mazima-setup.service` runs on `tty1` before the login
prompt and starts a guided console wizard. It asks for language, timezone,
administrator username, display name and password, then creates the first
administrator through the local API. The setup token is read from
`/var/lib/mazima/setup.token` and is removed after success. The timezone is
applied with `timedatectl`.

The wizard keeps the hostname `mazima`, so the browser URL is
`https://mazima.local`. Changing the OS hostname safely is not implemented yet.

If you cancel the wizard, or need to configure from another device, log in on
the console with the standard Debian live credentials (`user` / `live` by
default) and run:

```sh
sudo mazimactl setup-token --config /etc/mazima/config.json
```

Open `https://mazima.local`, paste the token and create the administrator
account. `mazima` is a service account with `/usr/sbin/nologin`; do not use it
for console login. If the console asks for a login and `user` is rejected, the
image is missing `user-setup` and must be rebuilt. Disable console autologin or
change the live user password before putting the machine on an untrusted
network.

## Persistence

The default image keeps `/var/lib/mazima` and `/data` in RAM. For a real
installation, create a data partition, mount it at `/data`, and bind or mount
`/var/lib/mazima` on persistent storage before enabling applications. Do not
store Docker volumes on the live root.

## Backups

Edit `/etc/mazima/backup.env` and create `/etc/mazima/restic.password` with mode
`0600`. Then initialize the repository once:

```sh
sudo RESTIC_INIT=1 systemctl start mazima-backup.service
```

The timer runs daily at 03:30 with a randomized delay. Use a remote restic
repository for disaster recovery; a repository on the same disk is not a
backup. See `scripts/backup.sh` for the retention policy.

## What is not covered

- No automated disk installer, RAID, or disk encryption.
- No GPU passthrough or hardware transcoding configuration.
- No Tailscale installation; install and authenticate it separately.
- The ISO and its systemd units have not been booted on real hardware yet.
  Treat the first boot as a manual test.
