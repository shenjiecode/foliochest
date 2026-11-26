import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  FileText,
  FolderCog,
  Plus,
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
import { ErrorNotice } from "@/components/forms";
import { groupBuckets, groupLabel } from "@/lib/groups";
import { type Summary, type Member, stateLabels } from "@/lib/types";
import { cn } from "@/lib/utils";
type Props = {
  entries: Summary[];
  group: string | null;
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
  onNew: () => void;
  onMove: () => void;
  onManage: () => void;
  onClear: () => void;
};
export function EntryList(p: Props) {
  const [collapsed, setCollapsed] = useState(new Set<string>());
  return (
    <section
      className={cn("entry-list", p.checked.size > 0 && "selection-mode")}
    >
      <header className="list-header">
        <div className="list-title">
          <h1>
            {p.trash
              ? "回收站"
              : p.group === null
                ? "全部资料"
                : groupLabel(p.group)}
          </h1>
          {p.group && p.format >= 3 && (
            <IconButton label="管理分组" onClick={p.onManage}>
              <FolderCog />
            </IconButton>
          )}
          {!p.trash && (
            <IconButton label="新建资料" onClick={p.onNew}>
              <Plus />
            </IconButton>
          )}
        </div>
        <div className="search-box">
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
                  aria-label="搜索选项"
                >
                  <SlidersHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                <DropdownMenuCheckboxItem
                  checked={p.bodySearch}
                  onCheckedChange={p.onBodySearch}
                >
                  同时搜索正文
                </DropdownMenuCheckboxItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </header>
      <div className="list-results scroll-region">
        <ErrorNotice message={p.error} />
        {p.loading && (
          <p className="list-feedback" role="status">
            正在读取…
          </p>
        )}
        {!p.loading && !p.entries.length && (
          <p className="list-feedback">
            {p.query ? "没有匹配的资料" : "没有资料"}
          </p>
        )}
        {groupBuckets(p.entries).map((bucket) => (
          <section key={bucket.id}>
            {p.group === null && (
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
                <span>{bucket.label}</span>
                <span>· {bucket.entries.length}</span>
              </button>
            )}
            {(!collapsed.has(bucket.id) || p.group !== null) &&
              bucket.entries.map((entry) => (
                <div
                  key={entry.key}
                  className={cn(
                    "entry-row",
                    entry.key === p.selected && "selected",
                  )}
                >
                  <button
                    className="entry-open"
                    onClick={() => p.onSelect(entry.key)}
                    aria-current={entry.key === p.selected ? "true" : undefined}
                  >
                    <span className="entry-key">
                      <FileText />
                      <span>{entry.key}</span>
                    </span>
                    <span className="entry-description">
                      {entry.description}
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
          </section>
        ))}
      </div>
      <footer className="list-footer">
        {p.checked.size ? (
          <>
            <span>已选 {p.checked.size} 条</span>
            <Button variant="ghost" onClick={p.onClear}>
              取消
            </Button>
            <Button variant="secondary" onClick={p.onMove}>
              移动到…
            </Button>
          </>
        ) : (
          <span>
            {p.entries.length} 条资料
            {p.bodySearch && !p.trash ? " · 包含正文" : ""}
          </span>
        )}
      </footer>
    </section>
  );
}
