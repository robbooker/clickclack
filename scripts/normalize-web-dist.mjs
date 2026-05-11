import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const dist = process.argv[2];
if (!dist) {
  throw new Error("usage: node scripts/normalize-web-dist.mjs <dist>");
}

for (const file of ["index.html", "200.html"]) {
  const path = join(dist, file);
  const input = readFileSync(path, "utf8");
  const output = input.replace(/[ \t]+$/gm, "");
  if (output !== input) writeFileSync(path, output);
}
