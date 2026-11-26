import assert from "node:assert/strict";
import { SessionClient, Sequence, StaleRequest } from "../src/lib/client";
import { state, json, deferred } from "./fixtures";
let current = structuredClone(state),
  transportFailure = false;
const queue: Array<ReturnType<typeof deferred<Response>>> = [],
  calls: { path: string; options?: RequestInit }[] = [];
const client = new SessionClient(async (path, options) => {
  calls.push({ path: String(path), options });
  if (String(path) === "/api/state") {
    if (transportFailure) throw new Error("offline");
    return json(current);
  }
  const response = deferred<Response>();
  queue.push(response);
  return response.promise;
});
await client.refresh();
assert.strictEqual(client.getSnapshot(), client.getSnapshot());
let changes = 0;
const unsubscribe = client.subscribe(() => changes++);
const first = client.request("/api/entries/save", { changes: [] });
assert.equal(calls.at(-1)?.options?.credentials, "same-origin");
assert.equal(calls.at(-1)?.options?.cache, "no-store");
assert.equal(
  (calls.at(-1)?.options?.headers as Record<string, string>)[
    "X-FolioChest-CSRF"
  ],
  state.csrf,
);
queue.shift()!.resolve(json({ ok: true }));
assert.deepEqual(await first, { ok: true });
for (const status of [200, 401, 409]) {
  const old = client.request("/api/entry");
  const epoch = client.epoch;
  client.invalidate();
  current = { ...current, csrf: "new-csrf-" + status, authenticated: true };
  await client.refresh();
  queue
    .shift()!
    .resolve(
      json(
        status === 200 ? { value: "STALE_PRIVATE_VALUE" } : { error: "stale" },
        status,
      ),
    );
  await assert.rejects(old, StaleRequest);
  assert.equal(client.getSnapshot().state?.authenticated, true);
  const count = calls.length;
  await assert.rejects(
    client.request("/api/entries/save", {}, epoch),
    StaleRequest,
  );
  assert.equal(calls.length, count);
}
const oldBlob = client.blob("/api/backup", {});
const blob = deferred<Blob>(),
  response = json({});
response.blob = () => blob.promise;
queue.shift()!.resolve(response);
await Promise.resolve();
await Promise.resolve();
client.invalidate();
blob.resolve(new Blob(["STALE_BACKUP"]));
await assert.rejects(oldBlob, StaleRequest);
await client.refresh();
const epoch = client.epoch;
transportFailure = true;
await client.refresh();
assert.equal(client.epoch, epoch);
assert.equal(client.getSnapshot().state?.authenticated, true);
assert.match(client.getSnapshot().error, /offline/);
transportFailure = false;
const denied = client.request("/api/entry");
current = { ...current, authenticated: false };
queue.shift()!.resolve(json({ error: "locked" }, 401));
await assert.rejects(denied, /locked/);
assert.equal(client.getSnapshot().state?.authenticated, false);
assert.ok(changes > 0);
unsubscribe();
const responses = [deferred<Response>(), deferred<Response>()];
let n = 0;
const ordered = new SessionClient(() => responses[n++].promise);
const older = ordered.refresh(),
  newer = ordered.refresh();
responses[1].resolve(json({ ...state, csrf: "latest" }));
await newer;
responses[0].resolve(json({ ...state, csrf: "stale", authenticated: false }));
await older;
assert.equal(ordered.getSnapshot().state?.csrf, "latest");
assert.equal(ordered.getSnapshot().state?.authenticated, true);
const seq = new Sequence(),
  a = seq.next();
assert.ok(seq.current(a));
seq.next();
assert.equal(seq.current(a), false);
console.log(
  "Session, CSRF, late 401/JSON/blob and state-ordering tests passed.",
);
