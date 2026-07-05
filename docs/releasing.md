---
read_when:
  - cutting a ClickClack release
  - changing GoReleaser, package artifacts, or release automation
---

# Releasing

ClickClack uses GoReleaser v2. The config is `.goreleaser.yml`; the GitHub
Actions publisher is `.github/workflows/release.yml`.

## Local Smoke Test

```sh
pnpm install
goreleaser check
CLICKCLACK_WEB_VERSION="$(git rev-parse --short=12 HEAD)" \
  goreleaser release --snapshot --clean
```

The snapshot build runs `pnpm build`, then cross-compiles `clickclack` for:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`
- `freebsd/amd64`
- `freebsd/arm64`

It also emits Linux `.deb` and `.rpm` packages and `sha256sums.txt`.

The same workflow builds native desktop installers on their matching GitHub
runner. The desktop app version comes from the release tag, and each runner
emits a platform checksum manifest:

- macOS: x64 and arm64 `.dmg` and `.zip`
- Windows: x64 NSIS `.exe` and `.zip`
- Linux: x64 `.AppImage` and `.deb`

GoReleaser leaves the GitHub Release as a draft after uploading the server
artifacts. The publish job downloads all three runner outputs, verifies every
SHA-256 manifest, attaches the installers and manifests to that draft, and only
then publishes it. A failed native build, upload, or checksum leaves a private
draft instead of exposing an incomplete release.

## Publish

Push a semver tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow checks out the tag, installs Go and pnpm, runs
`pnpm check`, sets `CLICKCLACK_WEB_VERSION` to the checked-out commit, then
runs `goreleaser release --clean` with `GITHUB_TOKEN`. In parallel, native
runners build the desktop apps. Once GoReleaser and all native builds succeed,
the verified desktop files are uploaded and the draft is published.

Manual release dispatch is available for an existing tag through the
`release` workflow's `tag_name` input. GoReleaser reuses an existing draft and
replaces matching server assets, while the desktop uploader replaces matching
desktop assets. This makes a failed draft release safe to retry without making
published releases mutable.
