import { API, esc } from "./ui";

type Context = {
  api: API;
  run: (action: () => Promise<void>) => Promise<void>;
  notice: (message: string, error?: boolean) => void;
  isDirty: () => boolean;
  onChanged: (key: string, deleted: boolean) => Promise<void>;
};
export function managementForms(key: string, format: number): string {
  if (format < 2)
    return '<p class="inline-warning">删除和重命名需要资料库格式 2。请先在「连接与设置」中备份并确认升级。</p>';
  return `<div class="entry-management"><details><summary>重命名 key</summary><form id="rename-form"><label for="rename-key">新 key（正文、附件和权限不变）</label><input id="rename-key" maxlength="160" required spellcheck="false" value="${esc(key)}"><button class="button secondary" type="submit">确认重命名</button></form></details><button id="delete-entry" class="text-button danger" type="button">移到回收站</button><p class="small muted">删除会保留加密历史和附件，不是安全擦除，也不会释放磁盘空间。</p></div>`;
}
export function bindManagement(
  host: HTMLElement,
  key: string,
  revision: string,
  ctx: Context,
): void {
  const form = host.querySelector<HTMLFormElement>("#rename-form");
  if (!form) return;
  const controls = () =>
    host.querySelectorAll<HTMLInputElement | HTMLButtonElement>(
      "input,button,select,textarea",
    );
  const change = async (deleted: boolean) => {
    if (ctx.isDirty())
      throw new Error("请先保存或放弃正文草稿，再进行删除或重命名。");
    const target = form.querySelector<HTMLInputElement>("#rename-key")!.value;
    if (!deleted && target === key) throw new Error("新 key 与当前名称相同");
    if (
      !confirm(
        deleted
          ? `将「${key}」移到回收站？历史与附件仍保留，可以恢复；这不是安全擦除。`
          : `将「${key}」改为「${target}」？旧 key 不再指向此条目，已有审批会失效。`,
      )
    )
      return;
    const prior = [...controls()].map((el) => [el, el.disabled] as const);
    prior.forEach(([el]) => (el.disabled = true));
    try {
      await ctx.api(
        deleted ? "/api/entry/delete" : "/api/entry/rename",
        deleted
          ? { key, expected_heads: [revision] }
          : { key, expected_revision: revision, new_key: target },
      );
      if (!form.isConnected) return;
      await ctx.onChanged(deleted ? key : target, deleted);
      ctx.notice(
        deleted
          ? "已移到回收站；历史仍保留，可选择删除前的版本恢复。"
          : "key 已重命名；正文、附件与权限保持不变。",
      );
    } finally {
      prior.forEach(([el, disabled]) => {
        if (el.isConnected) el.disabled = disabled;
      });
    }
  };
  form.onsubmit = (event) => {
    event.preventDefault();
    void ctx.run(() => change(false));
  };
  host.querySelector<HTMLButtonElement>("#delete-entry")!.onclick = () =>
    void ctx.run(() => change(true));
}
export function upgradeForm(format: number): string {
  if (format >= 2)
    return '<section class="settings-card"><p class="eyebrow">STORAGE FORMAT</p><h2>资料库格式 2</h2><p class="small muted">已支持删除记录与重命名。旧程序不能打开当前配置；旧备份仍保留升级前的内容。</p></section>';
  return '<section class="settings-card device-settings"><p class="eyebrow">STORAGE FORMAT</p><h2>升级资料库格式</h2><p class="small muted">当前是格式 1，可继续读写。删除和重命名需要格式 2。升级只重包装库密钥，不改写历史记录；完成后锁定并停用本机设备授权和浏览器记忆。主密码不变。</p><p class="inline-warning">先下载备份、暂停其他设备写入，并将所有设备程序更新到 0.4.0-dev 或更新版本。旧程序不能打开格式 2；外部同步不能保证配置与记录同时到达。</p><form id="upgrade-form"><input type="text" autocomplete="username" value="Keyfolio" class="sr-only" tabindex="-1"><label for="upgrade-password">当前主密码</label><input id="upgrade-password" type="password" autocomplete="current-password" required><label class="device-check"><input id="upgrade-confirmed" type="checkbox" required><span>我已备份，并已更新所有设备程序、暂停其他设备写入。</span></label><button class="button secondary" type="submit">确认升级并锁定</button></form></section>';
}
export function bindUpgrade(
  api: API,
  run: Context["run"],
  onUpgraded: (warning: string) => Promise<void>,
): void {
  const form = document.querySelector<HTMLFormElement>("#upgrade-form");
  if (!form) return;
  form.onsubmit = (event) => {
    event.preventDefault();
    void run(async () => {
      const password =
        form.querySelector<HTMLInputElement>("#upgrade-password")!;
      const confirmed =
        form.querySelector<HTMLInputElement>("#upgrade-confirmed")!;
      const button = form.querySelector<HTMLButtonElement>("button")!;
      if (!confirmed.checked)
        throw new Error("请先完成备份与其他设备的升级准备。");
      const value = password.value;
      button.disabled = true;
      password.disabled = true;
      confirmed.disabled = true;
      try {
        const result = await api<{ warning?: string }>("/api/vault/upgrade", {
          password: value,
          confirmed: true,
        });
        await onUpgraded(result.warning || "");
      } finally {
        password.value = "";
        button.disabled = false;
        password.disabled = false;
        confirmed.disabled = false;
      }
    });
  };
}
