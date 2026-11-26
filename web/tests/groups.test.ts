import assert from "node:assert/strict";
import {
  groupBuckets,
  groupInput,
  groupText,
  matchesGroup,
  GroupUI,
} from "../src/groups";
assert.equal(matchesGroup({ group: "A" }, null), true);
assert.equal(matchesGroup({}, ""), true);
assert.equal(matchesGroup({ group: "A" }, ""), false);
assert.equal(matchesGroup({ group: "A" }, "a"), false);
assert.equal(matchesGroup({ candidate_groups: ["A", "B"] }, "B"), true);
assert.equal(matchesGroup({ candidate_groups: ["A", "B"] }, ""), false);
assert.equal(groupText({ candidate_groups: ["", "B"] }), "未分组 / B");
const buckets = groupBuckets([
  { group: "B" },
  { group: "A" },
  { group: "B" },
  {},
  { candidate_groups: ["A", "B"] },
  { group: "__proto__" },
]);
assert.equal(buckets[0].id, "conflict");
assert.equal(buckets.at(-1)?.id, "group:");
assert.equal(buckets.find((g) => g.label === "B")?.entries.length, 2);
assert.equal(buckets.flatMap((g) => g.entries).length, 6);
assert.match(groupInput("A", 2), /disabled/);
assert.doesNotMatch(groupInput("A", 3), /disabled/);
assert.doesNotMatch(groupInput('"><img src=x onerror=alert(1)>', 3), /<img/);
const ctx: any = { format: () => 3 };
const ui = new GroupUI(ctx);
ui.filter = '"><img src=x onerror=alert(1)>';
assert.doesNotMatch(ui.toolsHTML(), /<img/);
assert.match(ui.toolsHTML(), /保留条目/);
ui.selected.set("key", { key: "key", expected_revision: "old" });
ui.folded.add("group:A");
ui.reset();
assert.equal(ui.filter, null);
assert.equal(ui.selected.size, 0);
assert.equal(ui.folded.size, 0);
// A completed metadata operation must not discard a draft/new selection opened
// while it was in flight; a response from an old session cannot unlock new UI.
const fakeDocument = { querySelectorAll: () => [], querySelector: () => null };
(globalThis as any).document = fakeDocument;
try {
  let revision = 1,
    dirty = false,
    changed = 0;
  let release!: () => void;
  const pending = new GroupUI({
    api: async () => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      return {} as any;
    },
    run: async (action) => {
      await action();
    },
    notice: () => {},
    isDirty: () => dirty,
    format: () => 3,
    view: () => "entries",
    revision: () => revision,
    onSelect: async () => {},
    onChanged: async () => {
      changed++;
    },
  });
  for (const interrupted of ["draft", "navigation", "session"]) {
    dirty = false;
    const done = (pending as any).submit({ isConnected: true }, "/test", {});
    if (interrupted === "draft") dirty = true;
    if (interrupted === "navigation") revision++;
    if (interrupted === "session") pending.reset();
    release();
    await done;
    assert.equal(changed, 0);
    assert.equal(pending.busy, false);
  }
  dirty = false;
  const done = (pending as any).submit({ isConnected: true }, "/test", {});
  release();
  await done;
  assert.equal(changed, 1);
} finally {
  delete (globalThis as any).document;
}
console.log(
  "Single-level group filtering, conflict grouping, escaping and session reset tests passed.",
);
