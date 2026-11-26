import assert from "node:assert/strict";
import { base64, encodeBase64, downloadAttachment, withBusy } from "../src/ui";

const bytes = Uint8Array.from({ length: 32769 }, (_, i) => i % 256);
const expected = Buffer.from(bytes).toString("base64");
assert.equal(encodeBase64(bytes), expected);
assert.equal(encodeBase64(new Uint8Array()), "");
assert.equal(await base64(new File([bytes], "synthetic.bin")), expected);
assert.equal(bytes[255], 255);

function fixture() {
  const controls = ["input", "button", "select", "textarea"].map((tag, i) => ({
    tag,
    disabled: i === 0,
    isConnected: true,
  }));
  const attributes = new Map<string, string>();
  const host = {
    querySelectorAll: (selector: string) => {
      assert.equal(selector, "input,button,select,textarea");
      return controls;
    },
    getAttribute: (name: string) => attributes.get(name) ?? null,
    setAttribute: (name: string, value: string) => attributes.set(name, value),
    removeAttribute: (name: string) => attributes.delete(name),
  } as unknown as HTMLElement;
  return { host, controls, attributes };
}
for (const outcome of ["success", "failure", "detached"]) {
  const { host, controls, attributes } = fixture();
  if (outcome === "failure") attributes.set("aria-busy", "false");
  let release!: () => void,
    entered = false;
  const failure = new Error("synthetic failure");
  const pending = withBusy(host, async () => {
    entered = true;
    assert.ok(controls.every((control) => control.disabled));
    assert.equal(attributes.get("aria-busy"), "true");
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    if (outcome === "failure") throw failure;
    return 42;
  });
  assert.equal(entered, true);
  if (outcome === "detached") {
    controls.forEach((control) => {
      control.isConnected = false;
    });
    // New controls were not part of the old operation's snapshot.
    controls.push({ tag: "input", disabled: false, isConnected: true });
  }
  release();
  if (outcome === "failure")
    await assert.rejects(pending, (error) => error === failure);
  else assert.equal(await pending, 42);
  assert.equal(
    attributes.get("aria-busy"),
    outcome === "failure" ? "false" : undefined,
  );
  assert.deepEqual(
    controls.slice(0, 4).map((control) => control.disabled),
    outcome === "detached"
      ? [true, true, true, true]
      : [true, false, false, false],
  );
  if (outcome === "detached") assert.equal(controls[4].disabled, false);
}
{
  const { host, controls, attributes } = fixture();
  await assert.rejects(
    withBusy(host, () => {
      throw new Error("sync failure");
    }),
  );
  assert.equal(attributes.has("aria-busy"), false);
  assert.deepEqual(
    controls.map((control) => control.disabled),
    [true, false, false, false],
  );
}
for (const order of [
  [0, 1],
  [1, 0],
]) {
  const { host, controls, attributes } = fixture();
  const release: (() => void)[] = [];
  const pending = [0, 1].map(() =>
    withBusy(host, async () => {
      await new Promise<void>((resolve) => {
        release.push(resolve);
      });
    }),
  );
  release[order[0]]();
  await pending[order[0]];
  assert.ok(controls.every((control) => control.disabled));
  assert.equal(attributes.get("aria-busy"), "true");
  release[order[1]]();
  await pending[order[1]];
  assert.deepEqual(
    controls.map((control) => control.disabled),
    [true, false, false, false],
  );
  assert.equal(attributes.has("aria-busy"), false);
}
const originals = {
  document: globalThis.document,
  create: URL.createObjectURL,
  revoke: URL.revokeObjectURL,
  timeout: globalThis.setTimeout,
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
  (globalThis as any).document = {
    createElement: (name: string) => {
      assert.equal(name, "a");
      return link;
    },
  };
  URL.createObjectURL = (value: Blob) => {
    blob = value;
    return "blob:synthetic";
  };
  URL.revokeObjectURL = (value: string) => {
    revoked = value;
  };
  (globalThis as any).setTimeout = (callback: () => void, delay: number) => {
    assert.equal(delay, 1000);
    cleanup = callback;
    return 1;
  };
  downloadAttachment({ name: 'synthetic "<file>.bin', data: expected });
  assert.equal(clicked, true);
  assert.equal(link.download, 'synthetic "<file>.bin');
  assert.equal(link.href, "blob:synthetic");
  assert.equal(blob?.type, "application/octet-stream");
  assert.deepEqual(new Uint8Array(await blob!.arrayBuffer()), bytes);
  assert.equal(revoked, "");
  cleanup();
  assert.equal(revoked, link.href);
} finally {
  if (originals.document === undefined) delete (globalThis as any).document;
  else globalThis.document = originals.document;
  URL.createObjectURL = originals.create;
  URL.revokeObjectURL = originals.revoke;
  globalThis.setTimeout = originals.timeout;
}
console.log(
  "Shared busy-state recovery, attachment encoding and download tests passed.",
);
