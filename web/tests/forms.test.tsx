import assert from "node:assert/strict";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { ClientContext } from "../src/lib/hooks";
import { SessionClient } from "../src/lib/client";
import { ConfirmProvider } from "../src/components/forms";
import { DeviceSettings } from "../src/features/settings-device";
import { Settings } from "../src/features/settings";
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
