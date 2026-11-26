import { build } from "esbuild";
import { mkdtemp, rm, readdir } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
const dir = await mkdtemp(join("node_modules", ".foliochest-web-test-"));
try {
  for (const test of (await readdir("tests"))
    .filter((name) => /\.test\.tsx?$/.test(name))
    .sort()) {
    const outfile = join(dir, test + ".mjs");
    await build({
      entryPoints: [join("tests", test)],
      bundle: true,
      platform: "node",
      packages: "external",
      jsx: "automatic",
      format: "esm",
      target: "node20",
      outfile,
    });
    const dom = test.endsWith(".tsx")
      ? (await import("../tests/dom.mjs")).setupDOM()
      : null;
    try {
      await import(pathToFileURL(outfile).href);
    } finally {
      dom?.();
    }
  }
} finally {
  await rm(dir, { recursive: true, force: true });
}
