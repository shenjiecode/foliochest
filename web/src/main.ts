import "./recovery.css";
import { esc } from "./ui";
import { mountHistory } from "./history";
import { backup, bindRestore, restoreForm } from "./recovery";
import { renderMarkdown } from "./markdown";

type Policy = "allow" | "ask" | "deny";
type Summary = {
  key: string;
  description: string;
  revision: string;
  read_policy: Policy;
  write_policy: Policy;
  state: string;
  versions: number;
};
type Attachment = { name: string; media_type: string; data: string };
type Entry = Summary & {
  value: string;
  format: string;
  attachments: Attachment[];
};
type State = {
  authenticated: boolean;
  csrf: string;
  version: string;
  vault: {
    directory: string;
    exists: boolean;
    unlocked: boolean;
    entries: number;
    issues: string[];
  };
};
type Pending = {
  id: string;
  client_name: string;
  operation: string;
  expires_at: string;
  reads?: {
    key: string;
    include_value: boolean;
    attachments?: string[];
    all_attachments?: boolean;
  }[];
  changes?: {
    key: string;
    expected_revision: string;
    value?: string;
    description?: string;
    attachments?: Attachment[];
  }[];
};
type Client = { id: string; name: string; created_at: string };

const root = document.querySelector<HTMLDivElement>("#app")!;
let state: State;
let entries: Summary[] = [];
let editor: Entry | null = null;
let draftAttachments: Attachment[] = [];
let dirty = false;
let query = "";
let currentView = "entries";
let polling = false;
let visibleToken = "";
let sessionEpoch = 0;
let bodySearch = false;
let bodyMatches: Set<string> | null = null;
let searchSequence = 0;
let searchTimer = 0;
let historyKey = "";
const labels: Record<Policy, string> = {
  allow: "免确认",
  ask: "需确认",
  deny: "拒绝",
};
const statusLabels: Record<string, string> = {
  ready: "可用",
  conflict: "版本冲突",
  incomplete: "同步未完整",
  missing: "文件缺失",
  invalid: "校验失败",
};
const $ = <T extends HTMLElement = HTMLElement>(selector: string): T =>
  document.querySelector<T>(selector)!;
const field = (id: string): string =>
  $<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(id).value;

async function api<T>(path: string, body?: unknown): Promise<T> {
  const epoch = sessionEpoch;
  const response = await fetch(path, {
    method: body === undefined ? "GET" : "POST",
    credentials: "same-origin",
    headers:
      body === undefined
        ? {}
        : {
            "Content-Type": "application/json",
            "X-Keyfolio-CSRF": state?.csrf ?? "",
          },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || "请求失败");
  if (epoch !== sessionEpoch && !path.startsWith("/api/vault/"))
    throw new Error("网页会话已变化，已丢弃旧请求的结果");
  return result as T;
}
function notice(message: string, error = false): void {
  const target = document.querySelector<HTMLElement>("#notice");
  if (!target) return;
  target.textContent = message;
  target.className = error ? "notice error" : "notice";
  target.hidden = !message;
}
async function run(action: () => Promise<void>): Promise<void> {
  try {
    await action();
  } catch (error) {
    notice(error instanceof Error ? error.message : "操作失败", true);
  }
}
function permissionSelect(id: string, value: Policy): string {
  return `<select id="${id}">${Object.entries(labels)
    .map(
      ([key, label]) =>
        `<option value="${key}" ${value === key ? "selected" : ""}>${label}</option>`,
    )
    .join("")}</select>`;
}
function pill(policy: Policy): string {
  return `<span class="pill ${policy}">${labels[policy]}</span>`;
}
function renderLock(): void {
  sessionEpoch++;
  searchSequence++;
  bodyMatches = null;
  bodySearch = false;
  query = "";
  historyKey = "";
  editor = null;
  dirty = false;
  draftAttachments = [];
  visibleToken = "";
  root.innerHTML = `<main class="lock-layout"><section class="intro"><div class="wordmark">Keyfolio<span class="dev-label">DEV</span></div><div class="intro-copy"><p class="eyebrow">YOUR LOCAL KNOWLEDGE VAULT</p><h1>资料留在本地，<br>使用由你决定。</h1><p class="intro-description">用 key 和 description 找到内容。<br>密码、文档与附件加密保存，AI 按你的权限使用。</p></div><div class="intro-footer">Go 后台 · 本地网页 · MCP 接入</div></section><section class="unlock-area"><div class="unlock-card"><p class="eyebrow">${state.vault.exists ? "WELCOME BACK" : "GET STARTED"}</p><h2>${state.vault.exists ? "解锁资料库" : "创建你的资料库"}</h2><p class="muted">${state.vault.exists ? "输入主密码，开启本次本地会话。" : "先选择一个空目录，再设置主密码。"}</p><div id="notice" class="notice" hidden role="status"></div><label for="directory">资料库目录</label><div class="directory-row"><input id="directory" value="${esc(state.vault.directory)}" spellcheck="false"><button id="select-directory" class="button secondary" type="button">切换</button></div><form id="unlock-form"><input type="text" name="username" autocomplete="username" value="Keyfolio" class="sr-only" aria-label="资料库" tabindex="-1"><label for="password">主密码</label><input id="password" type="password" autocomplete="${state.vault.exists ? "current-password" : "new-password"}" minlength="12" required placeholder="${state.vault.exists ? "输入主密码" : "至少 12 个字符，建议使用长口令"}">${state.vault.exists ? "" : '<label for="confirm-password">确认主密码</label><input id="confirm-password" type="password" autocomplete="new-password" minlength="12" required placeholder="再次输入主密码">'}<button class="button primary full" type="submit">${state.vault.exists ? "解锁资料库" : "创建并解锁"} <span aria-hidden="true">→</span></button></form>${restoreForm(state.vault.directory)}<p class="preview-note">开发预览 · 请先使用合成测试数据</p></div><p class="local-caption">网页和文件均由这台电脑上的服务提供。</p></section></main>`;
  bindRestore(api, run, async () => {
    await refreshState(true);
    notice("备份已恢复，请使用备份的主密码重新解锁。");
  });
  $("#select-directory").onclick = () =>
    void run(async () => {
      await api("/api/vault/select", { directory: field("#directory") });
      await refreshState(true);
    });
  $("#unlock-form").onsubmit = (event) => {
    event.preventDefault();
    void run(async () => {
      const password = field("#password");
      if (!state.vault.exists && password !== field("#confirm-password"))
        throw new Error("两次输入的主密码不一致");
      const button = $<HTMLButtonElement>("#unlock-form button");
      button.disabled = true;
      button.textContent = "正在解锁…";
      try {
        await api(
          state.vault.exists ? "/api/vault/unlock" : "/api/vault/create",
          { password },
        );
        await refreshState(true);
      } finally {
        if (button.isConnected) {
          button.disabled = false;
          button.textContent = state.vault.exists
            ? "解锁资料库 →"
            : "创建并解锁 →";
        }
      }
    });
  };
}
function renderShell(): void {
  root.innerHTML = `<div class="workspace"><aside class="sidebar"><div class="wordmark">Keyfolio<span class="dev-label">DEV</span></div><div class="workspace-label">个人工作区</div><nav aria-label="主导航"><button data-view="entries" class="nav-button active"><span>全部条目</span><span id="entry-count" class="count">0</span></button><button data-view="approvals" class="nav-button"><span>待审批</span><span id="approval-count" class="count">0</span></button><button data-view="settings" class="nav-button"><span>连接与设置</span><span aria-hidden="true">↗</span></button></nav><div class="sidebar-bottom"><div class="local-status"><span class="status-dot"></span>本地服务运行中</div><p>仅同步资料库文件<br>解锁与授权留在本机</p><button id="lock-button" class="button secondary full">锁定资料库</button></div></aside><main class="main-area"><header class="topbar"><span>PERSONAL / VAULT</span><span class="topbar-right">开发预览 <span class="dot-separator">·</span> 本地解锁</span></header><div id="notice" class="notice" hidden role="status"></div><div id="sync-issues" class="sync-issues" hidden></div><div id="view"></div></main></div>`;
  document.querySelectorAll<HTMLButtonElement>("[data-view]").forEach(
    (button) =>
      (button.onclick = () =>
        void run(async () => {
          if (dirty && !confirm("有未保存的内容，放弃当前草稿？")) return;
          dirty = false;
          editor = null;
          draftAttachments = [];
          visibleToken = "";
          currentView = button.dataset.view!;
          location.hash = currentView === "entries" ? "" : currentView;
          await renderView();
        })),
  );
  $("#lock-button").onclick = () =>
    void run(async () => {
      if (dirty && !confirm("锁定会清除未保存的草稿，继续？")) return;
      await api("/api/vault/lock", {});
      await refreshState(true);
    });
}
async function renderView(): Promise<void> {
  document
    .querySelectorAll<HTMLButtonElement>("[data-view]")
    .forEach((button) =>
      button.classList.toggle("active", button.dataset.view === currentView),
    );
  notice("");
  if (currentView === "approvals") {
    await renderApprovals();
    return;
  }
  if (currentView === "settings") {
    await renderSettings();
    return;
  }
  $("#view").innerHTML =
    `<section class="page-heading"><div><p class="eyebrow">COLLECTION</p><h1>全部条目</h1><p class="muted">先找到需要的资料，再决定怎样使用。</p></div><button id="new-entry" class="button primary">＋ 新建条目</button></section><div class="collection-layout"><section class="collection-panel"><div class="collection-tools"><input id="search" type="search" value="${esc(query)}" placeholder="搜索 key 或 description…" aria-label="搜索条目"><label class="search-scope"><input id="search-body" type="checkbox" ${bodySearch ? "checked" : ""}>含正文</label><span id="result-count" class="muted"></span></div><div class="table-wrap"><table><thead><tr><th>KEY / DESCRIPTION</th><th>AI 读取</th><th>AI 修改</th></tr></thead><tbody id="entry-list"></tbody></table></div><div class="collection-caption">key + description 是可检索索引。正文和附件在磁盘上加密保存。</div></section><section id="editor" class="editor-panel"><div class="empty-editor"><span class="empty-symbol" aria-hidden="true">⌘</span><h2>打开一份资料</h2><p>选择左侧条目查看正文、附件和权限，<br>或者创建你的第一条记录。</p></div></section></div>`;
  $("#new-entry").onclick = () => {
    if (dirty && !confirm("放弃当前草稿？")) return;
    editor = null;
    historyKey = "";
    draftAttachments = [];
    dirty = false;
    renderEditor(true);
  };
  $("#search").oninput = () => {
    query = field("#search");
    bodyMatches = null;
    searchSequence++;
    renderList();
    clearTimeout(searchTimer);
    if (bodySearch)
      searchTimer = window.setTimeout(() => void run(refreshBodySearch), 250);
  };
  $("#search-body").onchange = () => {
    bodySearch = $<HTMLInputElement>("#search-body").checked;
    bodyMatches = null;
    searchSequence++;
    renderList();
    if (bodySearch) void run(refreshBodySearch);
  };
  await refreshEntries();
}
async function refreshEntries(): Promise<void> {
  entries = await api<Summary[]>("/api/entries");
  const count = document.querySelector("#entry-count");
  if (count) count.textContent = String(entries.length);
  if (currentView === "entries") {
    renderList();
    if (bodySearch) await refreshBodySearch();
  }
}
async function refreshBodySearch(): Promise<void> {
  if (!bodySearch || currentView !== "entries") return;
  const sequence = ++searchSequence,
    searched = query;
  const hits = await api<Summary[]>(
    "/api/entries?scope=body&q=" + encodeURIComponent(searched),
  );
  if (
    sequence !== searchSequence ||
    !state.authenticated ||
    !bodySearch ||
    currentView !== "entries" ||
    searched !== query
  )
    return;
  bodyMatches = new Set(hits.map((hit) => hit.key));
  renderList();
}
function renderList(): void {
  const target = document.querySelector("#entry-list");
  if (!target) return;
  const q = query.toLowerCase();
  const selected = entries.filter((entry) =>
    bodySearch && q
      ? (bodyMatches?.has(entry.key) ?? false)
      : `${entry.key} ${entry.description}`.toLowerCase().includes(q),
  );
  $("#result-count").textContent =
    bodySearch && q && bodyMatches === null
      ? "检索中…"
      : `${selected.length} 个条目`;
  target.innerHTML = selected.length
    ? selected
        .map(
          (entry) =>
            `<tr class="${editor?.key === entry.key || historyKey === entry.key ? "selected" : ""}"><td><button class="entry-button" data-key="${esc(entry.key)}"><span class="entry-key">${esc(entry.key)}</span><span class="entry-description">${esc(entry.description)}</span>${entry.state !== "ready" ? `<span class="entry-warning">${statusLabels[entry.state] ?? esc(entry.state)}</span>` : ""}</button></td><td>${pill(entry.read_policy)}</td><td>${pill(entry.write_policy)}</td></tr>`,
        )
        .join("")
    : `<tr><td colspan="3"><div class="empty-list"><strong>${entries.length ? "没有匹配的条目" : "资料库还是空的"}</strong><p>${entries.length ? "试试其他 key 或内容说明。" : "从一份说明文档开始，熟悉保存与授权流程。"}</p></div></td></tr>`;
  target.querySelectorAll<HTMLButtonElement>("[data-key]").forEach(
    (button) =>
      (button.onclick = () =>
        void run(async () => {
          if (dirty && !confirm("放弃当前草稿？")) return;
          const key = button.dataset.key!;
          if (entries.find((item) => item.key === key)?.state !== "ready")
            await openHistory(key);
          else await openEntry(key);
        })),
  );
  if (editor) {
    const latest = entries.find((item) => item.key === editor!.key);
    const changed =
      !latest ||
      latest.revision !== editor.revision ||
      latest.state !== "ready";
    const banner = document.querySelector<HTMLElement>(
      "#editor-version-warning",
    );
    if (banner) {
      banner.hidden = !changed;
      banner.textContent = changed
        ? "外部版本已变化。当前内容保留，保存时会检查版本；请先处理差异。"
        : "";
    }
  }
}
async function openEntry(key: string): Promise<void> {
  const entry = await api<Entry>(`/api/entry?key=${encodeURIComponent(key)}`);
  if (currentView !== "entries" || !state.authenticated) return;
  historyKey = "";
  editor = entry;
  draftAttachments = entry.attachments || [];
  dirty = false;
  renderEditor(false);
  renderList();
  notice("");
}
async function openHistory(key: string): Promise<void> {
  editor = null;
  draftAttachments = [];
  dirty = false;
  historyKey = key;
  renderList();
  await mountHistory($("#editor"), key, {
    api,
    run,
    notice,
    setDirty: (value) => {
      dirty = value;
    },
    onSaved: async () => {
      await refreshEntries();
      await openEntry(key);
    },
  });
}
function renderEditor(isNew: boolean): void {
  const entry = editor;
  $("#editor").innerHTML =
    `<div class="editor-heading"><div><p class="eyebrow">${isNew ? "NEW ENTRY" : "ENTRY DETAILS"}</p><h2>${isNew ? "新建条目" : "编辑条目"}</h2></div><span id="dirty-state" class="muted">${isNew ? "尚未保存" : "已保存"}</span></div><div id="editor-version-warning" class="inline-warning" hidden></div><form id="entry-form"><label for="entry-key">key <span class="field-hint">明文名称</span></label><input id="entry-key" required maxlength="160" value="${esc(entry?.key ?? "")}" ${isNew ? "" : "disabled"} placeholder="例如：项目A/部署说明" spellcheck="false"><label for="entry-description">description <span class="field-hint">AI 优先读取</span></label><textarea id="entry-description" rows="2" required maxlength="1000" placeholder="说明这份资料是什么、什么时候会用到">${esc(entry?.description ?? "")}</textarea><div class="field-row"><label for="entry-value">value <span class="field-hint">加密正文</span></label><select id="entry-format" aria-label="正文格式">${[
      ["text", "纯文本"],
      ["markdown", "Markdown"],
      ["json", "JSON"],
    ]
      .map(
        ([value, label]) =>
          `<option value="${value}" ${(entry?.format ?? "markdown") === value ? "selected" : ""}>${label}</option>`,
      )
      .join(
        "",
      )}</select></div><textarea id="entry-value" class="code-editor" rows="10" spellcheck="false" placeholder="在这里保存正文…">${esc(entry?.value ?? "")}</textarea><div class="preview-toolbar"><button id="preview-toggle" class="text-button" type="button">预览正文</button><span class="small muted">不执行 HTML，不加载外部图片</span></div><article id="body-preview" class="markdown-preview" hidden></article><div class="attachment-heading"><label>附件 <span class="field-hint">每个最多 2 MiB</span></label><label for="upload" class="text-button">＋ 添加附件</label><input id="upload" type="file" multiple hidden></div><div id="attachments"></div><fieldset class="policy-fields"><legend>AI 访问权限</legend><div><label for="read-policy">读取正文 / 附件</label>${permissionSelect("read-policy", entry?.read_policy ?? "ask")}</div><div><label for="write-policy">修改内容</label>${permissionSelect("write-policy", entry?.write_policy ?? "ask")}</div></fieldset><footer class="editor-footer"><span class="muted">${entry ? `版本 ${esc(entry.revision.slice(0, 8))}` : "保存后生成首个加密版本"}</span><button type="submit" class="button primary">保存条目</button></footer></form>${entry ? '<div class="editor-tools"><button id="show-history" class="button secondary" type="button">查看历史 / 恢复版本</button></div>' : ""}`;
  if (entry)
    $("#show-history").onclick = () =>
      void run(async () => {
        if (dirty && !confirm("放弃当前草稿并查看历史？")) return;
        await openHistory(entry.key);
      });
  const updatePreview = () => {
    $("#body-preview").innerHTML =
      field("#entry-format") === "markdown"
        ? renderMarkdown(field("#entry-value"))
        : `<pre>${esc(field("#entry-value"))}</pre>`;
  };
  $("#preview-toggle").onclick = () => {
    const preview = $("#body-preview"),
      input = $("#entry-value");
    preview.hidden = !preview.hidden;
    input.hidden = !preview.hidden;
    $("#preview-toggle").textContent = preview.hidden ? "预览正文" : "编辑正文";
    if (!preview.hidden) updatePreview();
  };
  renderAttachments();
  $("#entry-form").oninput = () => {
    dirty = true;
    $("#dirty-state").textContent = "未保存";
    if (!$("#body-preview").hidden) updatePreview();
  };
  $("#entry-form").onsubmit = (event) => {
    event.preventDefault();
    void run(async () => {
      const button = $<HTMLButtonElement>("#entry-form button[type=submit]");
      button.disabled = true;
      button.textContent = "保存中…";
      try {
        const key = field("#entry-key");
        await api("/api/entries/save", {
          changes: [
            {
              key,
              expected_revision: entry?.revision ?? "",
              description: field("#entry-description"),
              value: field("#entry-value"),
              format: field("#entry-format"),
              attachments: draftAttachments,
              read_policy: field("#read-policy"),
              write_policy: field("#write-policy"),
            },
          ],
        });
        editor = await api<Entry>(`/api/entry?key=${encodeURIComponent(key)}`);
        dirty = false;
        draftAttachments = editor.attachments || [];
        renderEditor(false);
        await refreshEntries();
        notice("条目已保存为新的加密版本。");
      } finally {
        if (button.isConnected) {
          button.disabled = false;
          button.textContent = "保存条目";
        }
      }
    });
  };
  $("#upload").onchange = () =>
    void run(async () => {
      const input = $<HTMLInputElement>("#upload");
      const files = [...(input.files || [])];
      for (const file of files) {
        if (file.size > 2 * 1024 * 1024)
          throw new Error(`附件 ${file.name} 超过 2 MiB`);
        if (draftAttachments.some((item) => item.name === file.name))
          throw new Error(`已存在同名附件：${file.name}，请先移除旧附件`);
        const bytes = new Uint8Array(await file.arrayBuffer());
        let binary = "";
        for (let i = 0; i < bytes.length; i += 16384)
          binary += String.fromCharCode(...bytes.subarray(i, i + 16384));
        draftAttachments.push({
          name: file.name,
          media_type: file.type || "application/octet-stream",
          data: btoa(binary),
        });
      }
      input.value = "";
      dirty = true;
      $("#dirty-state").textContent = "未保存";
      renderAttachments();
    });
}
function renderAttachments(): void {
  $("#attachments").innerHTML = draftAttachments.length
    ? draftAttachments
        .map(
          (file, i) =>
            `<div class="attachment"><span class="attachment-name">${esc(file.name)}</span><span class="muted">${Math.ceil(atob(file.data).length / 1024)} KB</span><button type="button" class="text-button" data-download="${i}">下载</button><button type="button" class="text-button danger" data-remove="${i}">移除</button></div>`,
        )
        .join("")
    : '<div class="no-attachments">没有附件</div>';
  document.querySelectorAll<HTMLButtonElement>("[data-remove]").forEach(
    (button) =>
      (button.onclick = () => {
        draftAttachments.splice(Number(button.dataset.remove), 1);
        dirty = true;
        $("#dirty-state").textContent = "未保存";
        renderAttachments();
      }),
  );
  document.querySelectorAll<HTMLButtonElement>("[data-download]").forEach(
    (button) =>
      (button.onclick = () => {
        const file = draftAttachments[Number(button.dataset.download)];
        const bytes = Uint8Array.from(atob(file.data), (c) => c.charCodeAt(0));
        const href = URL.createObjectURL(
          new Blob([bytes], { type: "application/octet-stream" }),
        );
        const link = document.createElement("a");
        link.href = href;
        link.download = file.name;
        link.click();
        setTimeout(() => URL.revokeObjectURL(href), 1000);
      }),
  );
}
async function renderApprovals(): Promise<void> {
  const requests = await api<Pending[]>("/api/approvals");
  $("#view").innerHTML =
    `<section class="page-heading"><div><p class="eyebrow">HUMAN IN THE LOOP</p><h1>待审批</h1><p class="muted">每次同意只覆盖列出的条目、内容与操作。</p></div><button id="refresh-approvals" class="button secondary">刷新请求</button></section><div class="approval-list">${
      requests.length
        ? requests
            .sort((a, b) => a.expires_at.localeCompare(b.expires_at))
            .map(
              (p) =>
                `<article class="approval-card"><div class="approval-card-heading"><div><span class="pill ask">${p.operation === "read" ? "读取请求" : "替换请求"}</span><h2>${esc(p.client_name)}</h2></div><span class="muted">有效至 ${new Date(p.expires_at).toLocaleTimeString()}</span></div><ul class="approval-targets">${p.reads?.map((r) => `<li><code>${esc(r.key)}</code><span>${r.include_value ? "正文 " : ""}${r.all_attachments ? "全部附件" : esc((r.attachments || []).join("、"))}</span></li>`).join("") ?? p.changes?.map((c) => `<li><code>${esc(c.key)}</code><span>替换 ${c.value !== undefined ? "value " : ""}${c.description !== undefined ? "description " : ""}${c.attachments !== undefined ? "附件列表" : ""}</span></li>`).join("") ?? ""}</ul>${p.changes ? `<details><summary>查看将要写入的内容</summary>${p.changes.map((c) => `<h3>${esc(c.key)}</h3>${c.description !== undefined ? `<p>description：${esc(c.description)}</p>` : ""}${c.value !== undefined ? `<pre>${esc(c.value)}</pre>` : ""}${c.attachments ? `<p>附件：${c.attachments.map((a) => `${esc(a.name)}（${Math.ceil(atob(a.data).length / 1024)} KB）`).join("、") || "清空附件列表"}</p>` : ""}`).join("")}</details>` : '<p class="muted">同意后，所选内容的明文会交给该 AI 客户端。</p>'}<footer><button class="button secondary" data-decide="${p.id}" data-accept="false">拒绝</button><button class="button primary" data-decide="${p.id}" data-accept="true">同意本次操作</button></footer></article>`,
            )
            .join("")
        : '<div class="empty-state"><div class="empty-symbol" aria-hidden="true">✓</div><h2>暂时没有待审批请求</h2><p>AI 访问需要确认的内容时，请求会出现在这里。</p></div>'
    }</div>`;
  $("#refresh-approvals").onclick = () => void run(renderApprovals);
  document.querySelectorAll<HTMLButtonElement>("[data-decide]").forEach(
    (button) =>
      (button.onclick = () =>
        void run(async () => {
          button.disabled = true;
          try {
            await api("/api/approvals/decide", {
              id: button.dataset.decide,
              accept: button.dataset.accept === "true",
            });
            await renderApprovals();
            await refreshEntries();
            notice("请求已处理。AI 可以领取本次结果。");
          } finally {
            if (button.isConnected) button.disabled = false;
          }
        })),
  );
  $("#approval-count").textContent = String(requests.length);
}
async function renderSettings(): Promise<void> {
  const [clients, device] = await Promise.all([
    api<Client[]>("/api/clients"),
    api<{ idle_lock_minutes: number }>("/api/settings"),
  ]);
  if (currentView !== "settings" || !state.authenticated) return;
  $("#view").innerHTML =
    `<section class="page-heading"><div><p class="eyebrow">CONNECTIONS</p><h1>连接与设置</h1><p class="muted">每个 AI 客户端使用独立的接入凭据。</p></div></section><div class="settings-grid"><section class="settings-card"><h2>MCP 客户端</h2><p class="muted">本地地址 <code>${esc(location.origin)}/mcp</code></p><form id="client-form" class="client-form"><label class="sr-only" for="client-name">客户端名称</label><input id="client-name" required maxlength="120" placeholder="例如：本机 pi"><button class="button primary">创建接入凭据</button></form><div id="client-secret" hidden></div><div class="client-list">${clients.length ? clients.map((client) => `<div class="client-row"><div><strong>${esc(client.name)}</strong><p class="muted">创建于 ${new Date(client.created_at).toLocaleDateString()}</p></div><button class="text-button danger" data-revoke="${esc(client.id)}">撤销</button></div>`).join("") : '<p class="muted">还没有接入的客户端。</p>'}</div></section><section class="settings-card"><h2>这台电脑</h2><dl class="device-info"><dt>资料库目录</dt><dd class="mono">${esc(state.vault.directory)}</dd><dt>程序版本</dt><dd>${esc(state.version)}</dd><dt>文件变化</dt><dd>每 2 秒扫描；读取与保存前再次校验</dd><dt>自动启动 / 自动解锁</dt><dd>尚未实现，当前使用主密码手动解锁</dd></dl><p class="small muted">目录切换在锁定页面完成。系统凭据与自启动仍待实现。</p><form id="idle-form"><label for="idle-minutes">网页无操作后自动锁定（分钟）</label><input id="idle-minutes" type="number" min="0" max="1440" value="${device.idle_lock_minutes}" required><p class="small muted">0 表示关闭。只计算网页操作，后台轮询和 MCP 请求不会续期。</p><button class="button secondary" type="submit">保存锁定设置</button></form></section><section class="settings-card"><h2>备份与恢复</h2><p class="muted">备份包含明文索引、加密正文、附件及全部历史，不含本机接入凭据。</p><button id="download-backup" class="button primary" type="button">下载加密资料库备份</button>${restoreForm(state.vault.directory)}</section></div>`;
  $("#idle-form").onsubmit = (event) => {
    event.preventDefault();
    void run(async () => {
      await api("/api/settings", {
        idle_lock_minutes: Number(field("#idle-minutes")),
      });
      notice("本机自动锁定设置已保存。");
    });
  };
  $("#download-backup").onclick = () =>
    void run(async () => {
      const button = $<HTMLButtonElement>("#download-backup");
      button.disabled = true;
      try {
        await backup(state.csrf);
        notice("已生成备份，请保留下载文件与主密码。");
      } finally {
        if (button.isConnected) button.disabled = false;
      }
    });
  bindRestore(api, run, async () => {
    await refreshState(true);
    notice("备份已恢复，请使用备份的主密码重新解锁。");
  });
  $("#client-form").onsubmit = (event) => {
    event.preventDefault();
    void run(async () => {
      const result = await api<{ client: Client; token: string; url: string }>(
        "/api/clients/create",
        { name: field("#client-name") },
      );
      visibleToken = result.token;
      await renderSettings();
      $("#client-secret").hidden = false;
      $("#client-secret").innerHTML =
        `<div class="secret-box"><strong>接入凭据仅在此显示一次</strong><pre>${esc(visibleToken)}</pre><p class="small">客户端通过 <code>Authorization: Bearer &lt;凭据&gt;</code> 连接，不需要主密码。</p><button id="copy-token" class="button secondary" type="button">复制凭据</button><button id="hide-token" class="text-button" type="button">已保存，隐藏</button></div>`;
      $("#copy-token").onclick = () =>
        void run(async () => {
          await navigator.clipboard.writeText(visibleToken);
          notice("接入凭据已复制。");
        });
      $("#hide-token").onclick = () => {
        visibleToken = "";
        $("#client-secret").innerHTML = "";
        $("#client-secret").hidden = true;
      };
    });
  };
  document.querySelectorAll<HTMLButtonElement>("[data-revoke]").forEach(
    (button) =>
      (button.onclick = () =>
        void run(async () => {
          if (!confirm("撤销后，该客户端的新请求和待审批操作将被拒绝。继续？"))
            return;
          await api("/api/clients/revoke", { id: button.dataset.revoke });
          await renderSettings();
          notice("客户端接入已撤销。");
        })),
  );
}
async function refreshState(force = false): Promise<void> {
  const previous = state?.authenticated;
  state = await api<State>("/api/state");
  if (!state.authenticated) {
    if (force || previous !== false) renderLock();
    return;
  }
  if (force || !previous) {
    currentView = ["approvals", "settings"].includes(location.hash.slice(1))
      ? location.hash.slice(1)
      : "entries";
    renderShell();
    await renderView();
  }
  const issues = document.querySelector<HTMLElement>("#sync-issues");
  if (issues) {
    issues.hidden = state.vault.issues.length === 0;
    issues.textContent = state.vault.issues.join("；");
  }
}
let lastActivity = 0;
for (const name of ["pointerdown", "keydown", "input", "wheel"]) {
  window.addEventListener(
    name,
    (event) => {
      if (
        !event.isTrusted ||
        !state?.authenticated ||
        Date.now() - lastActivity < 15000
      )
        return;
      lastActivity = Date.now();
      void api("/api/activity", {}).catch(() => {
        lastActivity = 0;
      });
    },
    { passive: true },
  );
}
window.addEventListener("beforeunload", (event) => {
  if (dirty) {
    event.preventDefault();
    event.returnValue = "";
  }
});
window.addEventListener("hashchange", () => {
  const view = ["approvals", "settings"].includes(location.hash.slice(1))
    ? location.hash.slice(1)
    : "entries";
  if (state?.authenticated && currentView !== view && !dirty) {
    currentView = view;
    void run(renderView);
  }
});
root.innerHTML =
  '<main class="boot"><div class="wordmark">Keyfolio</div><p>正在连接本地服务…</p></main>';
refreshState(true).catch((error) => {
  root.innerHTML = `<main class="boot"><h1>暂时无法连接</h1><p>${esc(error.message)}</p><p>确认本地程序运行后刷新网页。</p></main>`;
});
setInterval(() => {
  if (polling || !state) return;
  polling = true;
  void (async () => {
    try {
      await refreshState();
      if (state.authenticated) {
        await refreshEntries();
        const requests = await api<Pending[]>("/api/approvals");
        const count = document.querySelector("#approval-count");
        if (count) count.textContent = String(requests.length);
      }
    } catch {
      /* Preserve unsaved input on transient disconnection; the next action reports errors. */
    } finally {
      polling = false;
    }
  })();
}, 3000);
