# Releasing and signing

The updater trusts one Ed25519 public key embedded at build time. The private
key never enters the repository or the image.

## One-time key setup

```sh
(cd daemon && go build -o ../bin/mazimaota ./cmd/mazimaota)
bin/mazimaota keygen -private signing/ota.key -public signing/ota.pub
cat signing/ota.pub
```

Store `signing/ota.key` in a password manager or CI secret manager. Back it up:
losing it means no future update can be installed without shipping a new image
with a new trust anchor. Never commit `signing/` (it is gitignored).

Add these GitHub Actions secrets to the repository:

- `MAZIMA_OTA_PRIVATE_KEY` — the single-line base64 seed from `signing/ota.key`
- `MAZIMA_OTA_PUBLIC_KEY` — the single-line base64 public key

The release workflow writes both to `signing/` with mode `0600` for the build
and does not upload the private key.

## Local dry run

```sh
./scripts/release.sh 1.0.0
```

The script:

1. checks that the private key matches the public key;
2. cross-compiles `mazimad` and `mazimactl` for `linux/amd64`;
3. embeds the public key with `UPDATE_PUBLIC_KEY`;
4. stages `bin/`, `ui/`, `store/`, `README.md` and `VERSION`;
5. creates `dist/mazima-<version>-linux-amd64.tar.gz`;
6. hashes the bundle into `dist/mazima-manifest.json`;
7. signs the manifest to `dist/mazima-manifest.json.sig`;
8. verifies the signature with `mazimaota verify`.

`dist/` is disposable; do not commit it. `PUBLISH=1` creates a GitHub release
and uploads the bundle, manifest and signature. Publishing is an external,
non-reversible action: do it only after the manual checklist and explicit
approval.

## GitHub release

Run the **Release** workflow with the version (without `v`) and the channel.
It needs `contents: write` permission and the two secrets above. The workflow
uses `scripts/release.sh` with `PUBLISH=1`; the tag `v<version>` is created by
`gh release create`.

After publishing, verify from a clean machine:

```sh
curl -L -o mazima.tar.gz \
  https://github.com/danyx67800/mazimaos/releases/download/v1.0.0/mazima-1.0.0-linux-amd64.tar.gz
curl -L -o manifest.json \
  https://github.com/danyx67800/mazimaos/releases/download/v1.0.0/mazima-manifest.json
curl -L -o manifest.json.sig \
  https://github.com/danyx67800/mazimaos/releases/download/v1.0.0/mazima-manifest.json.sig
bin/mazimaota verify -public signing/ota.pub -in manifest.json -sig manifest.json.sig
sha256sum mazima.tar.gz
```

The hash and size must match `manifest.json`.

## Image prerelease

The **Image** workflow builds the Debian ISO on an x86_64 GitHub runner inside
a privileged Debian trixie container. Run it manually with the version and
`publish=true`; it creates or updates the matching GitHub prerelease and
uploads the ISO plus SHA-256 file. The ISO is built with
`MAZIMA_OTA_PUBLIC_KEY`; without that repository secret it has no update trust
anchor.

The OTA bundle can be added to the same prerelease:

```sh
./scripts/release.sh 0.1.0-rc.3
gh release upload v0.1.0-rc.3 \
  dist/mazima-0.1.0-rc.3-linux-amd64.tar.gz \
  dist/mazima-manifest.json \
  dist/mazima-manifest.json.sig
```

Do not describe an image prerelease as 1.0.0 until the manual matrix in
[MANUAL-TESTING.md](MANUAL-TESTING.md) is complete.

## Rotation and revocation

There is no online revocation in `0.1.x`. To rotate the key, ship an image or
release that embeds the new public key, then stop signing with the old key.
Document the rotation date and keep the old public key for verifying historical
releases. Do not reuse a private key across channels or projects.

## Rollback

The updater keeps the previous release directory and swaps `current` only after
a successful extraction. Rollback is a separate, password-confirmed action and
must be tested with a real signed release before 1.0.
