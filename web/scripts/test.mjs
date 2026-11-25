import { build } from "esbuild";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
const dir = await mkdtemp(join(tmpdir(), "keyfolio-web-test-"));
try {
  const outfile = join(dir, "test.mjs");
  await build({
    entryPoints: ["tests/markdown.test.ts"],
    bundle: true,
    platform: "node",
    format: "esm",
    target: "node20",
    outfile,
  });
  await import(pathToFileURL(outfile).href);
} finally {
  await rm(dir, { recursive: true, force: true });
}
