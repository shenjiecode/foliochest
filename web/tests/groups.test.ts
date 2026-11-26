import assert from "node:assert/strict";
import {
  filterEntries,
  groupBuckets,
  groupText,
  matchesGroup,
} from "../src/lib/groups";
import { summary } from "./fixtures";
const ungrouped = { ...summary, key: "empty", group: "" };
const conflict = {
  ...summary,
  key: "conflict",
  group: "",
  candidate_groups: ["工作", ""],
  state: "conflict",
};
assert.ok(matchesGroup(conflict, "工作"));
assert.ok(matchesGroup(conflict, ""));
assert.ok(!matchesGroup(conflict, "别组"));
assert.equal(groupText(conflict), "工作 / 未分组");
const entries = [summary, ungrouped, conflict];
assert.deepEqual(
  groupBuckets(entries).map((b) => b.id),
  ["conflict", "group:工作", "group:"],
);
assert.equal(groupBuckets(entries).flatMap((b) => b.entries).length, 3);
assert.equal(filterEntries(entries, "", "", null).length, 2);
assert.equal(
  filterEntries(entries, null, "synthetic_private_body", null).length,
  0,
);
assert.deepEqual(
  filterEntries(
    entries,
    null,
    "synthetic_private_body",
    new Set(["empty"]),
  ).map((e) => e.key),
  ["empty"],
);
console.log(
  "Exact groups, conflict buckets and metadata/body-result separation passed.",
);
