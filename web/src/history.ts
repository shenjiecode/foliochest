import { API, esc, download } from "./ui";
type Version = {
  entry_id: string;
  revision: string;
  key: string;
  description: string;
  read_policy: string;
  write_policy: string;
  updated_at: string;
  head: boolean;
  available: boolean;
};
type History = {
  key: string;
  state: string;
  heads: string[];
  versions: Version[];
};
type Entry = Version & {
  value: string;
  format: string;
  attachments: { name: string; data: string; media_type: string }[];
};
type Context = {
  api: API;
  run: (fn: () => Promise<void>) => Promise<void>;
  notice: (message: string, error?: boolean) => void;
  setDirty: (dirty: boolean) => void;
  onSaved: () => Promise<void>;
};
export async function mountHistory(
  host: HTMLElement,
  key: string,
  ctx: Context,
): Promise<void> {
  host.innerHTML =
    '<section class="history-panel"><p class="muted">正在读取加密历史…</p></section>';
  const panel = host.firstElementChild as HTMLElement;
  const history = await ctx.api<History>(
    "/api/history?key=" + encodeURIComponent(key),
  );
  if (!panel.isConnected) return;
  let dirty = false,
    loadID = 0;
  const conflict = history.state === "conflict";
  const collision =
    new Set(history.versions.filter((v) => v.head).map((v) => v.entry_id))
      .size > 1;
  const blockedReason = collision
    ? "这是不同条目的同名冲突，当前不能直接合并。请先保留备份。"
    : !["ready", "conflict"].includes(history.state)
      ? "当前文件尚未完整，只能查看可用历史，不能恢复写入。"
      : "";
  panel.innerHTML = `<div class="editor-heading"><div><p class="eyebrow">HISTORY / RECOVERY</p><h2>历史与冲突</h2></div><button class="button secondary" data-refresh-history type="button">刷新版本（${history.versions.length}）</button></div><p class="history-key">${esc(key)}</p><p class="small muted">${conflict ? "选择一个分支作为基础，编辑正文合并。所有当前分支都会成为新版本的父版本，权限按当前分支取更严格的策略。" : "恢复将新建一个版本，不删除历史，也不恢复旧权限。"}</p><div class="version-list" aria-label="历史版本">${history.versions.map((v) => `<button class="version-button" data-revision="${esc(v.revision)}" ${v.available ? "" : "disabled"}><span>${v.head ? "当前分支" : "历史版本"} · ${esc(v.revision.slice(0, 8))}</span><time>${esc(new Date(v.updated_at).toLocaleString())}</time></button>`).join("")}</div><div class="history-detail"></div>`;
  const detail = panel.querySelector<HTMLElement>(".history-detail")!;
  panel.querySelector<HTMLButtonElement>("[data-refresh-history]")!.onclick =
    () =>
      void ctx.run(async () => {
        if (dirty && !confirm("放弃历史编辑区的草稿，重新读取版本？")) return;
        ctx.setDirty(false);
        await mountHistory(host, key, ctx);
      });
  async function load(revision: string) {
    if (dirty && !confirm("放弃历史编辑区里未保存的内容？")) return;
    const request = ++loadID;
    const entry = await ctx.api<Entry>(
      "/api/history/entry?key=" +
        encodeURIComponent(key) +
        "&revision=" +
        encodeURIComponent(revision),
    );
    if (!panel.isConnected || request !== loadID) return;
    dirty = false;
    ctx.setDirty(false);
    panel
      .querySelectorAll<HTMLButtonElement>("[data-revision]")
      .forEach((b) =>
        b.classList.toggle("selected", b.dataset.revision === revision),
      );
    const attachments = entry.attachments || [];
    const canRestore = !blockedReason;
    detail.innerHTML = `<form class="history-form"><label>description<textarea name="description" required maxlength="1000" rows="2">${esc(entry.description)}</textarea></label><label>正文格式<select name="format">${["text", "markdown", "json"].map((f) => `<option value="${f}" ${f === entry.format ? "selected" : ""}>${f}</option>`).join("")}</select></label><label>历史正文（可编辑后合并）<textarea name="value" class="code-editor" rows="10">${esc(entry.value)}</textarea></label><p class="small muted">附件沿用所选版本，其他版本的附件仍保留在历史里。</p><div class="history-attachments">${attachments.map((a, i) => `<div class="attachment"><span class="attachment-name">${esc(a.name)}</span><button type="button" class="text-button" data-history-download="${i}">下载</button></div>`).join("") || '<p class="muted">此版本没有附件。</p>'}</div><p class="small muted">恢复后 AI 读取/修改策略：${conflict ? "当前分支中更严格的策略" : "保持当前设置"}。</p><button class="button primary" type="submit" ${canRestore ? "" : "disabled"}>${conflict ? "保存合并版本" : "恢复为新版本"}</button>${canRestore ? "" : `<p class="inline-warning">${esc(blockedReason)}</p>`}</form>`;
    const form = detail.querySelector<HTMLFormElement>("form")!;
    form.oninput = () => {
      dirty = true;
      ctx.setDirty(true);
    };
    detail
      .querySelectorAll<HTMLButtonElement>("[data-history-download]")
      .forEach(
        (button) =>
          (button.onclick = () => {
            const file = attachments[Number(button.dataset.historyDownload)];
            download(
              new Blob(
                [Uint8Array.from(atob(file.data), (c) => c.charCodeAt(0))],
                { type: "application/octet-stream" },
              ),
              file.name,
            );
          }),
      );
    form.onsubmit = (event) => {
      event.preventDefault();
      if (
        !confirm(
          "将基于所选内容创建新版本。附件沿用所选版本，历史全部保留。继续？",
        )
      )
        return;
      void ctx.run(async () => {
        const button = form.querySelector<HTMLButtonElement>(
          'button[type="submit"]',
        )!;
        button.disabled = true;
        try {
          const values = new FormData(form);
          await ctx.api("/api/history/restore", {
            key,
            expected_heads: history.heads,
            source_revision: revision,
            description: values.get("description"),
            value: values.get("value"),
            format: values.get("format"),
          });
          dirty = false;
          ctx.setDirty(false);
          if (!panel.isConnected) return;
          await ctx.onSaved();
          ctx.notice("已保存新版本，原有历史与附件仍然保留。");
        } finally {
          if (button.isConnected) button.disabled = false;
        }
      });
    };
  }
  panel
    .querySelectorAll<HTMLButtonElement>("[data-revision]")
    .forEach(
      (button) =>
        (button.onclick = () =>
          void ctx.run(() => load(button.dataset.revision!))),
    );
  const first = history.versions.find((v) => v.available);
  if (first) await load(first.revision);
  else
    detail.innerHTML =
      '<p class="inline-warning">没有可读取的历史版本。请等待同步完整，或从备份恢复到新目录。</p>';
}
