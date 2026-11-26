import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
const root = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../.local/ui-v0",
);
const port = Number(process.env.FOLIOCHEST_PREVIEW_PORT || 7464);
if (!Number.isInteger(port) || port < 1024 || port > 65535)
  throw new Error("Invalid preview port");
const files = new Map([
  ["/", ["index.html", "text/html; charset=utf-8"]],
  ["/index.html", ["index.html", "text/html; charset=utf-8"]],
  ["/main.js", ["main.js", "text/javascript; charset=utf-8"]],
  ["/styles.css", ["styles.css", "text/css; charset=utf-8"]],
]);
// Static, synthetic-only preview. No API proxy, file uploads, cookies or request logging.
// Inline style permission is for Radix overlays in this design sandbox only;
// the production server's stricter CSP is unchanged and must be preserved on migration.
const server = createServer(async (req, res) => {
  res.setHeader("Cache-Control", "no-store");
  res.setHeader("X-Content-Type-Options", "nosniff");
  res.setHeader("X-Frame-Options", "DENY");
  res.setHeader("Referrer-Policy", "no-referrer");
  res.setHeader(
    "Content-Security-Policy",
    "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; connect-src 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
  );
  if (!["127.0.0.1:" + port, "localhost:" + port].includes(req.headers.host)) {
    res.writeHead(403).end();
    return;
  }
  if (!["GET", "HEAD"].includes(req.method)) {
    res.writeHead(405, { Allow: "GET, HEAD" }).end();
    return;
  }
  const path = req.url?.split("?")[0] || "/";
  if (path === "/favicon.ico") {
    res.writeHead(204).end();
    return;
  }
  const file = files.get(path);
  if (!file) {
    res.writeHead(404).end();
    return;
  }
  try {
    const data = await readFile(resolve(root, file[0]));
    res.setHeader("Content-Type", file[1]);
    res.writeHead(200).end(req.method === "HEAD" ? undefined : data);
  } catch {
    res
      .writeHead(503)
      .end("Build the preview first: npm --prefix web run build:preview");
  }
});
server.listen(port, "127.0.0.1", () =>
  console.log("FolioChest design preview http://127.0.0.1:" + port),
);
