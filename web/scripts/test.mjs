import { build } from "esbuild";
import { mkdtemp, rm, readdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
const dir = await mkdtemp(join(tmpdir(), "keyfolio-web-test-"));
try {
  for (const test of (await readdir("tests"))
    .filter((name) => name.endsWith(".test.ts"))
    .sort()) {
    const outfile = join(dir, test + ".mjs");
    await build({
      entryPoints: [join("tests", test)],
      bundle: true,
      platform: "node",
      format: "esm",
      target: "node20",
      outfile,
    });
    await import(pathToFileURL(outfile).href);
  }
} finally {
  await rm(dir, { recursive: true, force: true });
}
