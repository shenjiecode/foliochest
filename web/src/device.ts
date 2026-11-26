import { API, esc, withBusy } from "./ui";
import "./device.css";
export type DeviceState = {
  auto_unlock: boolean;
  matches_vault: boolean;
  remembered_browser: boolean;
  cleanup_pending: boolean;
  provider: string;
  startup: { supported: boolean; enabled: boolean; current: boolean };
  startup_error: string;
  notice: string;
};
export function deviceForms(device: DeviceState): string {
  return `<section class="settings-card device-settings"><h2>启动与设备授权</h2><p class="small muted">这些选项只影响这台电脑，不随资料库同步。默认均关闭。</p>
 <form id="startup-form"><label class="device-check"><input id="startup-enabled" type="checkbox" ${device.startup.enabled ? "checked" : ""} ${device.startup.supported ? "" : "disabled"}>登录系统后启动本地服务</label><p class="small muted">不打开浏览器，也不代表自动解锁。移动程序后需重新保存启动项。</p>${device.startup_error ? `<p class="inline-warning">${esc(device.startup_error)}</p>` : device.startup.enabled && !device.startup.current ? '<p class="inline-warning">启动项与当前程序位置不一致，请重新保存。</p>' : ""}<button class="button secondary" ${device.startup.supported ? "" : "disabled"}>保存启动设置</button></form>
 <form id="device-unlock-form"><label class="device-check"><input id="device-unlock-enabled" type="checkbox" ${device.auto_unlock ? "checked" : ""}>信任这台电脑，服务启动时自动解锁</label><p class="small muted">凭据存储：${esc(device.provider)}。它持有库密钥，不保存主密码；能够使用当前系统账户的程序可能读取该凭据。</p>${device.auto_unlock && !device.matches_vault ? '<p class="inline-warning">已有授权与当前资料库不一致，请输入主密码重新授权，或关闭并清理。</p>' : ""}
 <div id="device-enrollment"><label class="device-check"><input id="device-remember" type="checkbox" ${device.remembered_browser ? "checked" : ""}>同时记住这个浏览器（90 天）</label><p class="small muted">仅这个浏览器可在后台自动解锁后进入网页。新的授权会替换之前记住的浏览器；新浏览器仍需主密码。</p><input type="text" name="username" autocomplete="username" value="FolioChest device" class="sr-only" tabindex="-1"><label for="device-password">再次输入主密码以授权</label><input id="device-password" type="password" autocomplete="current-password" maxlength="1024"></div>
 <p class="small muted">每次服务启动只尝试一次。本次运行中，手动或空闲锁定后不会自动解开；重启服务仍按此设置执行。系统凭据不可用时改用主密码。</p><button class="button secondary">保存设备授权</button>${device.cleanup_pending ? '<button id="device-cleanup" class="text-button danger" type="button">重试清理旧系统凭据</button>' : ""}${device.notice ? `<p class="inline-warning">${esc(device.notice)}</p>` : ""}</form></section>
 <section class="settings-card device-settings"><h2>变更主密码</h2><p class="small muted">建议先在「备份与恢复」下载一份备份，并暂停其他设备的写入。变更完成后会锁定资料库，撤销当前会话和审批，并关闭本机自动解锁。</p><p class="inline-warning">只重新加密库密钥，不重新加密历史正文。旧备份仍使用旧密码；已复制的库密钥不能靠改密码撤销。其他设备需要同步新配置并重新授权。</p><form id="password-form"><input type="text" name="username" autocomplete="username" value="FolioChest" class="sr-only" tabindex="-1"><label for="current-password">当前主密码</label><input id="current-password" type="password" autocomplete="current-password" required maxlength="1024"><label for="new-password">新主密码</label><input id="new-password" type="password" autocomplete="new-password" required minlength="12" maxlength="1024"><label for="repeat-password">再次输入新主密码</label><input id="repeat-password" type="password" autocomplete="new-password" required minlength="12" maxlength="1024"><button class="button primary">变更密码并锁定</button></form></section>`;
}
export function bindDevice(
  api: API,
  run: (action: () => Promise<void>) => Promise<void>,
  saved: (message: string) => Promise<void>,
  changed: (message: string) => Promise<void>,
): void {
  const startup = document.querySelector<HTMLFormElement>("#startup-form")!;
  const enrollment = document.querySelector<HTMLFormElement>(
    "#device-unlock-form",
  )!;
  const passwordForm =
    document.querySelector<HTMLFormElement>("#password-form")!;
  const enabled = enrollment.querySelector<HTMLInputElement>(
    "#device-unlock-enabled",
  )!;
  const password =
    enrollment.querySelector<HTMLInputElement>("#device-password")!;
  const remember =
    enrollment.querySelector<HTMLInputElement>("#device-remember")!;
  const toggle = () => {
    enrollment.querySelector<HTMLElement>("#device-enrollment")!.hidden =
      !enabled.checked;
    password.required = enabled.checked;
    password.disabled = !enabled.checked;
    if (!enabled.checked) password.value = "";
  };
  enabled.onchange = toggle;
  toggle();
  const busy = (form: HTMLFormElement, action: () => Promise<void>) =>
    withBusy(form, async () => {
      try {
        await action();
      } catch (error) {
        if (form === enrollment && form.isConnected)
          await saved("").catch(() => {});
        throw error;
      } finally {
        form
          .querySelectorAll<HTMLInputElement>('input[type="password"]')
          .forEach((input) => {
            input.value = "";
          });
      }
    });
  startup.onsubmit = (event) => {
    event.preventDefault();
    void run(() =>
      busy(startup, async () => {
        await api("/api/device/startup", {
          enabled:
            startup.querySelector<HTMLInputElement>("#startup-enabled")!
              .checked,
        });
        if (startup.isConnected) await saved("本机启动设置已保存。");
      }),
    );
  };
  enrollment.onsubmit = (event) => {
    event.preventDefault();
    void run(() =>
      busy(enrollment, async () => {
        const result = await api<{ warning?: string }>("/api/device/unlock", {
          enabled: enabled.checked,
          password: password.value,
          remember_browser: remember.checked,
        });
        if (enrollment.isConnected)
          await saved(
            result.warning ||
              "设备授权已保存。关闭自动解锁不会结束当前会话；需要时请点击锁定。",
          );
      }),
    );
  };
  const cleanup =
    enrollment.querySelector<HTMLButtonElement>("#device-cleanup");
  if (cleanup)
    cleanup.onclick = () =>
      void run(() =>
        busy(enrollment, async () => {
          const result = await api<{ warning?: string }>("/api/device/unlock", {
            enabled: false,
          });
          if (enrollment.isConnected)
            await saved(result.warning || "旧系统凭据已清理。");
        }),
      );
  passwordForm.onsubmit = (event) => {
    event.preventDefault();
    void run(() =>
      busy(passwordForm, async () => {
        const current =
          passwordForm.querySelector<HTMLInputElement>(
            "#current-password",
          )!.value;
        const next =
          passwordForm.querySelector<HTMLInputElement>("#new-password")!.value;
        if (
          next !==
          passwordForm.querySelector<HTMLInputElement>("#repeat-password")!
            .value
        )
          throw new Error("两次输入的新主密码不一致");
        if (current === next) throw new Error("新主密码不能与当前主密码相同");
        if (
          !confirm(
            "确认已保留需要的备份？继续会变更主密码、关闭本机自动解锁并锁定资料库；旧备份仍使用旧密码。",
          )
        )
          return;
        const result = await api<{ warning?: string }>("/api/vault/password", {
          current_password: current,
          new_password: next,
        });
        await changed(
          "主密码已变更，请使用新密码重新解锁。" + (result.warning || ""),
        );
      }),
    );
  };
}
