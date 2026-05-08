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

## Publish

Push a semver tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow checks out the tag, installs Go and pnpm, runs
`pnpm check`, then runs `goreleaser release --clean` with `GITHUB_TOKEN`.

Manual release dispatch is available for an existing tag through the
`release` workflow's `tag_name` input.
