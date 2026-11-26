import { API, esc, download } from "./ui";
import { groupInput, groupLabel } from "./groups";
type Version = {
  entry_id: string;
  group?: string;
  revision: string;
  key: string;
  description: string;
  read_policy: string;
  write_policy: string;
  updated_at: string;
  head: boolean;
  available: boolean;
  deleted?: boolean;
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
  vaultFormat: number;
  api: API;
  run: (fn: () => Promise<void>) => Promise<void>;
  notice: (message: string, error?: boolean) => void;
  setDirty: (dirty: boolean) => void;
  onSaved: (key: string, deleted?: boolean) => Promise<void>;
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
  let busy = false;
  function beginBusy() {
    busy = true;
    loadID++;
    const prior = [
      ...panel.querySelectorAll<HTMLInputElement | HTMLButtonElement>(
        "input,button,select,textarea",
      ),
    ].map((el) => [el, el.disabled] as const);
    prior.forEach(([el]) => (el.disabled = true));
    return () => {
      busy = false;
      prior.forEach(([el, disabled]) => {
        if (el.isConnected) el.disabled = disabled;
      });
    };
  }
  const conflict = history.state === "conflict";
  const currentGroups = [
    ...new Set(
      history.versions.filter((v) => v.head).map((v) => v.group || ""),
    ),
  ];
  const groupConflict = currentGroups.length > 1;
  const collision =
    new Set(history.versions.filter((v) => v.head).map((v) => v.entry_id))
      .size > 1;
  const blockedReason = collision
    ? "这是不同条目的同名冲突，当前不能直接合并。请先保留备份。"
    : !["ready", "conflict", "deleted"].includes(history.state)
      ? "当前文件尚未完整，只能查看可用历史，不能恢复写入。"
      : "";
  panel.innerHTML = `<div class="editor-heading"><div><p class="eyebrow">HISTORY / RECOVERY</p><h2>历史与冲突</h2></div><button class="button secondary" data-refresh-history type="button">刷新版本（${history.versions.length}）</button></div><p class="history-key">${esc(key)}</p><p class="small muted">${conflict ? "选择一个分支作为基础，编辑正文合并。所有当前分支都会成为新版本的父版本，权限按当前分支取更严格的策略。" : "恢复将新建一个版本，不删除历史，也不恢复旧权限。"}</p><div class="version-list" aria-label="历史版本">${history.versions.map((v) => `<button class="version-button" data-revision="${esc(v.revision)}" ${v.available ? "" : "disabled"}><span>${v.deleted ? "删除记录" : v.head ? "当前分支" : "历史版本"} · ${esc(v.key)} · ${esc(groupLabel(v.group || ""))} · ${esc(v.revision.slice(0, 8))}</span><time>${esc(new Date(v.updated_at).toLocaleString())}</time></button>`).join("")}</div><div class="history-detail"></div>${conflict && !blockedReason && !groupConflict && ctx.vaultFormat >= 2 ? '<div class="editor-tools"><button type="button" class="text-button danger" data-delete-branches>删除全部当前分支</button></div>' : ""}`;
  const deleteBranches = panel.querySelector<HTMLButtonElement>(
    "[data-delete-branches]",
  );
  if (deleteBranches)
    deleteBranches.onclick = () =>
      void ctx.run(async () => {
        if (busy) return;
        if (
          !confirm(
            "将全部当前分支移到回收站？未保存的合并草稿会丢失，所有加密历史仍保留；这不是安全擦除。",
          )
        )
          return;
        const done = beginBusy();
        try {
          await ctx.api("/api/entry/delete", {
            key,
            expected_heads: history.heads,
          });
          if (!panel.isConnected) return;
          dirty = false;
          ctx.setDirty(false);
          await ctx.onSaved(key, true);
          ctx.notice("全部已确认分支已移到回收站，历史保留。");
        } finally {
          done();
        }
      });
  const detail = panel.querySelector<HTMLElement>(".history-detail")!;
  panel.querySelector<HTMLButtonElement>("[data-refresh-history]")!.onclick =
    () =>
      void ctx.run(async () => {
        if (dirty && !confirm("放弃历史编辑区的草稿，重新读取版本？")) return;
        ctx.setDirty(false);
        await mountHistory(host, key, ctx);
      });
  async function load(revision: string) {
    if (busy) return;
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
    const reason = entry.deleted
      ? "此版本是删除记录，不含正文。请选择删除前的内容版本恢复。"
      : blockedReason;
    const canRestore = !reason;
    detail.innerHTML = `<form class="history-form">${ctx.vaultFormat >= 2 ? `<label>恢复后的 key<input name="target_key" maxlength="160" required spellcheck="false" value="${esc(key)}"></label>` : ""}${ctx.vaultFormat >= 3 ? groupInput(groupConflict ? "" : currentGroups[0] || "", ctx.vaultFormat, "restore-group") : ""}${groupConflict ? `<label class="device-check"><input type="checkbox" name="confirm_group" required><span>当前分支的分组不一致（${currentGroups.map(groupLabel).map(esc).join("、")}），我已明确选择恢复后的分组；留空表示未分组。</span></label>` : ""}<p class="small muted">所选历史分组：${esc(groupLabel(entry.group || ""))}；恢复默认保留当前分组。所选版本原 key：${esc(entry.key)}。恢复默认保留当前名称，权限采用当前策略；历史不会被删除。</p><label>description<textarea name="description" required maxlength="1000" rows="2">${esc(entry.description)}</textarea></label><label>正文格式<select name="format">${["text", "markdown", "json"].map((f) => `<option value="${f}" ${f === entry.format ? "selected" : ""}>${f}</option>`).join("")}</select></label><label>历史正文（可编辑后合并）<textarea name="value" class="code-editor" rows="10">${esc(entry.value)}</textarea></label><p class="small muted">附件沿用所选版本，其他版本的附件仍保留在历史里。</p><div class="history-attachments">${attachments.map((a, i) => `<div class="attachment"><span class="attachment-name">${esc(a.name)}</span><button type="button" class="text-button" data-history-download="${i}">下载</button></div>`).join("") || '<p class="muted">此版本没有附件。</p>'}</div><p class="small muted">恢复后 AI 读取/修改策略：${conflict ? "当前分支中更严格的策略" : "保持当前设置"}。</p><button class="button primary" type="submit" ${canRestore ? "" : "disabled"}>${conflict ? "保存合并版本" : "恢复为新版本"}</button>${canRestore ? "" : `<p class="inline-warning">${esc(reason)}</p>`}</form>`;
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
      if (!canRestore || busy) return;
      if (
        !confirm(
          "将基于所选内容创建新版本。附件沿用所选版本，历史全部保留。继续？",
        )
      )
        return;
      void ctx.run(async () => {
        const values = new FormData(form);
        const done = beginBusy();
        try {
          await ctx.api("/api/history/restore", {
            key,
            target_key: values.get("target_key") || key,
            expected_heads: history.heads,
            source_revision: revision,
            ...(ctx.vaultFormat >= 3
              ? { group: String(values.get("group") || "").trim() }
              : {}),
            description: values.get("description"),
            value: values.get("value"),
            format: values.get("format"),
          });
          if (!panel.isConnected) return;
          dirty = false;
          ctx.setDirty(false);
          await ctx.onSaved(String(values.get("target_key") || key));
          ctx.notice("已保存新版本，原有历史与附件仍然保留。");
        } finally {
          done();
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
  const first =
    history.versions.find((v) => v.available && !v.deleted) ??
    history.versions.find((v) => v.available);
  if (first) await load(first.revision);
  else
    detail.innerHTML =
      '<p class="inline-warning">没有可读取的历史版本。请等待同步完整，或从备份恢复到新目录。</p>';
}
