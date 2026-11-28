import assert from "node:assert/strict";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { ClientContext } from "../src/lib/hooks";
import { SessionClient } from "../src/lib/client";
import { ConfirmProvider } from "../src/components/forms";
import { ExitContext } from "../src/components/exit-app";
import { DeviceSettings } from "../src/features/settings-device";
import { Settings } from "../src/features/settings";
import { UpdateSettings } from "../src/features/settings-update";
import { MCPSettings } from "../src/features/settings-mcp";
import { GroupManagement } from "../src/features/management";
import { validatePassword } from "../src/features/unlock";
import { state, json } from "./fixtures";
import type { Device, Group } from "../src/lib/types";

assert.throws(() => validatePassword("short", "short"), /至少/);
validatePassword("字".repeat(12), "字".repeat(12));
assert.throws(
  () => validatePassword("字".repeat(342), "字".repeat(342)),
  /1024/,
);
assert.throws(() => validatePassword("a".repeat(12), "b".repeat(12)), /不一致/);
const host = document.getElementById("app")!,
  root = createRoot(host);
function find(name: string): HTMLButtonElement {
  const button = [
    ...document.querySelectorAll<HTMLButtonElement>("button"),
  ].find(
    (b) => (b.getAttribute("aria-label") || b.textContent?.trim()) === name,
  );
  assert.ok(button, "Missing " + name);
  return button;
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
  const node = document.querySelector<HTMLInputElement>(selector)!;
  assert.ok(node, selector);
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(node, value);
    node.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function submit(selector: string) {
  await act(async () => {
    document
      .querySelector(selector)!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}
let device: Device = {
  auto_unlock: true,
  matches_vault: true,
  remembered_browser: true,
  cleanup_pending: false,
  provider: "Synthetic credentials",
  startup: { supported: true, enabled: false, current: true },
  startup_error: "",
  notice: "",
};
let readCount = 0,
  action: any;
const client = new SessionClient(async (path, options) => {
  if (path === "/api/state") return json(state);
  if (path === "/api/device") {
    readCount++;
    return json(device);
  }
  if (path === "/api/device/unlock") {
    action = JSON.parse(String(options?.body));
    device = {
      ...device,
      auto_unlock: false,
      remembered_browser: false,
      cleanup_pending: true,
    };
    return json({ error: "SYNTHETIC_CREDENTIAL_FAILURE" }, 500);
  }
  throw new Error("Unexpected device route " + path);
});
try {
  await client.refresh();
  await act(async () =>
    root.render(
      <ClientContext value={client}>
        <ConfirmProvider>
          <DeviceSettings />
        </ConfirmProvider>
      </ClientContext>,
    ),
  );
  await fill("#device-password", "synthetic-device-password");
  await submit("form:has(#device-password)");
  assert.equal(action.enabled, true);
  assert.equal(action.password, "synthetic-device-password");
  assert.equal(readCount, 2);
  assert.ok(
    document.body.textContent?.includes("SYNTHETIC_CREDENTIAL_FAILURE"),
  );
  assert.equal(find("自动解锁").getAttribute("aria-checked"), "false");
  assert.equal(document.querySelector("#device-password"), null);
  assert.ok(find("重试清理旧凭据"));
  assert.equal(find("保存设备授权").disabled, false);
  // Update checks are explicit, never triggered by mounting the settings page.
  let checks = 0;
  const updateClient = new SessionClient(async (path, options) => {
    if (path === "/api/state") return json(state);
    if (path === "/api/app/check-update") {
      checks++;
      assert.deepEqual(JSON.parse(String(options?.body)), { confirmed: true });
      if (checks > 1) return json({ error: "GitHub 请求受限" }, 502);
      return json({ latest: "0.6.3", status: "available", url: "https://github.com/shenjiecode/foliochest/releases/tag/v0.6.3" });
    }
    throw new Error("Unexpected update route");
  });
  await updateClient.refresh();
  await act(async () => root.render(
    <ClientContext value={updateClient}><UpdateSettings version="0.6.2" /></ClientContext>,
  ));
  assert.equal(checks, 0);
  await click(find("检查更新"));
  assert.equal(checks, 1);
  assert.ok(document.body.textContent?.includes("发现新版本 0.6.3"));
  assert.equal(document.querySelector("a")?.getAttribute("href"), "https://github.com/shenjiecode/foliochest/releases/tag/v0.6.3");
  assert.equal(document.querySelector("a")?.getAttribute("rel"), "noopener noreferrer");
  await click(find("检查更新"));
  assert.ok(document.body.textContent?.includes("GitHub 请求受限"));
  assert.equal(document.querySelector("a")?.getAttribute("href"), "https://github.com/shenjiecode/foliochest/releases/latest");
  // Download and installer handoff each require a separate human confirmation.
  let downloads = 0, installs = 0, exitMessage = "";
  const guidedClient = new SessionClient(async (path, options) => {
    if (path === "/api/state") return json(state);
    const body = JSON.parse(String(options?.body));
    assert.equal(body.confirmed, true);
    if (path === "/api/app/check-update") return json({ latest: "0.6.3", status: "available", url: "https://github.com/shenjiecode/foliochest/releases/tag/v0.6.3", installer: "synthetic.rpm", size: 1024, notes: "Synthetic release notes" });
    if (path === "/api/app/download-update") {
      downloads++;
      assert.equal(body.version, "0.6.3");
      return json({ id: "synthetic-ready", version: "0.6.3", name: "synthetic.rpm", size: 1024, path: "/synthetic/update.rpm" });
    }
    if (path === "/api/app/install-update") {
      installs++;
      assert.equal(body.id, "synthetic-ready");
      return json({ ok: true });
    }
    throw new Error("Unexpected update route " + path);
  });
  await guidedClient.refresh();
  await act(async () => root.render(
    <ClientContext value={guidedClient}><ConfirmProvider><ExitContext value={(message) => { exitMessage = message || ""; }}><UpdateSettings key="guided" version="0.6.2" /></ExitContext></ConfirmProvider></ClientContext>,
  ));
  assert.equal(downloads, 0);
  await click(find("检查更新"));
  await click(find("下载安装包"));
  await click(find("取消"));
  assert.equal(downloads, 0);
  await click(find("下载安装包"));
  await click(find("下载并校验"));
  assert.equal(downloads, 1);
  assert.equal(installs, 0);
  assert.ok(document.body.textContent?.includes("通过 SHA-256 校验"));
  await click(find("退出并更新"));
  await click(find("取消"));
  assert.equal(installs, 0);
  await click(find("退出并更新"));
  // The confirmation and underlying trigger share a label; choose the dialog action.
  const installerConfirmation = [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find((b) => b.textContent?.trim() === "退出并更新")!;
  await click(installerConfirmation);
  assert.equal(installs, 1);
  assert.ok(exitMessage.includes("已启动系统安装器"));
  assert.ok(exitMessage.includes("/synthetic/update.rpm"));
  // Default stdio does not fetch or expose HTTP credentials.
  let modeBody: any;
  const modeClient = new SessionClient(async (path, options) => {
    if (path === "/api/state") return json(state);
    if (path === "/api/mcp/settings") {
      if (options?.method === "POST") {
        modeBody = JSON.parse(String(options.body));
        return json({});
      }
      return json({ mode: "stdio", command: "/synthetic/foliochest", args: ["mcp", "--device-dir", "/synthetic/device"], url: "http://127.0.0.1:7463/mcp" });
    }
    throw new Error("stdio must not fetch HTTP client credentials: " + path);
  });
  await modeClient.refresh();
  await act(async () => root.render(
    <ClientContext value={modeClient}><ConfirmProvider><MCPSettings /></ConfirmProvider></ClientContext>,
  ));
  assert.ok(document.body.textContent?.includes("复制 stdio 配置"));
  assert.equal(document.querySelector("#client-name"), null);
  assert.equal(modeBody, undefined);
  await click(find("HTTP（高级）"));
  assert.equal(modeBody, undefined);
  await click(find("确认"));
  assert.deepEqual(modeBody, { mode: "http", confirmed: true });
  // Changing the source props after opening does not change the reviewed members.
  const source: Group = {
    name: "工作",
    count: 1,
    deleted_count: 1,
    blocked: 0,
    members: [
      { key: "normal", expected_revision: "a" },
      { key: "trash", expected_revision: "b" },
    ],
  };
  let groupBody: any;
  const groupClient = new SessionClient(async (path, options) => {
    if (path === "/api/state") return json(state);
    if (path === "/api/groups/edit") {
      groupBody = JSON.parse(String(options?.body));
      return json([]);
    }
    throw new Error("Unexpected group route");
  });
  await groupClient.refresh();
  const render = (group: Group) => (
    <ClientContext value={groupClient}>
      <ConfirmProvider>
        <GroupManagement
          source={group}
          members={[]}
          groups={[group]}
          format={3}
          onClose={() => {}}
          onChanged={() => {}}
        />
      </ConfirmProvider>
    </ClientContext>
  );
  await act(async () => root.render(render(source)));
  await fill("#target-group", "新组");
  await act(async () =>
    root.render(
      render({
        ...source,
        count: 2,
        members: [
          ...source.members,
          { key: "new-offline-member", expected_revision: "c" },
        ],
      }),
    ),
  );
  await submit("#group-form");
  await click(find("确认"));
  assert.deepEqual(groupBody.expected, source.members);
  assert.equal(groupBody.target, "新组");
  // Explicit upgrade must include target format and human confirmation.
  let upgradeBody: any,
    server = { ...state, vault: { ...state.vault, format: 2 } };
  const upgradeClient = new SessionClient(async (path, options) => {
    if (path === "/api/state") return json(server);
    if (path === "/api/vault/upgrade") {
      upgradeBody = JSON.parse(String(options?.body));
      server = { ...server, authenticated: false };
      return json({ warning: "" });
    }
    throw new Error("Unexpected upgrade route");
  });
  await upgradeClient.refresh();
  await act(async () =>
    root.render(
      <ClientContext value={upgradeClient}>
        <ConfirmProvider>
          <Settings state={server} onOpen={() => {}} />
        </ConfirmProvider>
      </ClientContext>,
    ),
  );
  await click(find("升级为格式 3"));
  await fill("#current-password", "synthetic-upgrade-password");
  await submit("#password-change");
  assert.equal(upgradeBody, undefined);
  await click(
    document.querySelector<HTMLInputElement>(
      '#password-change input[type="checkbox"]',
    )!,
  );
  assert.equal(
    document.querySelector<HTMLInputElement>("#current-password")?.value,
    "",
  );
  await fill("#current-password", "synthetic-upgrade-password");
  await submit("#password-change");
  assert.deepEqual(upgradeBody, {
    password: "synthetic-upgrade-password",
    confirmed: true,
    target_format: 3,
  });
  assert.equal(
    document.querySelector<HTMLInputElement>("#current-password")?.value,
    "",
  );
  assert.equal(upgradeClient.getSnapshot().state?.authenticated, false);
} finally {
  await act(async () => root.unmount());
}
console.log(
  "Device failure readback/password clearing, full reviewed group snapshots and explicit upgrade tests passed.",
);
