#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VERSION=${1:-$(cat VERSION)}
VERSION=${VERSION#v}
CHANNEL=${CHANNEL:-stable}
MIN_VERSION=${MIN_VERSION:-}
PRIVATE_KEY=${MAZIMA_OTA_PRIVATE_KEY:-signing/ota.key}
PUBLIC_KEY=${MAZIMA_OTA_PUBLIC_KEY:-signing/ota.pub}
DIST=${DIST:-dist}
PUBLISH=${PUBLISH:-0}
TARGET=${TARGET:-linux-amd64}
GOOS=${TARGET%%-*}
GOARCH=${TARGET#*-}

case "$VERSION" in
  ""|*dev*|*[!0-9A-Za-z.+-]*)
    echo "release: invalid version '$VERSION' (expected e.g. 1.0.0)" >&2
    exit 2
    ;;
esac

if [ ! -f "$PRIVATE_KEY" ]; then
  echo "release: missing private key $PRIVATE_KEY" >&2
  echo "run: bin/mazimaota keygen -private signing/ota.key -public signing/ota.pub" >&2
  exit 2
fi
if [ ! -f "$PUBLIC_KEY" ]; then
  echo "release: missing public key $PUBLIC_KEY" >&2
  exit 2
fi

mkdir -p bin
echo "Building host signing tool"
(cd daemon && go build -trimpath -o ../bin/mazimaota ./cmd/mazimaota)
GO_KEY=$(bin/mazimaota public -private "$PRIVATE_KEY")

FILE_KEY=$(tr -d '\r\n' < "$PUBLIC_KEY")
if [ "$GO_KEY" != "$FILE_KEY" ]; then
  echo "release: private key does not match $PUBLIC_KEY" >&2
  exit 2
fi

echo "Building MazimaOS $VERSION for $TARGET with OTA key $FILE_KEY"
rm -f bin/mazimad bin/mazimactl
GOOS="$GOOS" GOARCH="$GOARCH" make build VERSION="$VERSION" UPDATE_PUBLIC_KEY="$FILE_KEY"
file bin/mazimad

rm -rf "$DIST"
mkdir -p "$DIST/bundle/bin" "$DIST/bundle/ui" "$DIST/bundle/store"
cp bin/mazimad bin/mazimactl "$DIST/bundle/bin/"
cp -a ui/. "$DIST/bundle/ui/"
cp -a store/. "$DIST/bundle/store/"
find "$DIST/bundle" -name .DS_Store -delete
cp README.md "$DIST/bundle/"
printf '%s\n' "$VERSION" > "$DIST/bundle/VERSION"

BUNDLE="$DIST/mazima-${VERSION}-${TARGET}.tar.gz"
( cd "$DIST/bundle" && tar -czf "../$(basename "$BUNDLE")" . )
rm -rf "$DIST/bundle"

NOTES=${NOTES:-docs/releases/$VERSION.md}
if [ ! -f "$NOTES" ]; then
  NOTES="$DIST/release-notes.md"
  printf 'MazimaOS %s\n\nSee docs/FEATURE-AUDIT.md for the verified feature set.\n' "$VERSION" > "$NOTES"
fi

MANIFEST="$DIST/mazima-manifest.json"
SIGNATURE="$DIST/mazima-manifest.json.sig"
MIN_ARGS=()
if [ -n "$MIN_VERSION" ]; then
  MIN_ARGS=(-min-version "$MIN_VERSION")
fi
bin/mazimaota manifest \
  -version "$VERSION" \
  -channel "$CHANNEL" \
  ${MIN_ARGS[@]+"${MIN_ARGS[@]}"} \
  -bundle "$BUNDLE" \
  -notes "$NOTES" \
  -out "$MANIFEST"
bin/mazimaota sign -key "$PRIVATE_KEY" -in "$MANIFEST" -out "$SIGNATURE"
bin/mazimaota verify -public "$PUBLIC_KEY" -in "$MANIFEST" -sig "$SIGNATURE"

echo
echo "Release artifacts:"
ls -lh "$BUNDLE" "$MANIFEST" "$SIGNATURE"

if [ "$PUBLISH" != "1" ]; then
  echo
  echo "Dry run complete. Set PUBLISH=1 to create the GitHub release."
  exit 0
fi

gh release create "v$VERSION" \
  "$BUNDLE" "$MANIFEST" "$SIGNATURE" \
  --title "MazimaOS $VERSION" \
  --notes-file "$NOTES"
