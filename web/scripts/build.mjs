import { mkdir, copyFile, rm } from "node:fs/promises";
import { build } from "esbuild";
await rm("dist", { recursive: true, force: true });
await mkdir("dist", { recursive: true });
await build({
  entryPoints: ["src/main.ts"],
  bundle: true,
  format: "esm",
  target: "es2022",
  outfile: "dist/main.js",
  legalComments: "eof",
});
await Promise.all([
  copyFile("index.html", "dist/index.html"),
  copyFile("src/style.css", "dist/style.css"),
]);
