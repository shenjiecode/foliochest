import { API, esc, base64, download } from "./ui";
export function restoreForm(directory: string): string {
  return `<details class="restore-box"><summary>从加密备份恢复</summary><p class="small muted">恢复到新目录或空目录，不覆盖现有资料。完成后切换到恢复目录并重新解锁。</p><form id="restore-form"><label for="backup-file">FolioChest / Keyfolio 备份 ZIP（最多 66 MiB）</label><input id="backup-file" type="file" accept=".zip,application/zip" required><label for="restore-directory">恢复目录</label><input id="restore-directory" value="${esc(directory + "-restored")}" required spellcheck="false"><input type="text" name="username" autocomplete="username" value="FolioChest backup" class="sr-only" tabindex="-1"><label for="backup-password">备份的主密码</label><input id="backup-password" type="password" autocomplete="current-password" required><button class="button secondary" type="submit">验证并恢复到新目录</button></form></details>`;
}
export function bindRestore(
  api: API,
  run: (fn: () => Promise<void>) => Promise<void>,
  onRestored: () => Promise<void>,
): void {
  const form = document.querySelector<HTMLFormElement>("#restore-form");
  if (!form) return;
  form.onsubmit = (event) => {
    event.preventDefault();
    if (!confirm("恢复会切换并锁定当前资料库，不覆盖原目录。继续？")) return;
    void run(async () => {
      const input = form.querySelector<HTMLInputElement>("#backup-file")!,
        password = form.querySelector<HTMLInputElement>("#backup-password")!,
        button = form.querySelector<HTMLButtonElement>(
          'button[type="submit"]',
        )!;
      const file = input.files?.[0];
      if (!file) throw new Error("请选择备份文件");
      if (file.size > 66 * 1024 * 1024) throw new Error("备份超过 66 MiB");
      button.disabled = true;
      button.textContent = "正在校验并恢复…";
      try {
        await api("/api/vault/restore", {
          archive: await base64(file),
          directory:
            form.querySelector<HTMLInputElement>("#restore-directory")!.value,
          password: password.value,
        });
        password.value = "";
        input.value = "";
        await onRestored();
      } finally {
        password.value = "";
        if (button.isConnected) {
          button.disabled = false;
          button.textContent = "验证并恢复到新目录";
        }
      }
    });
  };
}
export async function backup(csrf: string): Promise<void> {
  const response = await fetch("/api/backup", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-FolioChest-CSRF": csrf },
    body: "{}",
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || "备份失败");
  }
  const name =
    response.headers
      .get("Content-Disposition")
      ?.match(/filename=([\w.-]+)/)?.[1] ?? "foliochest-backup.zip";
  download(await response.blob(), name);
}
