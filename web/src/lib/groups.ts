import type { Summary } from "./types";
export const groupLabel = (name: string) => name || "未分组";
export const matchesGroup = (
  entry: Pick<Summary, "group" | "candidate_groups">,
  group: string | null,
) =>
  group === null ||
  (entry.candidate_groups?.length
    ? entry.candidate_groups.includes(group)
    : (entry.group || "") === group);
export const groupText = (
  entry: Pick<Summary, "group" | "candidate_groups">,
) =>
  entry.candidate_groups?.length
    ? entry.candidate_groups.map(groupLabel).join(" / ")
    : groupLabel(entry.group || "");
export function groupBuckets(entries: Summary[]) {
  const buckets = new Map<
    string,
    { id: string; label: string; entries: Summary[] }
  >();
  for (const entry of entries) {
    const id = entry.candidate_groups?.length
      ? "conflict"
      : "group:" + (entry.group || "");
    if (!buckets.has(id))
      buckets.set(id, {
        id,
        label: id === "conflict" ? "分组待确认" : groupLabel(entry.group || ""),
        entries: [],
      });
    buckets.get(id)!.entries.push(entry);
  }
  return [...buckets.values()].sort((a, b) =>
    a.id === b.id
      ? 0
      : a.id === "conflict"
        ? -1
        : b.id === "conflict"
          ? 1
          : a.id === "group:"
            ? 1
            : b.id === "group:"
              ? -1
              : a.label.localeCompare(b.label, "zh-CN"),
  );
}
export function filterEntries(
  entries: Summary[],
  group: string | null,
  query: string,
  bodyMatches: Set<string> | null,
) {
  const q = query.trim().toLocaleLowerCase();
  return entries.filter(
    (e) =>
      matchesGroup(e, group) &&
      (!q ||
        (bodyMatches
          ? bodyMatches.has(e.key)
          : [e.key, e.description, groupText(e)].some((s) =>
              s.toLocaleLowerCase().includes(q),
            ))),
  );
}
