#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${VERSION:-$(cat "$ROOT/VERSION")}
ARCH=amd64

log() {
	printf 'image-build: %s\n' "$*" >&2
}

case "$VERSION" in
*[!0-9A-Za-z.+-]* | "")
	log "invalid VERSION: $VERSION"
	exit 1
	;;
esac

if [ "$(uname -m)" != "x86_64" ]; then
	log "run this on a Debian x86_64 host (live-build needs a native amd64 chroot)"
	exit 1
fi

if ! command -v lb >/dev/null 2>&1; then
	log "live-build is missing; install it with: sudo apt install live-build"
	exit 1
fi

SUDO=
if [ "$(id -u)" -ne 0 ]; then
	if ! command -v sudo >/dev/null 2>&1; then
		log "run as root or install sudo"
		exit 1
	fi
	SUDO=sudo
fi

BUILD=$(mktemp -d "${TMPDIR:-/tmp}/mazima-image.XXXXXX")
cleanup() {
	$SUDO rm -rf "$BUILD"
}
trap cleanup EXIT INT TERM

log "building binaries for linux/$ARCH"
rm -f "$ROOT/bin/mazimad" "$ROOT/bin/mazimactl"
if [ -n "${OTA_PUBLIC_KEY:-}" ]; then
	(cd "$ROOT" && GOOS=linux GOARCH="$ARCH" make build VERSION="$VERSION" UPDATE_PUBLIC_KEY="$OTA_PUBLIC_KEY")
else
	log "OTA_PUBLIC_KEY is empty; the image will not trust OTA updates"
	(cd "$ROOT" && GOOS=linux GOARCH="$ARCH" make build VERSION="$VERSION")
fi

cp -a "$ROOT/image/live-build/." "$BUILD/"
(cd "$BUILD" && lb config)

DEST="$BUILD/config/includes.chroot/opt/mazima/releases/$VERSION"
install -d "$DEST/bin"
install -m 0755 "$ROOT/bin/mazimad" "$DEST/bin/mazimad"
install -m 0755 "$ROOT/bin/mazimactl" "$DEST/bin/mazimactl"
cp -a "$ROOT/ui" "$DEST/ui"
cp -a "$ROOT/store" "$DEST/store"
printf '%s\n' "$VERSION" > "$DEST/VERSION"
chmod 0644 "$DEST/VERSION"
ln -sfn "releases/$VERSION" "$BUILD/config/includes.chroot/opt/mazima/current"
install -d "$BUILD/config/includes.chroot/usr/local/bin"
install -m 0755 "$ROOT/scripts/backup.sh" "$BUILD/config/includes.chroot/usr/local/bin/mazima-backup"
install -d "$BUILD/config/includes.chroot/var/backups/mazima"

log "running live-build"
(cd "$BUILD" && $SUDO lb build)

ISO=$(find "$BUILD" -maxdepth 1 -name '*.iso' -print -quit)
if [ -z "$ISO" ]; then
	log "live-build did not produce an ISO"
	exit 1
fi

mkdir -p "$ROOT/dist"
OUT="$ROOT/dist/mazima-${VERSION}-${ARCH}.iso"
$SUDO cp "$ISO" "$OUT"
$SUDO chown "$(id -u):$(id -g)" "$OUT"
(cd "$ROOT/dist" && sha256sum "$(basename "$OUT")" >"$(basename "$OUT").sha256")
log "wrote $OUT"
