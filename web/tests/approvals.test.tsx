import assert from "node:assert/strict";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { TooltipProvider } from "../src/components/ui/tooltip";
import { App } from "../src/app";
import { SessionClient } from "../src/lib/client";
import type { Pending } from "../src/lib/types";
import { state, json, deferred } from "./fixtures";

let server = structuredClone(state);
let pending: Pending[] = [];
let failApprovals = false;
let delayed: ReturnType<typeof deferred<Response>> | null = null;
let decisions = 0;
const request: Pending = {
  id: "synthetic-pending", client_name: "Synthetic stdio", operation: "read",
  expires_at: new Date(Date.now() + 300000).toISOString(),
  reads: [{ key: "synthetic/key", include_value: true }],
};
const client = new SessionClient(async (path, options) => {
  if (path === "/api/state") return json(server);
  // Catalog failure must never suppress approvals, including initial loading.
  if (path === "/api/entries" || path === "/api/groups") return json({ error: "Synthetic catalog failure" }, 500);
  if (path === "/api/approvals") {
    if (delayed) { const response = delayed; delayed = null; return response.promise; }
    if (failApprovals) return json({ error: "Synthetic approval outage" }, 503);
    return json(pending);
  }
  if (path === "/api/approvals/decide") {
    decisions++;
    assert.deepEqual(JSON.parse(String(options?.body)), { id: request.id, accept: false });
    pending = [];
    return json({ ok: true });
  }
  throw new Error("Unexpected approval test request " + path);
});
const root = createRoot(document.getElementById("app")!);
const text = () => document.body.textContent || "";
function button(label: string) {
  const match = [...document.querySelectorAll<HTMLButtonElement>("button")].find((node) => (node.getAttribute("aria-label") || node.textContent?.trim()) === label);
  assert.ok(match, "Missing " + label);
  return match;
}
async function click(label: string) { await act(async () => button(label).click()); }
async function tick() { await act(async () => { await (globalThis as any).tickIntervals(); }); }
try {
  history.replaceState(null, "", "/#approvals");
  await act(async () => root.render(<TooltipProvider><App client={client} /></TooltipProvider>));
  assert.ok(text().includes("没有待处理的授权请求"));
  assert.ok(text().includes("Synthetic catalog failure"));
  assert.equal(document.querySelector(".approval-notice"), null);
  pending = [request];
  await tick();
  assert.ok(button("1 个待处理授权"));
  assert.ok(text().includes("synthetic/key"));
  assert.equal(decisions, 0);
  await click("设置");
  assert.ok(button("1 个待处理授权"));
  await click("1 个待处理授权");
  assert.equal(location.hash, "#approvals");
  assert.ok(button("1 个待处理授权"));
  assert.equal(decisions, 0, "opening the approval page must not decide or clear requests");
  failApprovals = true;
  await tick();
  assert.ok(button("1 个待处理授权（状态待确认）"));
  assert.ok(text().includes("上次成功读取"));
  failApprovals = false;
  await click("刷新审批状态");
  assert.ok(button("1 个待处理授权"));
  await click("拒绝");
  assert.equal(decisions, 1);
  assert.equal(location.hash, "#approvals");
  assert.equal(document.querySelector(".approval-notice"), null);
  assert.ok(text().includes("没有待处理的授权请求"));
  assert.ok(text().includes("5 分钟"));
  failApprovals = true;
  await tick();
  assert.ok(text().includes("暂时无法确认待审批状态"));
  assert.ok(!text().includes("没有待处理的授权请求"));
  failApprovals = false;
  const late = deferred<Response>(); delayed = late;
  await tick();
  server = { ...server, authenticated: false };
  await act(async () => client.refresh());
  assert.ok(text().includes("解锁资料匣"));
  server = { ...server, authenticated: true };
  await act(async () => client.refresh());
  await act(async () => late.resolve(json([request])));
  assert.equal(document.querySelector(".approval-notice"), null, "late old-session response restored requests");
} finally {
  await act(async () => root.unmount());
  history.replaceState(null, "", "/");
}
console.log("Independent approval polling, persistent badges, dedicated page, outage and stale-session guards passed.");
