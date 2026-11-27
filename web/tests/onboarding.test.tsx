import assert from "node:assert/strict";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { App } from "../src/app";
import { TooltipProvider } from "../src/components/ui/tooltip";
import { SessionClient } from "../src/lib/client";
import { state, json, deferred } from "./fixtures";
import type { AppState } from "../src/lib/types";

let server: AppState = {
  ...state,
  authenticated: false,
  vault: {
    ...state.vault,
    directory: "",
    exists: false,
    unlocked: false,
    entries: 0,
    format: 0,
  },
};
const requests: { path: string; body: any }[] = [];
let pick: () => Promise<Response> = async () => json({ cancelled: true });
const client = new SessionClient(async (path, options) => {
  const body = options?.body ? JSON.parse(String(options.body)) : undefined;
  requests.push({ path, body });
  if (path === "/api/state") return json(server);
  if (path === "/api/directory/pick") return pick();
  if (path === "/api/vault/create") {
    server = {
      ...server,
      authenticated: true,
      vault: {
        ...server.vault,
        directory: body.directory,
        exists: true,
        unlocked: true,
        format: 3,
      },
    };
    return json({ ok: true });
  }
  if (
    path === "/api/entries" ||
    path === "/api/groups" ||
    path === "/api/approvals"
  )
    return json([]);
  throw new Error("Unexpected onboarding request: " + path);
});
const root = createRoot(document.getElementById("app")!);
function button(name: string) {
  const scope = document.querySelector('[role="dialog"]') || document;
  const node = [...scope.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.getAttribute("aria-label") || b.textContent?.trim()) === name,
  );
  assert.ok(node, "Missing button " + name);
  return node;
}
async function click(name: string) {
  await act(async () => {
    button(name).click();
  });
}
async function fill(selector: string, value: string) {
  const input = document.querySelector<HTMLInputElement>(selector)!;
  assert.ok(input, selector);
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function submit() {
  await act(async () =>
    document
      .querySelector("#vault-flow")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
}
const directory = () =>
  document.querySelector<HTMLInputElement>("#vault-directory")!;
try {
  await act(async () =>
    root.render(
      <TooltipProvider>
        <App client={client} />
      </TooltipProvider>,
    ),
  );
  assert.equal(document.querySelector("h1")?.textContent, "开始使用资料匣");
  assert.equal(document.querySelector(".unlock-vault, .lock-caption"), null);
  assert.ok(button("接入已有资料库"));
  assert.ok(button("从备份恢复"));
  await click("设置主密码并开始");
  assert.equal(directory().value, "");
  assert.equal(directory().readOnly, false);
  await fill("#vault-directory", "/synthetic/manual");
  await click("选择文件夹");
  assert.equal(
    directory().value,
    "/synthetic/manual",
    "cancellation must preserve typed path",
  );
  assert.equal(client.getSnapshot().state?.vault.directory, "");
  const selection = deferred<Response>();
  pick = () => selection.promise;
  const before = requests.filter(
    (r) => r.path === "/api/directory/pick",
  ).length;
  const choose = button("选择文件夹");
  await act(async () => {
    choose.click();
    choose.click();
  });
  assert.equal(
    requests.filter((r) => r.path === "/api/directory/pick").length,
    before + 1,
  );
  assert.equal(button("下一步").disabled, true);
  assert.equal(button("取消").disabled, true);
  await act(async () =>
    selection.resolve(
      json({ cancelled: false, directory: "/synthetic/selected" }),
    ),
  );
  assert.equal(directory().value, "/synthetic/selected");
  await submit();
  assert.ok(document.querySelector("#vault-password"));
  assert.equal(document.querySelector("#vault-directory"), null);
  await fill("#vault-password", "synthetic-onboarding-password");
  await fill("#vault-repeat", "synthetic-onboarding-password");
  await click("上一步");
  assert.equal(directory().value, "/synthetic/selected");
  await click("取消");
  assert.equal(document.querySelector('[role="dialog"]'), null);
  assert.equal(
    requests.filter(
      (r) => r.path === "/api/vault/create" || r.path === "/api/vault/select",
    ).length,
    0,
    "selecting/going back/cancelling must not switch or persist a vault",
  );
  await click("接入已有资料库");
  pick = async () =>
    json({ error: "无法打开系统文件夹选择器，请手动输入绝对路径" }, 503);
  await fill("#vault-directory", "/synthetic/kept");
  await click("选择文件夹");
  assert.equal(directory().value, "/synthetic/kept");
  assert.ok(document.body.textContent?.includes("无法打开系统文件夹选择器"));
  assert.equal(button("选择文件夹").disabled, false);
  await click("取消");
  await click("从备份恢复");
  assert.equal(
    directory().value,
    "",
    "restore must not invent a '-restored' path",
  );
  assert.ok(button("选择文件夹"));
  await click("取消");
  // Expiring the page session must discard a late native-dialog result.
  await click("设置主密码并开始");
  const late = deferred<Response>();
  pick = () => late.promise;
  await click("选择文件夹");
  server = { ...server, csrf: "synthetic-renewed-session" };
  await act(async () => client.refresh());
  assert.equal(document.querySelector('[role="dialog"]'), null);
  await act(async () =>
    late.resolve(json({ cancelled: false, directory: "/synthetic/stale" })),
  );
  await click("设置主密码并开始");
  assert.equal(directory().value, "");
  await fill("#vault-directory", "/synthetic/new-vault");
  await submit();
  assert.equal(
    document.querySelector<HTMLInputElement>("#vault-password")?.value,
    "",
    "back/cancel cleared password",
  );
  await fill("#vault-password", "synthetic-onboarding-password");
  await fill("#vault-repeat", "synthetic-onboarding-password");
  await submit();
  const creation = requests.filter((r) => r.path === "/api/vault/create");
  assert.deepEqual(
    creation.map((r) => r.body),
    [
      {
        directory: "/synthetic/new-vault",
        password: "synthetic-onboarding-password",
      },
    ],
  );
  assert.equal(document.querySelector("#vault-password"), null);
  assert.equal(client.getSnapshot().state?.authenticated, true);
  // A remembered missing library is not an empty first launch or a create form.
  server = {
    ...server,
    authenticated: false,
    vault: {
      ...server.vault,
      directory: "/synthetic/missing-vault",
      exists: false,
      unlocked: false,
    },
  };
  await act(async () => client.refresh());
  assert.equal(document.querySelector("h1")?.textContent, "找不到资料文件");
  assert.ok(
    document
      .querySelector(".lock-caption")
      ?.textContent?.includes("/synthetic/missing-vault"),
  );
  assert.equal(document.querySelector("#vault-password"), null);
  assert.equal(document.body.textContent?.includes("设置主密码"), false);
  assert.equal(document.body.textContent?.includes("创建新资料库"), false);
  await click("重新定位原资料库");
  assert.equal(
    directory().value,
    "",
    "must not suggest overwriting missing library",
  );
} finally {
  await act(async () => root.unmount());
}
console.log(
  "Initial empty state, two-step creation, native picker busy/cancel/error, stale results and missing-library tests passed.",
);
