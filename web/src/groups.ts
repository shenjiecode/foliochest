import { API, esc } from "./ui";

export type GroupMember = { key: string; expected_revision: string };
export type GroupInfo = {
  name: string;
  count: number;
  deleted_count: number;
  blocked: number;
  members: GroupMember[];
};
export type Grouped = { group?: string; candidate_groups?: string[] };
export const groupLabel = (name: string) => name || "未分组";
export function matchesGroup(entry: Grouped, group: string | null): boolean {
  return (
    group === null ||
    (entry.candidate_groups?.length
      ? entry.candidate_groups.includes(group)
      : (entry.group || "") === group)
  );
}
export function groupText(entry: Grouped): string {
  return entry.candidate_groups?.length
    ? entry.candidate_groups.map(groupLabel).join(" / ")
    : groupLabel(entry.group || "");
}
export function groupInput(
  value: string,
  format: number,
  id = "entry-group",
): string {
  return `<label for="${id}">分组 <span class="field-hint">明文 · 单层 · 留空为未分组</span></label><input id="${id}" name="group" maxlength="64" list="known-groups" value="${esc(value)}" placeholder="选择已有组或输入新组名" ${format < 3 ? "disabled" : ""}>${format < 3 ? '<p class="small muted">分组需要格式 3，请先在设置页备份并确认升级；原有内容仍可编辑。</p>' : ""}`;
}
export function groupBuckets<T extends Grouped>(
  entries: T[],
): { id: string; label: string; entries: T[] }[] {
  const map = new Map<string, { id: string; label: string; entries: T[] }>();
  for (const entry of entries) {
    const conflict = !!entry.candidate_groups?.length;
    const id = conflict ? "conflict" : "group:" + (entry.group || "");
    if (!map.has(id))
      map.set(id, {
        id,
        label: conflict ? "分组待确认" : groupLabel(entry.group || ""),
        entries: [],
      });
    map.get(id)!.entries.push(entry);
  }
  return [...map.values()].sort((a, b) => {
    if (a.id === b.id) return 0;
    if (a.id === "conflict") return -1;
    if (b.id === "conflict") return 1;
    if (a.id === "group:") return 1;
    if (b.id === "group:") return -1;
    return a.label.localeCompare(b.label, "zh-CN");
  });
}
type Context = {
  api: API;
  run: (action: () => Promise<void>) => Promise<void>;
  notice: (message: string, error?: boolean) => void;
  isDirty: () => boolean;
  format: () => number;
  view: () => string;
  revision: () => number;
  onSelect: (name: string) => Promise<void>;
  onChanged: () => Promise<void>;
};
export class GroupUI {
  filter: string | null = null;
  catalog: GroupInfo[] = [];
  selected = new Map<string, GroupMember>();
  folded = new Set<string>();
  busy = false;
  private operation = 0;
  constructor(private ctx: Context) {}
  reset(): void {
    this.operation++;
    this.busy = false;
    this.filter = null;
    this.catalog = [];
    this.selected.clear();
    this.folded.clear();
  }
  update(catalog: GroupInfo[]): void {
    this.catalog = catalog;
    this.renderNavigation();
    const list = document.querySelector("#known-groups");
    const options = catalog
      .filter((g) => g.name)
      .map((g) => `<option value="${esc(g.name)}"></option>`)
      .join("");
    if (list && list.innerHTML !== options) list.innerHTML = options;
    this.updateControls();
  }
  renderNavigation(): void {
    const host = document.querySelector("#group-navigation");
    if (!host) return;
    const groups = this.catalog.some((g) => g.name === "")
      ? this.catalog
      : [
          { name: "", count: 0, deleted_count: 0, blocked: 0, members: [] },
          ...this.catalog,
        ];
    const active = ["entries", "trash"].includes(this.ctx.view());
    const html =
      '<p class="group-nav-heading">单层分组</p>' +
      groups
        .map(
          (g) =>
            `<button type="button" class="nav-button group-nav-button ${active && this.filter === g.name ? "active" : ""}" data-group="${esc(g.name)}"><span>${esc(groupLabel(g.name))}</span><span class="count">${this.ctx.view() === "trash" ? g.deleted_count : g.count}</span></button>`,
        )
        .join("");
    if (host.innerHTML !== html) {
      host.innerHTML = html;
      host
        .querySelectorAll<HTMLButtonElement>("[data-group]")
        .forEach((button) => {
          button.onclick = () =>
            void this.ctx.run(() => this.ctx.onSelect(button.dataset.group!));
        });
    }
  }
  toolsHTML(): string {
    return `<div class="group-tools"><datalist id="known-groups"></datalist><form id="move-group-form" class="group-move-form"><span id="group-selected-count" class="small muted">未选择条目</span><label class="sr-only" for="move-group">移动到分组</label><input id="move-group" name="group" maxlength="64" list="known-groups" placeholder="目标组；留空为未分组"><button class="button secondary" type="submit">移动所选</button><button class="text-button" id="clear-group-selection" type="button">清除选择</button></form>${this.filter ? `<details class="group-edit"><summary>管理「${esc(this.filter)}」分组</summary><p id="group-member-note" class="small muted"></p><form id="edit-group-form"><label for="edit-group-name">新组名</label><input id="edit-group-name" name="name" maxlength="64" value="${esc(this.filter)}"><div class="group-actions"><button class="button secondary" type="submit" name="action" value="rename">重命名组</button><button class="text-button danger" type="submit" name="action" value="remove">移除分组，保留条目</button></div></form></details>` : ""}${this.ctx.format() < 3 ? '<p class="small muted">旧库可继续读写；使用分组前需在设置页显式升级为格式 3。</p>' : '<p class="small muted">每次最多移动 20 条；只改变分组，不改变 key 或权限。空组不单独保留。</p>'}</div>`;
  }
  bindTools(host: HTMLElement): void {
    const move = host.querySelector<HTMLFormElement>("#move-group-form")!;
    move.onsubmit = (event) => {
      event.preventDefault();
      void this.ctx.run(async () => {
        this.requireReady();
        const members = [...this.selected.values()];
        if (!members.length || members.length > 20)
          throw new Error("请选择 1–20 条可用条目。");
        const group = String(new FormData(move).get("group") || "").trim();
        if (
          !confirm(
            `将所选 ${members.length} 条移到「${groupLabel(group)}」？正文、附件、key 和权限不变；相关旧审批会失效。`,
          )
        )
          return;
        await this.submit(move, "/api/groups/move", {
          group,
          entries: members,
        });
      });
    };
    host.querySelector<HTMLButtonElement>("#clear-group-selection")!.onclick =
      () => {
        this.selected.clear();
        document
          .querySelectorAll<HTMLInputElement>("[data-select-entry]")
          .forEach((el) => {
            el.checked = false;
          });
        this.updateControls();
      };
    const edit = host.querySelector<HTMLFormElement>("#edit-group-form");
    if (edit)
      edit.onsubmit = (event) => {
        event.preventDefault();
        const remove =
          (event.submitter as HTMLButtonElement | null)?.value === "remove";
        void this.ctx.run(async () => {
          this.requireReady();
          const source = this.filter;
          const group = this.catalog.find((g) => g.name === source);
          if (!group || !source) throw new Error("分组已变化，请刷新后重试。");
          if (group.blocked)
            throw new Error("组内有冲突、缺失或同步未完整的条目，请先处理。");
          if (group.members.length > 20)
            throw new Error("此组超过 20 条，请先手动分批移动。");
          const target = remove
            ? ""
            : String(new FormData(edit).get("name") || "").trim();
          if (!remove && !target)
            throw new Error("请输入新组名；解除分组请使用「移除分组」。");
          if (target === source) throw new Error("新组名与当前相同。");
          const message = remove
            ? `解除「${source}」内全部 ${group.members.length} 条的分组？其中回收站 ${group.deleted_count} 条。内容、历史和权限全部保留，不删除资料。`
            : `将「${source}」改为「${target}」？影响 ${group.members.length} 条（含回收站 ${group.deleted_count} 条），不改 key 或权限。`;
          if (!confirm(message)) return;
          await this.submit(
            edit,
            "/api/groups/edit",
            { source, target, expected: group.members },
            target || null,
          );
        });
      };
    this.update(this.catalog);
  }
  private requireReady(): void {
    if (this.busy) throw new Error("上一个分组操作尚未完成。");
    if (this.ctx.format() < 3)
      throw new Error("请先在设置页备份并确认升级为格式 3。");
    if (this.ctx.isDirty())
      throw new Error("请先保存或放弃编辑区草稿，再批量管理分组。");
  }
  private async submit(
    form: HTMLFormElement,
    path: string,
    body: unknown,
    next?: string | null,
  ): Promise<void> {
    const operation = ++this.operation;
    const revision = this.ctx.revision();
    this.busy = true;
    this.updateControls();
    try {
      await this.ctx.api(path, body);
      if (!form.isConnected || operation !== this.operation) return;
      this.selected.clear();
      if (revision !== this.ctx.revision() || this.ctx.isDirty()) {
        this.ctx.notice(
          "分组已更新；当前编辑内容保留，请核对列表中的版本变化提示。",
        );
        return;
      }
      if (next !== undefined) this.filter = next;
      await this.ctx.onChanged();
      this.ctx.notice("分组已更新；正文、附件、key 和权限保持不变。");
    } finally {
      if (operation === this.operation) {
        this.busy = false;
        this.updateControls();
      }
    }
  }
  updateControls(): void {
    const blocked = this.busy || this.ctx.format() < 3;
    document
      .querySelectorAll<
        HTMLInputElement | HTMLButtonElement
      >(".group-tools input,.group-tools button")
      .forEach((el) => {
        el.disabled = blocked;
      });
    const count = document.querySelector("#group-selected-count");
    if (count)
      count.textContent = this.selected.size
        ? `已选 ${this.selected.size} 条`
        : "未选择条目";
    const button = document.querySelector<HTMLButtonElement>(
      "#move-group-form button[type=submit]",
    );
    if (button) button.disabled = blocked || !this.selected.size;
    const info = this.catalog.find((g) => g.name === this.filter);
    const note = document.querySelector("#group-member-note");
    if (note)
      note.textContent = info
        ? `${info.count} 条当前资料 · ${info.deleted_count} 条在回收站。${info.blocked ? "有不可用条目，请先处理冲突或同步问题。" : "操作会包含回收站成员，生成新版本；旧历史保留。"}`
        : "此组已没有成员，不保留空组。";
    document
      .querySelectorAll<HTMLButtonElement>("#edit-group-form button")
      .forEach((el) => {
        el.disabled = blocked || !info || !!info.blocked;
      });
  }
}
