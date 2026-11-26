import { mkdir, copyFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { build } from "esbuild";
const web = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const out = resolve(web, "../.local/ui-v0");
await mkdir(out, { recursive: true });
await build({
  absWorkingDir: web,
  entryPoints: ["src/preview/main.tsx"],
  bundle: true,
  format: "esm",
  target: "es2022",
  jsx: "automatic",
  minify: true,
  define: { "process.env.NODE_ENV": '"production"' },
  outfile: resolve(out, "main.js"),
  legalComments: "eof",
});
execFileSync(
  process.execPath,
  [
    resolve(web, "node_modules/@tailwindcss/cli/dist/index.mjs"),
    "-i",
    "src/preview/styles.css",
    "-o",
    resolve(out, "styles.css"),
    "--minify",
  ],
  { cwd: web, stdio: "inherit" },
);
await copyFile(
  resolve(web, "src/preview/index.html"),
  resolve(out, "index.html"),
);
console.log(
  "Design preview: .local/ui-v0 (not included in the Go application)",
);
