import { createHash } from "node:crypto";
import { existsSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

const releaseTargets = Object.freeze({
  linux: Object.freeze(["linux-amd64.deb", "linux-x86_64.AppImage"]),
  mac: Object.freeze(["mac-arm64.dmg", "mac-arm64.zip", "mac-x64.dmg", "mac-x64.zip"]),
  win: Object.freeze(["win-x64.exe", "win-x64.zip"]),
});

export function normalizeReleaseVersion(tag) {
  const match =
    /^v?((0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$/.exec(
      tag,
    );
  if (!match) {
    throw new Error(`Release version must be semantic version vMAJOR.MINOR.PATCH: ${tag}`);
  }
  return match[1];
}

export function expectedDesktopArtifactNames(platform, tag) {
  const target = releaseTargets[platform];
  if (!target) {
    throw new Error(`Unsupported desktop release platform: ${platform}`);
  }
  const version = normalizeReleaseVersion(tag);
  return target.map((suffix) => `ClickClack-${version}-${suffix}`);
}

export function writeDesktopChecksums(directory, platform, tag) {
  const version = normalizeReleaseVersion(tag);
  const artifacts = expectedDesktopArtifactNames(platform, version).sort();
  const lines = artifacts.map((name) => {
    const file = path.join(directory, name);
    if (!existsSync(file) || !statSync(file).isFile()) {
      throw new Error(`Desktop release artifact is not a file: ${file}`);
    }
    return `${createHash("sha256").update(readFileSync(file)).digest("hex")}  ${name}`;
  });
  const output = path.join(directory, `ClickClack-${version}-${platform}-SHA256SUMS.txt`);
  writeFileSync(output, `${lines.join("\n")}\n`);
  return output;
}

const isMain =
  process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url;
if (isMain) {
  const [platform, tag, directory = path.resolve("apps/desktop/release")] = process.argv.slice(2);
  if (!platform || !tag) {
    throw new Error(
      "Usage: node apps/desktop/scripts/release-artifacts.mjs <mac|win|linux> <version> [release-directory]",
    );
  }
  console.log(writeDesktopChecksums(path.resolve(directory), platform, tag));
}
