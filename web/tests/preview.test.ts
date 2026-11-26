import assert from "node:assert/strict";
import {
  exampleEntries,
  previewOptions,
  groupsOf,
  saveExample,
  visibleEntries,
} from "../src/preview/model";

assert.deepEqual(previewOptions(""), { unlocked: true, pendingRequest: false });
assert.deepEqual(previewOptions("?screen=unlock&approval=1"), {
  unlocked: false,
  pendingRequest: false,
});
assert.equal(previewOptions("?approval=1").pendingRequest, true);
assert.equal(previewOptions("?approval=0").pendingRequest, false);
assert.equal(previewOptions("?approved=true").pendingRequest, false);

const entries = exampleEntries();
assert.equal(entries.length, 9);
assert.equal(visibleEntries(entries, null, "", false, false).length, 8);
assert.equal(visibleEntries(entries, "", "", false, false).length, 1);
assert.equal(visibleEntries(entries, "工作资料", "", false, false).length, 3);
assert.equal(visibleEntries(entries, "工作资料", "", false, true).length, 1);
assert.equal(
  visibleEntries(entries, null, "保留上一版本", false, false).length,
  0,
);
assert.equal(
  visibleEntries(entries, null, "保留上一版本", true, false).length,
  1,
);
assert.equal(visibleEntries(entries, null, "SYNTHETIC", true, false).length, 1);
assert.equal(
  visibleEntries(entries, null, "不存在的内容", true, false).length,
  0,
);
assert.deepEqual(groupsOf(entries), ["工作资料", "开发资料", "个人资料"]);
const first = entries[0];
assert.notEqual(first.history[0].value, first.history[1].value);
assert.equal(first.history[0].attachments.length, 0);
assert.equal(first.history[1].attachments.length, 2);
const changed = saveExample(entries, { ...first, value: "SYNTHETIC_EDIT" });
assert.equal(changed[0].value, "SYNTHETIC_EDIT");
assert.equal(changed[0].history.length, first.history.length + 1);
assert.equal(changed[0].history.at(-1)?.value, "SYNTHETIC_EDIT");
assert.equal(first.history.length, 2);
assert.notEqual(first.value, "SYNTHETIC_EDIT");
assert.equal(changed[0].read, first.read);
assert.equal(changed[0].write, first.write);
assert.equal(changed[0].group, first.group);
assert.equal(changed[1], entries[1]);
assert.notEqual(changed[0].history.at(-1)?.attachments, changed[0].attachments);
assert.notEqual(exampleEntries()[0].attachments, entries[0].attachments);
assert.deepEqual(exampleEntries(), entries);
assert.ok(
  entries.every((entry) => entry.read === "ask" && entry.write === "ask"),
);
const deleted = entries.find((entry) => entry.deleted)!;
assert.equal(deleted.value, "");
assert.ok(deleted.history[0].value);
assert.ok(groupsOf([{ ...deleted, group: "仅回收站" }]).includes("仅回收站"));
console.log(
  "Synthetic React preview model: filters, groups, isolated changes and history passed.",
);
