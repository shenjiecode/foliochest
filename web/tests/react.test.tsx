import assert from "node:assert/strict";
import { act, useLayoutEffect } from "react";
import { createRoot } from "react-dom/client";
import { EditorView } from "@codemirror/view";
import { setNonce } from "get-nonce";
import { TooltipProvider } from "../src/components/ui/tooltip";
import { ConfirmProvider } from "../src/components/forms";
import { ClientContext, useResource } from "../src/lib/hooks";
import { SessionClient } from "../src/lib/client";
import { App } from "../src/app";
import { HistoryDialog, historyRules } from "../src/features/history";
import { useBodySearch } from "../src/lib/catalog";
import { state, entry, summary, json, deferred } from "./fixtures";
import type { Entry, History, Client } from "../src/lib/types";

setNonce("c".repeat(48));
let server = structuredClone(state);
const docs = new Map<string, Entry>([
  [entry.key, structuredClone(entry)],
  [
    "second",
    {
      ...entry,
      key: "second",
      value: "SYNTHETIC_SECOND",
      revision: "d".repeat(32),
    },
  ],
]);
let clients: Client[] = [],
  saves = 0,
  delayedEntry: ReturnType<typeof deferred<Response>> | null = null;
const requests: { path: string; body: any }[] = [];
const transport: typeof fetch = async (path, options) => {
  const url = new URL(String(path), location.origin),
    body = options?.body ? JSON.parse(String(options.body)) : undefined;
  requests.push({ path: url.pathname, body });
  switch (url.pathname) {
    case "/api/state":
      return json(server);
    case "/api/entries":
      return json(
        [...docs.values()].map((e) => ({ ...e, state: "ready", versions: 1 })),
      );
    case "/api/groups":
      return json([
        {
          name: "工作",
          count: 2,
          deleted_count: 0,
          blocked: 0,
          members: [...docs.values()].map((e) => ({
            key: e.key,
            expected_revision: e.revision,
          })),
        },
      ]);
    case "/api/approvals":
      return json([]);
    case "/api/entry":
      if (delayedEntry) {
        const pending = delayedEntry;
        delayedEntry = null;
        return pending.promise;
      }
      return json(docs.get(url.searchParams.get("key")!));
    case "/api/entries/save": {
      saves++;
      const change = body.changes[0],
        before = docs.get(change.key);
      if (before && before.revision !== change.expected_revision)
        return json({ error: "旧版本，拒绝覆盖" }, 409);
      docs.set(change.key, { ...entry, ...change, revision: "e".repeat(32) });
      return json([]);
    }
    case "/api/settings":
      return json({ idle_lock_minutes: 0, lock_on_system_events: true });
    case "/api/clients":
      return json(clients);
    case "/api/clients/create": {
      const item = {
        id: "test-client",
        name: body.name,
        created_at: "2026-01-01T00:00:00Z",
      };
      clients.push(item);
      return json({
        client: item,
        token: "SYNTHETIC_ONE_TIME_TOKEN",
        url: location.origin + "/mcp",
      });
    }
    case "/api/vault/lock":
      server = { ...server, authenticated: false };
      return json({ ok: true });
    default:
      throw new Error("Unexpected route " + url.pathname);
  }
};
const client = new SessionClient(transport),
  container = document.getElementById("app")!,
  root = createRoot(container);
const text = () => document.body.textContent || "";
function button(name: string, scope: ParentNode = document): HTMLButtonElement {
  const item = [...scope.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.getAttribute("aria-label") || b.textContent?.trim()) === name,
  );
  assert.ok(item, "Missing button: " + name);
  return item;
}
async function click(node: HTMLElement) {
  await act(async () => {
    node.dispatchEvent(
      new MouseEvent("mousedown", { bubbles: true, button: 0 }),
    );
    node.click();
  });
}
async function fill(selector: string, value: string) {
  const node = document.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    selector,
  );
  assert.ok(node, selector);
  const proto =
    node instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(node, value);
    node.dispatchEvent(new Event("input", { bubbles: true }));
    node.dispatchEvent(new Event("change", { bubbles: true }));
  });
}
function bodyEditor(): EditorView | null {
  const node = document.querySelector<HTMLElement>(
    '.cm-content[aria-label="正文"]',
  );
  return node ? EditorView.findFromDOM(node) : null;
}
async function fillBody(value: string) {
  const editor = bodyEditor();
  assert.ok(editor, "Missing body CodeMirror editor");
  await act(async () => {
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: value },
    });
  });
}
async function submit(selector: string) {
  await act(async () => {
    document
      .querySelector(selector)!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}
async function tick() {
  await act(async () => {
    await (globalThis as any).tickIntervals();
  });
}
try {
  history.replaceState(null, "", "/?approval=1&screen=unlock");
  await act(async () =>
    root.render(
      <TooltipProvider>
        <App client={client} />
      </TooltipProvider>,
    ),
  );
  assert.ok(text().includes("全部资料"));
  assert.ok(!text().includes("示例"));
  assert.equal(document.querySelector(".approval-notice"), null);
  // Selection and file lifetimes are independent of list polling and session polling.
  const first = [
    ...document.querySelectorAll<HTMLButtonElement>(".entry-open"),
  ].find((b) => b.textContent?.includes(entry.key))!;
  await click(first);
  assert.ok(text().includes(entry.value));
  await click(button("编辑"));
  await fillBody("SYNTHETIC_UNSAVED_DRAFT");
  docs.set(entry.key, {
    ...entry,
    revision: "f".repeat(32),
    value: "SYNTHETIC_EXTERNAL_CHANGE",
  });
  await tick();
  assert.equal(
    bodyEditor()?.state.doc.toString(),
    "SYNTHETIC_UNSAVED_DRAFT",
  );
  assert.ok(text().includes("未保存的草稿已保留"));
  await act(async () => {
    const form = document.querySelector("form.entry-panel")!;
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
  });
  assert.equal(saves, 1);
  assert.equal(
    requests.find((r) => r.path === "/api/entries/save")!.body.changes[0]
      .expected_revision,
    entry.revision,
  );
  assert.ok(text().includes("旧版本，拒绝覆盖"));
  await click(button("设置"));
  assert.ok(text().includes("放弃未保存的修改"));
  await click(button("取消", document.querySelector('[role="dialog"]')!));
  assert.equal(
    bodyEditor()?.state.doc.toString(),
    "SYNTHETIC_UNSAVED_DRAFT",
  );
  await click(button("设置"));
  await click(button("放弃修改"));
  assert.equal(bodyEditor(), null);
  await click(button("MCP"));
  await fill("#client-name", "Synthetic harness");
  await submit(
    ".client-create".replace(".client-create", "form:has(.client-create)"),
  );
  assert.ok(text().includes("SYNTHETIC_ONE_TIME_TOKEN"));
  server = { ...server, authenticated: false };
  await act(async () => client.refresh());
  assert.ok(!text().includes("SYNTHETIC_ONE_TIME_TOKEN"));
  assert.ok(!text().includes("SYNTHETIC_UNSAVED_DRAFT"));
  assert.ok(text().includes("解锁资料库"));
  // Authenticate again and finish a file read after the session has disappeared.
  server = { ...server, authenticated: true };
  await act(async () => client.refresh());
  await click(
    [...document.querySelectorAll<HTMLButtonElement>(".entry-open")].find((b) =>
      b.textContent?.includes(entry.key),
    )!,
  );
  await click(button("编辑"));
  const bytes = Uint8Array.of(1, 2, 3),
    fileRead = deferred<ArrayBuffer>();
  const upload =
    document.querySelector<HTMLInputElement>('input[type="file"]')!;
  assert.ok(upload, "Missing inline attachment upload");
  Object.defineProperty(upload, "files", {
    configurable: true,
    value: [
      {
        name: "late.txt",
        size: 3,
        type: "text/plain",
        arrayBuffer: () => fileRead.promise,
      },
    ],
  });
  await act(async () => {
    upload.dispatchEvent(new Event("change", { bubbles: true }));
  });
  server = { ...server, authenticated: false };
  await act(async () => client.refresh());
  await act(async () => {
    fileRead.resolve(bytes.buffer);
  });
  assert.ok(bytes.every((b) => b === 0));
  assert.ok(!text().includes("late.txt"));
  assert.equal(saves, 1);
  // A late entry response may not replace a newly selected document.
  server = { ...server, authenticated: true };
  await act(async () => client.refresh());
  const oldEntry = deferred<Response>();
  delayedEntry = oldEntry;
  await click(
    [...document.querySelectorAll<HTMLButtonElement>(".entry-open")].find((b) =>
      b.textContent?.includes(entry.key),
    )!,
  );
  await click(
    [...document.querySelectorAll<HTMLButtonElement>(".entry-open")].find((b) =>
      b.textContent?.includes("second"),
    )!,
  );
  assert.ok(text().includes("SYNTHETIC_SECOND"));
  await act(async () => {
    oldEntry.resolve(json({ ...entry, value: "STALE_ENTRY_MUST_NOT_RENDER" }));
  });
  assert.ok(text().includes("SYNTHETIC_SECOND"));
  assert.ok(!text().includes("STALE_ENTRY_MUST_NOT_RENDER"));
  // CSRF renewal clears an authenticated surface too, not just logout.
  await click(button("编辑"));
  await fillBody("DRAFT_BEFORE_CSRF_RENEWAL");
  server = { ...server, csrf: "renewed-csrf" };
  await act(async () => client.refresh());
  assert.equal(bodyEditor(), null);
  assert.ok(!text().includes("DRAFT_BEFORE_CSRF_RENEWAL"));
} finally {
  await act(async () => root.unmount());
}

// Historical restoration binds the reviewed heads, preserves current group/name,
// and never silently imports historical permissions.
const host = document.createElement("div");
document.body.append(host);
const historyRoot = createRoot(host);
const historical = {
  ...entry,
  revision: "1".repeat(32),
  group: "旧组",
  value: "SYNTHETIC_OLD_BODY",
  read_policy: "allow" as const,
};
const historyData: History = {
  key: entry.key,
  state: "conflict",
  heads: ["2".repeat(32), "3".repeat(32)],
  versions: [
    {
      ...entry,
      revision: "2".repeat(32),
      group: "当前组",
      head: true,
      available: true,
    },
    {
      ...entry,
      revision: "3".repeat(32),
      group: "另一组",
      head: true,
      available: true,
    },
    { ...historical, head: false, available: true },
  ],
};
assert.equal(historyRules(historyData).groupConflict, true);
assert.ok(
  historyRules({
    ...historyData,
    versions: [
      ...historyData.versions,
      { ...historyData.versions[0], entry_id: "another-id" },
    ],
  }).blocked,
);
let restore: any,
  changed = "";
const historyClient = new SessionClient(async (path, options) => {
  if (String(path) === "/api/state") return json(state);
  if (String(path).startsWith("/api/history?")) return json(historyData);
  if (String(path).startsWith("/api/history/entry?")) return json(historical);
  if (String(path) === "/api/history/restore") {
    restore = JSON.parse(String(options?.body));
    return json({ ok: true });
  }
  throw new Error("Unexpected history request");
});
try {
  await historyClient.refresh();
  await act(async () =>
    historyRoot.render(
      <ClientContext value={historyClient}>
        <ConfirmProvider>
          <TooltipProvider>
            <HistoryDialog
              entryKey={entry.key}
              format={3}
              groups={[]}
              onClose={() => {}}
              onChanged={(key) => {
                changed = key;
              }}
            />
          </TooltipProvider>
        </ConfirmProvider>
      </ClientContext>,
    ),
  );
  assert.equal(
    document.querySelector<HTMLInputElement>("#restore-key")?.value,
    entry.key,
  );
  assert.equal(
    document.querySelector<HTMLInputElement>("#restore-group")?.value,
    "",
  );
  await submit("form:has(#restore-value)");
  assert.equal(restore, undefined);
  assert.ok(text().includes("请明确确认恢复后的分组"));
  await fill("#restore-group", "明确选择组");
  await click(
    document.querySelector<HTMLInputElement>(
      'form:has(#restore-value) input[type="checkbox"]',
    )!,
  );
  await submit("form:has(#restore-value)");
  await click(
    button("确认", [...document.querySelectorAll('[role="dialog"]')].at(-1)!),
  );
  assert.deepEqual(restore.expected_heads, historyData.heads);
  assert.equal(restore.source_revision, historical.revision);
  assert.equal(restore.group, "明确选择组");
  assert.equal(restore.target_key, entry.key);
  assert.equal(restore.read_policy, undefined);
  assert.equal(changed, entry.key);
  assert.ok(
    [...document.querySelectorAll("style")].every(
      (node) => node.nonce === "c".repeat(48),
    ),
  );
} finally {
  await act(async () => historyRoot.unmount());
  host.remove();
}

// Body search has its own lifetime and discards out-of-order results.
const searchHost = document.createElement("div");
document.body.append(searchHost);
const searchRoot = createRoot(searchHost);
const searches = [deferred<Response>(), deferred<Response>()];
let index = 0;
const searchClient = new SessionClient(() => searches[index++].promise);
function Probe({ query }: { query: string }) {
  const result = useBodySearch(query, true, 0);
  return <output>{[...(result.matches || [])].join(",")}</output>;
}
const renderSearch = async (query: string) => {
  await act(async () =>
    searchRoot.render(
      <ClientContext value={searchClient}>
        <Probe query={query} />
      </ClientContext>,
    ),
  );
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 280));
  });
};
try {
  await renderSearch("old");
  await renderSearch("new");
  await act(async () =>
    searches[1].resolve(json([{ ...summary, key: "LATEST_SEARCH" }])),
  );
  await act(async () =>
    searches[0].resolve(json([{ ...summary, key: "STALE_SEARCH" }])),
  );
  assert.equal(searchHost.textContent, "LATEST_SEARCH");
} finally {
  await act(async () => searchRoot.unmount());
  searchHost.remove();
}
// Switching history paths must hide old data in the first commit, before the
// passive fetch effect runs. Input must not land in the outgoing document.
const resourceHost = document.createElement("div");
document.body.append(resourceHost);
const resourceRoot = createRoot(resourceHost);
const pendingResource = deferred<Response>();
const resourceClient = new SessionClient(async (path) =>
  String(path) === "/api/first"
    ? json({ value: "FIRST_PRIVATE" })
    : pendingResource.promise,
);
const observed: Array<string | null> = [];
function ResourceProbe({ path }: { path: string }) {
  const resource = useResource<{ value: string }>(path);
  useLayoutEffect(() => {
    observed.push(resource.data?.value ?? null);
  }, [path]);
  return <output>{resource.data?.value}</output>;
}
try {
  await act(async () =>
    resourceRoot.render(
      <ClientContext value={resourceClient}>
        <ResourceProbe path="/api/first" />
      </ClientContext>,
    ),
  );
  assert.equal(resourceHost.textContent, "FIRST_PRIVATE");
  await act(async () =>
    resourceRoot.render(
      <ClientContext value={resourceClient}>
        <ResourceProbe path="/api/second" />
      </ClientContext>,
    ),
  );
  assert.equal(observed.at(-1), null);
  assert.equal(resourceHost.textContent, "");
  await act(async () =>
    pendingResource.resolve(json({ value: "SECOND_PRIVATE" })),
  );
  assert.equal(resourceHost.textContent, "SECOND_PRIVATE");
} finally {
  await act(async () => resourceRoot.unmount());
  resourceHost.remove();
}
console.log(
  "React API flows, draft/version guards, token/CSRF cleanup, late files/entries/searches, history and resource-identity tests passed.",
);
