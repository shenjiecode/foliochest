import assert from "node:assert/strict";
import {
  addAttachments,
  downloadAttachment,
  encodeBase64,
  fileSize,
  readFile,
} from "../src/lib/files";
import { StaleRequest } from "../src/lib/client";
import { deferred } from "./fixtures";
const source = Uint8Array.from({ length: 32769 }, (_, i) => i % 256),
  encoded = Buffer.from(source).toString("base64");
assert.equal(encodeBase64(source), encoded);
assert.equal(fileSize(encoded), source.length);
assert.equal(fileSize("YQ=="), 1);
assert.equal(fileSize("YWI="), 2);
const data = source.slice(),
  buffer = deferred<ArrayBuffer>();
const file = {
  name: "synthetic.bin",
  size: data.length,
  type: "application/octet-stream",
  arrayBuffer: () => buffer.promise,
} as File;
let active = true;
const loading = readFile(file, 2 * 1024 * 1024, () => {
  if (!active) throw new StaleRequest();
});
active = false;
buffer.resolve(data.buffer);
await assert.rejects(loading, StaleRequest);
assert.ok(data.every((b) => b === 0));
const saved = [{ name: "same", media_type: "text/plain", data: "YQ==" }];
await assert.rejects(
  addAttachments(saved, [new File(["x"], "same")], () => {}),
  /同名/,
);
await assert.rejects(
  addAttachments(
    Array.from({ length: 16 }, (_, i) => ({ ...saved[0], name: String(i) })),
    [new File(["x"], "extra")],
    () => {},
  ),
  /16/,
);
await assert.rejects(
  readFile(
    { name: "huge", size: 66 * 1024 * 1024 + 1 } as File,
    66 * 1024 * 1024,
    () => {},
  ),
  /66 MiB/,
);
const next = await addAttachments(
  saved,
  [new File(["SYNTHETIC_ATTACHMENT"], "new")],
  () => {},
);
assert.equal(saved.length, 1);
assert.equal(next.length, 2);
assert.equal(atob(next[1].data), "SYNTHETIC_ATTACHMENT");
const original = {
  document: globalThis.document,
  create: URL.createObjectURL,
  revoke: URL.revokeObjectURL,
  timer: globalThis.setTimeout,
};
try {
  let blob: Blob | undefined,
    clicked = false,
    revoked = "",
    cleanup!: () => void;
  const link = {
    href: "",
    download: "",
    click: () => {
      clicked = true;
    },
  };
  (globalThis as any).document = { createElement: () => link };
  URL.createObjectURL = (value) => {
    blob = value;
    return "blob:synthetic";
  };
  URL.revokeObjectURL = (value) => {
    revoked = value;
  };
  (globalThis as any).setTimeout = (fn: () => void, delay: number) => {
    assert.equal(delay, 1000);
    cleanup = fn;
  };
  downloadAttachment({
    name: "synthetic.bin",
    data: encoded,
    media_type: "text/html",
  });
  assert.equal(clicked, true);
  assert.equal(blob?.type, "application/octet-stream");
  assert.equal(link.download, "synthetic.bin");
  assert.equal(revoked, "");
  cleanup();
  assert.equal(revoked, link.href);
} finally {
  if (original.document === undefined) delete (globalThis as any).document;
  else globalThis.document = original.document;
  URL.createObjectURL = original.create;
  URL.revokeObjectURL = original.revoke;
  globalThis.setTimeout = original.timer;
}
console.log(
  "Attachment limits, atomic draft append, stale-file clearing and download cleanup passed.",
);
