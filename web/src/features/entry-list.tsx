import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  FileText,
  Eye,
  EyeOff,
  FolderCog,
  FolderClosed,
  FolderOpen,
  FolderPlus,
  ArrowDownUp,
  Search,
  SlidersHorizontal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { IconButton } from "@/components/common";
import { ErrorNotice, Modal } from "@/components/forms";
import { useClient, useTask } from "@/lib/hooks";
import { groupBuckets } from "@/lib/groups";
import {
  type Summary,
  type Member,
  type Group,
  stateLabels,
} from "@/lib/types";
import { cn } from "@/lib/utils";
type Props = {
  entries: Summary[];
  groups: Group[];
  recentGroups: Set<string>;
  showEmpty: boolean;
  onShowEmpty: (value: boolean) => void;
  onGroupCreated: (name: string) => void;
  query: string;
  bodySearch: boolean;
  trash: boolean;
  selected: string | null;
  checked: Map<string, Member>;
  format: number;
  loading: boolean;
  error: string;
  onQuery: (value: string) => void;
  onBodySearch: (value: boolean) => void;
  onSelect: (key: string) => void;
  onCheck: (entry: Summary, value: boolean) => void;
  onNew: (group?: string) => void;
  onMove: () => void;
  onManage: (name: string) => void;
  onClear: () => void;
};
export function EntryList(p: Props) {
  const [collapsed, setCollapsed] = useState(new Set<string>());
  const [sort, setSort] = useState<"key" | "reverse">("key");
  const [creatingGroup, setCreatingGroup] = useState(false);
  const [groupName, setGroupName] = useState("");
  const task = useTask(),
    client = useClient();
  const buckets = groupBuckets(
    [...p.entries].sort((a, b) =>
      sort === "key"
        ? a.key.localeCompare(b.key, "zh-CN")
        : b.key.localeCompare(a.key, "zh-CN"),
    ),
  );
  if (!p.trash) {
    const names = new Set(
      p.groups.filter((g) => g.count === 0).map((g) => g.name),
    );
    for (const name of p.recentGroups) names.add(name);
    for (const name of names) {
      if (!name || buckets.some((g) => g.id === "group:" + name)) continue;
      if (!p.showEmpty && !p.recentGroups.has(name)) continue;
      if (
        p.query &&
        !name.toLocaleLowerCase().includes(p.query.trim().toLocaleLowerCase())
      )
        continue;
      buckets.push({ id: "group:" + name, label: name, entries: [] });
    }
    buckets.sort((a, b) =>
      a.id === b.id
        ? 0
        : a.id === "conflict"
          ? -1
          : b.id === "conflict"
            ? 1
            : a.id === "group:"
              ? 1
              : b.id === "group:"
                ? -1
                : a.label.localeCompare(b.label, "zh-CN"),
    );
  }
  return (
    <section
      className={cn("entry-list", p.checked.size > 0 && "selection-mode")}
    >
      <header className="list-header">
        <div className="search-box" data-body-search={p.bodySearch && !p.trash}>
          <Search />
          <Input
            aria-label="搜索资料"
            placeholder="搜索资料"
            value={p.query}
            onChange={(e) => p.onQuery(e.target.value)}
          />
          {!p.trash && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="search-options"
                  data-active={p.bodySearch}
                  aria-label={
                    p.bodySearch
                      ? "搜索选项：已开启正文搜索"
                      : "搜索选项：未开启正文搜索"
                  }
                  title={
                    p.bodySearch
                      ? "已包含正文，点击调整搜索范围"
                      : "调整搜索范围"
                  }
                >
                  <SlidersHorizontal />
                  {p.bodySearch && <span>正文</span>}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="search-options-menu">
                <DropdownMenuCheckboxItem
                  className="search-body-option"
                  checked={p.bodySearch}
                  onCheckedChange={p.onBodySearch}
                >
                  <span className="search-option-copy">
                    <span>同时搜索正文</span>
                    <small>在 Key、描述、分组之外搜索正文</small>
                  </span>
                  <span className="search-option-state">
                    {p.bodySearch ? "已开启" : "未开启"}
                  </span>
                </DropdownMenuCheckboxItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
        <div className="tree-toolbar">
          <div className="group-create-actions">
            <Button
              variant="ghost"
              size="sm"
              disabled={p.trash || p.format < 3}
              onClick={() => {
                setGroupName("");
                task.clearError();
                setCreatingGroup(true);
              }}
            >
              <FolderPlus />
              新建分组
            </Button>
            <IconButton
              label={p.showEmpty ? "显示空组：已开启" : "显示空组：已关闭"}
              className="empty-group-toggle"
              aria-pressed={p.showEmpty}
              disabled={p.trash}
              onClick={() => p.onShowEmpty(!p.showEmpty)}
            >
              {p.showEmpty ? <Eye /> : <EyeOff />}
            </IconButton>
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm">
                <ArrowDownUp />
                排序
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuCheckboxItem
                checked={sort === "key"}
                onCheckedChange={() => setSort("key")}
              >
                按名称排序
              </DropdownMenuCheckboxItem>
              <DropdownMenuCheckboxItem
                checked={sort === "reverse"}
                onCheckedChange={() => setSort("reverse")}
              >
                按名称倒序
              </DropdownMenuCheckboxItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>
      {creatingGroup && (
        <Modal
          title="新建分组"
          description="空组保存在本机，刷新或重启后仍保留；新建组本次临时显示，不受隐藏空组影响。暂不随资料库同步或备份。"
          busy={task.busy}
          onClose={() => setCreatingGroup(false)}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!groupName.trim()) return;
              void task.run(
                (ctx) =>
                  ctx.request<Group>("/api/groups/create", {
                    name: groupName.trim(),
                  }),
                (group) => {
                  setCreatingGroup(false);
                  setCollapsed((before) => {
                    const next = new Set(before);
                    next.delete("group:" + group.name);
                    return next;
                  });
                  p.onGroupCreated(group.name);
                  client.notice("分组已创建。");
                },
              );
            }}
          >
            <label htmlFor="new-group-name">分组名称</label>
            <Input
              id="new-group-name"
              value={groupName}
              required
              maxLength={64}
              disabled={task.busy}
              onChange={(event) => setGroupName(event.target.value)}
              placeholder="输入分组名称"
            />
            <div className="new-group-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreatingGroup(false)}
                disabled={task.busy}
              >
                取消
              </Button>
              <Button type="submit" disabled={task.busy}>
                {task.busy ? "正在创建…" : "创建分组"}
              </Button>
            </div>
            <ErrorNotice message={task.error} />
          </form>
        </Modal>
      )}
      <div className="list-results scroll-region">
        <ErrorNotice message={p.error} />
        {p.loading && (
          <p className="list-feedback" role="status">
            正在读取…
          </p>
        )}
        {!p.loading && !buckets.length && (
          <p className="list-feedback">
            {p.query ? "没有匹配的资料" : "没有资料"}
          </p>
        )}
        {buckets.map((bucket) => (
          <section key={bucket.id} className="tree-group">
            <div className="tree-group-row">
              <button
                className="list-group-heading"
                aria-expanded={!collapsed.has(bucket.id)}
                onClick={() =>
                  setCollapsed((before) => {
                    const next = new Set(before);
                    if (next.has(bucket.id)) next.delete(bucket.id);
                    else next.add(bucket.id);
                    return next;
                  })
                }
              >
                {collapsed.has(bucket.id) ? <ChevronRight /> : <ChevronDown />}
                {collapsed.has(bucket.id) ? <FolderClosed /> : <FolderOpen />}
                <span className="tree-group-name">{bucket.label}</span>
                {!bucket.entries.length && (
                  <span className="empty-group-count">0</span>
                )}
                <span className="tree-group-chevron" aria-hidden="true">
                  {collapsed.has(bucket.id) ? (
                    <ChevronRight />
                  ) : (
                    <ChevronDown />
                  )}
                </span>
              </button>
              {bucket.id.startsWith("group:") &&
                bucket.id !== "group:" &&
                p.format >= 3 && (
                  <IconButton
                    label={"管理分组 " + bucket.label}
                    onClick={() => p.onManage(bucket.id.slice(6))}
                  >
                    <FolderCog />
                  </IconButton>
                )}
            </div>
            {!collapsed.has(bucket.id) && (
              <div className="tree-group-items">
                {!bucket.entries.length && (
                  <Button
                    className="empty-group-new"
                    variant="ghost"
                    size="sm"
                    onClick={() => p.onNew(bucket.id.slice(6))}
                  >
                    添加资料
                  </Button>
                )}
                {bucket.entries.map((entry) => (
                  <div
                    key={entry.key}
                    className={cn(
                      "entry-row",
                      entry.key === p.selected && "selected",
                    )}
                  >
                    <button
                      className="entry-open"
                      title={entry.description}
                      onClick={() => p.onSelect(entry.key)}
                      aria-current={
                        entry.key === p.selected ? "true" : undefined
                      }
                    >
                      <span className="entry-key">
                        <FileText />
                        <span>{entry.key}</span>
                      </span>
                      {entry.state !== "ready" && (
                        <span className="entry-description state-warning">
                          {stateLabels[entry.state] || entry.state}
                        </span>
                      )}
                    </button>
                    <Checkbox
                      className="entry-checkbox"
                      aria-label={"选择 " + entry.key}
                      checked={p.checked.has(entry.key)}
                      disabled={
                        p.format < 3 ||
                        !["ready", "deleted"].includes(entry.state)
                      }
                      onCheckedChange={(v) => p.onCheck(entry, v === true)}
                    />
                  </div>
                ))}
              </div>
            )}
          </section>
        ))}
      </div>
      {p.checked.size > 0 && (
        <footer className="list-footer">
          <span>已选 {p.checked.size} 条</span>
          <Button variant="ghost" onClick={p.onClear}>
            取消
          </Button>
          <Button variant="secondary" onClick={p.onMove}>
            移动到…
          </Button>
        </footer>
      )}
    </section>
  );
}
