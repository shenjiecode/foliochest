import assert from "node:assert/strict";
import { deviceForms, DeviceState } from "../src/device";
const device: DeviceState = {
  auto_unlock: false,
  matches_vault: false,
  remembered_browser: false,
  cleanup_pending: false,
  provider: "Synthetic system provider",
  startup: { supported: true, enabled: false, current: false },
  startup_error: "",
  notice: "",
};
const defaults = deviceForms(device);
assert.doesNotMatch(defaults, /\bchecked\b/);
assert.match(defaults, /id="device-password" type="password"/);
assert.match(defaults, /id="current-password" type="password"/);
assert.match(defaults, /id="new-password" type="password"/);
assert.match(defaults, /旧备份仍使用旧密码/);
assert.match(defaults, /新浏览器仍需主密码/);
const unsafe = '<img src=x onerror="alert(1)">';
const output = deviceForms({
  ...device,
  provider: unsafe,
  notice: unsafe,
  startup_error: unsafe,
  cleanup_pending: true,
});
assert.doesNotMatch(output, /<img\b/);
assert.match(output, /&lt;img/);
assert.match(output, /id="device-cleanup"/);
console.log("Device settings defaults and escaping tests passed.");
