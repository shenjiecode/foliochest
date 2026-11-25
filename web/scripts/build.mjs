import { mkdir, copyFile } from "node:fs/promises";
await mkdir("dist", { recursive: true });
await Promise.all([
  copyFile("index.html", "dist/index.html"),
  copyFile("src/style.css", "dist/style.css"),
]);
